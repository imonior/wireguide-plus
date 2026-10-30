<script>
  import { createEventDispatcher } from 'svelte';
  import Icon from './Icon.svelte';
  import { tunnels, selectedTunnel, connectionStatus } from '../stores/tunnels.js';
  import { compactList, listSort, listActiveOnTop, saveListPrefs } from '../stores/ui.js';
  import { appSettings } from '../stores/settings.js';
  import { t } from '../i18n/index.js';

  // Bound backend service, injected by App.svelte. Optional so the list still
  // renders in isolation; the manual-override notice simply stays hidden.
  // 由 App.svelte 注入的后端服务。设为可选，列表仍可独立渲染，只是手动覆盖
  // 提示会保持隐藏。
  export let TunnelService = null;

  const dispatch = createEventDispatcher();
  let search = '';

  // Tunnels the user has taken over by hand (manual connect / disconnect).
  // While a latch holds, the matching automation rules are suppressed — which
  // is otherwise invisible, so the list states it beside the row. The latch
  // state lives in ONE shared store (see stores/manualoverride.js) that every
  // surface reads, so a restore on the detail page / editor clears it here
  // too, instantly and completely — there is no per-surface copy to fall out
  // of sync.
  // 被用户手动接管（手动连接/断开）的隧道。闩锁期间匹配的自动化规则被抑制，这一点
  // 原本不可见，故列表在行旁明示。闩锁状态只存在于一个共享 store（见
  // stores/manualoverride.js），所有表面都读它，因此在详情页/编辑器点恢复也会立刻
  // 在这里清除——没有会失同步的每面私有副本。
  import { manualOverrides, refreshManualOverrides, clearManualOverride, setManualOverrideService } from '../stores/manualoverride.js';

  let restoringName = '';

  // Keep the shared store's backend service current, and reload the latch map
  // exactly when the active/established sets change — that is the moment a
  // manual connect or disconnect has just latched (or released) a tunnel.
  // Keying on the status broadcast itself would re-query at the helper's 1 Hz
  // rate for no reason.
  // 维护共享 store 的后端服务，并在 active/established 集合变化时重拉闩锁映射——那正是
  // 手动连接/断开刚刚闩住（或释放）某条隧道的时刻。若直接以状态广播为键，会以 helper
  // 的 1 Hz 频率无意义地重复查询。
  $: if (TunnelService) setManualOverrideService(TunnelService);
  $: statusSignature = [
    ($connectionStatus?.active_tunnels || []).join(','),
    ($connectionStatus?.established_tunnels || []).join(','),
  ].join('|');
  $: if (TunnelService) void refreshManualOverrides(statusSignature);

  // Live backend latch per tunnel (name -> 'off' | 'on'), straight from the
  // shared store. The strip — and its restore button — appears only while a
  // latch is present; the moment it is cleared (from ANY surface) the strip
  // vanishes here too. There is no "restored" lingering state: with the latch
  // gone there is nothing left to restore, so no button is shown anywhere.
  // 每个隧道的实时后端闩锁（name -> 'off' | 'on'），直接来自共享 store。提示条及其
  // 恢复按钮只在闩锁存在时出现；一旦被任一表面清除，这里的提示条也立刻消失。不存在
  // 「已恢复」滞留态：闩锁既已不在，便无可恢复之物，故任何地方都不显示恢复按钮。
  $: manualOverrideMap = (() => {
    const m = {};
    for (const o of $manualOverrides || []) {
      if (o && o.name) m[o.name] = o.override;
    }
    return m;
  })();

  // Hand the tunnel back to its automation rules by releasing whichever manual
  // latch it carries (off or on — mutually exclusive). The shared clear
  // refreshes the store, so every surface drops its indicator at once.
  // 释放该隧道所带的手动闩锁（off 或 on，二者互斥），交还给自动化规则。共享清除会
  // 刷新 store，于是所有表面同时撤下提示。
  async function restoreAutomation(name) {
    if (!TunnelService || !name || restoringName) return;
    restoringName = name;
    try {
      await clearManualOverride(name);
    } finally {
      restoringName = '';
    }
  }

  $: filtered = ($tunnels || []).filter(tun =>
    tun.name.toLowerCase().includes(search.toLowerCase())
  );

  // Connection indicators from the status stream (multi-tunnel aware).
  //
  // Two sets, deliberately. activeSet drives *ordering* — a tunnel that is
  // dialling should still float to the top under "active on top". The
  // visuals, however, read establishedSet: active also contains a tunnel that
  // is still connecting and may yet fail (boot-time DNS miss, pending
  // handshake), and painting that green is exactly what made a failed attempt
  // look like "connected, then silently dropped".
  //
  // Green still needs a handshake on top of it: "established" only says the
  // interface came up, and it stays true forever for a tunnel whose peer never
  // answers. So the green pill is established AND handshook — the same claim
  // the tray bubble makes (see gui.upTunnelSet).
  $: activeSet = new Set($connectionStatus?.active_tunnels || []);
  // Older helpers send only active_tunnels; falling back to it there is the
  // best available answer, and newer ones always send established_tunnels
  // (possibly empty) so "active but not established" reliably means
  // "still transitioning".
  $: establishedSet = new Set(
    Array.isArray($connectionStatus?.established_tunnels)
      ? $connectionStatus.established_tunnels
      : ($connectionStatus?.active_tunnels || [])
  );
  // Dialling right now: active but not yet up. Rendered as a hollow ring so
  // "trying" is visibly neither "up" (solid green) nor "down" (faint grey).
  $: pendingSet = new Set([...activeSet].filter(name => !establishedSet.has(name)));

  // Sorted view. Array.sort is stable, so sorting by the chosen key first
  // and then stably floating active tunnels keeps key order within groups.
  function nameCmp(a, b) {
    return a.name.localeCompare(b.name, undefined, { sensitivity: 'base', numeric: true });
  }
  $: sorted = (() => {
    const arr = [...filtered];
    arr.sort((a, b) => {
      switch ($listSort) {
        case 'name_desc': return -nameCmp(a, b);
        // Newest first; ties (or missing timestamps) fall back to name.
        case 'added_desc': return (b.created_at_unix || 0) - (a.created_at_unix || 0) || nameCmp(a, b);
        case 'added_asc':  return (a.created_at_unix || 0) - (b.created_at_unix || 0) || nameCmp(a, b);
        case 'used_desc':  return (b.last_used_unix || 0) - (a.last_used_unix || 0) || nameCmp(a, b);
        case 'name_asc':
        default:           return nameCmp(a, b);
      }
    });
    if ($listActiveOnTop) {
      arr.sort((a, b) => (activeSet.has(b.name) ? 1 : 0) - (activeSet.has(a.name) ? 1 : 0));
    }
    return arr;
  })();

  function onSortChange(e) {
    listSort.set(e.target.value);
    saveListPrefs();
  }
  function onActiveTopChange(e) {
    listActiveOnTop.set(e.target.checked);
    saveListPrefs();
  }
  // Build a map of tunnel name → has handshake for dot color.
  $: tunnelHandshakes = (() => {
    const map = {};
    const tunnelStatuses = $connectionStatus?.tunnels || [];
    for (const ts of tunnelStatuses) {
      // A frozen handshake string (peer stopped answering, upstream down)
      // is not "handshaking" — the backend marks handshake_stale so the dot
      // reads warning instead of a falsely-green "connected".
      map[ts.tunnel_name] = !!ts.last_handshake && !ts.handshake_stale;
    }
    // Primary tunnel status
    if ($connectionStatus?.tunnel_name) {
      map[$connectionStatus.tunnel_name] =
        !!$connectionStatus.last_handshake && !$connectionStatus.handshake_stale;
    }
    return map;
  })();

  function select(tun) {
    selectedTunnel.set(tun);
  }
