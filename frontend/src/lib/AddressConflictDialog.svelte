<script>
  // AddressConflictDialog — raised when the helper finds another client
  // already holding one of our tunnel addresses on a different adapter (the
  // "two WireGuard clients running the same tunnel" conflict). This dialog is
  // PERSISTENT and blocking: the helper has paused this tunnel's
  // auto-connect, and it stays paused until the user answers here with one
  // of the two buttons. Escape and backdrop clicks are deliberately not
  // handled — closing the app window is not an answer, and the next GUI
  // launch re-pulls the unresolved conflicts, so the prompt returns.
  import { createEventDispatcher } from 'svelte';
  import { TunnelService } from '../../bindings/github.com/imonior/wireguide-plus/internal/app';
  import { t } from '../i18n/index.js';

  export let conflict = null; // { tunnel, address, adapter, software, state }
  const dispatch = createEventDispatcher();

  // The tunnel's durable automation opt-out (meta sidecar). null until
  // loaded; when it already says "automation stopped" the conflict is
  // informational only — the helper will never fight for the address again,
  // so there is nothing to decide and no buttons to offer.
  let automationDisabled = null;
  TunnelService.GetTunnelPolicies(conflict?.tunnel || '')
    .then((p) => { automationDisabled = !!(p && p.automation_disabled); })
    .catch(() => { automationDisabled = null; });

  function automationLabel() {
    if (automationDisabled === true) return $t('tunnel.automation_off');
    if (automationDisabled === false) return $t('conflict.address_conflict.automation_paused');
    return $t('conflict.address_conflict.automation_unknown');
  }

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

  // "Keep trying" answers the pause the other way: it lifts the conflict
  // pause so automation resumes immediately (the helper re-evaluates right
  // away) and closes the dialog. If the other client still holds the address
  // the next failed connect pauses it again and the dialog returns — which
  // is the point: the user chose to keep trying, once per attempt.
  async function resumeTrying() {
    if (!conflict?.tunnel) return;
    try {
      await TunnelService.ResumeAutoConnect(conflict.tunnel);
      dispatch('resolved', { action: 'resume' });
    } catch (e) {
      console.warn('failed to resume auto-connect for tunnel:', e);
      dispatch('resolved', { action: 'resume', error: String(e) });
    }
  }
</script>

<div class="modal-backdrop">
  <div
    class="modal"
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
      <div class="row"><span class="k">{$t('conflict.address_conflict.field_automation')}</span><span class="v">{automationLabel()}</span></div>
    </div>

    <p class="note">{$t('conflict.address_conflict.note')}</p>

    {#if automationDisabled === true}
      <p class="note">{$t('conflict.address_conflict.info_only')}</p>
    {:else}
      <div class="actions">
        <button class="btn btn-cancel" on:click={resumeTrying}>{$t('conflict.address_conflict.resume_retry')}</button>
        <button class="btn btn-warn" on:click={stopAutomation}>{$t('conflict.address_conflict.stop_automation')}</button>
      </div>
    {/if}
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
