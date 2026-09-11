<script>
  // ScriptEditor — PreUp/PostUp/PreDown/PostDown hook panel for the tunnel
  // editor. Each hook can reference a script file (picked from any folder,
  // typed-filtered) or hold an inline command. The .conf text (bound via
  // `content`) stays the single source of truth: display values are parsed
  // locally for zero-latency typing sync, and every WRITE goes through the
  // Go SetHookInText binding (unit-tested) which touches only the target
  // hook lines inside [Interface].
  import { createEventDispatcher } from 'svelte';
  import { TunnelService } from '../../bindings/github.com/imonior/wireguide-plus/internal/app';
  import CodeModal from './CodeModal.svelte';
  import { t } from '../i18n/index.js';

  export let content = ''; // two-way bound to the editor's .conf text
  export let name = ''; // tunnel name (preset for new script filenames)
  export let enabled = false; // settings.enable_wg_scripts

  const dispatch = createEventDispatcher();

  const HOOKS = ['PreUp', 'PostUp', 'PreDown', 'PostDown'];

  let busyHook = '';
  let error = '';
  // Code modal state
  let modalOpen = false;
  let modalTitle = '';
  let modalPath = '';
  let modalInitial = '';
  let modalIsFile = false;
  let modalHook = '';
  let modalBusy = false;

  // --- local mirror of the Go hook-text logic (read-only) ---

  function hookRe(hook) {
    return new RegExp('^\\s*' + hook + '\\s*=', 'i');
  }

  function getHookLocal(text, hook) {
    const sectionRe = /^\s*\[/;
    let inIface = false;
    const vals = [];
    for (const line of (text || '').split('\n')) {
      const trimmed = line.trim();
      if (sectionRe.test(trimmed)) {
        inIface = trimmed.toLowerCase() === '[interface]';
        continue;
      }
      if (inIface && hookRe(hook).test(line)) {
        const v = line.slice(line.indexOf('=') + 1).trim();
        if (v) vals.push(v);
      }
    }
    return vals.join('; ');
  }

  function resolveRefLocal(cmd) {
    const c = (cmd || '').trim();
    if (!c) return null;
    let m = c.match(/-file\s+"([^"]+)"/i);
    if (m) return m[1];
    m = c.match(/^(?:bash|sh|pwsh)\s+"([^"]+)"$/i);
    if (m) return m[1];
    m = c.match(/^"([^"]+)"$/);
    if (m && /\.(ps1|bat|cmd|sh)$/i.test(m[1])) return m[1];
    return null;
  }

  function baseName(p) {
    return String(p || '').replace(/[\\/]+$/, '').split(/[\\/]/).pop() || p;
  }

  // --- reactive display state ---

  $: preUp = enabled ? getHookLocal(content, 'PreUp') : '';
  $: postUp = enabled ? getHookLocal(content, 'PostUp') : '';
  $: preDown = enabled ? getHookLocal(content, 'PreDown') : '';
  $: postDown = enabled ? getHookLocal(content, 'PostDown') : '';

  function hookCmd(hook) {
    switch (hook) {
      case 'PreUp': return preUp;
      case 'PostUp': return postUp;
      case 'PreDown': return preDown;
      case 'PostDown': return postDown;
    }
    return '';
  }

  // --- actions ---

  async function applyHook(hook, cmd) {
    error = '';
    try {
      content = await TunnelService.SetHookInText(content, hook, cmd);
      // Notify the parent so unsaved-change indicators can react.
      dispatch('change', { hook, command: cmd });
    } catch (e) {
      error = e?.message || String(e);
    }
  }

  async function pickFile(hook) {
    if (busyHook) return;
    busyHook = hook;
    error = '';
    try {
      const path = await TunnelService.OpenScriptFile();
      if (path) {
        const cmd = await TunnelService.ScriptInvocation(path);
        await applyHook(hook, cmd);
      }
    } catch (e) {
      error = e?.message || String(e);
    }
    busyHook = '';
  }

  async function newBlank(hook) {
    if (busyHook) return;
    busyHook = hook;
    error = '';
    try {
      const preset = `${(name || 'tunnel').trim() || 'tunnel'}_${hook.toLowerCase()}`;
      const path = await TunnelService.SaveScriptFile(preset);
      if (path) {
        await TunnelService.WriteScriptFile(path, '');
        const cmd = await TunnelService.ScriptInvocation(path);
        await applyHook(hook, cmd);
        openModal(hook, path, '', true);
      }
    } catch (e) {
      error = e?.message || String(e);
    }
    busyHook = '';
  }

  async function editCode(hook) {
    const cmd = hookCmd(hook);
    if (!cmd || busyHook) return;
    busyHook = hook;
    error = '';
    try {
      const ref = resolveRefLocal(cmd);
      if (ref) {
        const text = await TunnelService.ReadScriptFile(ref);
        openModal(hook, ref, text, true);
      } else {
        // Inline command — edit the raw command text itself.
        openModal(hook, '', cmd, false);
      }
    } catch (e) {
      error = e?.message || String(e);
    }
    busyHook = '';
  }

  function openModal(hook, path, initial, isFile) {
    modalHook = hook;
    modalPath = path;
    modalInitial = initial;
    modalIsFile = isFile;
    modalTitle = isFile ? `${hook} — ${baseName(path)}` : `${hook}`;
    modalOpen = true;
  }

  async function onModalSave(e) {
    const { path, content: text } = e.detail;
    modalBusy = true;
    try {
      if (modalIsFile && path) {
        await TunnelService.WriteScriptFile(path, text);
      } else {
        await applyHook(modalHook, text.trim());
      }
      modalOpen = false;
    } catch (err) {
      error = err?.message || String(err);
    }
    modalBusy = false;
  }

  function statusKey(hook) {
    const cmd = hookCmd(hook);
    if (!cmd) return 'scripts.status_empty';
    return resolveRefLocal(cmd) ? 'scripts.status_file' : 'scripts.status_inline';
  }

  function rowClass(hook) {
    return hookCmd(hook) ? 'hook-row has-value' : 'hook-row';
  }
