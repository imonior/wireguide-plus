<script>
  import { createEventDispatcher, onDestroy, onMount } from 'svelte';
  import { t } from '../i18n/index.js';

  export let conflicts = [];
  // Policy-layer conflicts (routing / DNS / traffic protection) for the
  // tunnel being connected. Unlike interface overlaps these come from the
  // pure analyzers in internal/policy and carry a severity: only a
  // "blocking" one (a true tie, e.g. two tunnels claiming the same prefix)
  // stops the connect by default — the user can still override.
  export let policyReport = null;
  const dispatch = createEventDispatcher();

  // Severity → CSS class, so a tie reads louder than a deterministic
  // containment that longest-prefix match already resolves.
  function severityClass(sev) {
    return sev === 'blocking' ? 'sev-blocking' : sev === 'warning' ? 'sev-warning' : 'sev-info';
  }

  function policyRows(report) {
    if (!report) return [];
    const rows = [];
    for (const c of report.routes || []) {
      rows.push({ domain: $t('policy.domain_route'), severity: c.severity, text: `${c.a} ↔ ${c.b}: ${c.a_prefix}${c.a_prefix !== c.b_prefix ? ' / ' + c.b_prefix : ''}`, kind: c.kind });
    }
    for (const c of report.dns || []) {
      rows.push({ domain: $t('policy.domain_dns'), severity: c.severity, text: `${c.a} ↔ ${c.b}${c.domain ? ': ' + c.domain : ''}`, kind: c.kind });
    }
    for (const c of report.protection || []) {
      rows.push({ domain: $t('policy.domain_protection'), severity: c.severity, text: `${c.a} ↔ ${c.b}: ${c.a_prefix} / ${c.b_prefix}`, kind: c.kind });
    }
    return rows;
  }

  $: rows = policyRows(policyReport);
  $: blocking = rows.some((r) => r.severity === 'blocking');

  // Match the other modals: Escape cancels.
  function onKeyDown(e) {
    if (e.key === 'Escape') {
      e.preventDefault();
      dispatch('cancel');
    }
  }
  onMount(() => window.addEventListener('keydown', onKeyDown));
  onDestroy(() => window.removeEventListener('keydown', onKeyDown));
</script>

<div class="modal-backdrop" on:click={() => dispatch('cancel')}>
  <div class="modal" on:click|stopPropagation
    role="dialog" aria-modal="true" tabindex="-1" aria-labelledby="conflict-title">
    <h3 id="conflict-title">{$t('conflict.title')}</h3>

    <div class="conflict-list">
      {#each conflicts as conflict}
        <div class="conflict-item">
          <div class="conflict-header">
            <span class="iface">{conflict.interface_name}</span>
            <span class="owner">({conflict.owner})</span>
          </div>
          <div class="overlap-list">
            {#each conflict.overlapping_ips.slice(0, 3) as overlap}
              <code class="overlap">{overlap}</code>
            {/each}
            {#if conflict.overlapping_ips.length > 3}
              <span class="overlap-more">{$t('conflict.and_more', { n: conflict.overlapping_ips.length - 3 })}</span>
            {/if}
          </div>
        </div>
      {/each}
    </div>

    {#if rows.length > 0}
      <h4 class="policy-heading">{$t('policy.conflicts_heading')}</h4>
      <div class="policy-list">
        {#each rows as row}
          <div class="policy-item {severityClass(row.severity)}">
            <span class="policy-domain">{row.domain}</span>
            <span class="policy-text">{row.text}</span>
            <span class="policy-kind">{row.kind}</span>
          </div>
        {/each}
      </div>
      <p class="policy-note">
        {blocking ? $t('policy.blocking_note') : $t('policy.info_note')}
      </p>
    {/if}

    <p class="warning-text">{$t('conflict.message')}</p>

    <div class="modal-footer">
      <button class="btn btn-warn" on:click={() => dispatch('proceed')}>
        {$t('conflict.proceed')}
      </button>
      <button class="btn btn-edit" on:click={() => dispatch('edit')}>
        {$t('policy.edit_tunnel')}
      </button>
      <button class="btn btn-cancel" on:click={() => dispatch('cancel')}>
        {$t('conflict.cancel')}
      </button>
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
    z-index: 300;
  }
  .modal {
    background: var(--bg-primary);
    border: 1px solid var(--yellow);
    border-radius: 12px;
    padding: 24px;
    width: 480px;
    box-shadow: var(--shadow-md);
  }
  h3 { color: var(--yellow); margin: 0 0 16px; }
  .conflict-item {
    background: var(--warn-item-bg);
    border: 1px solid var(--border);
    border-radius: 6px;
    padding: 12px;
    margin-bottom: 8px;
  }
  .conflict-header { margin-bottom: 4px; }
  .iface { font-weight: 600; color: var(--text-primary); }
  .owner { color: var(--text-secondary); font-size: 13px; margin-left: 4px; }
  .overlap {
    display: block;
    font-size: 12px;
    color: var(--yellow);
    margin-top: 2px;
  }
  .overlap-more {
    font-size: 12px;
    color: var(--text-muted);
    margin-top: 2px;
    display: block;
  }
  .warning-text {
    font-size: 13px;
    color: var(--text-secondary);
    margin: 12px 0;
  }
  .modal-footer { display: flex; gap: 8px; justify-content: flex-end; }
  .btn {
    padding: 8px 16px;
    border: none;
    border-radius: 6px;
    cursor: pointer;
    font-size: 13px;
  }
  .btn-warn { background: var(--yellow); color: var(--bg-primary); font-weight: 600; }
  .btn-cancel { background: var(--bg-card); color: var(--text-primary); }
  .btn-edit { background: var(--bg-card); color: var(--text-primary); border: 1px solid var(--border); }

  .policy-heading {
    margin: 16px 0 8px;
    font-size: 13px;
    color: var(--text-secondary);
    text-transform: uppercase;
    letter-spacing: 0.04em;
  }
  .policy-list { display: flex; flex-direction: column; gap: 6px; }
  .policy-item {
    display: grid;
    grid-template-columns: minmax(0, 90px) minmax(0, 1fr) auto;
    gap: 8px;
    align-items: center;
    font-size: 12px;
    padding: 8px 10px;
    border-radius: 6px;
    border: 1px solid var(--border);
    background: var(--bg-card);
  }
  .sev-blocking { border-left: 3px solid var(--red, #e05252); }
  .sev-warning { border-left: 3px solid var(--yellow); }
  .sev-info { border-left: 3px solid var(--text-muted); }
  .policy-domain { color: var(--text-secondary); font-weight: 600; }
  .policy-text { color: var(--text-primary); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .policy-kind { color: var(--text-muted); font-family: ui-monospace, SFMono-Regular, Menlo, monospace; }
  .policy-note { font-size: 12px; color: var(--text-secondary); margin: 8px 0 0; }
</style>
