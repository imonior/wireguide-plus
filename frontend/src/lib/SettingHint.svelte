<script>
  // SettingHint — a hint / description line inside Settings.
  //
  // By default it renders exactly the plain <p class="setting-desc"> the
  // settings rows always used. When the settings_hints module is hidden (the
  // mini preset, or the custom preset with that box unticked) the text
  // collapses into a compact "+" toggle instead; clicking it reveals just
  // this one line, so the compact layout stays compact until a single
  // explanation is actually needed.
  //
  // Error / warning text must NOT use this component — it has to stay
  // visible, so those lines keep their plain <p> markup.
  //
  // SettingHint —— 设置界面内的一条提示 / 说明文字。
  // 默认渲染成设置行一直使用的普通 <p class="setting-desc">。当 settings_hints
  // 模块被隐藏（mini 预设，或自定义预设下未勾选该项）时，文字会收成一个紧凑的
  // 「+」开关；点击只展开这一条，使紧凑布局在真正需要某条说明之前保持紧凑。
  // 错误 / 警告文字不要用本组件——它们必须始终可见，因此保留普通 <p> 标记。
  import { t } from '../i18n/index.js';
  import { moduleFlags } from '../stores/ui.js';
  import Icon from './Icon.svelte';

  // Extra class carried over from the original <p> (hotkey-current,
  // path-speed-hint, vis-custom-hint, …).
  export let cls = '';

  let open = false;

  // Reactive read of the store into a plain value: the template must never
  // read reactive state through a helper call (Svelte 5 wraps those in
  // untrack and the UI freezes).
  $: collapsed = !$moduleFlags.settings_hints;
  // Reset the per-line expansion while hints are shown normally, so the
  // next collapse starts from a clean state.
  $: if (!collapsed) open = false;
</script>

{#if collapsed && !open}
  <button
    class="hint-plus"
    type="button"
    on:click={() => (open = true)}
    title={$t('settings.hint_show')}
    aria-label={$t('settings.hint_show')}
  >
    <Icon name="plus" size={11} strokeWidth={2.25} />
  </button>
{:else}
  <p class="setting-desc{cls ? ' ' + cls : ''}">
    <slot />
    {#if collapsed}
      <button
        class="hint-collapse"
        type="button"
        on:click={() => (open = false)}
        title={$t('settings.hint_hide')}
        aria-label={$t('settings.hint_hide')}
      >
        <Icon name="x" size={10} strokeWidth={2.25} />
      </button>
    {/if}
  </p>
{/if}

<style>
  /* Compact reveal control: sits exactly where the description would be, so
     collapsing a hint costs one small button instead of a block of prose. */
  .hint-plus {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 18px;
    height: 18px;
    margin: 2px 0 0;
    padding: 0;
    border: 1px solid var(--border);
    border-radius: 4px;
    background: transparent;
    color: var(--text-muted);
    cursor: pointer;
    transition: background 0.12s ease, color 0.12s ease;
  }

  .hint-plus:hover {
    background: var(--bg-hover);
    color: var(--text-primary);
  }

  /* Collapse control, inline at the end of a revealed hint. */
  .hint-collapse {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 15px;
    height: 15px;
    margin-left: var(--space-1, 4px);
    padding: 0;
    border: none;
    border-radius: 3px;
    background: transparent;
    color: var(--text-muted);
    cursor: pointer;
    vertical-align: middle;
  }

  .hint-collapse:hover {
    background: var(--bg-hover);
    color: var(--text-primary);
  }
</style>