</script>

{#if enabled}
  <div class="scripts-panel">
    <div class="scripts-head">
      <span class="scripts-title">{$t('scripts.title')}</span>
      <p class="scripts-hint">{$t('scripts.hint')}</p>
    </div>

    <!-- Two hooks per row (PreUp+PostUp, then PreDown+PostDown): the script
         panel no longer needs four stacked rows, so the .conf / fields editor
         above keeps the vertical space. Collapses to one column on a narrow
         dialog. -->
    <div class="hooks-grid">
      {#each HOOKS as hook}
        <div class={rowClass(hook)}>
        <div class="hook-info">
          <span class="hook-name">{hook}</span>
          <span class="hook-status" title={hookCmd(hook)}>
            {#if hookCmd(hook) && resolveRefLocal(hookCmd(hook))}
              {baseName(resolveRefLocal(hookCmd(hook)))}
            {:else if hookCmd(hook)}
              {$t('scripts.status_inline')} — {hookCmd(hook)}
            {:else}
              {$t('scripts.status_empty')}
            {/if}
          </span>
        </div>
        <div class="hook-actions">
          <button class="hook-btn" title={$t('scripts.select_file')} disabled={busyHook !== ''} on:click={() => pickFile(hook)}>
            {$t('scripts.select_file')}
          </button>
          <button class="hook-btn" title={$t('scripts.new_blank')} disabled={busyHook !== ''} on:click={() => newBlank(hook)}>
            {$t('scripts.new_blank')}
          </button>
          <button class="hook-btn" title={$t('scripts.edit_code')} disabled={busyHook !== '' || !hookCmd(hook)} on:click={() => editCode(hook)}>
            {$t('scripts.edit_code')}
          </button>
          <button class="hook-btn hook-btn-danger" title={$t('scripts.clear')} disabled={busyHook !== '' || !hookCmd(hook)} on:click={() => applyHook(hook, '')}>
            {$t('scripts.clear')}
          </button>
        </div>
      </div>
    {/each}
    </div>

    {#if error}
      <p class="scripts-error">{error}</p>
    {/if}
  </div>
{/if}

<CodeModal
  open={modalOpen}
  title={modalTitle}
  path={modalPath}
  initial={modalInitial}
  busy={modalBusy}
  on:save={onModalSave}
  on:close={() => (modalOpen = false)}
/>

<style>
  .scripts-panel {
    display: flex;
    flex-direction: column;
    gap: 6px;
    padding: 10px 16px;
    border-top: 0.5px solid var(--border);
    background: var(--bg-secondary);
    /* Grows to its natural height — the whole editor-stack scrolls, so the
       script panel is never compressed into its own scroll and always shows
       every hook row in full. */
    flex: 0 0 auto;
    min-height: 0;
  }
  /* Two hooks per row: halving the stacked rows keeps the scripts panel
     compact so the conf/fields editor above keeps the vertical space. */
  .hooks-grid {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 6px;
  }
  @media (max-width: 860px) {
    .hooks-grid { grid-template-columns: 1fr; }
  }
  .scripts-head {
    display: flex;
    flex-direction: column;
    gap: 2px;
  }
  .scripts-title {
    font: 600 12px/16px var(--font-sans);
    text-transform: uppercase;
    letter-spacing: 0.04em;
    color: var(--text-secondary);
  }
  .scripts-hint {
    margin: 0;
    font: 11.5px/14px var(--font-sans);
    color: var(--text-secondary);
  }
  .hook-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 8px;
    padding: 5px 10px;
    border: 0.5px solid var(--border);
    border-radius: 10px;
    background: var(--bg-card);
    /* Half-width cells: let the action buttons wrap under the name/status
       instead of forcing the row taller or clipping them. */
    flex-wrap: wrap;
  }
  .hook-row.has-value {
    border-color: color-mix(in srgb, var(--accent) 32%, var(--border));
  }
  .hook-info {
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 1px;
  }
  .hook-name {
    font: 600 12.5px/16px var(--font-mono, monospace);
    color: var(--text-primary);
  }
  .hook-status {
    font: 11px/14px var(--font-sans);
    color: var(--text-secondary);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    max-width: 100%;
  }
  .hook-actions {
    display: flex;
    gap: 5px;
    flex-shrink: 0;
    flex-wrap: wrap;
  }
  .hook-btn {
    height: 26px;
    padding: 0 9px;
    border: 0.5px solid var(--border);
    border-radius: 7px;
    background: var(--bg-card);
    font: 500 11.5px/15px var(--font-sans);
    color: var(--text-primary);
    cursor: pointer;
    white-space: nowrap;
  }
  .hook-btn:hover:not(:disabled) {
    background: var(--bg-hover);
    border-color: color-mix(in srgb, var(--accent) 30%, var(--border));
  }
  .hook-btn:disabled {
    opacity: 0.45;
    cursor: default;
  }
  .hook-btn-danger:hover:not(:disabled) {
    border-color: color-mix(in srgb, var(--red) 45%, var(--border));
    color: var(--red);
  }
  .scripts-error {
    margin: 0;
    font: 11.5px/15px var(--font-sans);
    color: var(--error-text);
  }
</style>
