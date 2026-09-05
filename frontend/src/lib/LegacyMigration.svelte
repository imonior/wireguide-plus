<script>
  import { TunnelService } from '../../bindings/github.com/imonior/wireguide-plus/internal/app';
  import { t } from '../i18n/index.js';

  // First-launch modal shown when an older ("wireguide") install left data
  // behind. The user picks what to do with that data:
  //
  //   • open the legacy folder (or the new folder) to inspect the files
  //   • move the data into the new install — when target files already exist
  //     we hand off to the conflict sub-modal first
  //   • cancel for this session only ("Remind me next launch", nothing
  //     persisted — the scan re-runs on next launch)
  //   • cancel permanently ("Don't ask again", DismissLegacyMigration is
  //     persisted so the prompt stays hidden while legacy data remains)
  export let report = null;
  export let onClose = null;
  // onMigrated fires after a successful migration so the caller refreshes the
  // tunnel list / settings.
  export let onMigrated = null;

  let includeLogs = false;
  let migrating = false;
  let opening = false;
  let errorMsg = '';

  // mode flows main → conflict → result. We keep the legacy data report
  // around in the conflict phase so we can present the conflicting file
  // list to the user without re-scanning.
  let mode = 'main';

  // result is set after MigrateLegacyData returns: { migrated, skipped, failed }.
  let result = null;

  async function openFolder(kind) {
    if (opening) return;
    opening = true;
    errorMsg = '';
    try {
      await TunnelService.OpenFolder(kind);
    } catch (e) {
      errorMsg = e?.message || String(e);
    } finally {
      opening = false;
    }
  }

  // "Move" primary action. When target files already exist, hand off to the
  // conflict sub-modal instead of running with overwrite=false (the old
  // behaviour silently skipped conflicting items; the new design surfaces the
  // decision explicitly).
  async function startMove() {
    if (migrating) return;
    errorMsg = '';
    if (report?.conflict_count) {
      mode = 'conflict';
      return;
    }
    await runMigrate(false);
  }

  async function runMigrate(overwrite) {
    migrating = true;
    errorMsg = '';
    try {
      result = await TunnelService.MigrateLegacyData({
        overwrite,
        include_logs: includeLogs,
      });
      mode = 'result';
    } catch (e) {
      errorMsg = e?.message || String(e);
    } finally {
      migrating = false;
    }
  }

  // Conflict sub-modal: "Overwrite and move" — same path as runMigrate(true).
  async function conflictOverwrite() {
    await runMigrate(true);
  }

  // Conflict sub-modal: open the CURRENT folder (where the conflicting target
  // files live) so the user can decide what to keep. Stay on the conflict
  // modal so they can pick an outcome after viewing.
  async function conflictOpenNewFolder() {
    await openFolder('config');
  }

  function finish() {
    if (onMigrated) onMigrated();
  }

  // "Remind me next launch": close without persisting. The next launch
  // re-detects the data and shows the modal again.
  function remindLater() {
    if (onClose) onClose();
  }

  // "Don't ask again": persist the dismissal so the modal never reappears
  // while the legacy data remains.
  async function neverRemind() {
    try {
      await TunnelService.DismissLegacyMigration();
    } catch (_) { /* best-effort: worst case the prompt shows again */ }
    if (onClose) onClose();
  }

  // Items whose target already exists — displayed in the conflict sub-modal.
  // Computed once per render so the list can't drift while the user pokes
  // around in the open-current-folder flow.
  $: conflictItems = (report?.items || []).filter((it) => it.target_exists);
</script>

