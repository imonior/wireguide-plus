<script>
  // Status bubble for macOS/Linux. Rendered by App.svelte when the window
  // URL carries ?popup=1 (the secondary Wails window opened from Go's
  // showStatusPopupWails). Mirrors the Windows Win32 bubble: title, status
  // dot + caption, the tunnel list, an amber "out of tunnel routes" warning,
  // and Open Window / Disconnect buttons. Status bubbles are persistent
  // (duration_ms <= 0: only the user, or a newer event replacing this one,
  // closes them); the launch overview carries a positive duration.
  import { onMount, onDestroy } from 'svelte';
  import { Events } from '@wailsio/runtime';
  import { t } from '../i18n/index.js';

  let names = [];
  let state = 'disconnected';
  let outOfRange = [];
  let rows = []; // overview/transition rows: [{ name, state, auto }]
  let durationMs = 10000;
  let closeTimer = null;
  let unsub = null;

  function statusText() {
    if (state === 'connected') return $t('popup.connected');
    if (state === 'connecting') return $t('popup.connecting');
    return $t('popup.not_connected');
  }
  function rowStatusText(s) {
    if (s === 'connected') return $t('popup.connected');
    if (s === 'connecting') return $t('popup.connecting');
    return $t('popup.not_connected');
  }
  function stateGlyph(s) {
    // Same circle vocabulary as the tray menu (🟢 connected / 🔴 down /
    // 🟡 connecting). The emoji carry their own fixed colour, so no CSS
    // ink is needed here — the row's stateClass colours the text.
    if (s === 'connected') return '🟢';
    if (s === 'connecting') return '🟡';
    return '🔴';
  }
  function stateClass(s) {
    if (s === 'connected') return 'st-ok';
    if (s === 'connecting') return 'st-try';
    return 'st-down';
  }

  function apply(d) {
    names = (d && d.names) || [];
    state = (d && d.state) || 'disconnected';
    outOfRange = (d && d.out_of_range) || [];
    rows = (d && d.rows) || [];
    // duration_ms <= 0 means persistent (status bubbles wait for the user;
    // only the launch overview carries a positive duration).
    durationMs = (d && typeof d.duration_ms === 'number') ? d.duration_ms : 10000;
    if (closeTimer) clearTimeout(closeTimer);
    closeTimer = null;
    if (durationMs > 0) closeTimer = setTimeout(close, durationMs);
  }

  function close() {
    Events.Emit('popup:close');
  }
  function openWindow() {
    Events.Emit('popup:open');
  }
  function disconnect() {
    Events.Emit('popup:disconnect');
  }

  onMount(() => {
    unsub = Events.On('popup:data', (e) => apply((e && e.data) || {}));
    // Tell Go we're subscribed so it can replay the latest payload if its
    // own emit raced ahead of this JS subscription.
    Events.Emit('popup:ready');
  });
  onDestroy(() => {
    if (unsub) unsub();
    if (closeTimer) clearTimeout(closeTimer);
  });
</script>