</script>

<div class="tunnel-list">
  <div class="list-header">
    <h2 class="list-title">{$t('tunnel.list_title')}</h2>
    {#if ($tunnels || []).length > 0}
      <span class="list-count">{($tunnels || []).length}</span>
    {/if}
  </div>

  <div class="search-wrap">
    <span class="search-icon"><Icon name="search" size={13} strokeWidth={2} /></span>
    <input
      type="text"
      class="search-input"
      placeholder={$t('tunnel.search')}
      bind:value={search} />
    {#if search}
      <button class="search-clear" on:click={() => search = ''} aria-label="Clear search">
        <Icon name="x" size={11} strokeWidth={2.5} />
      </button>
    {/if}
  </div>

  {#if ($tunnels || []).length > 1}
    <div class="sort-bar">
      <label class="sort-select-wrap">
        <span class="sort-label">{$t('tunnel.sort_by')}</span>
        <select class="sort-select" value={$listSort} on:change={onSortChange} aria-label={$t('tunnel.sort_by')}>
          <option value="name_asc">{$t('tunnel.sort_name_asc')}</option>
          <option value="name_desc">{$t('tunnel.sort_name_desc')}</option>
          <option value="used_desc">{$t('tunnel.sort_last_used')}</option>
          <option value="added_desc">{$t('tunnel.sort_added_new')}</option>
          <option value="added_asc">{$t('tunnel.sort_added_old')}</option>
        </select>
      </label>
      <label class="sort-active-top" title={$t('tunnel.sort_active_top')}>
        <input type="checkbox" checked={$listActiveOnTop} on:change={onActiveTopChange} />
        <span>{$t('tunnel.sort_active_top')}</span>
      </label>
    </div>
  {/if}

  <div class="list-items" class:compact={$compactList}>
    {#if filtered.length === 0 && search === ''}
      <div class="empty-state">
        <Icon name="shield-off" size={28} strokeWidth={1.5} className="empty-icon" />
        <p>{$t('tunnel.no_tunnels')}</p>
        <p class="hint">{$t('tunnel.drop_hint')}</p>
      </div>
    {:else}
      {#each sorted as tun (tun.name)}
        <!-- One card per tunnel. The row button and the manual-override
             strip are siblings INSIDE the card so the strip reads as the
             bottom of the info bar, not as a separate section beside it.
             The strip can never live inside the <button> (a button may not
             contain another button), hence the wrapper. -->
      <div
        class="tunnel-entry"
        class:entry-active={$selectedTunnel?.name === tun.name}
        class:entry-connected={establishedSet.has(tun.name) && tunnelHandshakes[tun.name]}
        class:entry-pending={pendingSet.has(tun.name)}
        class:entry-broken={!!tun.load_error}
      >
          <button
            class="tunnel-item"
            class:active={$selectedTunnel?.name === tun.name}
            aria-current={$selectedTunnel?.name === tun.name ? 'true' : undefined}
            on:click={() => select(tun)}
          >
            <span class="status-dot"
              class:on={establishedSet.has(tun.name) && tunnelHandshakes[tun.name]}
              class:pending={pendingSet.has(tun.name)}
              class:warning={establishedSet.has(tun.name) && !tunnelHandshakes[tun.name]}></span>
            <div class="tunnel-text">
              <span class="tunnel-name" title={tun.name}>{tun.name}</span>
            {#if tun.load_error}
              <span class="tunnel-error" title={tun.load_error}>
                <Icon name="triangle-alert" size={11} strokeWidth={2.25} />
                <span>{$t('tunnel.config_error')}</span>
              </span>
            {/if}
              {#if tun.protocol === 'amneziawg' && $appSettings.loaded}
                <!-- One element, one style, two states: "AmneziaWG ON" /
                     "AmneziaWG OFF". Only the colour modifier changes so the
                     badge keeps identical geometry in both states. -->
                <span
                  class="awg-badge"
                  class:awg-badge--off={!$appSettings.enable_awg}
                  title={$appSettings.enable_awg ? $t('tunnel.awg_on_tip') : $t('tunnel.awg_off_tip')}>
                  {$appSettings.enable_awg ? $t('tunnel.awg_on') : $t('tunnel.awg_off')}
                </span>
              {/if}
              {#if tun.endpoint}
                <span class="tunnel-meta" title={tun.endpoint}>{tun.endpoint}</span>
              {/if}
            </div>
          </button>
          <!-- Manual-override strip: the bottom of the tunnel's own card.
               The wording is a short state phrase, not a sentence — a long
               one wrapped into the restore button on a narrow list. Shown only
               while the latch is present; the shared store (and therefore this
               strip) clears the instant a restore happens on ANY surface, so the
               list, detail page and editor stay in lock-step. -->
          <!-- 手动接管提示条：隧道自身卡片的底边。措辞是简短状态短语而非整句——
               过长会在窄列表里挤进恢复按钮。仅在闩锁存在时显示；共享 store（亦即本
               提示条）会在任一表面点恢复的那一刻清空，于是列表、详情页、编辑器始终
               同步。 -->
          {#if manualOverrideMap[tun.name]}
            <div class="manual-override" role="status">
              <span class="mo-text">
                {$t(manualOverrideMap[tun.name] === 'off' ? 'tunnel.manual_override_off_short' : 'tunnel.manual_override_on_short')}
              </span>
              <button
                class="mo-restore"
                disabled={restoringName === tun.name}
                on:click={() => restoreAutomation(tun.name)}
              >{$t('tunnel.restore_automation')}</button>
            </div>
          {/if}
        </div>
      {/each}
    {/if}
  </div>

  <div class="list-footer">
    <button class="btn btn-primary" on:click={() => dispatch('new')}>
      <Icon name="plus" size={13} strokeWidth={2.25} />
      {$t('tunnel.new_tunnel')}
    </button>
    <button class="btn btn-secondary" on:click={() => dispatch('import')} title={$t('tunnel.import_hint')}>
      <Icon name="download" size={13} strokeWidth={2} />
      {$t('tunnel.import')}
    </button>
  </div>
</div>

<style>
  .tunnel-list {
    display: flex;
    flex-direction: column;
    height: 100%;
    background: var(--bg-secondary);
  }

  /* --- Section header (title + count badge) --- */
  .list-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 16px 16px 10px;
    gap: 8px;
  }
  .list-title {
    margin: 0;
    font: 700 10px/13px var(--font-sans);
    color: var(--text-muted);
    text-transform: uppercase;
    letter-spacing: 0.1em;
  }
  .list-count {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    min-width: 18px;
    height: 16px;
    padding: 0 6px;
    border-radius: 8px;
    background: color-mix(in srgb, var(--text-muted) 16%, transparent);
    color: var(--text-muted);
    font: 700 10px/1 var(--font-sans);
  }

  /* --- Search input with icon + clear button --- */
  .search-wrap {
    position: relative;
    padding: 0 12px 10px;
  }
  .search-icon {
    position: absolute;
    left: 21px;
    top: 50%;
    transform: translateY(calc(-50% - 5px));
    color: var(--text-muted);
    display: flex;
    align-items: center;
    pointer-events: none;
  }
  .search-input {
    width: 100%;
    height: 30px;
    padding: 0 30px 0 30px;
    background: var(--bg-card);
    border: 0.5px solid var(--border);
    border-radius: 8px;
    color: var(--text-primary);
    font: 13px/18px var(--font-sans);
    outline: none;
    box-sizing: border-box;
  }
  @media (prefers-reduced-motion: no-preference) {
    .search-input {
      transition: border-color 140ms ease, box-shadow 140ms ease, background 140ms ease;
    }
  }
  .search-input::placeholder { color: var(--text-muted); }
  .search-input:focus {
    border-color: var(--accent);
    box-shadow: 0 0 0 3px color-mix(in srgb, var(--accent) 18%, transparent);
    background: var(--bg-primary);
  }
  .search-clear {
    position: absolute;
    right: 18px;
    top: 50%;
    transform: translateY(calc(-50% - 5px));
    width: 18px;
    height: 18px;
    padding: 0;
    border: 0;
    border-radius: 50%;
    background: color-mix(in srgb, var(--text-muted) 22%, transparent);
    color: var(--text-muted);
    cursor: pointer;
    display: flex;
    align-items: center;
    justify-content: center;
  }
  .search-clear:hover {
    background: color-mix(in srgb, var(--text-muted) 36%, transparent);
    color: var(--text-primary);
  }

  /* --- Sort bar (issue #17) --- */
  .sort-bar {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 8px;
    padding: 0 var(--space-3) 6px;
  }
  .sort-select-wrap {
    display: flex;
    align-items: center;
    gap: 5px;
    min-width: 0;
  }
  .sort-label {
    font: 400 11px/1 var(--font-sans);
    color: var(--text-muted);
    white-space: nowrap;
  }
  .sort-select {
    font: 400 11px/1 var(--font-sans);
    color: var(--text-secondary);
    background: transparent;
    border: 0;
    padding: 2px 4px;
    border-radius: 6px;
    cursor: pointer;
  }
  .sort-select:hover {
    background: var(--bg-hover);
  }
  .sort-active-top {
    display: flex;
    align-items: center;
    gap: 5px;
    font: 400 11px/1 var(--font-sans);
    color: var(--text-muted);
    cursor: pointer;
    white-space: nowrap;
  }
  .sort-active-top input {
    width: 12px;
    height: 12px;
    accent-color: var(--accent);
    cursor: pointer;
  }

  /* --- List items: one card per tunnel ---
     The card (.tunnel-entry) owns every tunnel-level visual: the hover
     wash, the active-selection wash, and the spacing between tunnels. The
     row button inside is transparent and content-only, and the
     manual-override strip is the card's bottom edge — which is what makes
     it read as part of the info bar instead of a section beside it. */
  .list-items {
    flex: 1;
    min-height: 0;
    overflow-y: auto;
    padding: 0 var(--space-2) var(--space-2);
  }
  .tunnel-entry {
    position: relative;
    border-radius: 10px;
    overflow: hidden;
    margin-bottom: 2px;
  }
  @media (prefers-reduced-motion: no-preference) {
    .tunnel-entry {
      transition: background-color var(--dur-fast) var(--ease-out);
    }
    .status-dot {
      transition: background-color var(--dur-base) var(--ease-out),
                  box-shadow var(--dur-base) var(--ease-out);
    }
  }
  .tunnel-entry:hover {
    background: var(--bg-hover);
  }
  /* Declared after :hover so the selected tunnel keeps its wash while the
     pointer is anywhere on its card. */
  .tunnel-entry.entry-active {
    background: color-mix(in srgb, var(--accent) 12%, transparent);
  }
  .tunnel-entry.entry-active .tunnel-name {
    font-weight: 600;
    color: var(--text-primary);
  }
  .tunnel-item {
    display: flex;
    align-items: center;
    gap: 10px;
    width: 100%;
    min-height: 52px;
    padding: 8px 10px;
    background: transparent;
    border: 0;
    border-radius: 0;
    color: var(--text-primary);
    font: var(--text-body);
    cursor: pointer;
    text-align: left;
    position: relative;
    overflow: hidden;
  }

  /* --- Manual override strip: the bottom of the tunnel's own card ---
     The user took this tunnel over by hand, so its automation rules are
     suspended until restored. Indented to line up with the tunnel name
     (status dot 8px + gap 10px + row padding 10px) and worded as a short
     state phrase rather than a sentence: a full sentence wrapped into the
     restore button on a narrow list. The strip exists only while a latch is
     present — the shared store clears it (and the restore button with it) the
     moment a restore happens anywhere, so no "restored" state can linger. */
  /* --- 手动接管提示条：隧道自身卡片的底边 ---
     用户手动接管了这条隧道，于是其自动化规则在恢复前被挂起。缩进与隧道名对齐
     （状态点 8px + 间距 10px + 行内边距 10px），措辞为简短状态短语而非整句——整句会
     在窄列表里挤进恢复按钮。提示条仅在闩锁存在时出现——共享 store 会在任一处置处点
     恢复的瞬间把它（连同恢复按钮）一并清空，故不会留下「已恢复」滞留态。 */
  .manual-override {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 0 10px 9px 28px;
  }
  .mo-text {
    flex: 1;
    min-width: 0;
    font-size: 11px;
    line-height: 1.35;
    color: var(--text-muted);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .mo-restore {
    flex: none;
    height: 20px;
    padding: 0 8px;
    font-size: 11px;
    font-weight: 500;
    color: var(--accent);
    background: transparent;
    border: 1px solid color-mix(in srgb, var(--accent) 45%, transparent);
    border-radius: 5px;
    cursor: pointer;
  }
  .mo-restore:hover:not(:disabled) {
    background: color-mix(in srgb, var(--accent) 14%, transparent);
  }
  .mo-restore:disabled {
    opacity: 0.55;
    cursor: default;
  }

  /* Claiming-to-be-connected left-edge accent pill. Still centred on the
     ROW (the strip below never carried a state of its own), now addressed
     through the card's state class. */
  .tunnel-entry.entry-connected .tunnel-item::before {
    content: '';
    position: absolute;
    left: 0;
    top: 50%;
    transform: translateY(-50%);
    width: 3px;
    height: 26px;
    background: var(--green);
    border-radius: 0 2px 2px 0;
  }

  /* In-progress left-edge pill: same geometry, drained of colour. A tunnel
     that is still dialling gets the marker (it is doing something) without
     borrowing the green that means "carrying traffic". */
  .tunnel-entry.entry-pending .tunnel-item::before {
    content: '';
    position: absolute;
    left: 0;
    top: 50%;
    transform: translateY(-50%);
    width: 3px;
    height: 26px;
    background: color-mix(in srgb, var(--text-muted) 45%, transparent);
    border-radius: 0 2px 2px 0;
  }

  /* --- Connection dot (left of name) --- */
  .status-dot {
    width: 8px;
    height: 8px;
    border-radius: 50%;
    background: color-mix(in srgb, var(--text-muted) 50%, transparent);
    flex-shrink: 0;
  }
  @keyframes dot-pulse {
    0%, 100% {
      box-shadow: 0 0 0 0 color-mix(in srgb, var(--green) 55%, transparent);
    }
    55% {
      box-shadow: 0 0 0 5px color-mix(in srgb, var(--green) 0%, transparent);
    }
  }
  .status-dot.on {
    background: var(--green);
    box-shadow: 0 0 0 2px color-mix(in srgb, var(--green) 25%, transparent);
  }
  @media (prefers-reduced-motion: no-preference) {
    .status-dot.on {
      animation: dot-pulse 2.4s ease-out infinite;
    }
  }
  .status-dot.warning {
    background: var(--orange, #FF9500);
    box-shadow: 0 0 0 2px color-mix(in srgb, var(--orange, #FF9500) 25%, transparent);
  }
  /* Dialling: hollow ring, no fill. Distinct at a glance from the solid
     green ("up"), the solid orange ("up but no handshake") and the faint
     grey ("down"). */
  .status-dot.pending {
    background: transparent;
    box-sizing: border-box;
    border: 2px solid color-mix(in srgb, var(--text-muted) 70%, transparent);
  }

  /* --- Tunnel text block: name on top, endpoint below --- */
  .tunnel-text {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 1px;
  }
  .tunnel-name {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font: 500 13px/18px var(--font-sans);
    color: var(--text-primary);
    letter-spacing: -0.005em;
  }
  /* A tunnel whose .conf failed to parse: kept in the list (see the backend
     ListTunnels skip->keep change) but flagged red so it can never hide in
     plain sight — otherwise a tunnel that is still up in the OS shows up in
     the active badge yet never appears here. */
  .tunnel-entry.entry-broken {
    background: color-mix(in srgb, var(--danger, #d33) 9%, transparent);
  }
  .tunnel-entry.entry-broken .tunnel-name {
    color: var(--danger, #d33);
  }
  .tunnel-error {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    margin-top: 1px;
    font: 400 11px/14px var(--font-sans);
    color: var(--danger, #d33);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .tunnel-error :global(svg) {
    flex-shrink: 0;
  }
  .awg-badge {
    align-self: flex-start;
    margin-top: 3px;
    padding: 0 6px;
    border-radius: 999px;
    font: 600 9px/16px var(--font-sans);
    letter-spacing: 0.02em;
    /* No text-transform: the label is "AmneziaWG ON"/"AmneziaWG OFF" and
       uppercasing would mangle the AmneziaWG wordmark. */
    white-space: nowrap;
    /* Fixed width keeps the pill geometrically identical across both
       states, so toggling the setting does not shift the row layout. */
    min-width: 86px;
    text-align: center;
    color: var(--purple);
    background: color-mix(in srgb, var(--purple) 13%, transparent);
    border: 1px solid color-mix(in srgb, var(--purple) 32%, transparent);
  }
  /* AWG support disabled in Settings — same geometry, red palette. */
  .awg-badge--off {
    color: var(--danger, #d33);
    background: color-mix(in srgb, var(--danger, #d33) 13%, transparent);
    border-color: color-mix(in srgb, var(--danger, #d33) 32%, transparent);
  }
  .tunnel-meta {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font: 400 11px/14px var(--font-mono);
    color: var(--text-muted);
    letter-spacing: 0.01em;
  }

  /* --- Compact mode: dense single-line rows (issue #16) --- */
  .list-items.compact .tunnel-item {
    min-height: 34px;
    padding: 4px 10px;
    margin-bottom: 1px;
  }
  .list-items.compact .tunnel-meta {
    display: none;
  }
  .list-items.compact .tunnel-item.connected::before {
    height: 18px;
  }
  .list-items.compact .tunnel-item.pending::before {
    height: 18px;
  }

  /* --- Empty state --- */
  .empty-state {
    padding: 48px 20px;
    text-align: center;
    color: var(--text-muted);
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 8px;
  }
  :global(.empty-icon) {
    opacity: 0.4;
    margin-bottom: 4px;
  }
  .empty-state p {
    font: 500 13px/18px var(--font-sans);
    margin: 0;
  }
  .empty-state .hint {
    font: 11px/15px var(--font-sans);
    color: var(--text-muted);
  }

  /* --- Footer buttons (primary gradient + secondary card, no hard divider) --- */
  .list-footer {
    padding: 8px 12px 14px;
    display: flex;
    flex-direction: column;
    gap: 8px;
  }
  .btn {
    width: 100%;
    height: 34px;
    padding: 0 12px;
    border: 0;
    border-radius: 10px;
    font: 600 12px/16px var(--font-sans);
    cursor: pointer;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    gap: 6px;
    letter-spacing: -0.005em;
  }
  @media (prefers-reduced-motion: no-preference) {
    .btn {
      transition: background-color 140ms ease, filter 140ms ease,
                  border-color 140ms ease, transform 140ms ease,
                  box-shadow 140ms ease;
    }
  }
  .btn-primary {
    background: var(--accent);
    color: #fff;
    box-shadow:
      0 1px 3px color-mix(in srgb, var(--accent) 26%, transparent),
      0 1px 2px rgba(0,0,0,0.08);
  }
  .btn-primary:hover {
    background: color-mix(in srgb, #fff 8%, var(--accent));
    transform: translateY(-1px);
    box-shadow:
      0 4px 12px color-mix(in srgb, var(--accent) 32%, transparent),
      0 1px 2px rgba(0,0,0,0.10);
  }
  .btn-primary:active { background: color-mix(in srgb, #000 8%, var(--accent)); transform: translateY(0); }
  .btn-secondary {
    background: var(--bg-card);
    color: var(--text-primary);
    border: 0.5px solid var(--border);
  }
  .btn-secondary:hover {
    background: var(--bg-hover);
    border-color: color-mix(in srgb, var(--accent) 30%, var(--border));
  }
  .btn-secondary:active { background: var(--bg-active); }
</style>