<div class="modal-backdrop">
  <div class="modal migration-modal" role="dialog" aria-modal="true" tabindex="-1" aria-labelledby="migration-title">
    {#if mode === 'result'}
      <h3 id="migration-title">{$t('migration.result_title')}</h3>
      {#if result?.migrated?.length}
        <p class="summary-ok">{$t('migration.result_migrated', { n: result.migrated.length })}</p>
        <p class="summary-ok cleanup-note">{$t('migration.result_cleaned')}</p>
      {/if}
      {#if result?.skipped?.length}
        <p class="summary-warn">{$t('migration.result_skipped', { n: result.skipped.length })}</p>
        <ul class="item-list">
          {#each result.skipped as name}
            <li>{name}</li>
          {/each}
        </ul>
      {/if}
      {#if result?.failed?.length}
        <p class="summary-error">{$t('migration.result_failed', { n: result.failed.length })}</p>
        <ul class="item-list">
          {#each result.failed as name}
            <li>{name}</li>
          {/each}
        </ul>
      {/if}
      {#if !result?.migrated?.length && !result?.skipped?.length && !result?.failed?.length}
        <p class="summary-ok">{$t('migration.result_nothing')}</p>
      {/if}
      <div class="modal-actions">
        <button class="btn-primary" on:click={finish}>{$t('migration.done')}</button>
      </div>

    {:else if mode === 'conflict'}
      <h3 id="migration-title">{$t('migration.conflict_title')}</h3>
      <p class="modal-sub">{$t('migration.conflict_intro', { n: conflictItems.length })}</p>
      <ul class="item-list conflict-list">
        {#each conflictItems as item}
          <li>{item.name}</li>
        {/each}
      </ul>

      {#if errorMsg}
        <p class="error-text">{errorMsg}</p>
      {/if}

      <div class="modal-actions">
        <button class="btn-secondary" on:click={conflictOpenNewFolder} disabled={migrating || opening}>
          {$t('migration.conflict_open')}
        </button>
        <button class="btn-primary" on:click={conflictOverwrite} disabled={migrating}>
          {migrating ? $t('migration.migrating') : $t('migration.conflict_overwrite')}
        </button>
      </div>
      <p class="dismiss-row">
        <button class="btn-link" on:click={remindLater} disabled={migrating}>{$t('migration.remind_next')}</button>
        <span class="dismiss-sep">·</span>
        <button class="btn-link" on:click={neverRemind} disabled={migrating}>{$t('migration.never')}</button>
      </p>

    {:else}
      <h3 id="migration-title">{$t('migration.title')}</h3>
      <p class="modal-sub">{$t('migration.subtitle')}</p>

      <div class="item-summary">
        {#if report?.config_count}
          <span class="summary-chip">{$t('migration.config_count', { n: report.config_count })}</span>
        {/if}
        {#if report?.tunnel_count}
          <span class="summary-chip">{$t('migration.tunnel_count', { n: report.tunnel_count })}</span>
        {/if}
        {#if report?.log_count}
          <span class="summary-chip">{$t('migration.log_count', { n: report.log_count })}</span>
        {/if}
      </div>

      {#if report?.conflict_count}
        <p class="conflict-note">{$t('migration.conflicts', { n: report.conflict_count })}</p>
      {/if}

      <div class="option-row">
        {#if report?.log_count}
          <label class="check">
            <input type="checkbox" bind:checked={includeLogs} />
            <span>{$t('migration.include_logs')}</span>
          </label>
        {/if}
      </div>

      <div class="compare-row">
        <button class="btn-ghost" on:click={() => openFolder('legacy-config')} disabled={opening}>
          {$t('migration.open_legacy')}
        </button>
        <button class="btn-ghost" on:click={() => openFolder('config')} disabled={opening}>
          {$t('migration.open_current')}
        </button>
        {#if report?.log_count}
          <button class="btn-ghost" on:click={() => openFolder('legacy-logs')} disabled={opening}>
            {$t('migration.open_legacy_logs')}
          </button>
          <button class="btn-ghost" on:click={() => openFolder('logs')} disabled={opening}>
            {$t('migration.open_current_logs')}
          </button>
        {/if}
      </div>

      {#if errorMsg}
        <p class="error-text">{errorMsg}</p>
      {/if}

      <div class="modal-actions">
        <button class="btn-secondary" on:click={remindLater} disabled={migrating}>{$t('migration.remind_next')}</button>
        <button class="btn-primary" on:click={startMove} disabled={migrating}>
          {migrating ? $t('migration.migrating') : $t('migration.move_all')}
        </button>
      </div>
      <p class="dismiss-row">
        <button class="btn-link" on:click={neverRemind} disabled={migrating}>{$t('migration.never')}</button>
      </p>
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
    background: var(--bg-card);
    border: 0.5px solid var(--border);
    border-radius: 10px;
    padding: 20px 24px;
    width: 440px;
    max-width: calc(100vw - 48px);
    box-shadow: var(--shadow-md);
    color: var(--text-primary);
  }
  h3 { margin: 0 0 8px; font: 600 15px/20px var(--font-sans); }
  .modal-sub {
    margin: 0 0 12px;
    font: 400 12px/17px var(--font-sans);
    color: var(--text-secondary);
  }
  .item-summary {
    display: flex;
    flex-wrap: wrap;
    gap: 6px;
    margin-bottom: 10px;
  }
  .summary-chip {
    font: 500 11px/15px var(--font-sans);
    color: var(--accent);
    background: color-mix(in srgb, var(--accent) 10%, transparent);
    border-radius: 999px;
    padding: 3px 10px;
  }
  .conflict-note {
    margin: 0 0 10px;
    font: 400 11px/16px var(--font-sans);
    color: var(--warn-text, #b7791f);
  }
  .option-row {
    display: flex;
    flex-wrap: wrap;
    gap: 14px;
    margin-bottom: 12px;
  }
  .check {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    font: 400 12px/16px var(--font-sans);
    cursor: pointer;
  }
  .check input { accent-color: var(--accent); }
  .compare-row {
    display: flex;
    flex-wrap: wrap;
    gap: 8px;
    margin-bottom: 16px;
  }
  .btn-ghost {
    height: 26px;
    padding: 0 10px;
    background: var(--bg-secondary);
    color: var(--text-secondary);
    border: 0.5px solid var(--border);
    border-radius: 6px;
    font: 400 11px/14px var(--font-sans);
    cursor: pointer;
  }
  .btn-ghost:hover { background: var(--bg-hover); color: var(--text-primary); }
  .btn-ghost:disabled { opacity: 0.5; cursor: not-allowed; }
  .modal-actions {
    display: flex;
    gap: 8px;
    justify-content: flex-end;
  }
  .btn-primary {
    height: 30px;
    padding: 0 16px;
    background: var(--accent);
    color: #fff;
    border: none;
    border-radius: 6px;
    font: 600 12px/16px var(--font-sans);
    cursor: pointer;
  }
  .btn-primary:hover { filter: brightness(1.08); }
  .btn-primary:active { filter: brightness(0.94); }
  .btn-primary:disabled { opacity: 0.5; cursor: wait; }
  .btn-secondary {
    height: 30px;
    padding: 0 16px;
    background: var(--bg-secondary);
    color: var(--text-primary);
    border: 0.5px solid var(--border);
    border-radius: 6px;
    font: 400 12px/16px var(--font-sans);
    cursor: pointer;
  }
  .btn-secondary:hover { background: var(--bg-hover); }
  .btn-secondary:disabled { opacity: 0.5; cursor: not-allowed; }
  .dismiss-row {
    display: flex;
    justify-content: flex-end;
    align-items: center;
    gap: 6px;
    margin: 10px 0 0;
  }
  .dismiss-sep {
    color: var(--text-muted);
    font: 400 11px/16px var(--font-sans);
  }
  .btn-link {
    height: 24px;
    padding: 0 8px;
    color: var(--text-secondary);
    font: 400 11px/16px var(--font-sans);
    text-decoration: underline;
    text-underline-offset: 2px;
    cursor: pointer;
  }
  .btn-link:hover { color: var(--text-primary); }
  .btn-link:disabled { opacity: 0.5; cursor: not-allowed; }
  .error-text {
    margin: 0 0 12px;
    font: 400 11px/16px var(--font-sans);
    color: var(--error-text, #ff3b30);
    white-space: pre-wrap;
  }
  .summary-ok {
    margin: 0 0 6px;
    font: 400 13px/18px var(--font-sans);
    color: var(--text-primary);
  }
  .cleanup-note {
    margin: 0 0 10px;
    font-size: 12px;
    color: var(--text-secondary);
  }
  .summary-warn {
    margin: 0 0 4px;
    font: 400 12px/17px var(--font-sans);
    color: var(--warn-text, #b7791f);
  }
  .summary-error {
    margin: 0 0 4px;
    font: 400 12px/17px var(--font-sans);
    color: var(--error-text, #ff3b30);
  }
  .item-list {
    margin: 0 0 12px;
    padding: 0 0 0 18px;
    max-height: 160px;
    overflow-y: auto;
    font: 400 11px/16px var(--font-mono, ui-monospace, "SF Mono", Menlo, monospace);
    color: var(--text-secondary);
  }
  .conflict-list {
    background: color-mix(in srgb, var(--warn-text, #b7791f) 8%, transparent);
    border: 0.5px solid color-mix(in srgb, var(--warn-text, #b7791f) 30%, transparent);
    border-radius: 6px;
    padding: 8px 8px 8px 26px;
    margin: 0 0 14px;
  }
  @media (prefers-reduced-motion: no-preference) {
    .btn-ghost, .btn-primary, .btn-secondary {
      transition: filter 120ms ease, background-color 120ms ease;
    }
  }
</style>
