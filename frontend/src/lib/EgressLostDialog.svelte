<script>
  import { createEventDispatcher } from 'svelte';
  import { t } from '../i18n/index.js';

  export let open = false;
  // { tunnel, if_name, if_index, reason } — from event.egress_interface_lost
  export let info = null;

  const dispatch = createEventDispatcher();

  function close() {
    open = false;
    info = null;
  }

  // Keep waiting on the pinned NIC: nothing to change, just dismiss.
  function keepWaiting() {
    dispatch('wait');
    close();
  }

  // Switch this tunnel to auto-selection: clear the saved binding so the
  // next re-evaluation / reconnect picks the best available underlay.
  async function switchToAuto() {
    dispatch('auto');
    close();
  }

  // Hand-pick a different interface: open the tunnel editor (fields tab)
  // where the egress dropdown lives.
  function pickManually() {
    dispatch('manual');
    close();
  }

  $: reasonKey =
    info?.reason === 'missing' ? 'egress_lost.reason_missing'
    : info?.reason === 'invalid' ? 'egress_lost.reason_invalid'
    : 'egress_lost.reason_down';
</script>

{#if open && info}
  <div class="el-backdrop">
    <div class="el-modal" role="dialog" aria-modal="true" tabindex="-1"
      aria-labelledby="el-title">
      <h3 class="el-title" id="el-title">{$t('egress_lost.title')}</h3>
      <p class="el-body">
        {$t('egress_lost.body', { tunnel: info.tunnel, ifname: info.if_name || `#${info.if_index}` })}
      </p>
      <p class="el-reason">{$t(reasonKey)}</p>
      <p class="el-note">{$t('egress_lost.note')}</p>
      <div class="el-actions">
        <button type="button" class="btn" on:click={keepWaiting}>{$t('egress_lost.wait')}</button>
        <button type="button" class="btn" on:click={pickManually}>{$t('egress_lost.manual')}</button>
        <button type="button" class="btn btn--primary" on:click={switchToAuto}>{$t('egress_lost.auto')}</button>
      </div>
    </div>
  </div>
{/if}

<style>
  .el-backdrop {
    position: fixed;
    inset: 0;
    background: var(--overlay-bg, rgba(0, 0, 0, 0.45));
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 400;
  }
  .el-modal {
    width: 480px;
    max-width: calc(100vw - 32px);
    background: var(--bg-primary);
    border: 0.5px solid var(--border);
    border-radius: 14px;
    padding: 22px 22px 18px;
    box-shadow: 0 18px 48px rgba(0, 0, 0, 0.26);
  }
  .el-title {
    margin: 0 0 10px;
    font: 600 15px/20px var(--font-sans, -apple-system, BlinkMacSystemFont, sans-serif);
    color: var(--danger, #d33);
    letter-spacing: -0.01em;
  }
  .el-body {
    margin: 0 0 8px;
    font: 400 13px/19px var(--font-sans, -apple-system, BlinkMacSystemFont, sans-serif);
    color: var(--text-primary);
  }
  .el-reason {
    margin: 0 0 8px;
    font: 500 12.5px/18px var(--font-sans, -apple-system, BlinkMacSystemFont, sans-serif);
    color: var(--text-secondary);
  }
  .el-note {
    margin: 0 0 16px;
    font: 400 12.5px/18px var(--font-sans, -apple-system, BlinkMacSystemFont, sans-serif);
    color: var(--text-muted);
  }
  .el-actions {
    display: flex;
    gap: 8px;
    justify-content: flex-end;
    flex-wrap: wrap;
  }
</style>
