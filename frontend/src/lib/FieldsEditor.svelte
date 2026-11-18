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
  import Icon from './Icon.svelte';
  import { generatePrivateKey } from './wireguard-key.js';

  export let content = '';
  export let name = '';
  export let isNew = false;
  export let nameEditable = true;
  export let TunnelService = null; // injected binding namespace
  // Incremented by the parent when the fields tab becomes visible, so the
  // model re-parses the CURRENT conf text (it may have changed in the
  // conf-text view or via the script panel).
  export let reloadKey = 0;
  // Validation / save errors reported by the parent's doSave (name
  // required, invalid characters, backend ValidateConfig / ImportConfig /
  // UpdateConfig failures). Without this the Fields tab silently swallowed
  // every failure — the modal just stayed open with no feedback. Mirrors
  // ConfigEditor's `errors` prop.
  export let errors = [];

  const dispatch = createEventDispatcher();

  // Canonical field layout. Keys use native .conf spelling; values are raw
  // strings (lists stay comma-joined). An empty value deletes the key.
  const IFACE_STD = ['PrivateKey', 'Address', 'DNS', 'MTU', 'ListenPort', 'Table', 'FwMark'];
  const IFACE_AWG = ['Jc', 'Jmin', 'Jmax', 'S1', 'S2', 'S3', 'S4', 'H1', 'H2', 'H3', 'H4'];
  const PEER_KEYS = ['PublicKey', 'PresharedKey', 'Endpoint', 'AllowedIPs', 'PersistentKeepalive'];

  // Keys wg-quick refuses to start without: the interface needs a private
  // key and at least one address; every peer needs its public key, the
  // peer's Endpoint (without it the tunnel can never initiate a
  // handshake — it would only ever answer an inbound one, which is not
  // what a peer entry is for) and its routing (AllowedIPs). Everything
  // else is optional tuning.
  const REQUIRED = new Set(['PrivateKey', 'Address', 'PublicKey', 'Endpoint', 'AllowedIPs']);
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
      // Derive once for the freshly loaded private key (imported/reloaded
      // configs must show the key without waiting for a keystroke).
      // 为新载入的私钥派生一次（导入/重载的配置无需再敲一次键即可显示公钥）。
      deriveFrom(iface['PrivateKey'] || '');
    } catch (e) {
      loadErr = e?.message || String(e);
    }
  }

  onMount(load);

  // Re-parse when the PARENT asks for it (the fields tab becoming visible,
  // or the conf text having been edited elsewhere), and not because `content`
  // changed. This component rewrites `content` itself on every flush, so
  // depending on it would make each keystroke reload the model underneath
  // the user — the value they just typed gets replaced mid-edit.
  let seenReloadKey = 0;
  $: if (reloadKey !== undefined && reloadKey !== seenReloadKey) {
    seenReloadKey = reloadKey;
    load();
  }

  // Serialize every rewrite. Two in flight at once race each other: RPCs are
  // not ordered by issue time, so a slow first call can land after a fast
  // second one and quietly restore the older body.
  let applyChain = Promise.resolve();

  // Apply the working model back onto the conf text. Returns whether the
  // rewrite succeeded, because the caller decides what a failure means: a
  // blur can shrug and keep going, a Save must not.
  function apply() {
    applyChain = applyChain.then(applyOnce, applyOnce);
    return applyChain;
  }

  async function applyOnce() {
    if (!TunnelService) return true;
    const model = {
      interface: pick(iface, [...IFACE_STD, ...(awgOn ? IFACE_AWG : [])]),
      peers: peers.map((p, i) => pick(p, PEER_KEYS)),
    };
    try {
      const updated = await TunnelService.SetTunnelFields(content, model);
      if (typeof updated === 'string') {
        content = updated;
      }
      return true;
    } catch (e) {
      loadErr = e?.message || String(e);
      return false;
    }
  }

  function pick(src, keys) {
    const out = {};
    for (const k of keys) out[k] = (src[k] ?? '').trim();
    return out;
  }

  $: awgOn = $appSettings.enable_awg;
  $: currentPeer = peers[peerIdx] || {};

  // Every row writes to the model on `input` (keystroke) and flushes to the
  // conf text on `change` (blur). Keeping the two apart matters: a stale
  // model loses what the user typed, and relying on `change` alone meant an
  // edit whose blur never reached the input was silently dropped — the
  // tunnel then "saved" with the previous value, which looks exactly like a
  // broken Save button.
  function setIface(key, val) { iface = { ...iface, [key]: val }; }
  function setPeer(key, val) {
    peers = peers.map((p, i) => (i === peerIdx ? { ...p, [key]: val } : p));
  }

  function onNameInput(e) { name = e.target.value; }

  // The public key is derived from the PRIVATE KEY TEXT AS TYPED, not from
  // the already-written conf: the field editor only flushes to `content` on
  // blur (on:change), so reading the model here would leave the display one
  // keystroke behind. Wails binds TunnelService.DerivePublicKey; each edit
  // bumps a generation counter so a slow earlier reply cannot overwrite a
  // newer result. `derivedPublicKey` is a plain variable ASSIGNED from the
  // async reply — the template reads it directly, which keeps it reactive.
  // Never put it in `iface`: pick()/apply() would then write it into the conf.
  // 公钥从「正在输入的私钥文本」派生，而非已写回的 conf：字段编辑器只在失焦
  // 时（on:change）把模型刷回 content，若在这里读模型，显示会慢一个按键。
  // Wails 已绑定 TunnelService.DerivePublicKey；每次输入递增代际计数，避免
  // 较慢的旧回复覆盖较新的结果。derivedPublicKey 是由异步回复「赋值」的普通
  // 变量——模板直接读它即可保持响应式。切勿放进 iface，否则 pick()/apply()
  // 会把它写进 .conf。
  let derivedPublicKey = '';
  let derivedState = 'idle'; // idle | ok | invalid
  let deriveGen = 0;
  let copied = false;
  let copyTimer = null;

  async function deriveFrom(value) {
    const raw = (value || '').trim();
    const gen = ++deriveGen;
    if (!raw) {
      derivedPublicKey = '';
      derivedState = 'idle';
      return;
    }
    if (!TunnelService || typeof TunnelService.DerivePublicKey !== 'function') return;
    try {
      const key = await TunnelService.DerivePublicKey(raw);
      if (gen !== deriveGen) return; // a newer keystroke already superseded this
      derivedPublicKey = key || '';
      derivedState = derivedPublicKey ? 'ok' : 'invalid';
    } catch (_) {
      if (gen !== deriveGen) return;
      derivedPublicKey = '';
      derivedState = 'invalid';
    }
  }

  // Regenerate the interface private key in place. The value is written to
  // the working model and flushed to the conf text immediately (unlike a
  // keystroke, which waits for blur), then the read-only public key is
  // re-derived from the SAME new key. PublicKey stays out of `iface` so it
  // can never be persisted into [Interface].
  // 原地重新生成接口私钥：写入工作模型并立即刷回 conf 文本（按键输入要等
  // 失焦才写，这里不等），再用同一个新私钥重新派生只读公钥。PublicKey 始终
  // 不进 iface，因此绝不会被写进 [Interface]。
  function regeneratePrivateKey() {
    const key = generatePrivateKey();
    setIface('PrivateKey', key);
    deriveFrom(key);
    apply();
  }

  async function copyPublicKey() {
    if (!derivedPublicKey) return;
    try {
      await navigator.clipboard.writeText(derivedPublicKey);
      copied = true;
      if (copyTimer) clearTimeout(copyTimer);
      copyTimer = setTimeout(() => { copied = false; }, 1500);
    } catch (e) {
      console.error('[fields] copy public key failed:', e);
    }
  }

  // Save has to flush before it dispatches. The last keystroke's own
  // blur-apply is an async RPC that may still be in flight by the time the
  // user reaches for the button, and `content` is only rewritten when it
  // resolves — dispatching immediately hands the parent the body as it was
  // BEFORE those edits, so the tunnel saves and nothing changes.
  //
  // Every exit path leaves a visible message: a save that reports nothing
  // reads as "the button is broken", which is indistinguishable from a
  // genuine backend refusal.
  const SAVE_TIMEOUT_MS = 12000;
  let saving = false;
  async function doSave() {
    if (saving) return;
    saving = true;
    // Breadcrumb: the flush below is the one step the parent cannot see;
    // if the save dies here the log shows the click with no follow-up.
    try { TunnelService.LogFrontend('fields editor save click name="' + name + '"'); } catch (_) {}
    loadErr = '';
    let timedOut = false;
    try {
      const guard = new Promise((resolve) => setTimeout(() => {
        timedOut = true;
        resolve(false);
      }, SAVE_TIMEOUT_MS));
      const ok = await Promise.race([
        apply().catch((e) => {
          loadErr = e?.message || String(e) || $t('editor.save_failed');
          return false;
        }),
        guard,
      ]);
      if (!ok) {
        // A hung RPC must not poison the chain for every later save either —
        // drop the queue so the next attempt starts from a clean promise.
        if (timedOut) applyChain = Promise.resolve();
        if (!loadErr) loadErr = timedOut ? $t('editor.save_timeout') : $t('editor.save_failed');
        return;
      }
      dispatch('save', { name, content });
    } catch (e) {
      loadErr = e?.message || String(e) || $t('editor.save_failed');
    } finally {
      saving = false;
    }
  }
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
      <span class="fe-title" title={name}>{name}</span>
    {/if}
    <div class="fe-actions">
      <button class="fe-btn fe-btn-ghost" on:click={doCancel} disabled={saving}>{$t('editor.cancel')}</button>
      <button class="fe-btn fe-btn-primary" on:click={doSave} disabled={saving}>
        {saving ? $t('editor.saving') : $t('editor.save')}
      </button>
    </div>
  </div>

  {#if loadErr}<p class="fe-error">{loadErr}</p>{/if}

  {#if errors.length > 0}
    <div class="editor-errors">
      {#each errors as err}
        <p>{err}</p>
      {/each}
    </div>
  {/if}

  <div class="fe-scroll">
    <div class="fe-grid">
      <div class="fe-group">
        <h5 class="fe-group-title">{$t('fields.section_interface')}</h5>
        {#each IFACE_STD as key}
          <div class="fe-row">
            <label class="fe-label" for="fe-if-{key}">{key}{#if isRequired(key)}<span class="fe-req" title={$t('fields.required_hint')}>*</span>{/if}</label>
            <input class="fe-input" id="fe-if-{key}" type="text" value={iface[key] ?? ''}
              spellcheck="false"
              on:input={(e) => { if (key === 'PrivateKey') deriveFrom(e.target.value); }}
              on:change={(e) => { setIface(key, e.target.value); apply(); }} />
            {#if key === 'PrivateKey'}
              <!-- Regenerate in place: new private key + synchronously refreshed
                   read-only public key, both persisted in one apply().
                   原地重新生成：新私钥 + 同步刷新的只读公钥，一次 apply() 落盘。 -->
              <button class="fe-key-regen" type="button"
                title={$t('fields.private_key_regen')} aria-label={$t('fields.private_key_regen')}
                on:click={regeneratePrivateKey}>
                <Icon name="rotate-ccw" size={13} strokeWidth={1.75} />
              </button>
            {/if}
          </div>
        {/each}
        <!-- Read-only derived public key: lives BELOW the field rows, never
             inside `iface`, so apply()/pick() cannot write it into [Interface]
             (WireGuard derives it itself and rejects an explicit value).
             Pair it with the Copy button so it can be pasted into the remote
             peer's config without exposing the private key.
             只读派生公钥：置于字段行之外、永不进入 iface，因此 apply()/pick()
             不会把它写进 [Interface]（WireGuard 会自行派生并拒绝显式值）。
             旁边提供复制按钮，便于粘贴到对端配置，无需暴露私钥。 -->
        <div class="fe-row fe-pk-row">
          <span class="fe-label fe-pk-label">PublicKey</span>
          <span class="fe-pk-value" class:fe-pk-empty={derivedState !== 'ok'} title={derivedState === 'ok' ? derivedPublicKey : ''}>
            {#if derivedState === 'ok'}
              {derivedPublicKey}
            {:else if derivedState === 'invalid'}
              {$t('fields.public_key_invalid')}
            {:else}
              {$t('fields.public_key_placeholder')}
            {/if}
          </span>
          <button class="fe-pk-copy" class:copied={copied} disabled={derivedState !== 'ok'}
            title={$t('fields.public_key_copy')} aria-label={$t('fields.public_key_copy')}
            on:click={copyPublicKey}>
            <Icon name={copied ? 'check' : 'copy'} size={12} strokeWidth={1.75} />
            <span>{copied ? $t('fields.public_key_copied') : $t('fields.public_key_copy')}</span>
          </button>
        </div>
      </div>

      {#if awgOn}
        <div class="fe-group fe-group--awg">
          <h5 class="fe-group-title">{$t('fields.section_awg')}</h5>
          {#each IFACE_AWG as key}
            <div class="fe-row">
              <label class="fe-label" for="fe-awg-{key}">{key}</label>
            <input class="fe-input" id="fe-awg-{key}" type="text" value={iface[key] ?? ''}
              spellcheck="false"
              on:input={(e) => setIface(key, e.target.value)}
              on:change={apply} />
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
            {#if key === 'PersistentKeepalive'}
              <!-- WireGuard's PersistentKeepalive is written as a unitless integer that
                   is always interpreted as SECONDS (0 = disabled, range 0–65535). The
                   numeric input cannot carry a unit, so surface it as a hint beside it.
                   WireGuard 的 PersistentKeepalive 写作无单位整数，恒按「秒」解释
                   （0 = 禁用，范围 0–65535）。数字输入框无法承载单位，故在旁标注。 -->
              <span class="fe-unit" title={$t('fields.keepalive_unit_hint')}>{$t('fields.keepalive_unit')}</span>
            {/if}
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
  /* Derived public-key row: read-only, selectable, never editable. The value
     is identity data, so it uses the same monospace as the key fields. */
  .fe-pk-row { margin-top: 8px; padding-top: 8px; border-top: 1px dashed var(--border, #ccc); }
  .fe-pk-label { opacity: 0.7; }
  .fe-pk-value {
    flex: 1; min-width: 0; font-size: 12px; font-family: ui-monospace, monospace;
    overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
    user-select: text; -webkit-user-select: text;
  }
  .fe-pk-value.fe-pk-empty { font-family: var(--font-sans); font-style: italic; opacity: 0.5; }
  .fe-pk-copy {
    display: inline-flex; align-items: center; gap: 4px; flex-shrink: 0;
    height: 26px; padding: 0 9px; border: 0.5px solid var(--border, #ccc);
    border-radius: 6px; background: var(--bg-card); color: var(--text-secondary);
    font: 500 11px/1 var(--font-sans); cursor: pointer;
  }
  .fe-pk-copy:disabled { opacity: 0.4; cursor: not-allowed; }
  .fe-pk-copy:not(:disabled):hover { border-color: var(--accent); color: var(--text-primary); }
  .fe-pk-copy.copied { color: var(--green, #30a46c); border-color: var(--green, #30a46c); }
  /* Compact regenerate button beside the editable PrivateKey field. */
  .fe-key-regen {
    flex-shrink: 0; display: inline-flex; align-items: center; justify-content: center;
    width: 26px; height: 26px; padding: 0; border: 0.5px solid var(--border, #ccc);
    border-radius: 6px; background: var(--bg-card); color: var(--text-secondary);
    cursor: pointer;
  }
  .fe-key-regen:hover { border-color: var(--accent); color: var(--text-primary); }
  .fe-input {
    flex: 1; min-width: 0; padding: 5px 8px; font-size: 13px;
    font-family: ui-monospace, monospace;
    border: 1px solid var(--border, #ccc); border-radius: 6px;
    background: var(--bg-input, transparent); color: inherit;
  }
  /* Unit hint shown beside unitless numeric fields (e.g. PersistentKeepalive,
     whose value is always seconds in WireGuard). Muted, non-interactive.
     无单位数值字段（如 PersistentKeepalive，WireGuard 中恒为秒）旁的单位提示，
     弱色、不可交互。 */
  .fe-unit {
    flex-shrink: 0; font-size: 11px; font-family: ui-monospace, monospace;
    opacity: 0.55; padding: 2px 6px; border: 1px dashed var(--border, #ccc);
    border-radius: 5px; user-select: none;
  }
  .fe-peer-select { font-size: 12px; padding: 2px 6px; border-radius: 6px; border: 1px solid var(--border, #ccc); background: transparent; color: inherit; }
  .fe-error { color: #d33; font-size: 12px; margin: 4px 0; }
  /* Validation / save errors from the parent (name required, backend
     ValidateConfig / ImportConfig / UpdateConfig failures). Mirrors
     ConfigEditor.editor-errors so the Fields and conf tabs behave the same. */
  .editor-errors {
    padding: 10px 16px;
    background: var(--error-bg, #2a1416);
    border-top: 0.5px solid var(--red, #e5484d);
    flex-shrink: 0;
    max-height: 100px;
    overflow-y: auto;
  }
  .editor-errors p {
    margin: 3px 0;
    color: var(--error-text, #ff8a8a);
    font: 12px/16px var(--font-sans);
  }
</style>
