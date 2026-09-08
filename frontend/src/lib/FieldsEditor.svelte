<script>
  // Field-editor view of the tunnel config: one input per key, applied back
  // to the conf TEXT via TunnelService.SetTunnelFields (targeted rewrite —
  // comments and hand-ordered lines survive). AWG keys render only while
  // AmneziaWG support is enabled in Settings.
  //
  // NOTE: the per-tunnel physical-egress binding deliberately does NOT live
  // here — it is a tunnel property, not a conf-field property, so it renders
  // as a top-level panel (EgressBinding.svelte) above the script panel where
  // BOTH the conf and fields tabs can see it.
  import { createEventDispatcher, onMount } from 'svelte';
  import { t } from '../i18n/index.js';
  import { appSettings } from '../stores/settings.js';

  export let content = '';
  export let name = '';
  export let isNew = false;
  export let nameEditable = true;
  export let TunnelService = null; // injected binding namespace
  // Incremented by the parent when the fields tab becomes visible, so the
  // model re-parses the CURRENT conf text (it may have changed in the
  // conf-text view or via the script panel).
  export let reloadKey = 0;

  const dispatch = createEventDispatcher();

  // Canonical field layout. Keys use native .conf spelling; values are raw
  // strings (lists stay comma-joined). An empty value deletes the key.
  const IFACE_STD = ['PrivateKey', 'Address', 'DNS', 'MTU', 'ListenPort', 'Table', 'FwMark'];
  const IFACE_AWG = ['Jc', 'Jmin', 'Jmax', 'S1', 'S2', 'S3', 'S4', 'H1', 'H2', 'H3', 'H4'];
  const PEER_KEYS = ['PublicKey', 'PresharedKey', 'Endpoint', 'AllowedIPs', 'PersistentKeepalive'];

  // Keys wg-quick refuses to start without: the interface needs a private
  // key and at least one address; every peer needs its public key and its
  // routing (AllowedIPs). Everything else is optional tuning.
  const REQUIRED = new Set(['PrivateKey', 'Address', 'PublicKey', 'AllowedIPs']);
  function isRequired(key) { return REQUIRED.has(key); }

  let iface = {};        // canonical-key → value (working model)
  let peers = [];        // [{ key: value }]
  let peerIdx = 0;
  let loadErr = '';

  async function load() {
    if (!TunnelService) return;
    try {
      const f = await TunnelService.GetTunnelFields(content);
      iface = { ...(f?.interface || {}) };
      peers = (f?.peers || []).map(p => ({ ...p }));
      if (peerIdx >= peers.length) peerIdx = 0;
      loadErr = '';
    } catch (e) {
      loadErr = e?.message || String(e);
    }
  }

  onMount(load);
  $: if (reloadKey !== undefined && reloadKey > 0 && typeof content === 'string') {
    load();
  }

  // Apply the working model back onto the conf text. Fired on every input
  // change (blur) so the parent's bind:content is always current by the
  // time Save runs.
  async function apply() {
    if (!TunnelService) return;
    const model = {
      interface: pick(iface, [...IFACE_STD, ...(awgOn ? IFACE_AWG : [])]),
      peers: peers.map((p, i) => pick(p, PEER_KEYS)),
    };
    try {
      const updated = await TunnelService.SetTunnelFields(content, model);
      if (typeof updated === 'string') {
        content = updated;
      }
    } catch (e) {
      loadErr = e?.message || String(e);
    }
  }

  function pick(src, keys) {
    const out = {};
    for (const k of keys) out[k] = (src[k] ?? '').trim();
    return out;
  }

  $: awgOn = $appSettings.enable_awg;
  $: currentPeer = peers[peerIdx] || {};

  function setIface(key, val) { iface = { ...iface, [key]: val }; }
  function setPeer(key, val) {
    peers = peers.map((p, i) => (i === peerIdx ? { ...p, [key]: val } : p));
  }

  function onNameInput(e) { name = e.target.value; }
  function doSave() { dispatch('save', { name, content }); }
  function doCancel() { dispatch('cancel'); }
