<script>
  import { onMount, onDestroy } from 'svelte';
  import { t } from '../i18n/index.js';
  import { Browser } from '@wailsio/runtime';
  import { TunnelService } from '../../bindings/github.com/imonior/wireguide-plus/internal/app';

  let result = null;
  let loading = false;
  let error = '';
  // Browser-level (third-party) check: opens browserleaks.com in the default
  // browser for a real-browser DNS + WebRTC leak test. Complements the local
  // test above, which only inspects system DNS configuration. Any failure to
  // launch the browser surfaces here.
  let browserError = '';

  // --- public-resolver cross-check editor ---
  // A user-editable list of public DNS servers the leak test probes in
  // addition to the system-configured DNS. Persisted in settings
  // (dns_test_public_servers). The effective list is:
  //   user's custom list (non-empty) > network-fetched list (non-empty) >
  //   built-in defaults.
  // Clearing the custom list NEVER disables public probing — it restores the
  // default cross-check set (network-fetched or built-in). The system
  // (local/VPN) DNS block is independent and always shown on top.
  let publicServers = []; // [{id, value}]
  let publicNew = '';
  let publicSaved = false;
  let publicError = ''; // edit / save / reset failures
  let publicFetchError = ''; // "Fetch from network" failures (shown with its own prefix)
  let publicLoading = true;
  let publicRefreshing = false;
  let publicRestoredMsg = false; // transient "restored defaults" hint
  let pubId = 0;
  let saveTimer = null;

  onMount(loadPublicServers);
  onDestroy(() => clearTimeout(saveTimer));

  // Map the backend's raw timeout error (context deadline exceeded etc.) to
  // a friendly hint so the user isn't shown low-level Go networking text.
  function isFetchTimeout(raw) {
    return /context deadline exceeded|Client\.Timeout|i\/o timeout|deadline/i.test(raw || '');
  }

  async function loadPublicServers() {
    publicLoading = true;
    publicError = '';
    publicFetchError = '';
    publicRestoredMsg = false;
    try {
      const list = await TunnelService.GetPublicDNSServers();
      publicServers = (list || []).map((v) => ({ id: ++pubId, value: v }));
    } catch (e) {
      publicError = e?.message || String(e);
    }
    publicLoading = false;
  }

  function scheduleSave() {
    clearTimeout(saveTimer);
    saveTimer = setTimeout(savePublicServers, 500);
  }

  async function savePublicServers() {
    clearTimeout(saveTimer);
    const values = publicServers.map((p) => p.value.trim()).filter(Boolean);
    try {
      await TunnelService.SavePublicDNSServers(values);
      publicSaved = true;
      publicError = '';
      publicFetchError = '';
      if (values.length === 0) {
        // Empty custom list = restore the default cross-check set. Reload so
        // the editor shows the now-effective defaults instead of an empty box.
        publicRestoredMsg = true;
        const list = await TunnelService.GetPublicDNSServers();
        publicServers = (list || []).map((v) => ({ id: ++pubId, value: v }));
        clearTimeout(saveTimer);
      }
    } catch (e) {
      publicSaved = false;
      publicError = e?.message || String(e);
    }
  }

  function addPublicServer() {
    const v = publicNew.trim();
    if (!v) return;
    publicServers = [...publicServers, { id: ++pubId, value: v }];
    publicNew = '';
    scheduleSave();
  }

  function removePublicServer(id) {
    publicServers = publicServers.filter((p) => p.id !== id);
    scheduleSave();
  }

  async function restorePublicDefaults() {
    clearTimeout(saveTimer);
    publicLoading = true;
    publicError = '';
    publicFetchError = '';
    publicRestoredMsg = false;
    try {
      await TunnelService.ResetPublicDNSServers();
      const list = await TunnelService.GetPublicDNSServers();
      publicServers = (list || []).map((v) => ({ id: ++pubId, value: v }));
      publicSaved = true;
    } catch (e) {
      publicError = e?.message || String(e);
    }
    publicLoading = false;
  }

  async function refreshPublicServers() {
    clearTimeout(saveTimer);
    publicRefreshing = true;
    publicError = '';
    publicFetchError = '';
    publicRestoredMsg = false;
    try {
      const res = await TunnelService.RefreshPublicDNSServers();
      if (res?.error) {
        // Fetch failed — the effective list (old cache / built-in) is
        // unchanged; surface the error but keep the editor usable.
        publicFetchError = res.error;
      } else {
        // Show the freshly fetched list and persist it so the cross-check
        // set matches what the user sees; they can still edit afterwards.
        publicServers = ((res && res.fetched) || []).map((v) => ({ id: ++pubId, value: v }));
        await TunnelService.SavePublicDNSServers(publicServers.map((p) => p.value));
        publicSaved = true;
      }
    } catch (e) {
      publicFetchError = e?.message || String(e);
    }
    publicRefreshing = false;
  }

  // Pin physical hardware-interface (local) resolvers to the top of the
  // list, then VPN-tunnel resolvers, then public cross-check resolvers.
  // The backend already returns them in that order; this stable sort is a
  // defensive fallback so the "Local" group always leads.
  $: orderedServers = result
    ? [...result.dns_servers].sort((a, b) =>
        Number(b.is_local) - Number(a.is_local) ||
        Number(b.is_vpn) - Number(a.is_vpn))
    : [];

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

  // Browser-level check: delegate to a third-party online test in the user's
  // default browser. This is intentionally NOT in-app — a real-browser test
  // needs a remote authoritative NS to observe where DNS queries actually
  // land, which the local backend cannot provide without self-hosted
  // infrastructure. browserleaks.com also covers WebRTC leaks, which a local
  // probe cannot.
  async function openBrowserCheck() {
    browserError = '';
    try {
      await Browser.OpenURL('https://browserleaks.com/dns');
    } catch (e) {
      browserError = e?.message || String(e);
    }
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

    <div class="test-actions">
      <button class="btn-run" on:click={runTest} disabled={loading}>
        {loading ? $t('tools.dns_leak_checking') : $t('tools.dns_leak_run')}
      </button>
      <button class="btn-browser" on:click={openBrowserCheck} title={$t('tools.dns_browser_title')}>
        {$t('tools.dns_browser_run')}
      </button>
    </div>
    <p class="browser-hint">{$t('tools.dns_browser_hint')}</p>

    {#if error}
      <div class="error-msg">{error}</div>
    {/if}
    {#if browserError}
      <div class="error-msg">{browserError}</div>
    {/if}

    <div class="public-section">
      <div class="public-header">
        <span class="public-title">{$t('tools.dns_public_section_title')}</span>
        {#if publicSaved && !publicError}
          <span class="public-saved">{$t('tools.dns_public_saved')}</span>
        {/if}
        {#if publicRestoredMsg && !publicError}
          <span class="public-saved">{$t('tools.dns_public_restored')}</span>
        {/if}
      </div>
      <p class="public-desc">{$t('tools.dns_public_section_desc')}</p>

      <div class="public-list">
        {#each publicServers as item (item.id)}
          <div class="public-row">
            <input
              class="public-input"
              type="text"
              bind:value={item.value}
              placeholder="8.8.8.8"
              spellcheck="false"
              on:input={scheduleSave}
              on:change={savePublicServers}
            />
            <button
              class="public-del"
              title={$t('tools.dns_public_remove')}
              aria-label={$t('tools.dns_public_remove')}
              on:click={() => removePublicServer(item.id)}
            >×</button>
          </div>
        {:else}
          {#if !publicLoading && !publicRestoredMsg}
            <div class="public-empty">{$t('tools.dns_public_none')}</div>
          {/if}
        {/each}
      </div>

      <div class="public-add-row">
        <input
          class="public-input"
          type="text"
          bind:value={publicNew}
          placeholder={$t('tools.dns_public_add_placeholder')}
          spellcheck="false"
          on:keydown={(e) => { if (e.key === 'Enter') addPublicServer(); }}
        />
        <button class="public-add" on:click={addPublicServer} disabled={!publicNew.trim()}>
          {$t('tools.dns_public_add')}
        </button>
        <button
          class="public-reset"
          on:click={refreshPublicServers}
          disabled={publicLoading || publicRefreshing}
          title={$t('tools.dns_public_refresh_title')}
        >
          {publicRefreshing ? $t('tools.dns_public_refreshing') : $t('tools.dns_public_refresh')}
        </button>
        <button class="public-reset" on:click={restorePublicDefaults} disabled={publicLoading}>
          {$t('tools.dns_public_reset')}
        </button>
      </div>

      {#if publicError}
        <div class="public-error">{$t('tools.dns_public_save_failed')}: {publicError}</div>
      {/if}
      {#if publicFetchError}
        <div class="public-error">
          {$t('tools.dns_public_fetch_failed')}: {publicFetchError}
          {#if isFetchTimeout(publicFetchError)}
            <span class="public-error-hint">{$t('tools.dns_public_fetch_timeout')}</span>
          {/if}
        </div>
      {/if}
    </div>

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
          {#each orderedServers as server}
            <div class="server" class:vpn={server.status === 'vpn'} class:leak={server.status === 'leak'} class:timeout={server.status === 'timeout'} class:inuse={isInUse(server)}>
              <span class="server-ip">
                {server.ip}
                {#if server.is_local}
                  <span class="server-sys">{$t('tools.dns_local_dns')}</span>
                {/if}
                {#if server.is_vpn}
                  <span class="server-vpn">{$t('tools.dns_vpn_dns')}</span>
                {/if}
                {#if !server.is_local && !server.is_vpn}
                  <span class="server-pub">{$t('tools.dns_public_dns')}</span>
                {/if}
                {#if isInUse(server)}
                  <span class="server-current">{$t('tools.dns_current_dns')}</span>
                {/if}
              </span>
              <span class="server-host">
                {server.hostname || ''}
                {#if server.source_iface}
                  <span class="server-iface">{server.source_iface}</span>
                {/if}
              </span>
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
  .test-actions {
    display: flex;
    gap: var(--space-2);
    align-items: center;
  }
  .btn-browser {
    height: 28px;
    padding: 0 var(--space-3);
    background: transparent;
    border: 0.5px solid var(--border);
    border-radius: var(--radius-sm);
    color: var(--text-secondary);
    font: var(--text-footnote);
    font-weight: 600;
    cursor: pointer;
  }
  .btn-browser:hover { background: var(--bg-hover); color: var(--text-primary); }
  .browser-hint {
    margin: var(--space-2) 0 0;
    font: var(--text-footnote);
    color: var(--text-tertiary);
    line-height: 1.5;
  }

  .public-section {
    margin-top: var(--space-4);
    padding: var(--space-3);
    background: var(--bg-card);
    border: 0.5px solid var(--border);
    border-radius: var(--radius-sm);
  }
  .public-header {
    display: flex;
    align-items: center;
    gap: var(--space-2);
  }
  .public-title {
    font: var(--text-footnote);
    font-weight: 700;
    text-transform: uppercase;
    letter-spacing: 0.06em;
    color: var(--text-primary);
  }
  .public-saved {
    font: var(--text-footnote);
    color: var(--green);
  }
  .public-desc {
    margin: var(--space-1) 0 var(--space-2);
    font: var(--text-footnote);
    color: var(--text-secondary);
    line-height: 1.5;
  }
  .public-list {
    display: flex;
    flex-direction: column;
    gap: var(--space-1);
    max-height: 180px;
    overflow-y: auto;
  }
  .public-row {
    display: flex;
    gap: var(--space-1);
    align-items: center;
  }
  .public-input {
    flex: 1;
    min-width: 0;
    height: 26px;
    padding: 0 var(--space-2);
    background: var(--bg-input, var(--bg-hover));
    border: 0.5px solid var(--border);
    border-radius: var(--radius-xs);
    color: var(--text-primary);
    font: var(--font-mono);
    font-size: 12px;
  }
  .public-input:focus {
    outline: none;
    border-color: var(--accent);
  }
  .public-del {
    width: 26px;
    height: 26px;
    flex-shrink: 0;
    background: transparent;
    border: 0;
    border-radius: var(--radius-xs);
    color: var(--text-tertiary);
    font-size: 15px;
    line-height: 1;
    cursor: pointer;
  }
  .public-del:hover { color: var(--red); background: var(--error-bg); }
  .public-empty {
    font: var(--text-footnote);
    color: var(--text-tertiary);
    padding: var(--space-1) 0;
  }
  .public-add-row {
    display: flex;
    gap: var(--space-1);
    align-items: center;
    margin-top: var(--space-2);
  }
  .public-add {
    height: 26px;
    flex-shrink: 0;
    padding: 0 var(--space-3);
    background: var(--accent);
    border: 0;
    border-radius: var(--radius-xs);
    color: var(--text-inverse);
    font: var(--text-footnote);
    font-weight: 600;
    cursor: pointer;
  }
  .public-add:disabled { opacity: 0.5; cursor: default; }
  .public-reset {
    height: 26px;
    flex-shrink: 0;
    padding: 0 var(--space-3);
    background: transparent;
    border: 0.5px solid var(--border);
    border-radius: var(--radius-xs);
    color: var(--text-secondary);
    font: var(--text-footnote);
    font-weight: 600;
    cursor: pointer;
  }
  .public-reset:hover:not(:disabled) { background: var(--bg-hover); }
  .public-reset:disabled { opacity: 0.5; cursor: default; }
  .public-error {
    margin-top: var(--space-2);
    font: var(--text-footnote);
    color: var(--red);
  }
  .public-error-hint {
    display: block;
    margin-top: 2px;
    color: var(--text-secondary);
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
  .server-vpn {
    display: inline-block;
    margin-left: var(--space-1);
    padding: 0 4px;
    border-radius: var(--radius-xs);
    background: color-mix(in srgb, var(--green) 16%, transparent);
    color: var(--green);
    font: 9px/1.6 var(--font-sans);
    font-weight: 700;
    letter-spacing: 0.04em;
    text-transform: uppercase;
    vertical-align: middle;
  }
  .server-pub {
    display: inline-block;
    margin-left: var(--space-1);
    padding: 0 4px;
    border-radius: var(--radius-xs);
    background: color-mix(in srgb, var(--text-secondary) 12%, transparent);
    color: var(--text-secondary);
    font: 9px/1.6 var(--font-sans);
    font-weight: 700;
    letter-spacing: 0.04em;
    text-transform: uppercase;
    vertical-align: middle;
  }
  .server-iface {
    display: inline-block;
    margin-left: var(--space-1);
    padding: 0 4px;
    border-radius: var(--radius-xs);
    background: var(--bg-hover);
    color: var(--text-tertiary);
    font: 9px/1.6 var(--font-sans);
    font-weight: 600;
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
