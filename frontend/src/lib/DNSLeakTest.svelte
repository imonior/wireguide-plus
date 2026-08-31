<script>
  import { t } from '../i18n/index.js';
  import { TunnelService } from '../../bindings/github.com/imonior/wireguide-plus/internal/app';

  let result = null;
  let loading = false;
  let error = '';

  async function runTest() {
    loading = true;
    error = '';
    result = null;
    try {
      result = await TunnelService.RunDNSLeakTest();
    } catch (e) {
      error = e?.message || String(e);
    }
    loading = false;
  }

  function statusLabel(server) {
    switch (server.status) {
      case 'vpn':
        return $t('tools.dns_status_vpn');
      case 'leak':
        return $t('tools.dns_status_leak');
      case 'ok':
        return $t('tools.dns_status_ok');
      default:
        return $t('tools.dns_status_timeout');
    }
  }

  // Transport fingerprint from the backend probe: which encrypted channel
  // (if any) this resolver offers besides/besides cleartext UDP 53.
  function encryptionLabel(enc) {
    switch (enc) {
      case 'plain':
        return $t('tools.dns_enc_plain');
      case 'dot':
        return $t('tools.dns_enc_dot');
      case 'doh':
        return $t('tools.dns_enc_doh');
      case 'plain+dot':
        return $t('tools.dns_enc_plain_dot');
      case 'plain+doh':
        return $t('tools.dns_enc_plain_doh');
      default:
        return $t('tools.dns_enc_unknown');
    }
  }

  // "In use" = the resolver actually answered the probe query, i.e. it is
  // the DNS server traffic really went out through — the leak signal.
  function isInUse(server) {
    return server.status === 'leak' || (server.status === 'vpn' && server.responds);
  }
</script>

