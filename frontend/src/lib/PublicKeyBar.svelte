<script>
  // Shared read-only public-key indicator. Rendered directly below the name
  // field in BOTH the conf and fields editor tabs so the key is visible while
  // editing either representation. It derives the public key from a supplied
  // private-key string via TunnelService.DerivePublicKey; the value is NEVER
  // written into `content` (WireGuard derives it itself and rejects an
  // explicit PublicKey under [Interface]).
  //
  // In the fields tab the private key is the live input value; in the conf
  // tab it is parsed from the conf text on every keystroke. A generation
  // counter prevents a slow earlier RPC from overwriting a newer result.
  // Validation is NOT here: it happens at SAVE time in App.doSave, which
  // raises a single modal on success or failure. Keeping validation out of
  // this bar avoids a dialog on every keystroke and a second "confirm"
  // button competing with the editor's Save.
  //
  // 共享只读公钥指示器。在 conf 与字段两个编辑 tab 中均渲染于名称栏正下方，
  // 使两种编辑视图下都能看到公钥。它通过 TunnelService.DerivePublicKey 从传入的
  // 私钥字符串派生公钥；该值绝不写回 content（WireGuard 自行派生，会拒绝
  // [Interface] 下显式的 PublicKey）。
  //
  // 字段 tab 传入实时输入值；conf tab 则从每次按键的 conf 文本中解析。代际计数
  // 避免较慢的旧 RPC 覆盖新结果。校验不在此处：统一放到 App.doSave 的保存时，
  // 成功或失败只弹一次弹窗。把校验移出本条，既避免逐字符弹窗，也避免第二个
  // 「确认」按钮与编辑器的 Save 语义撞车。
  import { t } from '../i18n/index.js';
  import Icon from './Icon.svelte';

  export let privateKey = '';
  export let TunnelService = null;

  let publicKey = '';
  let state = 'idle'; // idle | ok | invalid
  let gen = 0;
  let copied = false;
  let copyTimer = null;

  async function derive(value) {
    const raw = (value || '').trim();
    const g = ++gen;
    if (!raw) {
      publicKey = '';
      state = 'idle';
      return;
    }
    if (!TunnelService || typeof TunnelService.DerivePublicKey !== 'function') return;
    try {
      const key = await TunnelService.DerivePublicKey(raw);
      if (g !== gen) return; // a newer text already superseded this reply
      publicKey = key || '';
      state = publicKey ? 'ok' : 'invalid';
    } catch (_) {
      if (g !== gen) return;
      publicKey = '';
      state = 'invalid';
    }
  }

  // Private key state is passed as a prop (not read from a helper call inside
  // the template), so this assignment stays reactive across both tabs.
  // 私钥状态以 prop 传入（而非模板内辅助函数读取），因此赋值在两个 tab 中都保持响应式。
  $: derive(privateKey);

  async function copy() {
    if (!publicKey) return;
    try {
      await navigator.clipboard.writeText(publicKey);
      copied = true;
      if (copyTimer) clearTimeout(copyTimer);
      copyTimer = setTimeout(() => { copied = false; }, 1500);
    } catch (e) {
      console.error('[public-key-bar] copy failed:', e);
    }
  }
</script>

<div class="pk-bar">
  <span class="pk-label">PublicKey</span>
  <span class="pk-value" class:pk-empty={state !== 'ok'}
    title={state === 'ok' ? publicKey : ''}>
    {#if state === 'ok'}
      {publicKey}
    {:else if state === 'invalid'}
      {$t('fields.public_key_invalid')}
    {:else}
      {$t('fields.public_key_placeholder')}
    {/if}
  </span>
  {#if state === 'ok'}
    <button class="pk-copy" class:copied={copied}
      title={$t('fields.public_key_copy')} aria-label={$t('fields.public_key_copy')}
      on:click={copy}>
      <Icon name={copied ? 'check' : 'copy'} size={12} strokeWidth={1.75} />
      <span>{copied ? $t('fields.public_key_copied') : $t('fields.public_key_copy')}</span>
    </button>
  {/if}
</div>

<style>
  .pk-bar {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 8px 16px;
    border-bottom: 0.5px solid var(--border);
    background: var(--bg-primary);
    flex-shrink: 0;
    min-width: 0;
  }
  .pk-label {
    flex-shrink: 0;
    font-size: 12px;
    font-family: ui-monospace, monospace;
    opacity: 0.7;
  }
  .pk-value {
    flex: 1;
    min-width: 0;
    font-size: 12px;
    font-family: ui-monospace, monospace;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    user-select: text;
    -webkit-user-select: text;
  }
  .pk-value.pk-empty {
    font-family: var(--font-sans);
    font-style: italic;
    opacity: 0.5;
  }
  .pk-copy {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    flex-shrink: 0;
    height: 26px;
    padding: 0 9px;
    border: 0.5px solid var(--border, #ccc);
    border-radius: 6px;
    background: var(--bg-card);
    color: var(--text-secondary);
    font: 500 11px/1 var(--font-sans);
    cursor: pointer;
  }
  .pk-copy:not(:disabled):hover { border-color: var(--accent); color: var(--text-primary); }
  .pk-copy.copied { color: var(--green, #30a46c); border-color: var(--green, #30a46c); }
</style>
