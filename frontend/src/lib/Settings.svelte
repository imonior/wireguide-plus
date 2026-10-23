<script>
  import { onDestroy, onMount } from 'svelte';
  import { Events } from '@wailsio/runtime';
  import { t, setLanguage, getLanguage, detectLanguage } from '../i18n/index.js';
  import { applyTheme } from '../stores/theme.js';
  import { connectionStatus, tunnels } from '../stores/tunnels.js';
  import { compactList } from '../stores/ui.js';
  import { appSettings } from '../stores/settings.js';
  import Icon from './Icon.svelte';
  import { modalDrag } from './actions/modal-drag.js';

  export let TunnelService;
  export let onClose = () => {};
  export let updateInfo = null;
  export let onInstall = null;
  export let onOpenRelease = null;
  // Optional toast helper forwarded by App.svelte so Settings can surface
  // synchronous failures (toggle toggles that were rolled back by the
  // helper's live-apply step, unsupported platform features, …) without
  // having to depend on App's local state directly.
  export let showToast = null;

  let aboutUpdating = false;
  let aboutShowVpnWarn = false;
  // Live phase + percent mirrored from the backend `update_progress`
  // event so the About tab shows the same download progress the main
  // window banner does (see UpdateNotice.svelte). Without this the
  // Update button sat on a static "Updating…" label that read as a hang.
  // 实时阶段与百分比镜像自后端 `update_progress` 事件，使 About 页与主窗口
  // 横幅一样显示下载进度（参见 UpdateNotice.svelte）。否则更新按钮会一直
  // 停在静态的 “Updating…” 文案，看起来像卡死了。
  let aboutUpdatePhase = '';
  let aboutUpdatePercent = 0;
  let unsubUpdateProgress = null;
  // updateState mirrors UpdateState DTO from GetUpdateState. Used by the
  // About tab to show last-checked timestamp + brew detection + an
  // explicit "Check now" button (the launch-only check we used to do
  // could leave About showing "up to date" for days after a release).
  let updateState = null;
  let aboutChecking = false;
  let aboutCheckResult = '';
  // nowTick drives formatLastChecked re-evaluation. Without this, opening
  // Settings and leaving it on the About tab for 10 minutes would still
  // show "just now". Updated every 60 s by an interval started in
  // onMount and torn down in onDestroy.
  let nowTick = Date.now();
  let nowTimer = null;

  async function refreshUpdateState() {
    try { updateState = await TunnelService.GetUpdateState(); } catch (_) {}
  }

  // Last *attempted* manual check, including failures — used to throttle
  // rapid-click. Distinct from updateState.last_check_unix which only
  // tracks *successful* checks (the scheduler's contract).
  let lastManualCheckMs = 0;
  const MANUAL_CHECK_MIN_INTERVAL_MS = 5000;

  async function aboutCheckNow() {
    if (aboutChecking) return;
    // Rate-limit guard. Without this, holding-down "Check now" sends a
    // request per response-time interval (~few hundred ms) and burns the
    // 60-req/hour anonymous GitHub quota for everyone behind the same
    // public IP. The 5 s floor lines up with what a frustrated human can
    // tolerate; a programmatic caller will hit it harmlessly.
    const now = Date.now();
    if (now - lastManualCheckMs < MANUAL_CHECK_MIN_INTERVAL_MS) {
      return;
    }
    lastManualCheckMs = now;
    aboutChecking = true;
    aboutCheckResult = '';
    try {
      const info = await TunnelService.CheckForUpdate();
      await refreshUpdateState();
      if (info?.available) {
        // New version found: the green "up to date" pill flips to the
        // "Update" pill automatically (driven by `updateInfo`), and we
        // mirror the found version into the result line under the button
        // so the click has immediate inline feedback.
        updateInfo = info;
        aboutCheckResult = $t('update.check_result_update_found', { version: info.version });
      } else {
        aboutCheckResult = $t('update.check_result_up_to_date', { version: appVersion || '—' });
      }
    } catch (e) {
      aboutCheckResult = ($t('update.check_failed') || 'Check failed') + ': ' + (e?.message || e);
    } finally {
      aboutChecking = false;
    }
  }

  // `_tick` parameter forces Svelte to re-run this whenever nowTick
  // changes (every 60 s). Without it, Svelte couldn't tell the result
  // depends on time and would cache the first evaluation forever.
  function formatLastChecked(unix, _tick) {
    if (!unix) {
      // First-launch placeholder: when auto-check is on and the scheduler
      // hasn't fired its first tick yet (30–120 s after startup), show
      // a "scheduled" hint instead of the misleading "Never checked".
      // Dev builds never schedule, so they still get "Never checked".
      if (updateState?.auto_enabled && !updateState?.is_dev_build) {
        return $t('update.first_check_scheduled');
      }
      return $t('update.never_checked');
    }
    const diff = Math.max(0, Date.now() / 1000 - unix);
    let time;
    if (diff < 60) {
      time = $t('update.time_just_now');
    } else if (diff < 3600) {
      time = $t('update.time_minutes_ago', { n: Math.floor(diff / 60) });
    } else if (diff < 86400) {
      time = $t('update.time_hours_ago', { n: Math.floor(diff / 3600) });
    } else {
      time = $t('update.time_days_ago', { n: Math.floor(diff / 86400) });
    }
    return $t('update.last_checked', { time });
  }

  // Reactive label: the template must read this variable directly. Calling
  // formatLastChecked() there compiles inside $.untrack, so the
  // late-arriving GetUpdateState result only surfaced on the next 60 s tick.
  // 响应式标签：模板必须直接读取该变量。在模板里调用 formatLastChecked() 会被
  // 编译进 $.untrack，晚到的 GetUpdateState 结果要等下一次 60 秒 tick 才刷新。
  $: lastCheckedLabel = formatLastChecked(updateState?.last_check_unix, nowTick);

  // Same phase-label mapping as UpdateNotice. Reactive variable (not a
  // template function call) so it tracks phase/percent outside untrack.
  // 与 UpdateNotice 相同的阶段文案映射。用响应式变量（而非模板内函数调用），
  // 确保 phase/percent 在 untrack 之外被追踪、UI 实时刷新。
  $: aboutPhaseLabel = aboutUpdatePhase === 'download' ? $t('update.progress_download', { percent: aboutUpdatePercent })
    : aboutUpdatePhase === 'refresh' ? $t('update.progress_refresh')
    : aboutUpdatePhase === 'install' ? $t('update.progress_install')
    : '';


  function aboutRequestUpdate() {
    if ($connectionStatus?.state === 'connected') {
      aboutShowVpnWarn = true;
    } else {
      doAboutUpdate();
    }
  }

  async function doAboutUpdate() {
    aboutShowVpnWarn = false;
    aboutUpdating = true;
    aboutCheckResult = '';
    try {
      if (onInstall) await onInstall();
    } catch (e) {
      // Inline, next to the button that failed — the toast alone renders
      // under this modal.
      aboutCheckResult = ($t('update.install_failed') || 'Update failed') + ': ' + (e?.message || e);
    } finally {
      aboutUpdating = false;
      aboutUpdatePhase = '';
      aboutUpdatePercent = 0;
    }
  }

  function aboutOpenRelease() {
    if (onOpenRelease) onOpenRelease();
  }

  let activeTab = 'general';

  // Closed set of notification-duration choices the <select> offers.
  // A config.json value outside this set renders the select blank (the
  // browser cannot select an <option> that doesn't exist), so load()
  // normalizes any out-of-set value back to the default.
  const VALID_NOTIFY_DURATIONS = [5000, 10000, 15000, 30000, 60000];
  const DEFAULT_NOTIFY_DURATION = 10000;
  // Log retention choices offered by the advanced tab.
  const VALID_LOG_RETENTION_DAYS = [1, 3, 7, 14, 30, 90];
  const DEFAULT_LOG_RETENTION_DAYS = 7;
  // History retention choices. 0 = no time-based pruning (only the
  // hard 200-record cap applies).
  const VALID_HISTORY_RETENTION_DAYS = [0, 1, 3, 7, 14, 30, 90];
  const DEFAULT_HISTORY_RETENTION_DAYS = 7;

  let settings = {
    language: getLanguage(),
    theme: 'system',
    auto_start: false,
    start_minimized: false,
    // Backend default is true (a missing key means true); mirrored here so
    // the toggle renders ON for a config.json written before the key
    // existed, instead of showing an OFF the user never chose.
    disconnect_on_quit: true,
    notify_duration_ms: DEFAULT_NOTIFY_DURATION,
    health_check: true,
    // Backend default is true (a missing key means true); mirrored here so the
    // toggle renders ON for a config.json written before the key existed,
    // instead of showing an OFF the user never chose.
    keep_connection_on_idle: true,
    // Backend default is false (opt-in): an invasive power override that
    // blocks system sleep — leave it OFF until the user chooses it.
    prevent_system_sleep: false,
    dns_resolve_path: false,
    pin_interface: false,
    log_level: 'info',
    tray_icon_style: 'color',
    auto_update_check: true,
    compact_list: false,
    proxy_mode: 'direct',
    proxy_url: '',
    // null = "never customized" → the backend falls back to its built-in
    // defaults. An empty array is a real, persisted choice: the user
    // deleted every mirror and wants none offered.
    proxy_mirrors: null,
    // Public IP / geolocation lookup on page open. Opt-out, not opt-in,
    // so the readout is discoverable; the provider is always shown.
    public_ip_lookup: true,
    log_retention_days: DEFAULT_LOG_RETENTION_DAYS,
    history_retention_days: DEFAULT_HISTORY_RETENTION_DAYS,
    enable_wg_scripts: false,
    // Strict default: the gate is OFF until the persisted settings are
    // actually read back as ON. Never assume an unread feature flag.
    enable_awg: false,
    // Endpoint failover (multi-endpoint automatic selection). Master switch
    // defaults ON (a missing key means enabled); the tuning knobs default
    // to the backend's safe values but are clamped on save.
    endpoint_failover_enabled: true,
    endpoint_threshold_ms: 150,
    endpoint_margin_pct: 30,
    endpoint_sustained_sec: 30,
    endpoint_cooldown_sec: 300,
  };
  let loaded = false;
  let appVersion = '';
  let folderError = '';
  let wgScriptsWarn = false;
  // AWG inline confirm (same pattern as wg-scripts-warn): shown when the
  // user flips the toggle ON, dismissed by Confirm/Cancel.
  let awgWarn = false;

  async function load() {
    try {
      const s = await TunnelService.GetSettings();
      if (s) {
        settings.language = s.language || 'auto';
        settings.theme = s.theme || 'system';
        settings.auto_start = s.auto_start ?? false;
        settings.start_minimized = s.start_minimized ?? false;
        settings.disconnect_on_quit = s.disconnect_on_quit ?? true;
        // Normalize notification duration into the offered set so a
        // hand-edited config.json can't leave the <select> blank.
        settings.notify_duration_ms = VALID_NOTIFY_DURATIONS.includes(Number(s.notify_duration_ms))
          ? Number(s.notify_duration_ms)
          : DEFAULT_NOTIFY_DURATION;
        settings.health_check = s.health_check ?? false;
        // Backend default is true; a pre-existing config.json has no key, so
        // `?? true` keeps the toggle ON rather than silently flipping it off.
        settings.keep_connection_on_idle = s.keep_connection_on_idle ?? true;
        // Opt-in power override: OFF unless explicitly persisted as true.
        settings.prevent_system_sleep = s.prevent_system_sleep ?? false;
        settings.dns_resolve_path = s.dns_resolve_path ?? false;
        settings.pin_interface = s.pin_interface ?? false;
        settings.log_level = s.log_level || 'info';
        settings.tray_icon_style = s.tray_icon_style || 'color';
        settings.compact_list = s.compact_list ?? false;
        // Legacy settings.json predates this field — *bool null on
        // the Go side becomes undefined here; default to true to match
        // Settings.AutoUpdateCheckEnabled() semantics.
        settings.auto_update_check = (s.auto_update_check === false) ? false : true;
        settings.proxy_mode = s.proxy_mode || 'direct';
        // proxy_url is intentionally kept even in direct mode: it is the
        // remembered value restored when switching back to a custom
        // mirror / manual proxy.
        settings.proxy_url = s.proxy_url || '';
        // Mirror list: a persisted array is authoritative (even when
        // empty); null/undefined means "never customized" → defaults.
        settings.proxy_mirrors = Array.isArray(s.proxy_mirrors) ? [...s.proxy_mirrors] : null;
        settings.public_ip_lookup = s.public_ip_lookup === false ? false : true;
        // Log retention: 0 in config.json means "default (7)".
        const retention = Number(s.log_retention_days);
        settings.log_retention_days = VALID_LOG_RETENTION_DAYS.includes(retention) ? retention : DEFAULT_LOG_RETENTION_DAYS;
        // History retention: 0 in config.json means "default (7)".
        const histRetention = Number(s.history_retention_days);
        settings.history_retention_days = VALID_HISTORY_RETENTION_DAYS.includes(histRetention)
          ? histRetention
          : DEFAULT_HISTORY_RETENTION_DAYS;
        settings.enable_wg_scripts = s.enable_wg_scripts ?? false;
        settings.enable_awg = s.enable_awg === true;
        // Endpoint failover: master switch defaults ON (nil on the Go side →
        // enabled); tuning knobs fall back to the backend's safe defaults.
        settings.endpoint_failover_enabled = s.endpoint_failover_enabled === false ? false : true;
        settings.endpoint_threshold_ms = Number(s.endpoint_threshold_ms) >= 1 ? Number(s.endpoint_threshold_ms) : 150;
        settings.endpoint_margin_pct = Number(s.endpoint_margin_pct) >= 1 ? Number(s.endpoint_margin_pct) : 30;
        settings.endpoint_sustained_sec = Number(s.endpoint_sustained_sec) >= 0 ? Number(s.endpoint_sustained_sec) : 30;
        settings.endpoint_cooldown_sec = Number(s.endpoint_cooldown_sec) >= 240 ? Number(s.endpoint_cooldown_sec) : 300;
        // Measure every update path and resolve the public IP as soon as the
        // page opens — both are read-only lookups the user would otherwise
        // have to click for. Fire-and-forget: a failed lookup must never
        // block the settings page from rendering.
        runPathScan();
        loadSystemProxy();
        loadPublicIP();
      }
    } catch (e) {
      console.error('load settings:', e);
    }
    loaded = true;
  }

  // Keep the global appSettings store in sync so TunnelList/TunnelDetail
  // badges and the editor field gating react to changes live.
  $: if (loaded) {
    appSettings.set({
      // `loaded` MUST be carried: consumers hide the AWG badge while it is
      // false, so dropping it here would make every badge vanish the first
      // time the Settings dialog is opened.
      loaded: true,
      enable_awg: settings.enable_awg === true,
      enable_wg_scripts: settings.enable_wg_scripts === true,
      // Interface binding reuses the existing pin_interface switch — it
      // is the "bind traffic to a physical interface" master toggle.
      pin_interface: settings.pin_interface,
    });
  }

  load();

  async function save() {
    // Read-modify-write: re-fetch the freshest settings.json and spread it
    // as the BASE of the payload, then overlay only the fields this screen
    // actually renders (the same pattern stores/ui.js uses for the list
    // prefs).
    //
    // SaveSettings replaces the whole settings object, so every field NOT
    // listed below rides along from `fresh` and every field listed below is
    // owned here. There is no third state — which is exactly why this
    // shape: a field can no longer be silently dropped. Enumerating the
    // payload by hand is what caused prevent_system_sleep to reset to OFF
    // on every save (v2.3.6) and would equally have wiped the manual on/off
    // latches and the DNS-leak-test resolver lists, none of which this
    // screen owns.
    //
    // If the fresh fetch fails (helper restarting, IPC flake) we abort
    // rather than write a partial payload — a deferred save is far better
    // than clobbering state another screen owns.
    let fresh;
    try {
      fresh = await TunnelService.GetSettings();
    } catch (e) {
      console.warn('settings save aborted: fresh fetch failed (will retry on next change)', e);
      return;
    }
    try {
      await TunnelService.SaveSettings({
        // Everything this screen does not own: wifi_rules (both halves) and
        // automation (per-tunnel rule editor), the list prefs (tunnel-list
        // header), the manual on/off latches (connect / disconnect), and
        // the DNS leak-test public resolver lists (diagnostics panel).
        ...fresh,
        // ---- owned by this screen ----
        language: settings.language,
        theme: settings.theme,
        tray_icon_style: settings.tray_icon_style,
        auto_start: settings.auto_start,
        start_minimized: settings.start_minimized,
        disconnect_on_quit: settings.disconnect_on_quit,
        notify_duration_ms: settings.notify_duration_ms,
        health_check: settings.health_check,
        keep_connection_on_idle: settings.keep_connection_on_idle,
        prevent_system_sleep: settings.prevent_system_sleep,
        dns_resolve_path: settings.dns_resolve_path,
        pin_interface: settings.pin_interface,
        log_level: settings.log_level,
        auto_update_check: settings.auto_update_check,
        compact_list: settings.compact_list,
        proxy_mode: settings.proxy_mode,
        proxy_url: settings.proxy_url,
        proxy_mirrors: settings.proxy_mirrors,
        public_ip_lookup: settings.public_ip_lookup,
        log_retention_days: settings.log_retention_days,
        history_retention_days: settings.history_retention_days,
        enable_wg_scripts: settings.enable_wg_scripts,
        enable_awg: settings.enable_awg,
        endpoint_failover_enabled: settings.endpoint_failover_enabled,
        endpoint_threshold_ms: settings.endpoint_threshold_ms,
        endpoint_margin_pct: settings.endpoint_margin_pct,
        endpoint_sustained_sec: settings.endpoint_sustained_sec,
        endpoint_cooldown_sec: settings.endpoint_cooldown_sec,
      });
    } catch (e) {
      console.error('save settings:', e);
    }
  }

  let saveTimer = null;
  function scheduleSave() {
    if (saveTimer) clearTimeout(saveTimer);
    saveTimer = setTimeout(() => {
      saveTimer = null;
      save();
    }, 300);
  }

  function onThemeChange(e) {
    settings.theme = e.target.value;
    applyTheme(settings.theme);
    scheduleSave();
  }

  function onLanguageChange(e) {
    settings.language = e.target.value;
    const resolved = settings.language === 'auto' ? detectLanguage() : settings.language;
    setLanguage(resolved);
    scheduleSave();
  }

  function onCompactListChange(e) {
    settings.compact_list = e.target.checked;
    // Update the shared store so the tunnel list re-renders immediately,
    // then persist.
    compactList.set(settings.compact_list);
    scheduleSave();
  }

  function onAutoStartChange(e) {
    settings.auto_start = e.target.checked;
    scheduleSave();
  }

  // Master switch for the per-tunnel "DNS resolve path" policy. It gates the
  // feature itself: off means no per-tunnel switch is shown, nothing is
  // enforced and no conflict is reported. No live IPC — the helper reads the
  // setting when it evaluates a connect.
  function onDnsResolvePathChange(e) {
    settings.dns_resolve_path = e.target.checked;
    scheduleSave();
  }

  function onStartMinimizedChange(e) {
    settings.start_minimized = e.target.checked;
    scheduleSave();
  }

  // What "Quit" does to live tunnels. The GUI reads this at exit time (see
  // doShutdown in internal/gui/gui.go), so no live IPC is needed here — the
  // next quit picks up the new value.
  function onDisconnectOnQuitChange(e) {
    settings.disconnect_on_quit = e.target.checked;
    scheduleSave();
  }

  function onNotifyDurationChange(e) {
    settings.notify_duration_ms = Number(e.target.value);
    scheduleSave();
  }

  function onLogRetentionChange(e) {
    settings.log_retention_days = Number(e.target.value);
    scheduleSave();
  }

  function onHistoryRetentionChange(e) {
    settings.history_retention_days = Number(e.target.value);
    scheduleSave();
  }

  function onWgScriptsChange(e) {
    // enable_wg_scripts has no live-apply IPC: the setting only takes
    // effect on the NEXT connect. So we do NOT need applyLiveChange —
    // BUT the confirmation flow must not write a stale value either.
    //
    // OLD BUG: the code below wrote settings.enable_wg_scripts = true
    // before the user had clicked Confirm — the checkbox was visually on
    // AND persisted on disk, while the confirmation modal was still up.
    // If the user cancelled, settings went back to false in memory but
    // scheduleSave() had already fired so disk stayed true — the classic
    // "write + optimistic + never persist the rollback" bug.
    if (e.target.checked && !wgScriptsWarn) {
      wgScriptsWarn = true;
      // Do NOT mutate settings or call scheduleSave() here: the user
      // either confirms (→ onWgScriptsWarnConfirm writes + saves) or
      // cancels (→ onWgScriptsWarnCancel writes + reactive binding
      // resets the native checkbox). settings.enable_wg_scripts stays at
      // its persisted value for the whole dialog lifetime.
      return;
    }
    settings.enable_wg_scripts = e.target.checked;
    scheduleSave();
  }

  function onWgScriptsWarnConfirm() {
    wgScriptsWarn = false;
    settings.enable_wg_scripts = true;
    scheduleSave();
  }

  function onWgScriptsWarnCancel() {
    wgScriptsWarn = false;
    settings.enable_wg_scripts = false;
    scheduleSave();
  }

  function onAwgChange(e) {
    // Exact same flow as wg-scripts: ENABLING pops the inline highlight
    // bar and does NOT write the value until the user confirms; the
    // checkbox stays at its persisted value for the whole bar lifetime.
    // DISABLING applies immediately (no confirmation needed — turning a
    // protocol off is not a risk the bar exists to prevent).
    if (e.target.checked && !awgWarn) {
      awgWarn = true;
      // Do NOT mutate settings or call scheduleSave() here: the user
      // either confirms (→ onAwgWarnConfirm writes + saves) or cancels
      // (→ onAwgWarnCancel writes; the reactive binding reverts the
      // native checkbox). settings.enable_awg stays at its persisted
      // value for the whole bar lifetime.
      return;
    }
    settings.enable_awg = e.target.checked;
    scheduleSave();
  }

  function onAwgWarnConfirm() {
    awgWarn = false;
    settings.enable_awg = true;
    scheduleSave();
  }

  function onAwgWarnCancel() {
    awgWarn = false;
    settings.enable_awg = false;
    scheduleSave();
  }

  let openingFolder = false;
  async function openFolder(kind) {
    if (openingFolder) return;
    openingFolder = true;
    folderError = '';
    try {
      await TunnelService.OpenFolder(kind);
    } catch (e) {
      folderError = e?.message || String(e);
    }
    openingFolder = false;
  }

  // --- settings backup: export / import (tunnels + scripts, no logs) ---
  let backupBusy = false;
  function notifyBackup(msg) {
    if (typeof showToast === 'function') showToast(msg);
    else console.log(msg);
  }
  async function exportSettings() {
    if (backupBusy) return;
    backupBusy = true;
    try {
      const path = await TunnelService.ExportSettings();
      if (path) notifyBackup($t('settings.export_ok', { path }));
    } catch (e) {
      notifyBackup($t('settings.export_fail', { error: e?.message || String(e) }));
    }
    backupBusy = false;
  }
  async function importSettings() {
    if (backupBusy) return;
    backupBusy = true;
    try {
      const res = await TunnelService.ImportSettings();
      if (res) {
        // config.json may have been replaced — reload into this screen so
        // toggles reflect the imported values instead of stale memory.
        await load();
        const nTunnels = res.tunnels?.length ?? 0;
        const nScripts = res.scripts?.length ?? 0;
        notifyBackup(
          $t('settings.import_ok', { t: nTunnels, s: nScripts }) +
            (res.settings_applied ? ' · ' + $t('settings.import_settings_applied') : '')
        );
      }
    } catch (e) {
      notifyBackup($t('settings.import_fail', { error: e?.message || String(e) }));
    }
    backupBusy = false;
  }

  function onLogLevelChange(e) {
    // Live-apply first — the helper needs to see the new slog level before
    // we record it on disk, so a "DEBUG selected but helper rejected it"
    // failure leaves the settings.json untouched and the <select> snaps
    // back. See applyLiveChange.
    applyLiveChange({
      stateKey: 'log_level',
      domValue: e.target.value,
      friendlyName: $t('settings.log_level'),
      call: (v) => TunnelService.SetLogLevel(v),
    });
  }

  // applyLiveChange runs the canonical "helper → persist → write" cycle
  // used by Kill Switch / DNS Protection / Pin Interface / Health Check
  // (booleans) and Log Level (select / string values).
  //
  // OLD (buggy) pattern:   write settings → scheduleSave() → IPC → catch rollback
  //   Problems: (1) a failed IPC leaves the wrong value on disk because
  //   scheduleSave already persisted before the rejection, (2) the catch
  //   rollback only flips in-memory state, (3) no failure feedback to the
  //   user so the toggle appeared to do nothing (EXP-100014872 — first
  //   click only "corrects" stale state instead of executing an action).
  //
  // NEW (safe) pattern:     IPC → success → write settings → scheduleSave
  //   The native control keeps the user's desired value momentarily via
  //   normal DOM behaviour, then once the helper acknowledges, we write
  //   the single source of truth into `settings` and persist. On failure
  //   the reactively-bound checked={…}/value={…} props automatically snap
  //   the control back to the previously persisted value; a toast tells
  //   the user exactly why. settings.json never contains a value the
  //   helper hasn't actually accepted.
  async function applyLiveChange({ stateKey, domValue, friendlyName, call, valueMatches = (a, b) => a === b }) {
    if (valueMatches(settings[stateKey], domValue)) return; // no-op
    try {
      await call(domValue);
    } catch (err) {
      console.error(`${friendlyName} change failed:`, err);
      showToast?.($t('settings.toggle_failed', { name: friendlyName, error: err?.message || err }));
      if (err?.message?.includes?.('not supported')) {
        showToast?.($t('settings.feature_not_supported', { name: friendlyName }));
      }
      return;
    }
    settings[stateKey] = domValue;
    scheduleSave();
  }
  // Legacy alias kept so every existing call site doesn't need a rename.
  async function applyLiveToggle({ stateKey, domChecked, friendlyName, call }) {
    return applyLiveChange({ stateKey, domValue: domChecked, friendlyName, call });
  }

  function onPinInterfaceChange(e) {
    applyLiveToggle({
      stateKey: 'pin_interface',
      domChecked: e.target.checked,
      friendlyName: $t('settings.pin_interface'),
      call: (v) => TunnelService.SetPinInterface(v),
    });
  }

  function onHealthCheckChange(e) {
    applyLiveToggle({
      stateKey: 'health_check',
      domChecked: e.target.checked,
      friendlyName: $t('settings.health_check'),
      call: (v) => TunnelService.SetHealthCheck(v),
    });
  }

  function onKeepConnectionOnIdleChange(e) {
    applyLiveToggle({
      stateKey: 'keep_connection_on_idle',
      domChecked: e.target.checked,
      friendlyName: $t('settings.keep_connection_on_idle'),
      call: (v) => TunnelService.SetKeepConnectionOnIdle(v),
    });
  }

  function onPreventSystemSleepChange(e) {
    applyLiveToggle({
      stateKey: 'prevent_system_sleep',
      domChecked: e.target.checked,
      friendlyName: $t('settings.prevent_system_sleep'),
      call: (v) => TunnelService.SetPreventSystemSleep(v),
    });
  }

  // --- Proxy (for update checks) ---
  // Only two things leave this app: the update check and the installer
  // download. Both go out through one chosen "update path", and a path is a
  // mutually exclusive category rather than a set of stacked options:
  //   - direct / system: how the request leaves (nothing / the OS proxy).
  //   - mirror: where it goes — the GitHub URL is rewritten through a
  //     public accelerator prefix. No local proxy software needed.
  //   - manual: still api.github.com, but tunneled through a locally
  //     running HTTP/SOCKS5 proxy (Clash etc.).
  // Picking one row therefore resets the other dimension: a mirror is not
  // "a proxy plus a prefix", it is a different way out entirely.
  //
  // 只有两样东西会离开本应用：更新检查与安装包下载。二者都经一条选定的
  // 「升级通路」外出，而通路是互斥的分类，不是可叠加的选项：
  //   - direct / system：请求怎么出去（不用代理 / 用系统代理）。
  //   - mirror：请求去哪里——GitHub 的 URL 经公共加速前缀改写。无需本地代理软件。
  //   - manual：仍然去 api.github.com，但经本地运行的 HTTP/SOCKS5 代理（Clash 等）。
  // 因此选中一行会把另一维退回：镜像不是「代理 + 前缀」，它是完全不同的一种走法。
  //
  // Built-in starting point for the mirror list. It is only a seed:
  // every entry is deletable, and "restore defaults" brings them back.
  // Kept here as well as in Go so the list never renders empty while
  // settings are still loading.
  const DEFAULT_MIRRORS = [
    'https://ghfast.top',
    'https://gh-proxy.com',
    'https://ghproxy.net',
    'https://mirror.ghproxy.com',
  ];

  // Effective list: a persisted array wins (including an empty one, which
  // is how "delete them all" survives a restart); null → defaults.
  $: mirrorList = Array.isArray(settings.proxy_mirrors) ? settings.proxy_mirrors : DEFAULT_MIRRORS;
  // The path list: one row per way an update request can go out, rendered
  // as a single-select list. Rows come from the saved settings plus the
  // managed mirror list; a manual address or a custom mirror that is not in
  // the list still gets its own row — otherwise it would keep working
  // invisibly, with no way to see, re-select or delete it.
  //
  // 通路列表：每行是一种「升级请求外出的方式」，以单选列表呈现。行来自已保存
  // 的设置加受管的镜像列表；不在列表里的手动地址或自定义镜像也会各占一行——
  // 否则它会继续生效却看不见、选不中、删不掉。
  //
  // A reactive statement, not a template helper: Svelte cannot see which
  // state a function body reads, so `pathRows` is computed here and only
  // read (never computed) from the markup.
  // 必须是响应式语句而不是模板里的辅助函数：Svelte 看不到函数体读了哪些状态，
  // 所以 pathRows 在这里算好，模板里只负责读。
  $: pathRows = buildPathRows(settings.proxy_mode, settings.proxy_url, mirrorList);

  function buildPathRows(mode, url, mirrors) {
    // Addresses and prefixes are facts, not copy: they are printed verbatim
    // (label). Only the two rows with no address carry a translated key.
    // 地址与前缀是事实而非文案，原样印出（label）。只有那两个没有地址的行
    // 才带待翻译的键。
    const rows = [];
    rows.push({ id: 'direct', kind: 'direct', mode: 'direct', url: '', labelKey: 'settings.proxy_direct_github' });
    rows.push({ id: 'system', kind: 'system', mode: 'system', url: '', labelKey: 'settings.proxy_follow_system' });
    if (mode === 'manual' && url) {
      rows.push({ id: 'proxy:' + url, kind: 'proxy', mode: 'manual', url, label: url, deletable: true });
    }
    for (const m of mirrors) {
      rows.push({ id: 'mirror:' + m, kind: 'mirror', mode: 'mirror', url: m, label: m, deletable: true });
    }
    if (mode === 'mirror' && url && !mirrors.includes(url)) {
      rows.push({ id: 'mirror:' + url, kind: 'mirror', mode: 'mirror', url, label: url, deletable: true });
    }
    return rows;
  }

  // Which row is selected. Derived from the saved pair so the highlight can
  // never drift from what the backend will actually use.
  // 选中哪一行。由已保存的那对值派生，高亮因此不会与后端真正会走的路偏离。
  $: activePathId = (() => {
    const mode = settings.proxy_mode;
    const url = settings.proxy_url || '';
    if (mode === 'mirror' && url) return 'mirror:' + url;
    if (mode === 'manual' && url) return 'proxy:' + url;
    if (mode === 'system') return 'system';
    return 'direct';
  })();

  // --- Path measurement ---
  // Every row is timed with the real request an update check would send, out
  // through that row's own way of leaving. Fired when the page opens so the
  // user sees which path actually works right now instead of guessing;
  // "Test speed" repeats it on demand.
  //
  // 每一行都用更新检查真会发的那个请求、经该行自己的出口方式计时。页面打开时
  // 自动跑一次，让用户看到当下哪条路真的可用而不必猜；「测速」按钮按需重跑。
  let pathScan = { running: false, rows: [] };
  let newMirror = '';
  let newProxy = '';
  // Why the typed address is refused, shown next to the add rows.
  // 拒绝所填地址的原因，显示在添加行旁边。
  let pathErr = '';
  let publicIP = { loading: false, info: null, errAt: 0, error: '' };

  // Keyed by path id. A reactive map (not a template function call) so the
  // rows re-render the moment a scan lands.
  // 以通路 id 为键。响应式 map（而非模板里的函数调用），扫描一落地行就会重绘。
  $: pathSpeedMap = (() => {
    const map = {};
    for (const r of pathScan.rows || []) {
      if (!r) continue;
      if (r.mode === 'manual') map['proxy:' + r.url] = r;
      else if (r.mode === 'mirror') map['mirror:' + r.url] = r;
      else map[r.mode] = r;
    }
    return map;
  })();

  // What the OS reports for "follow system": null until loaded, '' when it
  // reports no proxy. Shown on that row so it is never taken on faith.
  // 「跟随系统」读到什么：未加载为 null，系统没配代理为空串。显示在那一行的
  // 旁边，于是这一行不必靠猜。
  let systemProxyAddr = null;

  $: publicLocation = (() => {
    const i = publicIP.info;
    if (!i || !i.ok) return '';
    return [i.country, i.region, i.city].filter((x) => x && String(x).trim() !== '').join(' / ');
  })();

  // Read the list straight off `settings` rather than the `mirrorList`
  // derived value: this runs from load(), before Svelte has flushed
  // reactive statements, so mirrorList would still be stale there.
  function currentMirrorList() {
    return Array.isArray(settings.proxy_mirrors) ? settings.proxy_mirrors : DEFAULT_MIRRORS;
  }

  async function runPathScan() {
    // Built from the raw state instead of the `pathRows` derived value: this
    // runs from load() and right after a selection, before Svelte has
    // flushed reactive statements — pathRows would still hold the previous
    // list there.
    // 用原始状态构建而不是读 `pathRows` 派生值：本函数在 load() 里、以及刚
    // 选中一行之后就会被调用，此时 Svelte 还没刷新响应式语句，pathRows 里
    // 还是上一份列表。
    const specs = buildPathRows(settings.proxy_mode, settings.proxy_url, currentMirrorList())
      .map((p) => ({ mode: p.mode, url: p.url || '' }));
    if (specs.length === 0) return;
    pathScan = { running: true, rows: pathScan.rows };
    try {
      const res = await TunnelService.TestUpdatePaths(specs);
      pathScan = { running: false, rows: Array.isArray(res) ? res : [] };
    } catch (e) {
      console.warn('path scan failed:', e);
      pathScan = { running: false, rows: [] };
    }
  }

  async function loadSystemProxy() {
    try {
      const v = await TunnelService.SystemProxy();
      systemProxyAddr = v || '';
    } catch (e) {
      systemProxyAddr = '';
    }
  }

  // Sequence token: a newer load (page reopen, toggle flip) invalidates an
  // in-flight attempt so a stale answer can never overwrite fresh state.
  // 序号令牌：新一次加载（重开页面、切换开关）会让在途尝试作废，过期的
  // 结果绝不能覆盖新状态。
  let publicIPSeq = 0;
  async function loadPublicIP() {
    if (!settings.public_ip_lookup) {
      publicIPSeq++;
      publicIP = { loading: false, info: null, errAt: 0, error: '' };
      return;
    }
    const seq = ++publicIPSeq;
    const attempt = async () => {
      const prev = publicIP.info;
      publicIP = { ...publicIP, loading: true };
      try {
        // Pass the resolved UI language so the backend can prefer a
        // geolocation provider that answers in the same language (see
        // GetPublicIPInfo). 'auto' resolves through navigator.language.
        // 传入已解析的界面语言，让后端优先选同语言的归属地服务商（见
        // GetPublicIPInfo）。'auto' 经 navigator.language 解析。
        const lang = settings.language === 'auto' ? detectLanguage() : settings.language;
        const info = await TunnelService.GetPublicIPInfo(lang);
        if (seq !== publicIPSeq) return;
        if (info?.ok) {
          publicIP = { loading: false, info, errAt: 0, error: '' };
        } else {
          // Keep showing the previous readout: an IP that was right a moment
          // ago beats an empty dash. The failure time is shown next to it.
          // 保留上次的读数：一分钟前还是对的 IP 好过空的「—」。失败时间显示
          // 在读数旁边。
          publicIP = { loading: false, info: prev, errAt: Date.now(), error: info?.error || '' };
        }
      } catch (e) {
        if (seq !== publicIPSeq) return;
        publicIP = { loading: false, info: prev, errAt: Date.now(), error: String(e) };
      }
    };
    await attempt();
    // One silent retry. On networks where the provider list is only
    // intermittently reachable (tunnel up/down transitions, GFW flakiness)
    // a first attempt can lose the race with a route change even though the
    // very same lookup succeeds seconds later — observed in real testing.
    // 静默重试一次。在服务源只能间歇性到达的网络里（隧道上下线切换、被墙
    // 抖动），首次尝试可能恰好输给一次路由变化，而几秒后同样的查询就能
    // 成功——真实测试中实测出现过。
    if (seq === publicIPSeq && !publicIP.info?.ok) {
      await new Promise((r) => setTimeout(r, 2000));
      if (seq === publicIPSeq) await attempt();
    }
  }

  $: staleTime = publicIP.errAt ? new Date(publicIP.errAt).toLocaleTimeString() : '';

  // Selecting a row replaces the whole path: the two dimensions are
  // categories, not layers, so the other one goes back to its default.
  // 选中一行就是换掉整条通路：两维是分类而非叠加，因此另一维退回默认。
  async function selectPath(row) {
    pathErr = '';
    settings.proxy_mode = row.mode;
    settings.proxy_url = row.url || '';
    settings = settings;
    // Saved first: the two follow-up lookups read the backend's own copy of
    // the settings, and a probe that runs before the write lands would
    // measure the previous path.
    // 先保存：后面两个查询读的是后端自己的那份设置，写在落地之前就探测等于
    // 在量上一条通路。
    await save();
    runPathScan();
    loadPublicIP();
  }

  async function deletePath(row) {
    pathErr = '';
    if (row.kind === 'mirror') {
      if (mirrorList.includes(row.url)) {
        settings.proxy_mirrors = currentMirrorList().filter((m) => m !== row.url);
      }
      // Deleting the row in use falls back to direct: leaving proxy_url
      // pointing at a deleted address would keep sending update checks
      // somewhere the user just rejected.
      // 删掉正在用的那一行回退到直连：让 proxy_url 继续指向被删的地址，等于
      // 继续把更新检查发往用户刚拒绝的地方。
      if (settings.proxy_mode === 'mirror' && settings.proxy_url === row.url) {
        settings.proxy_mode = 'direct';
        settings.proxy_url = '';
      }
    } else if (row.kind === 'proxy') {
      if (settings.proxy_mode === 'manual' && settings.proxy_url === row.url) {
        settings.proxy_mode = 'direct';
        settings.proxy_url = '';
      }
    }
    settings = settings;
    await save();
    runPathScan();
    loadPublicIP();
  }

  // An address the backend cannot use is refused here rather than stored:
  // PathTransport falls back to no proxy on an unparsable one, so accepting
  // it would leave a row that is selected and does nothing — and the panel
  // would never say why.
  // 后端用不了的地址在这里就拒绝而不存下来：PathTransport 遇到解析不了的地址
  // 会退回不用代理，收下它会留下「选中了却不起作用」的一行，而面板永远不会
  // 说明原因。
  const PROXY_SCHEMES = ['http:', 'https:', 'socks4:', 'socks4a:', 'socks5:', 'socks5h:'];

  function parseProxyAddr(v) {
    if (!v || /\s/.test(v)) return null;
    let u;
    try {
      u = new URL(v);
    } catch (e) {
      return null;
    }
    // A path segment has no meaning on a proxy address; accepting one would
    // only make the user believe it took effect.
    // 代理地址上的路径段没有语义，收下只会让人以为它起了作用。
    if (!PROXY_SCHEMES.includes(u.protocol) || !u.hostname) return null;
    if (u.pathname !== '' && u.pathname !== '/') return null;
    return u.href.replace(/\/+$/, '');
  }

  function parseMirrorPrefix(v) {
    if (!v || /\s/.test(v)) return null;
    let u;
    try {
      u = new URL(v);
    } catch (e) {
      return null;
    }
    if (u.protocol !== 'http:' && u.protocol !== 'https:') return null;
    if (!u.hostname) return null;
    if (u.pathname !== '' && u.pathname !== '/') return null;
    return u.origin;
  }

  // A manual address becomes the selected path immediately — an address that
  // was added but not chosen is indistinguishable from one that does nothing.
  // 手动地址一加进来就成为选中的通路——加了却没选中的地址，与「加了但不起作用」
  // 看起来毫无区别。
  async function addManualProxy() {
    const u = parseProxyAddr(String(newProxy || '').trim());
    if (!u) {
      pathErr = $t('settings.proxy_rejected');
      return;
    }
    pathErr = '';
    settings.proxy_mode = 'manual';
    settings.proxy_url = u;
    newProxy = '';
    settings = settings;
    await save();
    runPathScan();
    loadPublicIP();
  }

  function onProxyAddKey(e) {
    if (e.key === 'Enter') {
      e.preventDefault();
      addManualProxy();
    }
  }

  function addMirror() {
    const v = parseMirrorPrefix(String(newMirror || '').trim().replace(/\/+$/, ''));
    if (!v) {
      pathErr = $t('settings.proxy_mirror_rejected');
      return;
    }
    pathErr = '';
    const list = [...currentMirrorList()];
    if (!list.includes(v)) list.push(v);
    settings.proxy_mirrors = list;
    settings.proxy_mode = 'mirror';
    settings.proxy_url = v;
    newMirror = '';
    settings = settings;
    scheduleSave();
    runPathScan();
  }

  function onMirrorAddKey(e) {
    if (e.key === 'Enter') {
      e.preventDefault();
      addMirror();
    }
  }

  function restoreMirrors() {
    pathErr = '';
    settings.proxy_mirrors = [...DEFAULT_MIRRORS];
    settings = settings;
    scheduleSave();
    runPathScan();
  }

  function onPublicIPToggle(e) {
    settings.public_ip_lookup = !!e.target.checked;
    settings = settings;
    scheduleSave();
    loadPublicIP();
  }

  // Reflect settings applied by another client (the CLI) live, so the
  // toggle doesn't sit stale while the helper actually changed state.
  let settingsChangedUnsub = null;
  function onSettingsChanged(event) {
    const p = event?.data || {};
    if (p.health_check != null) settings.health_check = p.health_check;
    if (p.keep_connection_on_idle != null) settings.keep_connection_on_idle = p.keep_connection_on_idle;
    if (p.prevent_system_sleep != null) settings.prevent_system_sleep = p.prevent_system_sleep;
    if (p.pin_interface != null) settings.pin_interface = p.pin_interface;
    if (p.log_level != null) settings.log_level = p.log_level;
    // Proxy fields too — otherwise a `wireguideplus ctl` mode switch
    // leaves the select and the URL input stale until the modal is
    // reopened (and direct mode can linger showing a dead address).
    if (p.proxy_mode != null) settings.proxy_mode = p.proxy_mode;
    if (p.proxy_url != null) settings.proxy_url = p.proxy_url;
    settings = settings; // trigger reactivity
  }

  onDestroy(() => {
    window.removeEventListener('keydown', onKeyDown);
    if (settingsChangedUnsub) settingsChangedUnsub();
    if (unsubUpdateProgress) unsubUpdateProgress();
    if (saveTimer) {
      clearTimeout(saveTimer);
      save();
    }
    if (nowTimer) clearInterval(nowTimer);
  });

  function stopEvent(e) { e.stopPropagation(); }

  function handleBackdropMousedown(e) {
    if (e.target === e.currentTarget) close();
  }

  function close() {
    if (saveTimer) {
      clearTimeout(saveTimer);
      saveTimer = null;
      save();
    }
    onClose();
  }

  function onKeyDown(e) {
    if (e.key === 'Escape') {
      e.preventDefault();
      close();
    }
  }

  // Register the listener synchronously and tear it down via
  // onDestroy. An `onMount(async ...)` callback returns a Promise,
  // not a cleanup function, so the cleanup we used to put after the
  // `await` was silently dropped — every Settings open leaked another
  // window-level keydown listener that called save() on stale state.
  onMount(() => {
    window.addEventListener('keydown', onKeyDown);
    settingsChangedUnsub = Events.On('settings_changed', onSettingsChanged);
    // Mirror the backend download/install progress into the About tab
    // (same event the main window banner listens to). Torn down in
    // onDestroy below so each Settings open owns exactly one subscription.
    // 把后端下载/安装进度镜像进 About 页（与主窗口横幅监听同一事件）。
    // 在下方 onDestroy 中拆除，保证每次打开设置只持有一份订阅。
    unsubUpdateProgress = Events.On('update_progress', (e) => {
      aboutUpdatePhase = e?.data?.phase || '';
      aboutUpdatePercent = e?.data?.percent || 0;
    });
    // 60 s tick so the "Last checked Nm ago" line keeps current while
    // the Settings modal is open. Cleanup is in onDestroy above.
    nowTimer = setInterval(() => { nowTick = Date.now(); }, 60000);
    (async () => {
      try { appVersion = await TunnelService.GetVersion(); } catch (_) {}
      await refreshUpdateState();
    })();
  });