<div class="dns-test">
  <div class="page-toolbar">
    <h2 class="page-title">{$t('tools.dns_leak_title')}</h2>
  </div>

  <div class="page-body">
    <p class="page-description">{$t('tools.dns_leak_desc')}</p>

    <button class="btn-run" on:click={runTest} disabled={loading}>
      {loading ? $t('tools.dns_leak_checking') : $t('tools.dns_leak_run')}
    </button>

    {#if error}
      <div class="error-msg">{error}</div>
    {/if}

    {#if result}
      <div class="result" class:leaked={result.leaked} class:safe={!result.leaked}>
        <div class="status-icon">{result.leaked ? '⚠' : '✓'}</div>
        <div class="status-text">
          {result.leaked ? $t('tools.dns_leak_leaked') : $t('tools.dns_leak_safe')}
        </div>
      </div>

      <div class="server-section">
        <div class="section-label">{$t('tools.dns_servers_detected')}</div>
        <div class="server-list">
          {#each result.dns_servers || [] as server}
            <div class="server" class:vpn={server.status === 'vpn'} class:leak={server.status === 'leak'} class:timeout={server.status === 'timeout'} class:inuse={isInUse(server)}>
              <span class="server-ip">
                {server.ip}
                {#if server.is_system}
                  <span class="server-sys">{$t('tools.dns_system_dns')}</span>
                {/if}
                {#if isInUse(server)}
                  <span class="server-current">{$t('tools.dns_current_dns')}</span>
                {/if}
              </span>
              <span class="server-host">{server.hostname || ''}</span>
              <span class="server-enc">
                <span class="enc-chip enc-{server.encryption || 'none'}">{encryptionLabel(server.encryption)}</span>
              </span>
              <span class="server-latency">{server.latency_ms > 0 ? `${server.latency_ms}ms` : '—'}</span>
              <span class="server-badge">{statusLabel(server)}</span>
            </div>
          {/each}
        </div>
        <p class="server-note">{$t('tools.dns_server_note')}</p>
      </div>

      <div class="dns-explainer">
        <h4 class="explainer-title">{$t('tools.dns_how_to_read')}</h4>
        {#if result.leaked}
          <p>{$t('tools.dns_explain_leaked')}</p>
        {:else}
          <p>{$t('tools.dns_explain_safe')}</p>
        {/if}
        <h4 class="explainer-title">{$t('tools.dns_prevent_leaks_title')}</h4>
        <ul class="explainer-list">
          <li>{$t('tools.dns_prevent_1')}</li>
          <li>{$t('tools.dns_prevent_2')}</li>
          <li>{$t('tools.dns_prevent_3')}</li>
          <li>{$t('tools.dns_prevent_4')}</li>
        </ul>
      </div>
    {/if}
  </div>
</div>

<style>
  .dns-test {
    display: flex;
    flex-direction: column;
    flex: 1;
    min-height: 0;
  }

  /* Toolbar — matches History/LogViewer pattern: 0.5px bottom rule,
   * text-headline title, small action buttons on the right. */
  .page-toolbar {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding: var(--space-2) var(--space-4);
    border-bottom: 0.5px solid var(--border);
    gap: var(--space-2);
    flex-shrink: 0;
  }
  .page-title {
    margin: 0;
    font: var(--text-headline);
    color: var(--text-primary);
  }
  .page-body {
    flex: 1;
    min-height: 0;
    overflow-y: auto;
    padding: var(--space-4) var(--space-4) var(--space-5);
  }
  .page-description {
    margin: 0 0 var(--space-3);
    font: var(--text-body);
    color: var(--text-secondary);
    line-height: 1.5;
  }
  .btn-run {
    height: 28px;
    padding: 0 var(--space-4);
    background: var(--accent);
    border: 0;
    border-radius: var(--radius-sm);
    color: var(--text-inverse);
    cursor: pointer;
    font: var(--text-headline);
  }
  .btn-run:hover:not(:disabled) { filter: brightness(1.08); }
  .btn-run:active:not(:disabled) { filter: brightness(0.94); }
  .btn-run:disabled { opacity: 0.6; cursor: progress; }
  @media (prefers-reduced-motion: no-preference) {
    .btn-run { transition: filter var(--dur-fast, 140ms) var(--ease-out, ease); }
  }

  .result {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    padding: var(--space-3);
    border-radius: var(--radius-md);
    margin: var(--space-3) 0;
  }
  .result.safe { background: var(--green-tint); border: 0.5px solid color-mix(in srgb, var(--green) 35%, transparent); }
  .result.leaked { background: var(--error-bg); border: 0.5px solid color-mix(in srgb, var(--red) 35%, transparent); }
  .status-icon { font-size: 18px; line-height: 1; }
  .safe .status-text { color: var(--green); font: var(--text-headline); }
  .leaked .status-text { color: var(--red); font: var(--text-headline); }

  .server-section {
    margin-top: var(--space-4);
  }
  .section-label {
    font: var(--text-footnote);
    text-transform: uppercase;
    letter-spacing: 0.06em;
    color: var(--text-secondary);
    margin: 0 var(--space-1) var(--space-2);
  }
  /* No outer border on the list — each .server card already has its own
   * border, so an outer wrapper would create a visible double rule. */
  .server-list {
    display: flex;
    flex-direction: column;
    gap: var(--space-1);
    max-height: 300px;
    overflow-y: auto;
  }
  .server {
    display: flex;
    gap: var(--space-2);
    align-items: center;
    padding: var(--space-2) var(--space-3);
    background: var(--bg-card);
    border: 0.5px solid var(--border);
    border-radius: var(--radius-sm);
    font: var(--text-body);
  }
  .server-ip { font-family: var(--font-mono); }
  .server-host { color: var(--text-secondary); flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .server-latency {
    font-family: var(--font-mono);
    font: var(--text-footnote);
    color: var(--text-secondary);
    white-space: nowrap;
  }
  .server-badge {
    padding: 1px var(--space-2);
    border-radius: var(--radius-xs);
    font: var(--text-footnote);
    font-weight: 600;
    white-space: nowrap;
  }
  .vpn .server-badge { background: var(--green); color: var(--text-inverse); }
  .leak .server-badge { background: var(--red); color: var(--text-inverse); }
  .timeout .server-badge { background: var(--text-tertiary); color: var(--text-inverse); }
  .timeout .server-latency { color: var(--text-tertiary); }
  .error-msg {
    margin-top: var(--space-3);
    padding: var(--space-2) var(--space-3);
    background: var(--error-bg);
    border: 0.5px solid color-mix(in srgb, var(--red) 35%, transparent);
    border-radius: var(--radius-sm);
    color: var(--error-text);
    font: var(--text-body);
  }
  .server-sys {
    display: inline-block;
    margin-left: var(--space-1);
    padding: 0 4px;
    border-radius: var(--radius-xs);
    background: color-mix(in srgb, var(--blue) 14%, transparent);
    color: var(--blue);
    font: 9px/1.6 var(--font-sans);
    font-weight: 700;
    letter-spacing: 0.04em;
    text-transform: uppercase;
    vertical-align: middle;
  }
  .server-current {
    display: inline-block;
    margin-left: var(--space-1);
    padding: 0 4px;
    border-radius: var(--radius-xs);
    background: color-mix(in srgb, var(--accent) 18%, transparent);
    color: var(--accent);
    font: 9px/1.6 var(--font-sans);
    font-weight: 700;
    letter-spacing: 0.04em;
    text-transform: uppercase;
    vertical-align: middle;
  }
  .server-enc { flex-shrink: 0; }
  .enc-chip {
    display: inline-block;
    padding: 1px var(--space-2);
    border-radius: var(--radius-xs);
    font: 10px/1.6 var(--font-sans);
    font-weight: 600;
    letter-spacing: 0.02em;
    white-space: nowrap;
  }
  .enc-plain, .enc-plain\+dot, .enc-plain\+doh { background: color-mix(in srgb, var(--yellow) 16%, transparent); color: var(--yellow); }
  .enc-dot, .enc-doh { background: color-mix(in srgb, var(--green) 14%, transparent); color: var(--green); }
  .enc-none { background: var(--bg-hover); color: var(--text-tertiary); }
  .server-note {
    margin: var(--space-2) var(--space-1) 0;
    font: var(--text-footnote);
    color: var(--text-tertiary);
    line-height: 1.5;
  }
  .dns-explainer {
    margin-top: var(--space-4);
    padding: var(--space-3);
    background: var(--bg-card);
    border: 0.5px solid var(--border);
    border-radius: var(--radius-sm);
    font: var(--text-body);
    color: var(--text-secondary);
    line-height: 1.6;
  }
  .explainer-title {
    margin: 0 0 var(--space-1);
    font: var(--text-footnote);
    font-weight: 700;
    text-transform: uppercase;
    letter-spacing: 0.06em;
    color: var(--text-primary);
  }
  .explainer-title:not(:first-child) { margin-top: var(--space-3); }
  .explainer-list {
    margin: var(--space-1) 0 0;
    padding-left: var(--space-4);
    display: flex;
    flex-direction: column;
    gap: 4px;
  }
</style>
