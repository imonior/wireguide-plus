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
  // is otherwise invisible, so the list states it next to the row.
  // 被用户手动接管（手动连接/断开）的隧道。闩锁期间匹配的自动化规则是被抑制
  // 的——这一点原本不可见，故列表在行旁明示。
  let manualOverrides = [];
  let restoringName = '';
  let overrideSeq = 0;
  $: manualOverrideMap = (() => {
    const m = {};
    for (const o of manualOverrides || []) {
      if (o && o.name) m[o.name] = o.override;
    }
    return m;
  })();

  // Reload exactly when the active/established sets change — that is the
  // moment a manual connect or disconnect has just latched (or released) a
  // tunnel. Keying on the status broadcast itself would re-query at the
  // helper's 1 Hz rate for no reason.
  // 仅在 active/established 集合变化时重载——那正是手动连接或断开刚刚闩住
  // （或释放）某条隧道的时刻。若直接以状态广播为键，会以 helper 的 1 Hz 频率
  // 无意义地重复查询。
  $: statusSignature = [
    ($connectionStatus?.active_tunnels || []).join(','),
    ($connectionStatus?.established_tunnels || []).join(','),
  ].join('|');
  $: if (TunnelService) void loadManualOverrides(statusSignature);

  async function loadManualOverrides(_sig) {
    if (!TunnelService) return;
    const seq = ++overrideSeq;
    try {
      const list = await TunnelService.GetManualOverrides();
      if (seq !== overrideSeq) return;
      manualOverrides = Array.isArray(list) ? list : [];
    } catch (_) {
      // A failed read must never hide the list: no overrides shown is the
      // pre-feature behaviour, not a broken one.
      if (seq === overrideSeq) manualOverrides = [];
    }
  }

  // Hand the tunnel back to its automation rules by releasing whichever
  // manual latch it carries (off or on — they are mutually exclusive).
  // 释放该隧道所带的手动闩锁（off 或 on，二者互斥），把它交还给自动化规则。
  async function restoreAutomation(name) {
    if (!TunnelService || !name || restoringName) return;
    restoringName = name;
    try {
      await TunnelService.ClearManualOverride(name);
      manualOverrides = (manualOverrides || []).filter((o) => o.name !== name);
    } catch (e) {
      console.warn('failed to restore automation for tunnel:', name, e);
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
        <button
          class="tunnel-item"
          class:active={$selectedTunnel?.name === tun.name}
          class:connected={establishedSet.has(tun.name) && tunnelHandshakes[tun.name]}
          class:pending={pendingSet.has(tun.name)}
          aria-current={$selectedTunnel?.name === tun.name ? 'true' : undefined}
          on:click={() => select(tun)}
        >
          <span class="status-dot"
            class:on={establishedSet.has(tun.name) && tunnelHandshakes[tun.name]}
            class:pending={pendingSet.has(tun.name)}
            class:warning={establishedSet.has(tun.name) && !tunnelHandshakes[tun.name]}></span>
          <div class="tunnel-text">
            <span class="tunnel-name">{tun.name}</span>
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
              <span class="tunnel-meta">{tun.endpoint}</span>
            {/if}
          </div>
        </button>
        <!-- Manual-override notice. Sits as a SIBLING of the row button, never
             inside it: a <button> may not contain another <button>, and the
             restore action needs its own focusable control. -->
        {#if manualOverrideMap[tun.name]}
          <div class="manual-override" role="status">
            <span class="mo-text">{$t(manualOverrideMap[tun.name] === 'off' ? 'tunnel.manual_override_off' : 'tunnel.manual_override_on')}</span>
            <button
              class="mo-restore"
              disabled={restoringName === tun.name}
              on:click={() => restoreAutomation(tun.name)}
            >{$t('tunnel.restore_automation')}</button>
          </div>
        {/if}
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

  /* --- List items: card-style with name + endpoint --- */
  .list-items {
    flex: 1;
    min-height: 0;
    overflow-y: auto;
    padding: 0 var(--space-2) var(--space-2);
  }
  .tunnel-item {
    display: flex;
    align-items: center;
    gap: 10px;
    width: 100%;
    min-height: 52px;
    padding: 8px 10px;
    margin-bottom: 2px;
    background: transparent;
    border: 0;
    border-radius: 10px;
    color: var(--text-primary);
    font: var(--text-body);
    cursor: pointer;
    text-align: left;
    position: relative;
    overflow: hidden;
  }
  @media (prefers-reduced-motion: no-preference) {
    .tunnel-item {
      transition: background-color var(--dur-fast) var(--ease-out),
                  border-color var(--dur-fast) var(--ease-out);
    }
    .status-dot {
      transition: background-color var(--dur-base) var(--ease-out),
                  box-shadow var(--dur-base) var(--ease-out);
    }
  }
  /* --- Manual override notice ---
     The user took this tunnel over by hand, so its automation rules are dead
     until restored. Rendered directly under the row so the reason is visible
     exactly where the connect/disconnect was clicked. */
  .manual-override {
    display: flex;
    align-items: center;
    gap: 8px;
    margin: 2px 0 4px;
    padding: 5px 10px 6px 24px;
    border-radius: 6px;
    background: color-mix(in srgb, var(--text-muted) 9%, transparent);
  }
  .mo-text {
    flex: 1;
    min-width: 0;
    font-size: 11px;
    line-height: 1.35;
    color: var(--text-muted);
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

  .tunnel-item:hover {
    background: var(--bg-hover);
  }
  .tunnel-item.active {
    background: color-mix(in srgb, var(--accent) 12%, transparent);
  }
  .tunnel-item.active .tunnel-name {
    font-weight: 600;
    color: var(--text-primary);
  }

  /* Claiming-to-be-connected left-edge accent pill */
  .tunnel-item.connected::before {
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
  .tunnel-item.pending::before {
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
