<script>
  // AddressConflictDialog — raised when the helper finds another client
  // already holding one of our tunnel addresses on a different adapter (the
  // "two WireGuard clients running the same tunnel" conflict). This is an
  // INTERACTIVE dialog: it names the conflicting software/adapter, shows the
  // current connection state, and offers to stop automation for that tunnel.
  // The helper never auto-stops anything — per the "don't forcibly stop, hand
  // it to the user" rule — so the only action we take is the one the user
  // clicks.
  import { createEventDispatcher } from 'svelte';
  import { TunnelService } from '../../bindings/github.com/imonior/wireguide-plus/internal/app';
  import { t } from '../i18n/index.js';

  export let conflict = null; // { tunnel, address, adapter, software, state }
  const dispatch = createEventDispatcher();

  function stateLabel(state) {
    if (state === 'active') return $t('conflict.address_conflict.state_active');
    if (state === 'down') return $t('conflict.address_conflict.state_down');
    return $t('conflict.address_conflict.state_unknown');
  }
  function softwareLabel(software) {
    if (software === 'WireGuard (official client)') return $t('conflict.address_conflict.software_official_wg');
    if (software === 'unknown') return $t('conflict.address_conflict.software_unknown');
    return software || '';
  }

  async function stopAutomation() {
    if (!conflict?.tunnel) return;
    try {
      const p = (await TunnelService.GetTunnelPolicies(conflict.tunnel)) || {};
      await TunnelService.SetTunnelPolicies(conflict.tunnel, { ...p, automation_disabled: true });
      dispatch('resolved', { action: 'stop' });
    } catch (e) {
      console.warn('failed to stop automation for tunnel:', e);
      dispatch('resolved', { action: 'stop', error: String(e) });
    }
  }
  function later() {
    dispatch('resolved', { action: 'later' });
  }
  function onKeyDown(e) {
    if (e.key === 'Escape') {
      e.preventDefault();
      later();
    }
  }
</script>

<svelte:window on:keydown={onKeyDown} />

<div class="modal-backdrop" on:click={later}>
  <div
    class="modal"
    on:click|stopPropagation
    role="alertdialog"
    aria-modal="true"
    tabindex="-1"
    aria-labelledby="addr-conflict-title"
  >
    <h3 id="addr-conflict-title">{$t('conflict.address_conflict.title')}</h3>
    <p class="msg">
      {$t('conflict.address_conflict.message', {
        software: softwareLabel(conflict?.software),
        address: conflict?.address || '',
        tunnel: conflict?.tunnel || '',
        adapter: conflict?.adapter || '',
        state: stateLabel(conflict?.state),
      })}
    </p>

    <div class="detail">
      <div class="row"><span class="k">{$t('conflict.address_conflict.field_tunnel')}</span><span class="v">{conflict?.tunnel}</span></div>
      <div class="row"><span class="k">{$t('conflict.address_conflict.field_address')}</span><span class="v">{conflict?.address}</span></div>
      <div class="row"><span class="k">{$t('conflict.address_conflict.field_adapter')}</span><span class="v">{conflict?.adapter}</span></div>
      <div class="row"><span class="k">{$t('conflict.address_conflict.field_software')}</span><span class="v">{softwareLabel(conflict?.software)}</span></div>
      <div class="row"><span class="k">{$t('conflict.address_conflict.field_state')}</span><span class="v">{stateLabel(conflict?.state)}</span></div>
    </div>

    <p class="note">{$t('conflict.address_conflict.note')}</p>

    <div class="actions">
      <button class="btn btn-cancel" on:click={later}>{$t('conflict.address_conflict.later')}</button>
      <button class="btn btn-warn" on:click={stopAutomation}>{$t('conflict.address_conflict.stop_automation')}</button>
    </div>
  </div>
</div>

<style>
  .modal-backdrop {
    position: fixed;
    inset: 0;
    background: var(--overlay-bg);
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 320;
  }
  .modal {
    background: var(--bg-primary);
    border: 1px solid var(--red, #e05252);
    border-radius: 12px;
    padding: 24px;
    width: 520px;
    max-width: 92vw;
    box-shadow: var(--shadow-md);
  }
  h3 { color: var(--red, #e05252); margin: 0 0 14px; }
  .msg {
    font-size: 13px;
    color: var(--text-primary);
    line-height: 1.55;
    margin: 0 0 14px;
  }
  .detail {
    background: var(--bg-card);
    border: 1px solid var(--border);
    border-radius: 8px;
    padding: 10px 12px;
    margin-bottom: 12px;
  }
  .row {
    display: flex;
    gap: 10px;
    font-size: 12px;
    padding: 3px 0;
  }
  .k {
    color: var(--text-secondary);
    min-width: 84px;
    font-weight: 600;
  }
  .v {
    color: var(--text-primary);
    font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
    word-break: break-all;
  }
  .note {
    font-size: 12px;
    color: var(--text-muted);
    line-height: 1.5;
    margin: 0 0 16px;
  }
  .actions { display: flex; gap: 8px; justify-content: flex-end; }
  .btn {
    padding: 8px 16px;
    border: none;
    border-radius: 6px;
    cursor: pointer;
    font-size: 13px;
  }
  .btn-warn { background: var(--red, #e05252); color: #fff; font-weight: 600; }
  .btn-cancel { background: var(--bg-card); color: var(--text-primary); border: 1px solid var(--border); }
</style>
