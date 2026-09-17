<script>
  // Status bubble for macOS/Linux. Rendered by App.svelte when the window
  // URL carries ?popup=1 (the secondary Wails window opened from Go's
  // showStatusPopupWails). Mirrors the Windows Win32 bubble: title, status
  // dot + caption, the tunnel list, an amber "out of tunnel routes" warning,
  // and Open Window / Disconnect buttons, auto-closing after the configured
  // duration.
  import { onMount, onDestroy } from 'svelte';
  import { Events } from '@wailsio/runtime';
  import { t } from '../i18n/index.js';

  let names = [];
  let state = 'disconnected';
  let outOfRange = [];
  let durationMs = 10000;
  let closeTimer = null;
  let unsub = null;

  function statusText() {
    if (state === 'connected') return $t('popup.connected');
    if (state === 'connecting') return $t('popup.connecting');
    return $t('popup.not_connected');
  }

  function apply(d) {
    names = (d && d.names) || [];
    state = (d && d.state) || 'disconnected';
    outOfRange = (d && d.out_of_range) || [];
    durationMs = (d && d.duration_ms) || 10000;
    if (closeTimer) clearTimeout(closeTimer);
    closeTimer = setTimeout(close, durationMs);
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

  <div class="popup-status">
    <span class="dot" class:connected={state === 'connected'} class:connecting={state === 'connecting'}></span>
    <span class="status-text">{statusText()}</span>
  </div>

  {#if names.length > 0}
    <div class="popup-names">{names.join(', ')}</div>
  {/if}

  {#if outOfRange.length > 0}
    <div class="popup-warn">{$t('popup.out_of_range')}: {outOfRange.join(', ')}</div>
  {/if}

  <div class="popup-actions">
    <button class="btn" on:click={disconnect}>{$t('popup.disconnect')}</button>
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
  .dot {
    width: 9px;
    height: 9px;
    border-radius: 50%;
    background: var(--text-muted, #888);
    flex-shrink: 0;
  }
  .dot.connected {
    background: var(--green, #34c759);
    box-shadow: 0 0 8px color-mix(in srgb, var(--green, #34c759) 50%, transparent);
  }
  .status-text {
    font-weight: 600;
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
