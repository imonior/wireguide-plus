<script>
  // KeepaliveZeroDialog — raised at SAVE time when a peer's PersistentKeepalive
  // is unset or set to 0. A peer with no keepalive behind a NAT/firewall lets
  // the mapping time out, so the next handshake cannot reach the peer and the
  // tunnel silently stalls until a manual reconnect. This dialog turns that
  // silent failure into an explicit, informed choice — it never rewrites the
  // file on its own, and it does NOT auto-connect a disconnected tunnel: the
  // caller only proceeds (or rewrites + proceeds) after the user answers.
  // Three outcomes:
  //   • keep0  — server / non-NAT peer; user keeps 0 deliberately.
  //   • use25  — NAT'd client; caller rewrites the value to 25 and saves.
  //   • cancel — return to the editor, user edits manually.
  import { createEventDispatcher } from 'svelte';
  import { t } from '../i18n/index.js';

  const dispatch = createEventDispatcher();

  function keep0() {
    dispatch('resolved', { action: 'keep0' });
  }
  function use25() {
    dispatch('resolved', { action: 'use25' });
  }
  function cancel() {
    dispatch('resolved', { action: 'cancel' });
  }
</script>

<div class="modal-backdrop">
  <div
    class="modal"
    role="alertdialog"
    aria-modal="true"
    tabindex="-1"
    aria-labelledby="keepalive-zero-title"
  >
    <h3 id="keepalive-zero-title">{$t('keepalive_zero.title')}</h3>
    <p class="msg">{$t('keepalive_zero.message')}</p>

    <div class="actions">
      <button class="btn btn-cancel" on:click={cancel}>{$t('keepalive_zero.option_cancel')}</button>
      <button class="btn btn-secondary" on:click={keep0}>{$t('keepalive_zero.option_keep0')}</button>
      <button class="btn btn-primary" on:click={use25}>{$t('keepalive_zero.option_use25')}</button>
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
    border: 1px solid var(--border);
    border-radius: 12px;
    padding: 22px;
    width: 540px;
    max-width: 92vw;
    box-shadow: var(--shadow-md);
  }
  h3 {
    margin: 0 0 12px;
    color: var(--text-primary);
    font-size: 16px;
  }
  .msg {
    font-size: 13px;
    color: var(--text-primary);
    line-height: 1.6;
    margin: 0 0 18px;
  }
  .actions {
    display: flex;
    gap: 8px;
    justify-content: flex-end;
    flex-wrap: wrap;
  }
  .btn {
    padding: 8px 14px;
    border: none;
    border-radius: 6px;
    cursor: pointer;
    font-size: 13px;
    text-align: center;
  }
  .btn-primary { background: var(--accent, #3b82f6); color: #fff; font-weight: 600; }
  .btn-secondary { background: var(--bg-card); color: var(--text-primary); border: 1px solid var(--border); }
  .btn-cancel { background: transparent; color: var(--text-secondary); border: 1px solid var(--border); }
</style>
