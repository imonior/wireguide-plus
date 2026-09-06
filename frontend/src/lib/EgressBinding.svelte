<script>
  // EgressBinding — per-tunnel physical-egress selector. A TOP-LEVEL editor
  // panel (rendered above the Pre/Post script panel, outside the conf/fields
  // tabs) because binding is a tunnel property, not a property of either
  // config view: the user must see and change it no matter which tab is open.
  //
  // The binding is NOT part of the .conf text — it lives in the tunnel's
  // .meta.json sidecar (TunnelService.SetTunnelBinding) and the helper
  // injects it into the runtime config at connect time:
  //   Windows → IP_UNICAST_IF socket pinning (BindIfIndex)
  //   Linux   → `ip route add <endpoint> dev <iface>` (BindIfName)
  //   macOS   → `route add ... -ifscope <iface>`     (BindIfName)
  // Changes take effect on the NEXT connect/reconnect of this tunnel.
  import { TunnelService } from '../../bindings/github.com/imonior/wireguide-plus/internal/app';
  import { appSettings } from '../stores/settings.js';
  import { t } from '../i18n/index.js';

  export let name = ''; // SAVED tunnel name — the meta sidecar key
  export let isNew = false;

  let ifaces = [];
  let bindIfIndex = 0;
  let bindIfName = '';
  let bindErr = '';
  let lastLoadedName = '';

  // Gated by the pre-existing pin_interface master switch; a brand-new
  // tunnel has no meta sidecar to persist a binding into yet.
  $: available = !!$appSettings.pin_interface && !isNew && !!name;

  // Reload whenever the panel becomes available or the tunnel name changes
  // (edit → rename → save keeps the binding keyed by the ORIGINAL name).
  $: if (available && name !== lastLoadedName) {
    lastLoadedName = name;
    load();
  }

  async function load() {
    if (!TunnelService) return;
    try {
      ifaces = (await TunnelService.ListPhysicalInterfaces()) || [];
      const b = await TunnelService.GetTunnelBinding(name);
      bindIfIndex = b?.bind_if_index || 0;
      bindIfName = b?.bind_if_name || '';
      bindErr = '';
    } catch (e) {
      bindErr = e?.message || String(e);
    }
  }

  function ifcLabel(ifc) {
    const generic = ifc.friendly || ifc.name;
    const hw = ifc.hardware && ifc.hardware !== generic ? ` — ${ifc.hardware}` : '';
    const idx = ifc.index > 0 ? ` (#${ifc.index})` : '';
    const state = !ifc.is_up ? ` · ${$t('fields.bind_down')}` : '';
    return `${generic}${hw}${idx}${state}`;
  }

  async function onBindChange() {
    const idx = Number(bindIfIndex) || 0;
    bindIfIndex = idx;
    const sel = ifaces.find((i) => i.index === idx);
    bindIfName = sel?.name || '';
    bindErr = '';
    try {
      await TunnelService.SetTunnelBinding(name, idx, bindIfName);
    } catch (err) {
      bindErr = err?.message || String(err);
    }
  }
</script>

{#if available}
  <div class="egress-panel">
    <div class="egress-head">
      <span class="egress-title">{$t('fields.section_bind')}</span>
      <p class="egress-hint">{$t('fields.bind_hint')}</p>
    </div>
    <div class="egress-row">
      <label class="egress-label" for="egress-select">{$t('fields.bind_egress')}</label>
      <select id="egress-select" class="egress-select" bind:value={bindIfIndex} on:change={onBindChange}>
        <option value={0}>{$t('fields.bind_auto')}</option>
        {#each ifaces as ifc (ifc.index)}
          <option value={ifc.index}>{ifcLabel(ifc)}</option>
        {/each}
      </select>
    </div>
    {#if bindErr}<p class="egress-error">{bindErr}</p>{/if}
  </div>
{/if}

<style>
  .egress-panel {
    display: flex;
    flex-direction: column;
    gap: 6px;
    padding: 10px 16px;
    border-top: 0.5px solid var(--border);
    background: var(--bg-secondary);
    flex-shrink: 0;
  }
  .egress-title {
    font: 600 12px/16px var(--font-sans);
    text-transform: uppercase;
    letter-spacing: 0.04em;
    color: var(--text-secondary);
  }
  .egress-hint {
    margin: 0;
    font: 11.5px/15px var(--font-sans);
    color: var(--text-secondary);
  }
  .egress-row {
    display: flex;
    align-items: center;
    gap: 10px;
  }
  .egress-label {
    font: 500 12.5px/16px var(--font-sans);
    color: var(--text-primary);
    flex-shrink: 0;
  }
  .egress-select {
    flex: 1;
    min-width: 0;
    height: 30px;
    padding: 0 8px;
    font: 500 12.5px/16px var(--font-sans);
    color: var(--text-primary);
    background: var(--bg-card);
    border: 0.5px solid var(--border);
    border-radius: 8px;
    outline: none;
  }
  .egress-select:focus {
    border-color: var(--accent);
    box-shadow: 0 0 0 3px color-mix(in srgb, var(--accent) 18%, transparent);
  }
  .egress-error {
    margin: 0;
    font: 11.5px/15px var(--font-sans);
    color: var(--error-text);
  }
</style>