</script>

<div class="fields-editor">
  <!-- Toolbar mirrors ConfigEditor exactly: name input flexes, actions sit
       right after it, same 32px geometry and ghost/primary colours. -->
  <div class="fe-toolbar">
    {#if nameEditable}
      <input class="fe-name" type="text" value={name} on:input={onNameInput}
        placeholder={$t('editor.name_placeholder')} spellcheck="false" />
    {:else}
      <span class="fe-title">{name}</span>
    {/if}
    <div class="fe-actions">
      <button class="fe-btn fe-btn-ghost" on:click={doCancel}>{$t('editor.cancel')}</button>
      <button class="fe-btn fe-btn-primary" on:click={doSave}>{$t('editor.save')}</button>
    </div>
  </div>

  {#if loadErr}<p class="fe-error">{loadErr}</p>{/if}

  <div class="fe-scroll">
    <div class="fe-grid">
      <div class="fe-group">
        <h5 class="fe-group-title">{$t('fields.section_interface')}</h5>
        {#each IFACE_STD as key}
          <div class="fe-row">
            <label class="fe-label" for="fe-if-{key}">{key}{#if isRequired(key)}<span class="fe-req" title={$t('fields.required_hint')}>*</span>{/if}</label>
            <input class="fe-input" id="fe-if-{key}" type="text" value={iface[key] ?? ''}
              spellcheck="false" on:change={(e) => { setIface(key, e.target.value); apply(); }} />
          </div>
        {/each}
      </div>

      {#if awgOn}
        <div class="fe-group fe-group--awg">
          <h5 class="fe-group-title">{$t('fields.section_awg')}</h5>
          {#each IFACE_AWG as key}
            <div class="fe-row">
              <label class="fe-label" for="fe-awg-{key}">{key}</label>
              <input class="fe-input" id="fe-awg-{key}" type="text" value={iface[key] ?? ''}
                spellcheck="false" on:change={(e) => { setIface(key, e.target.value); apply(); }} />
            </div>
          {/each}
        </div>
      {/if}

      <div class="fe-group fe-group--peer">
        <h5 class="fe-group-title">
          {$t('fields.section_peer')}
          {#if peers.length > 1}
            <select class="fe-peer-select" bind:value={peerIdx}>
              {#each peers as _, i}
                <option value={i}>{$t('fields.peer_n', { n: i + 1 })}</option>
              {/each}
            </select>
          {/if}
        </h5>
        {#each PEER_KEYS as key}
          <div class="fe-row">
            <label class="fe-label" for="fe-peer-{key}">{key}{#if isRequired(key)}<span class="fe-req" title={$t('fields.required_hint')}>*</span>{/if}</label>
            <input class="fe-input" id="fe-peer-{key}" type="text" value={currentPeer[key] ?? ''}
              spellcheck="false" on:change={(e) => { setPeer(key, e.target.value); apply(); }} />
          </div>
        {/each}
      </div>
    </div>
  </div>
</div>

<style>
  .fields-editor {
    display: flex;
    flex-direction: column;
    flex: 1;
    min-height: 0;
    background: var(--bg-primary);
  }
  .fe-toolbar {
    display: flex;
    justify-content: space-between;
    align-items: center;
    gap: 12px;
    padding: 12px 16px;
    border-bottom: 0.5px solid var(--border);
    background: var(--bg-secondary);
    flex-shrink: 0;
  }
  .fe-title {
    flex: 1;
    font: 600 14px/18px var(--font-sans);
    color: var(--text-primary);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .fe-name {
    flex: 1;
    height: 32px;
    padding: 0 12px;
    font: 600 14px/18px var(--font-sans);
    letter-spacing: -0.005em;
    color: var(--text-primary);
    background: var(--bg-card);
    border: 0.5px solid var(--border);
    border-radius: 8px;
    min-width: 0;
    outline: none;
    box-sizing: border-box;
  }
  @media (prefers-reduced-motion: no-preference) {
    .fe-name {
      transition: border-color 140ms ease, box-shadow 140ms ease, background 140ms ease;
    }
  }
  .fe-name:focus {
    border-color: var(--accent);
    box-shadow: 0 0 0 3px color-mix(in srgb, var(--accent) 18%, transparent);
    background: var(--bg-primary);
  }
  .fe-actions {
    display: flex;
    gap: 8px;
    flex-shrink: 0;
  }
  /* Same 32px gradient-primary + ghost geometry as ConfigEditor's toolbar. */
  .fe-btn {
    height: 32px;
    min-width: 72px;
    padding: 0 14px;
    border: 0;
    border-radius: 9px;
    font: 600 13px/18px var(--font-sans);
    letter-spacing: -0.005em;
    cursor: pointer;
    color: var(--text-primary);
    display: inline-flex;
    align-items: center;
    justify-content: center;
  }
  @media (prefers-reduced-motion: no-preference) {
    .fe-btn {
      transition: filter 140ms ease, background-color 140ms ease,
                  border-color 140ms ease, transform 140ms ease, box-shadow 140ms ease;
    }
  }
  .fe-btn-primary {
    background: var(--accent);
    color: #fff;
    box-shadow:
      0 1px 3px color-mix(in srgb, var(--accent) 26%, transparent),
      0 1px 2px rgba(0, 0, 0, 0.08);
  }
  .fe-btn-primary:hover {
    background: color-mix(in srgb, #fff 8%, var(--accent));
    transform: translateY(-1px);
    box-shadow:
      0 4px 10px color-mix(in srgb, var(--accent) 30%, transparent),
      0 1px 2px rgba(0, 0, 0, 0.10);
  }
  .fe-btn-primary:active {
    background: color-mix(in srgb, #000 8%, var(--accent));
    transform: translateY(0);
  }
  .fe-btn-ghost {
    background: var(--bg-card);
    border: 0.5px solid var(--border);
  }
  .fe-btn-ghost:hover {
    background: var(--bg-hover);
    border-color: color-mix(in srgb, var(--accent) 30%, var(--border));
  }
  .fe-btn-ghost:active { background: var(--bg-active); }

  .fe-scroll {
    flex: 1;
    min-height: 0;
    overflow-y: auto;
    padding: 12px 16px 8px;
  }
  /* Single full-width column: every section (interface / AWG / peer) fills
     the editor width the same way — inputs get the full remaining space. */
  .fe-grid { display: grid; grid-template-columns: 1fr; gap: 12px; }
  .fe-group { border: 1px solid var(--border, #ddd); border-radius: 10px; padding: 10px 12px; }
  .fe-group-title { margin: 0 0 8px; font-size: 12px; text-transform: uppercase; letter-spacing: 0.04em; opacity: 0.75; display: flex; align-items: center; gap: 8px; }
  .fe-row { display: flex; align-items: center; gap: 8px; margin-bottom: 6px; }
  .fe-row:last-child { margin-bottom: 0; }
  /* Wide enough for the longest key (PersistentKeepalive) at 12px mono so
     the text never sits under the input's left edge. */
  .fe-label { width: 175px; flex-shrink: 0; font-size: 12px; font-family: ui-monospace, monospace; opacity: 0.85; }
  /* Required-field marker: a small accent-coloured asterisk glued to the
     key name; hovering (or focusing the row) explains it via the title. */
  .fe-req { margin-left: 3px; color: var(--red, #e5484d); font-weight: 700; }
  .fe-input {
    flex: 1; min-width: 0; padding: 5px 8px; font-size: 13px;
    font-family: ui-monospace, monospace;
    border: 1px solid var(--border, #ccc); border-radius: 6px;
    background: var(--bg-input, transparent); color: inherit;
  }
  .fe-peer-select { font-size: 12px; padding: 2px 6px; border-radius: 6px; border: 1px solid var(--border, #ccc); background: transparent; color: inherit; }
  .fe-error { color: #d33; font-size: 12px; margin: 4px 0; }
</style>
