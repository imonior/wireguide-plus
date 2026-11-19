<script>
  // KeySaveDialog — raised at SAVE time after validating the [Interface]
  // PrivateKey. Two modes:
  //   • invalid — the key cannot be parsed as a WireGuard private key; the
  //     config was NOT written and the user must enter a valid one.
  //   • saved   — the key was valid, the config was written, and the derived
  //     public key is up to date. Reminds the user to update the public key
  //     stored on the remote peer, which otherwise keeps trusting the old
  //     identity and silently drops the tunnel.
  // This dialog is the single place key-validation feedback reaches the user,
  // replacing the old per-keystroke bar warning and the separate Verify button.
  //
  // 保存时校验 [Interface] PrivateKey 后的结果弹窗，两种模式：
  //   • invalid —— 私钥不是合法的 WireGuard 密钥，配置未写入，须输入合法私钥。
  //   • saved   —— 私钥合法、配置已写入、派生公钥已更新；提醒同步更新对端服务器
  //     保存的公钥，否则对端仍信任旧身份、隧道会静默失效。
  // 这里是私钥校验反馈的唯一出口，取代原先逐字符的行内警告与独立的校验按钮。
  import { createEventDispatcher } from 'svelte';
  import { t } from '../i18n/index.js';
  import Icon from './Icon.svelte';

  export let mode = 'invalid'; // 'invalid' | 'saved'

  const dispatch = createEventDispatcher();

  function acknowledge() {
    dispatch('resolved', { mode });
  }
</script>

<div class="modal-backdrop" on:click={acknowledge} role="presentation">
  <div class="modal" role="alertdialog" aria-modal="true" tabindex="-1"
    aria-label={mode === 'saved' ? $t('key_save.saved_title') : $t('key_save.invalid_title')}
    on:click|stopPropagation>
    <div class="modal-icon" class:ok={mode === 'saved'}>
      <Icon name={mode === 'saved' ? 'check' : 'triangle-alert'} size={20} strokeWidth={1.75} />
    </div>
    {#if mode === 'saved'}
      <h3>{$t('key_save.saved_title')}</h3>
      <p class="msg">{$t('key_save.saved_msg')}</p>
    {:else}
      <h3>{$t('key_save.invalid_title')}</h3>
      <p class="msg">{$t('key_save.invalid_msg')}</p>
    {/if}
    <div class="actions">
      <button class="btn" on:click={acknowledge}>{$t('editor.close')}</button>
    </div>
  </div>
</div>

<style>
  .modal-backdrop {
    position: fixed;
    inset: 0;
    background: var(--overlay-bg, rgba(0, 0, 0, 0.45));
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 1100; /* above the editor modal (1000) so the result is unmissable */
  }
  .modal {
    background: var(--bg-primary);
    border: 1px solid var(--border);
    border-radius: 12px;
    padding: 22px;
    width: 480px;
    max-width: 92vw;
    box-shadow: var(--shadow-md, 0 20px 56px rgba(0, 0, 0, 0.3));
  }
  .modal-icon { color: var(--red, #e5484d); margin-bottom: 10px; }
  .modal-icon.ok { color: var(--green, #30a46c); }
  h3 { margin: 0 0 12px; color: var(--text-primary); font-size: 16px; }
  .msg { font-size: 13px; color: var(--text-primary); line-height: 1.6; margin: 0 0 18px; }
  .actions { display: flex; gap: 8px; justify-content: flex-end; }
  .btn {
    padding: 8px 16px; border: 0; border-radius: 6px; cursor: pointer;
    font-size: 13px; background: var(--accent, #3b82f6); color: #fff; font-weight: 600;
  }
</style>
