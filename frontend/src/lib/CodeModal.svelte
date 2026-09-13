<script>
  // CodeModal — lightweight plain-text CodeMirror modal for editing hook
  // script files (.ps1/.bat/.sh). Deliberately minimal: line numbers,
  // undo history and the shared light/dark theme handling from
  // ConfigEditor, but no WireGuard grammar or completion.
  import { onMount, onDestroy, createEventDispatcher } from 'svelte';
  import { EditorView, keymap, lineNumbers, highlightActiveLine } from '@codemirror/view';
  import { EditorState, Compartment } from '@codemirror/state';
  import { defaultKeymap, history, historyKeymap, indentWithTab } from '@codemirror/commands';
  import { oneDark } from '@codemirror/theme-one-dark';
  import { resolvedTheme } from '../stores/theme.js';
  import { t } from '../i18n/index.js';

  export let open = false;
  export let title = '';
  export let path = '';
  export let initial = '';
  export let busy = false;

  const dispatch = createEventDispatcher();

  let editorContainer;
  let view;
  let text = initial;
  const themeCompartment = new Compartment();

  const lightAppearance = EditorView.theme(
    {
      '&': { backgroundColor: 'var(--editor-bg)', color: 'var(--text-primary)' },
      '.cm-content': { caretColor: 'var(--accent)' },
      '.cm-activeLine': { backgroundColor: 'var(--bg-hover)' },
      '.cm-activeLineGutter': { backgroundColor: 'var(--bg-hover)' },
      '.cm-selectionBackground, &.cm-focused .cm-selectionBackground, .cm-content ::selection': {
        backgroundColor: 'var(--accent-tint)',
      },
    },
    { dark: false }
  );

  function themeExt(resolved) {
    return resolved === 'light' ? lightAppearance : oneDark;
  }

  const unsubTheme = resolvedTheme.subscribe((resolved) => {
    if (view) {
      view.dispatch({ effects: themeCompartment.reconfigure(themeExt(resolved)) });
    }
  });

  onMount(() => {
    const initialThemeExt = themeExt(
      (typeof document !== 'undefined' ? document.documentElement.getAttribute('data-theme') : 'dark') === 'light'
        ? 'light'
        : 'dark'
    );
    const state = EditorState.create({
      doc: initial,
      extensions: [
        lineNumbers(),
        highlightActiveLine(),
        history(),
        keymap.of([indentWithTab, ...defaultKeymap, ...historyKeymap]),
        themeCompartment.of(initialThemeExt),
        EditorView.theme({
          '&': { height: '100%', fontSize: '13px' },
          '.cm-content': { fontFamily: 'monospace' },
          '.cm-gutters': {
            background: 'var(--editor-gutter-bg)',
            borderRight: '1px solid var(--editor-border)',
          },
        }),
        EditorView.updateListener.of((update) => {
          if (update.docChanged) {
            text = update.state.doc.toString();
          }
        }),
      ],
    });
    view = new EditorView({ state, parent: editorContainer });
  });

  onDestroy(() => {
    unsubTheme();
    if (view) view.destroy();
  });

  function save() {
    dispatch('save', { path, content: text });
  }

  function close() {
    dispatch('close');
  }

  function handleKeydown(e) {
    if (e.key === 'Escape') {
      e.stopPropagation();
      close();
    }
  }
</script>

<svelte:window on:keydown={handleKeydown} />

{#if open}
  <div class="code-backdrop" on:click={close}>
    <div class="code-modal" on:click|stopPropagation role="dialog" aria-modal="true" tabindex="-1" aria-label={title}>
      <div class="code-toolbar">
        <div class="code-title-wrap">
          <span class="code-title">{title}</span>
          {#if path}
            <span class="code-path" title={path}>{path}</span>
          {/if}
        </div>
        <div class="code-actions">
          <button class="code-btn code-btn-ghost" on:click={close} disabled={busy}>{$t('editor.cancel')}</button>
          <button class="code-btn code-btn-primary" on:click={save} disabled={busy}>{$t('editor.save')}</button>
        </div>
      </div>
      <div class="code-container" bind:this={editorContainer}></div>
    </div>
  </div>
{/if}

<style>
  .code-backdrop {
    position: fixed;
    inset: 0;
    background: rgba(0, 0, 0, 0.45);
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 1000;
  }
  .code-modal {
    display: flex;
    flex-direction: column;
    width: min(760px, calc(100vw - 48px));
    height: min(560px, calc(100vh - 96px));
    background: var(--bg-primary);
    border: 0.5px solid var(--border);
    border-radius: 14px;
    overflow: hidden;
    box-shadow: 0 24px 64px rgba(0, 0, 0, 0.28);
  }
  .code-toolbar {
    display: flex;
    justify-content: space-between;
    align-items: center;
    gap: 12px;
    padding: 10px 14px;
    border-bottom: 0.5px solid var(--border);
    background: var(--bg-secondary);
    flex-shrink: 0;
  }
  .code-title-wrap {
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 1px;
  }
  .code-title {
    font: 600 14px/18px var(--font-sans);
    color: var(--text-primary);
  }
  .code-path {
    font: 11px/14px var(--font-mono, monospace);
    color: var(--text-secondary);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    max-width: 420px;
  }
  .code-actions {
    display: flex;
    gap: 8px;
    flex-shrink: 0;
  }
  .code-container {
    flex: 1;
    overflow: auto;
    min-height: 0;
  }
  .code-container :global(.cm-editor) {
    height: 100%;
  }
  .code-btn {
    height: 30px;
    min-width: 68px;
    padding: 0 12px;
    border: 0;
    border-radius: 8px;
    font: 600 12.5px/16px var(--font-sans);
    cursor: pointer;
    color: var(--text-primary);
    display: inline-flex;
    align-items: center;
    justify-content: center;
  }
  .code-btn:disabled {
    opacity: 0.55;
    cursor: default;
  }
  .code-btn-primary {
    background: var(--accent);
    color: #fff;
  }
  .code-btn-primary:hover:not(:disabled) {
    background: color-mix(in srgb, #fff 8%, var(--accent));
  }
  .code-btn-ghost {
    background: var(--bg-card);
    border: 0.5px solid var(--border);
  }
  .code-btn-ghost:hover:not(:disabled) {
    background: var(--bg-hover);
  }
</style>