<div class="popup-card">
  <div class="popup-header">
    <span class="popup-title">WireGuide Plus</span>
    <button class="popup-close" on:click={close} aria-label="Close">×</button>
  </div>

  {#if state === 'overview'}
    {#each rows as r (r.name)}
      <div class="popup-row">
        <span class="glyph {stateClass(r.state)}">{stateGlyph(r.state)}</span>
        <span class="row-name" title={r.name}>{r.name}</span>
        <span class="row-meta"><span class={stateClass(r.state)}>{rowStatusText(r.state)}</span> · {r.auto ? $t('popup.overview_auto') : $t('popup.overview_manual')}</span>
      </div>
    {/each}
  {:else if rows.length > 0}
    {#each rows as r (r.name)}
      <div class="popup-row {stateClass(r.state)}">
        <span class="glyph {stateClass(r.state)}">{stateGlyph(r.state)}</span>
        <span class="row-text" title={r.name}>{rowStatusText(r.state)}: {r.name}</span>
      </div>
    {/each}
  {:else}
    <div class="popup-status">
      <span class="glyph {stateClass(state)}">{stateGlyph(state)}</span>
      <span class="status-text {stateClass(state)}">{statusText()}</span>
    </div>

    {#if names.length > 0}
      <div class="popup-names">{names.join(', ')}</div>
    {/if}
  {/if}

  {#if outOfRange.length > 0}
    <div class="popup-warn">{$t('popup.out_of_range')}: {outOfRange.join(', ')}</div>
  {/if}

  <div class="popup-actions">
    {#if state !== 'overview'}
      <button class="btn" on:click={disconnect}>{$t('popup.disconnect')}</button>
    {/if}
    <button class="btn btn-primary" on:click={openWindow}>{$t('popup.open_window')}</button>
  </div>
</div>

<style>
  :global(body) {
    margin: 0;
    background: transparent;
    overflow: hidden;
  }
  .popup-card {
    box-sizing: border-box;
    width: 100%;
    min-height: 120px;
    padding: 12px 14px;
    background: var(--bg-primary, #1e1e1e);
    color: var(--text-primary, #fff);
    border: 0.5px solid var(--border, #3c3c3c);
    border-radius: 12px;
    font-family: var(--font-sans, system-ui);
    font-size: 13px;
    line-height: 1.4;
    /* Wails frameless-window dragging: runtime drag.js starts a window
       drag when the element under the pressed button computes
       --wails-draggable: drag (inherited by every child unless a nearer
       rule opts out, which is what the interactive bits below do). The
       bubble is parkable anywhere; a standing notification that covers
       content should be movable out of the way. */
    --wails-draggable: drag;
    user-select: none;
    -webkit-user-select: none;
    cursor: default;
    display: flex;
    flex-direction: column;
    gap: 8px;
    box-shadow: 0 8px 24px rgba(0, 0, 0, 0.4);
  }
  .popup-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
  }
  .popup-title {
    font-weight: 700;
    letter-spacing: -0.01em;
  }
  .popup-close {
    /* Interactive: must not hand the press to the window drag. */
    --wails-draggable: no-drag;
    background: none;
    border: none;
    color: var(--text-secondary, #aaa);
    font-size: 16px;
    line-height: 1;
    cursor: pointer;
    padding: 0 4px;
    border-radius: 6px;
  }
  .popup-close:hover {
    background: var(--bg-hover, rgba(127, 127, 127, 0.12));
    color: var(--text-primary, #fff);
  }
  .popup-status {
    display: flex;
    align-items: center;
    gap: 8px;
  }
  .popup-row {
    display: flex;
    align-items: center;
    gap: 8px;
    font-size: 12px;
    min-height: 16px;
  }
  .row-name {
    flex: 1;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    color: var(--text-primary, #fff);
  }
  /* Transition rows: same ellipsis box as .row-name but no colour of its
     own, so the state class on the row (green/red/amber) paints it. */
  .row-text {
    flex: 1;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .row-meta {
    flex-shrink: 0;
    color: var(--text-secondary, #ccc);
  }
  .glyph {
    font-size: 11px;
    line-height: 1;
    flex-shrink: 0;
  }
  .status-text {
    font-weight: 600;
  }
  .st-ok {
    color: var(--green, #34c759);
  }
  .st-try {
    color: #f5c518;
  }
  .st-down {
    color: var(--red, #ff453a);
  }
  .popup-names {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    color: var(--text-secondary, #ccc);
  }
  .popup-warn {
    color: var(--warn, #f0b45e);
    font-size: 12px;
  }
  .popup-actions {
    display: flex;
    justify-content: flex-end;
    gap: 8px;
    margin-top: 2px;
  }
  .btn {
    --wails-draggable: no-drag;
    height: 28px;
    padding: 0 12px;
    background: var(--bg-card, #2a2a2a);
    border: 0.5px solid var(--border, #3c3c3c);
    border-radius: 6px;
    color: var(--text-primary, #fff);
    cursor: pointer;
    font: 500 13px/1 var(--font-sans, system-ui);
  }
  .btn:hover {
    background: var(--bg-hover, rgba(127, 127, 127, 0.12));
  }
  .btn-primary {
    background: var(--accent, #4a90d9);
    border: none;
    color: var(--text-inverse, #fff);
  }
  .btn-primary:hover {
    filter: brightness(1.08);
  }
</style>
