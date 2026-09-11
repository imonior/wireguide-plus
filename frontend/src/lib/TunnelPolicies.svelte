<script>
  // TunnelPolicies — the WireGuide Plus private policy panel of the tunnel
  // editor (principles 3-11). A TOP-LEVEL editor panel like EgressBinding,
  // because these are tunnel properties rather than properties of either
  // config view (conf/fields) — they must stay visible from both tabs.
  //
  // None of these values are written into the .conf. They live in the
  // tunnel's .meta.json sidecar (TunnelService.SetTunnelPolicies) so the
  // .conf remains a portable standard WireGuard/AWG document that wg-quick
  // and the official clients can read unchanged (principles 1/2).
  //
  // Naming matters: the switch is "Traffic Protect", never "Kill Switch"
  // (principle 8). It is tunnel-scoped — it protects this tunnel's
  // AllowedIPs from leaking out another egress when this tunnel is down.
  // A full-tunnel config is simply the maximum scope, not a separate
  // product concept (principle 9).
  //
  // Editing is always allowed; the analyzers report conflicts and the
  // Conflict Policy decides what to block at connect time (principle 22).
  import { createEventDispatcher } from 'svelte';
  import { TunnelService } from '../../bindings/github.com/imonior/wireguide-plus/internal/app';
  import { t } from '../i18n/index.js';

  export let name = ''; // SAVED tunnel name — the meta sidecar key
  export let isNew = false;

  const dispatch = createEventDispatcher();

  let trafficProtect = false;
  let domainsText = '';
  let useAsDefaultDNS = false;
  let dnsResolvePath = false;
  let dnsPathEnabled = false; // master switch from Settings
  let conflictWarn = '';
  let loadErr = '';
  let lastLoadedName = '';
  let loadedForNew = false;

  // Existing tunnel: reload whenever the name changes (a rename keeps the
  // sidecar keyed by the ORIGINAL name until the parent saves).
  $: if (!isNew && name && name !== lastLoadedName) {
    lastLoadedName = name;
    load();
  }
  $: if (isNew && !loadedForNew) {
    loadedForNew = true;
    load();
  }

  async function load() {
    if (!TunnelService) return;
    try {
      if (isNew) {
        trafficProtect = false;
        domainsText = '';
        useAsDefaultDNS = false;
        dnsResolvePath = false;
        await loadMaster();
        return;
      }
      const p = await TunnelService.GetTunnelPolicies(name);
      trafficProtect = !!p?.traffic_protect;
      domainsText = (p?.domains || []).join(', ');
      useAsDefaultDNS = !!p?.use_as_default_dns;
      dnsResolvePath = !!p?.dns_resolve_path;
      await loadMaster();
      loadErr = '';
    } catch (e) {
      loadErr = e?.message || String(e);
    }
  }

  // New tunnels have no sidecar yet, so the parent persists these together
  // with the config (same contract as EgressBinding's pendingBinding).
  function current() {
    return {
      traffic_protect: trafficProtect,
      domains: domainsText
        .split(',')
        .map((d) => d.trim())
        .filter(Boolean),
      use_as_default_dns: useAsDefaultDNS,
      dns_resolve_path: dnsResolvePath,
    };
  }

  // The master switch gates the feature: with it off the per-tunnel switch
  // is not shown at all (and the stored value is inert).
  async function loadMaster() {
    try {
      const s = await TunnelService.GetSettings();
      dnsPathEnabled = !!s?.dns_resolve_path;
    } catch {
      dnsPathEnabled = false;
    }
  }

  // Saving is always allowed (principle 22) — a second tunnel claiming the
  // DNS resolve path is a legal configuration, the user may run them at
  // different times. What saving does do is say so: the conflict only bites
  // at connect time, when the other tunnel is actually up.
  async function warnOnConflict() {
    conflictWarn = '';
    if (!dnsResolvePath || isNew || !name) return;
    try {
      const report = await TunnelService.PolicyReport(name);
      const hit = (report?.dns || []).find(
        (c) => c.kind === 'dns_resolve_path_multiple' ||
               c.kind === 'dns_resolve_path_vs_default_dns'
      );
      if (hit) conflictWarn = $t('policy.dns_resolve_path_conflict_saved');
    } catch {
      /* a failed report must never block the save */
    }
  }

  async function persist() {
    if (!TunnelService) return;
    if (isNew || !name) {
      dispatch('policychange', current());
      return;
    }
    try {
      await TunnelService.SetTunnelPolicies(name, current());
      await warnOnConflict();
    } catch (e) {
      loadErr = e?.message || String(e);
    }
  }
</script>

<div class="policies">
  <div class="policies-head">
    <span class="brand">WireGuide Plus</span>
    <span class="hint">{$t('policy.traffic_protect')}</span>
  </div>

  <label class="row">
    <input type="checkbox" bind:checked={trafficProtect} on:change={persist} />
    <span class="label-text">{$t('policy.traffic_protect')}</span>
  </label>
  <p class="desc">{$t('policy.traffic_protect_hint')}</p>

  <div class="field">
    <span class="label-text">{$t('policy.domains_through_tunnel')}</span>
    <input
      type="text"
      class="text-input"
      placeholder={$t('policy.domains_placeholder')}
      bind:value={domainsText}
      on:change={persist}
    />
    <p class="desc">{$t('policy.domains_through_tunnel_hint')}</p>
  </div>

  <label class="row">
    <input type="checkbox" bind:checked={useAsDefaultDNS} on:change={persist} />
    <span class="label-text">{$t('policy.use_as_default_dns')}</span>
  </label>
  <p class="desc">{$t('policy.use_as_default_dns_hint')}</p>

  {#if dnsPathEnabled}
    <label class="row">
      <input type="checkbox" bind:checked={dnsResolvePath} on:change={persist} />
      <span class="label-text">{$t('policy.dns_resolve_path')}</span>
    </label>
    <p class="desc">{$t('policy.dns_resolve_path_hint')}</p>
    {#if conflictWarn}
      <p class="warn">{conflictWarn}</p>
    {/if}
  {/if}

  {#if loadErr}
    <p class="err">{loadErr}</p>
  {/if}
</div>

<style>
  .policies {
    border: 1px solid var(--border);
    border-radius: 8px;
    padding: 10px 14px;
    margin-top: 6px;
    background: var(--bg-card);
  }
  .policies-head {
    display: flex;
    align-items: baseline;
    gap: 8px;
    margin-bottom: 10px;
  }
  .brand {
    font-size: 12px;
    font-weight: 600;
    color: var(--text-secondary);
    text-transform: uppercase;
    letter-spacing: 0.04em;
  }
  .hint { font-size: 12px; color: var(--text-muted); }
  .row {
    display: flex;
    align-items: center;
    gap: 8px;
    cursor: pointer;
  }
  .label-text { font-size: 13px; color: var(--text-primary); }
  .desc {
    font-size: 12px;
    color: var(--text-muted);
    margin: 4px 0 10px;
    line-height: 1.45;
  }
  .field { margin-top: 6px; }
  .text-input {
    width: 100%;
    margin-top: 4px;
    padding: 6px 8px;
    font-size: 13px;
    border: 1px solid var(--border);
    border-radius: 6px;
    background: var(--bg-primary);
    color: var(--text-primary);
  }
  .err { font-size: 12px; color: var(--red, #e05252); margin: 6px 0 0; }
  .warn {
    font-size: 12px;
    color: var(--orange, #d98b2b);
    margin: -4px 0 10px;
    line-height: 1.45;
  }
</style>