</script>

<div class="modal-backdrop" on:mousedown={handleBackdropMousedown}>
  <div class="modal" use:modalDrag={'.settings-head'} on:mousedown={stopEvent} role="dialog" aria-modal="true" tabindex="-1" aria-labelledby="settings-title">
    <div class="settings-head">
      <h3 id="settings-title">{$t('settings.title')}</h3>
      <button class="settings-close" aria-label={$t('editor.close')} title={$t('editor.close')} on:click={close}>✕</button>
    </div>

    <div class="settings-layout">
      <div class="settings-sidebar" role="tablist" aria-label="Settings sections">
        <button role="tab" aria-selected={activeTab === 'general'} class:active={activeTab === 'general'} on:click={() => activeTab = 'general'}>
          <Icon name="settings" size={14} strokeWidth={1.75} />
          <span>{$t('settings.general')}</span>
        </button>
        <button role="tab" aria-selected={activeTab === 'advanced'} class:active={activeTab === 'advanced'} on:click={() => activeTab = 'advanced'}>
          <Icon name="wrench" size={14} strokeWidth={1.75} />
          <span>{$t('settings.advanced')}</span>
        </button>
        <button role="tab" aria-selected={activeTab === 'about'} class:active={activeTab === 'about'} on:click={() => activeTab = 'about'}>
          <Icon name="info" size={14} strokeWidth={1.75} />
          <span>{$t('settings.about')}</span>
        </button>
      </div>

      <div class="settings-content" role="tabpanel">
        {#if activeTab === 'general'}
          <div class="settings-section">
            <h4 class="section-title">{$t('settings.section_appearance')}</h4>
            <div class="settings-card">
              <div class="setting-row">
                <label class="setting-label" for="theme-select">{$t('settings.theme')}</label>
                <select id="theme-select" value={settings.theme} on:change={onThemeChange}>
                  <option value="dark">{$t('settings.theme_dark')}</option>
                  <option value="light">{$t('settings.theme_light')}</option>
                  <option value="system">{$t('settings.theme_system')}</option>
                </select>
              </div>
              <div class="setting-row">
                <label class="setting-label" for="lang-select">{$t('settings.language')}</label>
                <select id="lang-select" value={settings.language} on:change={onLanguageChange}>
                  <option value="auto">{$t('settings.lang_auto')}</option>
                  <option value="en">English</option>
                  <option value="ko">한국어</option>
                  <option value="ja">日本語</option>
                  <option value="zh">简体中文</option>
                  <option value="zh-TW">繁體中文</option>
                </select>
              </div>
              <div class="setting-row setting-row--toggle">
                <div class="setting-info">
                  <label class="setting-label" for="compact-list">{$t('settings.compact_list')}</label>
                  <p class="setting-desc">{$t('settings.compact_list_hint')}</p>
                </div>
                <label class="toggle">
                  <input id="compact-list" type="checkbox" checked={settings.compact_list} on:change={onCompactListChange} />
                  <span class="toggle-track"></span>
                </label>
              </div>
            </div>
          </div>

          <div class="settings-section">
            <h4 class="section-title">{$t('settings.section_startup')}</h4>
            <div class="settings-card">
              <div class="setting-row setting-row--toggle">
                <div class="setting-info">
                  <label class="setting-label" for="auto-start">{$t('settings.auto_start')}</label>
                  <p class="setting-desc">{$t('settings.auto_start_hint')}</p>
                </div>
                <label class="toggle">
                  <input id="auto-start" type="checkbox" checked={settings.auto_start} on:change={onAutoStartChange} />
                  <span class="toggle-track"></span>
                </label>
              </div>
              <div class="setting-row setting-row--toggle">
                <div class="setting-info">
                  <label class="setting-label" for="start-minimized">{$t('settings.start_minimized')}</label>
                  <p class="setting-desc">{$t('settings.start_minimized_hint')}</p>
                </div>
                <label class="toggle">
                  <input id="start-minimized" type="checkbox" checked={settings.start_minimized} on:change={onStartMinimizedChange} />
                  <span class="toggle-track"></span>
                </label>
              </div>
              <div class="setting-row setting-row--toggle">
                <div class="setting-info">
                  <label class="setting-label" for="disconnect-on-quit">{$t('settings.disconnect_on_quit')}</label>
                  <p class="setting-desc">{$t('settings.disconnect_on_quit_hint')}</p>
                </div>
                <label class="toggle">
                  <input id="disconnect-on-quit" type="checkbox" checked={settings.disconnect_on_quit} on:change={onDisconnectOnQuitChange} />
                  <span class="toggle-track"></span>
                </label>
              </div>
              <div class="setting-row">
                <div class="setting-info">
                  <label class="setting-label" for="notify-duration">{$t('settings.notify_duration')}</label>
                  <p class="setting-desc">{$t('settings.notify_duration_hint')}</p>
                </div>
                <!-- value= + on:change (NOT bind:value) + numeric option
                     values — exactly the pattern used by the log-retention
                     select below, which persists reliably. The static
                     <option value="5000"> string form caused number/string
                     mismatches in some Svelte versions, so options are
                     rendered from VALID_NOTIFY_DURATIONS (numbers), and
                     load() normalizes into the offered set so it can't
                     render blank either. -->
                <select id="notify-duration" value={settings.notify_duration_ms} on:change={onNotifyDurationChange}>
                  {#each VALID_NOTIFY_DURATIONS as ms}
                    <option value={ms}>{ms / 1000}s</option>
                  {/each}
                </select>
              </div>
            </div>
          </div>

          <div class="settings-section">
            <h4 class="section-title">{$t('settings.section_updates')}</h4>
            <div class="settings-card">
              <div class="setting-row setting-row--toggle">
                <div class="setting-info">
                  <label class="setting-label" for="auto-update">{$t('settings.auto_update_check')}</label>
                  <p class="setting-desc">{$t('settings.auto_update_check_hint')}</p>
                </div>
                <label class="toggle">
                  <input id="auto-update" type="checkbox" bind:checked={settings.auto_update_check} on:change={scheduleSave} />
                  <span class="toggle-track"></span>
                </label>
              </div>
            </div>
          </div>

          <div class="settings-section">
            <h4 class="section-title">{$t('settings.proxy')}</h4>
            <div class="settings-card">
              <!-- Where this machine appears to be — the fastest way to see
                   whether the configured path actually changes the egress. -->
              <div class="setting-row">
                <div class="setting-info">
                  <label class="setting-label">{$t('settings.public_ip')}</label>
                  <p class="setting-desc">
                    {#if !settings.public_ip_lookup}
                      {$t('settings.public_ip_disabled')}
                    {:else if publicIP.loading && !publicIP.info}
                      {$t('settings.public_ip_loading')}
                    {:else if publicIP.info?.ok}
                      {publicLocation}{#if publicIP.info.isp} · {publicIP.info.isp}{/if}
                      {#if publicIP.info.provider}
                        {' '}— {$t('settings.public_ip_hint', { provider: publicIP.info.provider })}
                      {/if}
                      {#if publicIP.errAt}
                        {' '}— <span class="public-ip-stale">{$t('settings.public_ip_stale', { time: staleTime })}</span>
                      {/if}
                    {:else if publicIP.info || publicIP.errAt}
                      {$t('settings.public_ip_failed', { err: publicIP.info?.error || publicIP.error || '' })}
                    {/if}
                  </p>
                </div>
                <div class="public-ip-value">{publicIP.info?.ok ? publicIP.info.ip : '—'}</div>
              </div>
              <div class="setting-row">
                <div class="setting-info">
                  <label class="setting-label" for="public-ip-toggle">{$t('settings.public_ip_toggle')}</label>
                </div>
                <input id="public-ip-toggle" type="checkbox"
                  checked={settings.public_ip_lookup}
                  on:change={onPublicIPToggle} />
              </div>
              <!-- The path list: one row per way an update request can go
                   out, single-select. The measured latency tells the user
                   which one to keep. -->
              <div class="setting-row setting-row--column">
                <div class="path-list-head">
                  <div class="setting-info">
                    <h5 class="path-subhead">{$t('settings.proxy_path')}</h5>
                    <p class="setting-desc">{$t('settings.proxy_path_hint')}</p>
                  </div>
                  <button class="btn-secondary path-retest" type="button"
                    on:click={runPathScan} disabled={pathScan.running}>
                    {pathScan.running ? $t('settings.proxy_scanning') : $t('settings.proxy_test_speed')}
                  </button>
                </div>
                <div class="path-list">
                  {#each pathRows as p (p.id)}
                    <label class="path-row" class:path-row--active={p.id === activePathId}>
                      <input type="radio" name="update-path"
                        checked={p.id === activePathId}
                        on:change={() => selectPath(p)} />
                      <span class="path-label" title={p.label || ''}>
                        {p.labelKey ? $t(p.labelKey) : p.label}
                      </span>
                      {#if p.kind === 'system'}
                        <!-- Naming the address the OS reports keeps this row
                             from being a guess the user has to trust. -->
                        <span class="path-note" title={systemProxyAddr === null ? '' : (systemProxyAddr || $t('settings.proxy_system_none'))}>
                          {systemProxyAddr === null ? '' : (systemProxyAddr || $t('settings.proxy_system_none'))}
                        </span>
                      {/if}
                      <span class="path-ms"
                        class:ok={pathSpeedMap[p.id]?.ok}
                        class:bad={pathSpeedMap[p.id] && !pathSpeedMap[p.id].ok}>
                        {#if pathScan.running}
                          {$t('settings.proxy_scanning')}
                        {:else if pathSpeedMap[p.id]?.ok}
                          {$t('settings.proxy_test_ok', { n: pathSpeedMap[p.id].latency_ms })}
                        {:else if pathSpeedMap[p.id]}
                          {$t('settings.proxy_test_fail', { err: pathSpeedMap[p.id].error })}
                        {/if}
                      </span>
                      {#if p.deletable}
                        <!-- Inside a <label>, so the click has to be stopped
                             from also flipping the radio. -->
                        <button class="path-del" type="button" title={$t('settings.proxy_mirror_delete')}
                          on:click={(e) => { e.preventDefault(); e.stopPropagation(); deletePath(p); }}>×</button>
                      {/if}
                    </label>
                  {/each}
                  <p class="setting-desc path-speed-hint">{$t('settings.proxy_speed_hint')}</p>
                  <div class="path-add">
                    <span class="path-add-label">{$t('settings.proxy_manual')}</span>
                    <input type="text" bind:value={newProxy}
                      placeholder={$t('settings.proxy_url_placeholder')}
                      on:keydown={onProxyAddKey} />
                    <button class="btn-secondary" type="button" on:click={addManualProxy}>
                      {$t('settings.proxy_mirror_add')}
                    </button>
                  </div>
                  <div class="path-add">
                    <span class="path-add-label">{$t('settings.proxy_mirror_custom')}</span>
                    <input type="text" bind:value={newMirror}
                      placeholder={$t('settings.proxy_mirror_add_placeholder')}
                      on:keydown={onMirrorAddKey} />
                    <button class="btn-secondary" type="button" on:click={addMirror}>
                      {$t('settings.proxy_mirror_add')}
                    </button>
                    <button class="btn-secondary" type="button" on:click={restoreMirrors}>
                      {$t('settings.proxy_mirror_restore')}
                    </button>
                  </div>
                  {#if pathErr}
                    <p class="setting-desc path-err">{pathErr}</p>
                  {/if}
                  <p class="setting-desc">{$t('settings.proxy_path_proxy_hint')}</p>
                  <p class="setting-desc">{$t('settings.proxy_mirror_hint')}</p>
                </div>
              </div>
            </div>
          </div>

        {:else if activeTab === 'advanced'}
          <div class="settings-section">
            <h4 class="section-title">{$t('settings.section_wg_scripts')}</h4>
            <div class="settings-card">
              <div class="setting-row setting-row--toggle">
                <div class="setting-info">
                  <label class="setting-label" for="wg-scripts">{$t('settings.wg_scripts')}</label>
                  <p class="setting-desc">{$t('settings.wg_scripts_hint')}</p>
                </div>
                <label class="toggle">
                  <input id="wg-scripts" type="checkbox" checked={settings.enable_wg_scripts} on:change={onWgScriptsChange} />
                  <span class="toggle-track"></span>
                </label>
              </div>
              {#if wgScriptsWarn}
                <div class="wg-scripts-warn">
                  <p class="wg-scripts-warn-text">{$t('settings.wg_scripts_warn')}</p>
                  <div class="wg-scripts-warn-actions">
                    <button class="btn btn--primary" on:click={onWgScriptsWarnConfirm}>{$t('settings.wg_scripts_enable')}</button>
                    <button class="btn" on:click={onWgScriptsWarnCancel}>{$t('settings.wg_scripts_cancel')}</button>
                  </div>
                </div>
              {/if}
              {#if settings.enable_wg_scripts}
                <div class="setting-row">
                  <div class="setting-info">
                    <span class="setting-label">{$t('settings.scripts_folder')}</span>
                    <p class="setting-desc">{$t('settings.scripts_folder_hint')}</p>
                  </div>
                  <button class="folder-btn" on:click={() => openFolder('scripts')} disabled={openingFolder}>
                    {$t('settings.open_folder')}
                  </button>
                </div>
              {/if}
            </div>
          </div>

          <div class="settings-section">
            <h4 class="section-title">{$t('settings.section_awg')}</h4>
            <div class="settings-card">
              <div class="setting-row setting-row--toggle">
                <div class="setting-info">
                  <label class="setting-label" for="awg-support">{$t('settings.enable_awg')}</label>
                  <p class="setting-desc">{$t('settings.enable_awg_hint')}</p>
                </div>
                <label class="toggle">
                  <input id="awg-support" type="checkbox" checked={settings.enable_awg} on:change={onAwgChange} />
                  <span class="toggle-track"></span>
                </label>
              </div>
              {#if awgWarn}
                <!-- Inline highlight bar, identical structure and styling to
                     the wg-scripts confirm — a nested modal was explicitly
                     rejected by the user. -->
                <div class="wg-scripts-warn">
                  <p class="wg-scripts-warn-text">{$t('settings.awg_confirm')}</p>
                  <div class="wg-scripts-warn-actions">
                    <button class="btn btn--primary" on:click={onAwgWarnConfirm}>{$t('settings.awg_confirm_enable')}</button>
                    <button class="btn" on:click={onAwgWarnCancel}>{$t('settings.awg_confirm_cancel')}</button>
                  </div>
                </div>
              {/if}
            </div>
          </div>

          <div class="settings-section">
            <h4 class="section-title">{$t('settings.section_connection')}</h4>
            <div class="settings-card">
              <div class="setting-row setting-row--toggle">
                <div class="setting-info">
                  <label class="setting-label" for="health-check">{$t('settings.health_check')}</label>
                  <p class="setting-desc">{$t('settings.health_check_hint')}</p>
                </div>
                <label class="toggle">
                  <input id="health-check" type="checkbox"
                    checked={settings.health_check}
                    on:change={onHealthCheckChange} />
                  <span class="toggle-track"></span>
                </label>
              </div>
              <div class="setting-row setting-row--toggle">
                <div class="setting-info">
                  <label class="setting-label" for="keep-connection-on-idle">{$t('settings.keep_connection_on_idle')}</label>
                  <p class="setting-desc">{$t('settings.keep_connection_on_idle_hint')}</p>
                </div>
                <label class="toggle">
                  <input id="keep-connection-on-idle" type="checkbox"
                    checked={settings.keep_connection_on_idle}
                    on:change={onKeepConnectionOnIdleChange} />
                  <span class="toggle-track"></span>
                </label>
              </div>
              <div class="setting-row setting-row--toggle">
                <div class="setting-info">
                  <label class="setting-label" for="prevent-system-sleep">{$t('settings.prevent_system_sleep')}</label>
                  <p class="setting-desc">{$t('settings.prevent_system_sleep_hint')}</p>
                </div>
                <label class="toggle">
                  <input id="prevent-system-sleep" type="checkbox"
                    checked={settings.prevent_system_sleep}
                    on:change={onPreventSystemSleepChange} />
                  <span class="toggle-track"></span>
                </label>
              </div>
              <div class="setting-row setting-row--toggle">
                <div class="setting-info">
                  <label class="setting-label" for="pin-interface">{$t('settings.pin_interface')}</label>
                  <p class="setting-desc">{$t('settings.pin_interface_hint')}</p>
                </div>
                <label class="toggle">
                  <input id="pin-interface" type="checkbox"
                    checked={settings.pin_interface}
                    on:change={onPinInterfaceChange} />
                  <span class="toggle-track"></span>
                </label>
              </div>
            </div>
          </div>

          <div class="settings-section">
            <h4 class="section-title">{$t('settings.endpoint_failover')}</h4>
            <div class="settings-card">
              <div class="setting-row setting-row--toggle">
                <div class="setting-info">
                  <label class="setting-label" for="endpoint-failover-master">{$t('settings.endpoint_failover_master')}</label>
                  <p class="setting-desc">{$t('settings.endpoint_failover_hint')}</p>
                </div>
                <label class="toggle">
                  <input id="endpoint-failover-master" type="checkbox"
                    checked={settings.endpoint_failover_enabled}
                    on:change={(e) => { settings.endpoint_failover_enabled = e.target.checked; scheduleSave(); }} />
                  <span class="toggle-track"></span>
                </label>
              </div>
              <div class="setting-row">
                <div class="setting-info">
                  <label class="setting-label" for="endpoint-threshold">{$t('settings.endpoint_threshold')}</label>
                  <p class="setting-desc">{$t('settings.endpoint_threshold_hint')}</p>
                </div>
                <input id="endpoint-threshold" type="number" min="1" step="1"
                  disabled={!settings.endpoint_failover_enabled}
                  bind:value={settings.endpoint_threshold_ms} on:change={scheduleSave} />
              </div>
              <div class="setting-row">
                <div class="setting-info">
                  <label class="setting-label" for="endpoint-margin">{$t('settings.endpoint_margin')}</label>
                  <p class="setting-desc">{$t('settings.endpoint_margin_hint')}</p>
                </div>
                <input id="endpoint-margin" type="number" min="1" step="1"
                  disabled={!settings.endpoint_failover_enabled}
                  bind:value={settings.endpoint_margin_pct} on:change={scheduleSave} />
              </div>
              <div class="setting-row">
                <div class="setting-info">
                  <label class="setting-label" for="endpoint-sustained">{$t('settings.endpoint_sustained')}</label>
                  <p class="setting-desc">{$t('settings.endpoint_sustained_hint')}</p>
                </div>
                <input id="endpoint-sustained" type="number" min="0" step="1"
                  disabled={!settings.endpoint_failover_enabled}
                  bind:value={settings.endpoint_sustained_sec} on:change={scheduleSave} />
              </div>
              <div class="setting-row">
                <div class="setting-info">
                  <label class="setting-label" for="endpoint-cooldown">{$t('settings.endpoint_cooldown')}</label>
                  <p class="setting-desc">{$t('settings.endpoint_cooldown_hint')}</p>
                </div>
                <input id="endpoint-cooldown" type="number" min="240" step="1"
                  disabled={!settings.endpoint_failover_enabled}
                  bind:value={settings.endpoint_cooldown_sec} on:change={scheduleSave} />
              </div>
            </div>
          </div>

          <div class="settings-section">
            <h4 class="section-title">{$t('settings.section_dns')}</h4>
            <div class="settings-card">
              <div class="setting-row setting-row--toggle">
                <div class="setting-info">
                  <label class="setting-label" for="dns-resolve-path">{$t('settings.dns_resolve_path')}</label>
                  <p class="setting-desc">{$t('settings.dns_resolve_path_hint')}</p>
                </div>
                <label class="toggle">
                  <input id="dns-resolve-path" type="checkbox"
                    checked={settings.dns_resolve_path}
                    on:change={onDnsResolvePathChange} />
                  <span class="toggle-track"></span>
                </label>
              </div>
            </div>
          </div>

          <div class="settings-section">
            <h4 class="section-title">{$t('settings.section_logging')}</h4>
            <div class="settings-card">
              <div class="setting-row">
                <label class="setting-label" for="log-level">{$t('settings.log_level')}</label>
                <select id="log-level" value={settings.log_level} on:change={onLogLevelChange}>
                  <option value="all">{$t('settings.log_level_all')}</option>
                  <option value="debug">{$t('settings.log_level_debug')}</option>
                  <option value="info">{$t('settings.log_level_info')}</option>
                  <option value="warn">{$t('settings.log_level_warn')}</option>
                  <option value="error">{$t('settings.log_level_error')}</option>
                </select>
              </div>
              <div class="setting-row">
                <div class="setting-info">
                  <label class="setting-label" for="log-retention">{$t('settings.log_retention')}</label>
                  <p class="setting-desc">{$t('settings.log_retention_hint')}</p>
                </div>
                <select id="log-retention" value={settings.log_retention_days} on:change={onLogRetentionChange}>
                  {#each VALID_LOG_RETENTION_DAYS as days}
                    <option value={days}>{$t('settings.log_retention_days', { n: days })}</option>
                  {/each}
                </select>
              </div>
              <div class="setting-row">
                <div class="setting-info">
                  <label class="setting-label" for="history-retention">{$t('settings.history_retention')}</label>
                  <p class="setting-desc">{$t('settings.history_retention_hint')}</p>
                </div>
                <select id="history-retention" value={settings.history_retention_days} on:change={onHistoryRetentionChange}>
                  <option value="0">{$t('settings.history_retention_off')}</option>
                  {#each VALID_HISTORY_RETENTION_DAYS as days}
                    {#if days !== 0}
                      <option value={days}>{$t('settings.log_retention_days', { n: days })}</option>
                    {/if}
                  {/each}
                </select>
              </div>
              <div class="setting-row">
                <div class="setting-info">
                  <span class="setting-label">{$t('settings.log_folder')}</span>
                  <p class="setting-desc">{$t('settings.log_folder_hint')}</p>
                </div>
                <button class="folder-btn" on:click={() => openFolder('logs')} disabled={openingFolder}>
                  {$t('settings.open_folder')}
                </button>
              </div>
              <div class="setting-row">
                <div class="setting-info">
                  <span class="setting-label">{$t('settings.tunnels_folder')}</span>
                  <p class="setting-desc">{$t('settings.tunnels_folder_hint')}</p>
                </div>
                <button class="folder-btn" on:click={() => openFolder('tunnels')} disabled={openingFolder}>
                  {$t('settings.open_folder')}
                </button>
              </div>
              {#if folderError}
                <div class="folder-error">{$t('settings.open_folder_failed')}: {folderError}</div>
              {/if}
            </div>
          </div>

          <div class="settings-section">
            <h4 class="section-title">{$t('settings.section_backup')}</h4>
            <div class="settings-card">
              <div class="setting-row">
                <div class="setting-info">
                  <span class="setting-label">{$t('settings.export_settings')}</span>
                  <p class="setting-desc">{$t('settings.export_settings_hint')}</p>
                </div>
                <button class="folder-btn" on:click={exportSettings} disabled={backupBusy}>
                  {$t('settings.export_settings')}
                </button>
              </div>
              <div class="setting-row">
                <div class="setting-info">
                  <span class="setting-label">{$t('settings.import_settings')}</span>
                  <p class="setting-desc">{$t('settings.import_settings_hint')}</p>
                </div>
                <button class="folder-btn" on:click={importSettings} disabled={backupBusy}>
                  {$t('settings.import_settings')}
                </button>
              </div>
            </div>
          </div>

        {:else if activeTab === 'about'}
          <div class="about-tab">
            <div class="about-hero">
              <img src="/appicon.png" alt="WireGuide Plus" class="about-logo" />
              <div class="about-hero-text">
                <h2 class="about-name">WireGuide Plus</h2>
                <p class="about-tagline-text">{$t('settings.about_tagline')}</p>
                <div class="about-pills">
                  <span class="pill pill-version">{appVersion ? `v${appVersion}` : 'v—'}</span>
                  {#if updateInfo?.available}
                    <button class="pill pill-update" on:click={aboutRequestUpdate} disabled={aboutUpdating}>
                      <span class="pill-dot"></span>
                      {aboutUpdating ? $t('update.updating') : $t('update.update_now')}
                    </button>
                    <button class="pill pill-release" on:click={aboutOpenRelease} disabled={aboutUpdating}>
                      {$t('update.open_release_page')}
                    </button>
                  {:else}
                    <span class="pill pill-ok">
                      <Icon name="check" size={11} strokeWidth={2.5} />
                      {$t('settings.up_to_date')}
                    </span>
                  {/if}
                </div>
                {#if aboutUpdating}
                  <div class="about-progress" role="status" aria-live="polite">
                    <div class="about-progress-label">{aboutPhaseLabel || $t('update.updating')}</div>
                    <div class="about-progress-track">
                      <div class="about-progress-fill" style="width: {aboutUpdatePercent}%"></div>
                    </div>
                  </div>
                {/if}
                <div class="about-check-row">
                  <button class="check-btn" on:click={aboutCheckNow} disabled={aboutChecking}>
                    {aboutChecking ? $t('update.checking') : $t('update.check_now')}
                  </button>
                  <span class="check-meta">{lastCheckedLabel}</span>
                </div>
                {#if updateState?.is_dev_build}
                  <!-- Explains why "Never checked" sticks even on a healthy install:
                       dev builds intentionally skip the auto-check loop so local
                       iteration doesn't burn the GitHub API quota. Manual button
                       still works, hence the "use the button" framing. -->
                  <div class="check-hint">{$t('update.dev_build_hint')}</div>
                {:else if updateState && !updateState.auto_enabled}
                  <div class="check-hint">{$t('update.auto_off_hint')}</div>
                {/if}
                {#if aboutCheckResult}
                  <div class="check-result">{aboutCheckResult}</div>
                {/if}
              </div>
            </div>

            <p class="about-desc-paragraph">{$t('settings.about_desc')}</p>

            {#if aboutShowVpnWarn}
              <div class="about-vpn-warn">
                <Icon name="triangle-alert" size={16} strokeWidth={2} />
                <div class="vpn-warn-body">
                  <p>{$t('update.vpn_warning')}</p>
                  <div class="vpn-warn-actions">
                    <button class="warn-btn warn-proceed" on:click={doAboutUpdate}>{$t('update.proceed')}</button>
                    <button class="warn-btn warn-cancel" on:click={() => aboutShowVpnWarn = false}>{$t('update.cancel')}</button>
                  </div>
                </div>
              </div>
            {/if}

            <div class="about-links">
              <button class="link-btn" on:click={() => TunnelService.OpenURL('https://github.com/imonior/wireguide-plus')}>GitHub</button>
              <button class="link-btn" on:click={() => TunnelService.OpenURL('https://github.com/imonior/wireguide-plus/releases')}>{$t('settings.about_releases')}</button>
              <button class="link-btn" on:click={() => TunnelService.OpenURL('https://github.com/imonior/wireguide-plus/issues')}>{$t('settings.about_issues')}</button>
              <button class="link-btn" on:click={() => TunnelService.OpenURL('https://github.com/imonior/wireguide-plus/blob/main/LICENSE')}>{$t('settings.about_license')}</button>
            </div>

            <p class="about-credits-line">{$t('settings.about_credits')}</p>
            <p class="about-credits-line about-fork-line">
              <span>{$t('settings.about_forked_from')}</span>
              <button class="footer-link" on:click={() => TunnelService.OpenURL('https://github.com/korjwl1/wireguide')}>korjwl1/wireguide</button>
              <span>{$t('settings.about_fork_thanks')}</span>
            </p>
          </div>
        {/if}
      </div>
    </div>

    <div class="modal-footer">
      <div class="footer-info">
        <span class="footer-credit-label">{$t('settings.about_made_by')}</span>
        <button class="footer-link" on:click={() => TunnelService.OpenURL('https://github.com/imonior')}>@imonior</button>
        <span class="footer-sep">·</span>
        <span class="footer-credit-text">{$t('settings.about_footer_copyright')}</span>
      </div>
      <button type="button" class="btn-close" on:mousedown|stopPropagation={close}>{$t('settings.close')}</button>
    </div>
  </div>
</div>

<style>
  .modal-backdrop {
    position: fixed;
    inset: 0;
    background: var(--overlay-bg, rgba(0,0,0,0.45));
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 200;
  }
  .modal {
    background: var(--bg-primary);
    border: 0.5px solid var(--border);
    border-radius: 14px;
    padding: 20px 22px 14px;
    width: 720px;
    height: 520px;
    display: flex;
    flex-direction: column;
    box-shadow: var(--shadow-lg);
    overflow: hidden;
    box-sizing: border-box;
    position: relative;
    resize: both;
    min-width: 560px;
    min-height: 420px;
    max-width: calc(100vw - 40px);
    max-height: calc(100vh - 40px);
  }
  /* Title row doubles as the drag handle for the dialog. */
  .settings-head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 12px;
    flex-shrink: 0;
    margin-bottom: 14px;
    cursor: move;
    user-select: none;
    touch-action: none;
  }
  .settings-head h3 {
    margin: 0;
  }
  .settings-close {
    width: 26px;
    height: 26px;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    padding: 0;
    border: 0.5px solid var(--border);
    border-radius: 7px;
    background: var(--bg-card);
    color: var(--text-secondary);
    font: 500 13px/13px var(--font-sans);
    cursor: pointer;
    flex-shrink: 0;
  }
  .settings-close:hover {
    background: var(--bg-hover);
    border-color: color-mix(in srgb, var(--red) 40%, var(--border));
    color: var(--red);
  }
  h3 {
    margin: 0 0 14px;
    font: 700 16px/22px var(--font-sans, -apple-system, BlinkMacSystemFont, sans-serif);
    color: var(--text-primary);
    letter-spacing: -0.01em;
    flex-shrink: 0;
  }

  /* Split layout — fixed height with internal scroll */
  .settings-layout {
    display: flex;
    gap: 16px;
    flex: 1;
    min-height: 0;
    overflow: hidden;
  }

  /* Sidebar — sits on the same modal surface as the content area.
     No background panel, no border. Visual hierarchy comes from
     the active tab item's own fill + content-area structure
     (Linear / Notion / macOS System Settings pattern). */
  .settings-sidebar {
    display: flex;
    flex-direction: column;
    gap: 2px;
    width: 148px;
    flex-shrink: 0;
    padding: 0 6px 0 0;
    overflow-y: auto;
  }
  .settings-sidebar button {
    display: flex;
    align-items: center;
    gap: 9px;
    padding: 8px 10px;
    background: none;
    border: none;
    border-radius: 8px;
    color: var(--text-secondary);
    font: 500 13px/18px var(--font-sans, -apple-system, BlinkMacSystemFont, sans-serif);
    cursor: pointer;
    text-align: left;
    min-height: 34px;
    width: 100%;
    letter-spacing: -0.005em;
  }
  .settings-sidebar button :global(.icon) {
    color: var(--text-muted);
    flex-shrink: 0;
  }
  .settings-sidebar button:hover {
    background: var(--bg-hover);
    color: var(--text-primary);
  }
  .settings-sidebar button:hover :global(.icon) {
    color: var(--text-primary);
  }
  /* Active state: neutral gray fill + bold primary text + accent ICON.
     macOS HIG sidebar pattern — only the icon carries brand color,
     the row itself stays calm so the user's eye lands on content,
     not the active nav row. */
  .settings-sidebar button.active {
    background: var(--bg-active);
    color: var(--text-primary);
    font-weight: 600;
  }
  .settings-sidebar button.active :global(.icon) {
    color: var(--accent);
  }
  .settings-sidebar button:focus,
  .settings-sidebar button:focus-visible {
    outline: none;
  }
  @media (prefers-reduced-motion: no-preference) {
    .settings-sidebar button {
      transition: background-color 140ms ease, color 140ms ease;
    }
  }

  /* Content area — independent scroll */
  .settings-content {
    flex: 1;
    min-width: 0;
    overflow-y: auto;
    padding-right: 4px;
  }
  .settings-content::-webkit-scrollbar { width: 8px; }
  .settings-content::-webkit-scrollbar-track { background: transparent; }
  .settings-content::-webkit-scrollbar-thumb {
    background-color: color-mix(in srgb, var(--text-muted) 40%, transparent);
    border-radius: 4px;
    border: 2px solid transparent;
    background-clip: content-box;
  }
  .settings-content::-webkit-scrollbar-thumb:hover {
    background-color: var(--text-muted);
  }

  /* ---- Section + Card layout (M3 / iOS Settings style) ---- */
  .settings-section {
    margin-bottom: var(--space-4, 12px);
  }
  .section-title {
    margin: 0 0 6px var(--space-1, 4px);
    font: 500 10px/13px var(--font-sans, -apple-system, BlinkMacSystemFont, sans-serif);
    color: var(--text-muted);
    text-transform: uppercase;
    letter-spacing: 0.08em;
  }
  .settings-card {
    background: var(--bg-card);
    border: 0.5px solid var(--border);
    border-radius: 10px;
    overflow: hidden;
  }

  /* Setting rows inside cards — spacing alone separates (no hairlines).
     The card surface itself is the grouping cue (Linear / Notion pattern). */
  .setting-row {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding: 11px 14px;
    gap: var(--space-3);
  }
  .setting-row + .setting-row {
    padding-top: 3px;
  }
  .setting-row--toggle {
    align-items: flex-start;
    padding: 12px 14px;
  }
  .setting-row--toggle + .setting-row--toggle {
    padding-top: 4px;
  }
  .setting-info {
    flex: 1;
    min-width: 0;
  }
  .setting-label {
    display: block;
    font: 400 13px/18px var(--font-sans, -apple-system, BlinkMacSystemFont, sans-serif);
    color: var(--text-primary);
    cursor: pointer;
  }
  .setting-desc {
    margin: 3px 0 0;
    padding: 0;
    font: 400 11px/15px var(--font-sans, -apple-system, BlinkMacSystemFont, sans-serif);
    color: var(--text-muted);
    letter-spacing: 0.01em;
  }

  /* ---- Toggle switch (34×20px, 200ms slide) ---- */
  .toggle {
    position: relative;
    display: inline-block;
    width: 34px;
    height: 20px;
    flex-shrink: 0;
    cursor: pointer;
    margin-top: 1px;
  }
  .toggle input {
    opacity: 0;
    /* Pin the hidden native checkbox to the full .toggle area so any click
       on the track / slider lands on the checkbox element — otherwise an
       absolutely-positioned element with width:0 / height:0 and no inset
       sits at the label's normal-flow end, which means the visual toggle
       surface and the actual event surface can diverge (especially at the
       tail of a setting-card when there is no outer <label for="…"> that
       could forward the click by ID). The track/slider are decoration. */
    inset: 0;
    width: 100%;
    height: 100%;
    position: absolute;
    cursor: inherit;
    margin: 0;
  }
  .toggle-track {
    position: absolute;
    inset: 0;
    background: color-mix(in srgb, var(--text-muted) 35%, var(--bg-input));
    border-radius: 10px;
    border: 0.5px solid color-mix(in srgb, var(--border) 60%, transparent);
  }
  .toggle-track::before {
    content: '';
    position: absolute;
    width: 16px;
    height: 16px;
    left: 2px;
    top: 2px;
    background: #fff;
    border-radius: 50%;
    box-shadow: 0 1px 3px rgba(0, 0, 0, 0.25);
  }
  @media (prefers-reduced-motion: no-preference) {
    .toggle-track {
      transition: background-color 200ms ease, border-color 200ms ease;
    }
    .toggle-track::before {
      transition: transform 200ms ease;
    }
  }
  .toggle input:checked + .toggle-track {
    background: var(--green);
    border-color: var(--green);
  }
  .toggle input:checked + .toggle-track::before {
    transform: translateX(14px);
  }
  .toggle input:focus-visible + .toggle-track {
    outline: 2px solid var(--accent);
    outline-offset: 2px;
  }

  select {
    -webkit-appearance: none;
    appearance: none;
    height: 28px;
    padding: 0 28px 0 12px;
    background-color: var(--bg-input);
    background-image: url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='10' height='6' viewBox='0 0 10 6'%3E%3Cpath d='M1 1l4 4 4-4' stroke='%233C3C43' stroke-opacity='.56' stroke-width='1.5' fill='none' stroke-linecap='round' stroke-linejoin='round'/%3E%3C/svg%3E");
    background-repeat: no-repeat;
    background-position: right 9px center;
    border: 1px solid rgba(60, 60, 67, 0.5);
    border-radius: 6px;
    color: var(--text-primary);
    font-size: 13px;
    font-family: var(--font-sans, -apple-system, BlinkMacSystemFont, sans-serif);
    cursor: pointer;
  }
  :global([data-theme="dark"]) select {
    background-image: url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='10' height='6' viewBox='0 0 10 6'%3E%3Cpath d='M1 1l4 4 4-4' stroke='%23FFFFFF' stroke-opacity='.55' stroke-width='1.5' fill='none' stroke-linecap='round' stroke-linejoin='round'/%3E%3C/svg%3E");
    border-color: rgba(84, 84, 88, 0.72);
  }
  /* The native listbox popup renders in a separate tree, so scoped selectors
     cannot reach it. Color options explicitly to match the theme; otherwise
     they inherit the select's light text on the popup's default (light)
     background and appear blank in dark mode. */
  :global(select option) {
    background-color: var(--bg-card);
    color: var(--text-primary);
  }
  input[type="text"] {
    height: 28px;
    padding: 0 12px;
    background-color: var(--bg-input);
    border: 1px solid rgba(60, 60, 67, 0.5);
    border-radius: 6px;
    color: var(--text-primary);
    font-size: 13px;
    font-family: var(--font-sans, -apple-system, BlinkMacSystemFont, sans-serif);
    box-sizing: border-box;
    min-width: 200px;
  }
  :global([data-theme="dark"]) input[type="text"] {
    border-color: rgba(84, 84, 88, 0.72);
  }
  .setting-row--test {
    justify-content: flex-end;
  }
  .proxy-test-wrap {
    display: flex;
    align-items: center;
    gap: 10px;
    flex-wrap: wrap;
    justify-content: flex-end;
  }
  /* ---- Public IP readout + managed mirror list ---- */
  .setting-row--column {
    flex-direction: column;
    align-items: stretch;
    gap: 8px;
  }
  .public-ip-value {
    font: 500 13px/18px var(--font-mono, ui-monospace, SFMono-Regular, Menlo, monospace);
    color: var(--text-primary);
    white-space: nowrap;
  }
  .public-ip-stale {
    color: var(--text-muted);
  }
  .path-list-head {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 8px;
    width: 100%;
  }
  .path-subhead {
    margin: 0;
    font: 600 12px/16px var(--font-sans);
    color: var(--text-primary);
  }
  .path-retest {
    flex-shrink: 0;
    white-space: nowrap;
  }
  .path-list {
    display: flex;
    flex-direction: column;
    gap: 4px;
  }
  .path-row {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 4px 6px;
    border-radius: 6px;
    background-color: var(--bg-secondary, rgba(120, 120, 128, 0.08));
    cursor: pointer;
  }
  .path-row--active {
    box-shadow: inset 0 0 0 1px var(--green);
  }
  .path-row input[type="radio"] {
    flex-shrink: 0;
    margin: 0;
  }
  /* Addresses and prefixes are facts, so they are set in monospace and
     never wrapped — a wrapped URL reads as two different addresses. */
  .path-label {
    flex: 1;
    min-width: 0;
    font: 12px/16px var(--font-mono, ui-monospace, SFMono-Regular, Menlo, monospace);
    color: var(--text-primary);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .path-note {
    flex-shrink: 0;
    max-width: 38%;
    font: 11px/16px var(--font-mono, ui-monospace, SFMono-Regular, Menlo, monospace);
    color: var(--text-muted);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .path-ms {
    flex-shrink: 0;
    min-width: 76px;
    text-align: right;
    font-size: 11px;
    color: var(--text-muted);
    white-space: nowrap;
  }
  .path-ms.ok {
    color: var(--green);
  }
  .path-ms.bad {
    color: var(--red, #FF453A);
    max-width: 45%;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .path-del {
    flex-shrink: 0;
    width: 20px;
    height: 20px;
    padding: 0;
    border: none;
    border-radius: 4px;
    background: transparent;
    color: var(--text-muted);
    font-size: 15px;
    line-height: 1;
    cursor: pointer;
  }
  .path-del:hover {
    background-color: var(--bg-hover);
    color: var(--text-primary);
  }
  .path-add {
    display: flex;
    align-items: center;
    gap: 6px;
    flex-wrap: wrap;
    margin-top: 2px;
  }
  .path-add-label {
    flex-shrink: 0;
    min-width: 104px;
    font-size: 12px;
    color: var(--text-muted);
  }
  .path-add input[type="text"] {
    flex: 1;
    min-width: 140px;
    font-size: 12px;
    padding: 4px 8px;
  }
  .path-err {
    margin-top: 2px;
    color: var(--red, #FF453A);
  }
  select:hover {
    background-color: var(--bg-hover);
  }
  select:focus,
  select:focus-visible {
    outline: none;
    box-shadow: none;
  }
  input[type="checkbox"] {
    width: 16px;
    height: 16px;
    accent-color: var(--green, #34C759);
    min-width: 16px;
  }
  input[type="checkbox"]:focus,
  input[type="checkbox"]:focus-visible {
    outline: none;
    box-shadow: none;
  }

  /* ========== About tab — redesigned ========== */
  .about-tab {
    display: flex;
    flex-direction: column;
    height: 100%;
  }
  .about-hero {
    display: flex;
    align-items: center;
    gap: 18px;
    padding: 4px 0 4px;
  }
  .about-logo {
    width: 64px;
    height: 64px;
    flex-shrink: 0;
    filter: drop-shadow(0 4px 12px color-mix(in srgb, var(--accent) 28%, transparent));
  }
  .about-hero-text {
    flex: 1;
    min-width: 0;
  }
  .about-name {
    margin: 0;
    font: 700 22px/28px var(--font-sans);
    color: var(--text-primary);
    letter-spacing: -0.02em;
  }
  .about-tagline-text {
    margin: 2px 0 10px;
    font: 500 13px/18px var(--font-sans);
    color: var(--text-secondary);
    letter-spacing: -0.005em;
  }
  .about-pills {
    display: flex;
    align-items: center;
    gap: 8px;
    flex-wrap: wrap;
  }
  .pill {
    display: inline-flex;
    align-items: center;
    gap: 5px;
    font: 600 11px/14px var(--font-sans);
    padding: 4px 10px;
    border-radius: 999px;
    letter-spacing: 0.02em;
  }
  .pill-version {
    color: var(--text-secondary);
    background: color-mix(in srgb, var(--text-muted) 18%, transparent);
    font-feature-settings: "tnum";
  }
  .pill-ok {
    color: var(--green);
    background: color-mix(in srgb, var(--green) 16%, transparent);
  }
  .pill-update {
    color: #fff;
    background: var(--accent);
    border: 0;
    cursor: pointer;
    box-shadow: 0 1px 3px color-mix(in srgb, var(--accent) 26%, transparent),
                0 1px 2px rgba(0,0,0,0.08);
  }
  @media (prefers-reduced-motion: no-preference) {
    .pill-update { transition: background-color 140ms ease, transform 140ms ease, box-shadow 140ms ease; }
  }
  .pill-update:hover:not(:disabled) {
    background: color-mix(in srgb, #fff 8%, var(--accent));
    transform: translateY(-1px);
  }
  .pill-update:disabled { opacity: 0.65; cursor: wait; }
  .pill-release {
    color: var(--text-primary);
    background: var(--bg-secondary);
    border: 0.5px solid var(--border);
    cursor: pointer;
  }
  .pill-release:hover:not(:disabled) { background: var(--bg-hover); }
  .pill-release:disabled { opacity: 0.65; cursor: wait; }

  .about-check-row {
    display: flex;
    align-items: center;
    gap: 10px;
    margin-top: 8px;
  }

  /* Live download/install progress under the About update pills — mirrors
     the main window banner so the two surfaces stay consistent.
     关于页更新按钮下方的实时下载/安装进度条，与主窗口横幅保持一致。 */
  .about-progress {
    margin-top: 10px;
  }
  .about-progress-label {
    font: 500 12px/16px var(--font-sans);
    color: var(--accent);
    margin-bottom: 6px;
    font-feature-settings: "tnum";
  }
  .about-progress-track {
    height: 6px;
    border-radius: 999px;
    background: color-mix(in srgb, var(--accent) 14%, var(--bg-card));
    overflow: hidden;
  }
  .about-progress-fill {
    height: 100%;
    border-radius: 999px;
    background: var(--accent);
    transition: width 200ms ease;
    min-width: 0;
  }
  @media (prefers-reduced-motion: reduce) {
    .about-progress-fill { transition: none; }
  }
  .check-btn {
    height: 24px;
    padding: 0 10px;
    font: 500 11px/14px var(--font-sans);
    color: var(--text-primary);
    background: var(--bg-card);
    border: 0.5px solid var(--border);
    border-radius: 6px;
    cursor: pointer;
  }
  .check-btn:hover:not(:disabled) { background: var(--bg-hover); }
  .check-btn:disabled { opacity: 0.6; cursor: wait; }
  .check-meta {
    font: 400 11px/14px var(--font-sans);
    color: var(--text-secondary);
    font-feature-settings: "tnum";
  }
  .check-result {
    margin-top: 6px;
    font: 400 11px/15px var(--font-sans);
    color: var(--text-secondary);
  }
  .check-hint {
    margin-top: 6px;
    font: 400 11px/15px var(--font-sans);
    color: var(--text-muted, var(--text-secondary));
    font-style: italic;
  }
  .pill-dot {
    width: 6px;
    height: 6px;
    border-radius: 50%;
    background: #fff;
  }

  .about-desc-paragraph {
    margin: 18px 0 16px;
    font: 13px/20px var(--font-sans);
    color: var(--text-secondary);
    letter-spacing: -0.005em;
  }

  /* VPN warn inline alert card */
  .about-vpn-warn {
    display: flex;
    align-items: flex-start;
    gap: 10px;
    background: color-mix(in srgb, var(--orange, #FF9500) 10%, var(--bg-card));
    border: 0.5px solid color-mix(in srgb, var(--orange, #FF9500) 35%, var(--border));
    border-radius: 10px;
    padding: 12px 14px;
    margin-bottom: 14px;
    color: var(--orange, #FF9500);
  }
  .vpn-warn-body {
    flex: 1;
    min-width: 0;
  }
  .about-vpn-warn p {
    margin: 0 0 8px;
    font: 12px/16px var(--font-sans);
    color: var(--text-primary);
  }
  .vpn-warn-actions {
    display: flex;
    gap: 8px;
  }
  .warn-btn {
    height: 26px;
    padding: 0 12px;
    border: 0;
    border-radius: 7px;
    font: 600 11px/14px var(--font-sans);
    cursor: pointer;
  }
  .warn-proceed {
    background: var(--orange, #FF9500);
    color: #fff;
  }
  .warn-cancel {
    background: transparent;
    color: var(--text-secondary);
  }
  .warn-cancel:hover { background: var(--bg-hover); }

  /* Simple text-link row (original style — restored). */
  .about-links {
    display: flex;
    flex-wrap: wrap;
    gap: 18px;
    margin-bottom: 18px;
  }
  .link-btn {
    font: 500 12px/16px var(--font-sans);
    color: var(--accent);
    background: none;
    border: 0;
    padding: 0;
    cursor: pointer;
    letter-spacing: -0.005em;
  }
  .link-btn:hover { text-decoration: underline; }
  .link-btn:disabled { opacity: 0.5; cursor: wait; text-decoration: none; }
  .link-btn:focus, .link-btn:focus-visible { outline: none; }

  /* Quiet credit line at the very bottom of the About tab. */
  .about-credits-line {
    margin: 4px 0 0;
    font: 11px/15px var(--font-sans);
    color: var(--text-muted);
  }

  /* ========== Advanced tab: folder links, log retention, WG scripts ========== */
  .folder-btn {
    height: 26px;
    padding: 0 12px;
    font: 500 12px/16px var(--font-sans);
    color: var(--accent);
    background: var(--bg-hover);
    border: 0.5px solid var(--border);
    border-radius: 6px;
    cursor: pointer;
    white-space: nowrap;
  }
  .folder-btn:hover:not(:disabled) { background: var(--bg-card); }
  .folder-btn:disabled { opacity: 0.6; cursor: wait; }
  .folder-error {
    margin-top: 6px;
    font: 11px/15px var(--font-sans);
    color: var(--red);
    word-break: break-all;
  }
  .wg-scripts-warn {
    margin-top: 10px;
    padding: 10px 12px;
    background: color-mix(in srgb, var(--yellow, #d9a406) 12%, transparent);
    border: 0.5px solid color-mix(in srgb, var(--yellow, #d9a406) 45%, transparent);
    border-radius: var(--radius-sm);
  }
  .wg-scripts-warn-text {
    margin: 0 0 8px;
    font: 400 11px/16px var(--font-sans);
    color: var(--text-primary);
  }
  .wg-scripts-warn-actions {
    display: flex;
    gap: 8px;
  }
  .wg-scripts-warn-actions .btn {
    height: 24px;
    padding: 0 12px;
    font: 500 11px/14px var(--font-sans);
    border-radius: 6px;
    cursor: pointer;
  }
  .wg-scripts-warn-actions .btn--primary {
    color: #fff;
    background: var(--red);
    border: 0;
  }
  .wg-scripts-warn-actions .btn--primary:hover { filter: brightness(1.08); }
  .wg-scripts-warn-actions .btn:not(.btn--primary) {
    color: var(--text-secondary);
    background: var(--bg-hover);
    border: 0.5px solid var(--border);
  }

  /* ========== Modal footer — credits row + close button on one line ==========
     The credit info shows in every tab now (not just About) — persistent
     identity reminder. justify-content: space-between pins the credit
     block to the left and the close button to the right. */
  .modal-footer {
    display: flex;
    justify-content: space-between;
    align-items: center;
    gap: 16px;
    margin-top: 14px;
    padding-top: 4px;
    flex-shrink: 0;
  }
  .footer-info {
    display: flex;
    align-items: baseline;
    gap: 5px;
    flex-wrap: wrap;
    min-width: 0;
    font: 11px/15px var(--font-sans);
    color: var(--text-muted);
    letter-spacing: 0.01em;
  }
  .footer-credit-label,
  .footer-credit-text { color: var(--text-muted); }
  .footer-sep { opacity: 0.55; }
  .footer-link {
    background: none;
    border: 0;
    padding: 0;
    cursor: pointer;
    font: inherit;
    color: var(--accent);
    letter-spacing: -0.005em;
  }
  .footer-link:hover { text-decoration: underline; }
  .footer-link:focus, .footer-link:focus-visible { outline: none; }
  .btn-close {
    min-width: 84px;
    height: 32px;
    padding: 0 18px;
    background: var(--accent);
    color: #fff;
    border: 0;
    border-radius: 10px;
    font: 600 13px/18px var(--font-sans, -apple-system, BlinkMacSystemFont, sans-serif);
    letter-spacing: -0.005em;
    cursor: pointer;
    box-shadow:
      0 1px 3px color-mix(in srgb, var(--accent) 26%, transparent),
      0 1px 2px rgba(0,0,0,0.08);
  }
  @media (prefers-reduced-motion: no-preference) {
    .btn-close {
      transition: background-color 140ms ease, box-shadow 140ms ease, transform 140ms ease;
    }
  }
  .btn-close:hover {
    background: color-mix(in srgb, #fff 8%, var(--accent));
    transform: translateY(-1px);
    box-shadow:
      0 4px 10px color-mix(in srgb, var(--accent) 30%, transparent),
      0 1px 2px rgba(0,0,0,0.10);
  }
  .btn-close:active { background: color-mix(in srgb, #000 8%, var(--accent)); transform: translateY(0); }
  .btn-close:focus,
  .btn-close:focus-visible {
    outline: none;
  }

  @media (prefers-reduced-motion: no-preference) {
    .settings-sidebar button {
      transition: background-color 120ms cubic-bezier(0.2, 0, 0.1, 1),
                  color 120ms cubic-bezier(0.2, 0, 0.1, 1);
    }
    .btn-close {
      transition: filter 120ms cubic-bezier(0.2, 0, 0.1, 1);
    }
  }
</style>
