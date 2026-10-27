<script>
  // Per-tunnel Automation policy editor: Default State + Ordered Rules
  // (first-match-wins). The tunnel's rules form ONE ordered list — rule
  // position IS priority, and rules may mix connect/disconnect actions
  // freely (drag to reorder). Within a rule, conditions are always
  // combined with AND: every condition must match for the rule to fire.
  // "on any Wi-Fi" is a condition that needs no value.
  // When NO rule matches, the tunnel converges to its Default State
  // (connected / disconnected), edited at the top of this dialog.
  // Persisted to Settings.automation.per_tunnel_rules[tunnelName] (in
  // UI order) and Settings.automation.default_state[tunnelName] via
  // SaveAutomationRules. The whole settings object is re-fetched and
  // spread on save so other screens' edits (and other tunnels' rules)
  // are never clobbered.
  import { afterUpdate, onMount, onDestroy } from 'svelte';
  import { Events } from '@wailsio/runtime';
  import { AutomationEvaluate } from '../../bindings/github.com/imonior/wireguide-plus/internal/app/tunnelservice.js';
  import Icon from './Icon.svelte';
  import { t } from '../i18n/index.js';
  import { errText } from './errors.js';
  import SSIDPermissionBanner from './SSIDPermissionBanner.svelte';
  import { modalDrag } from './actions/modal-drag.js';
  import { manualOverrides, clearManualOverride, setManualOverrideService } from '../stores/manualoverride.js';
  import { loadPhysicalInterfaces, ifaceLabel } from './interface-utils.js';
  import { getAutomationDisabled, setAutomationDisabled } from './tunnel-automation.js';
  import { moduleFlags } from '../stores/ui.js';
  export let TunnelService;
  export let tunnelName = '';
  export let open = false;
  // rules = [ rule, ... ] in priority order; rule = { _gid, do: 'connect'|'disconnect', conds: [ {_id, type, ...} ] }
  let rules = [];
  // The state the tunnel converges to when NO rule matches.
  let defaultState = 'disconnect';
  // Durable automation switch (TunnelMeta.AutomationDisabled). Note the
  // polarity: `true` means automation is switched OFF. Lives in the tunnel
  // sidecar, persisted via SetTunnelPolicies. Surfaced here as an
  // "Automation on/off" checkbox and in the hero toggle, both writing the
  // SAME flag through the shared read-modify-write helper.
  let automationDisabled = false;
  // Local-only identity for {#each} keys; never persisted.
  let condId = 0;
  let ruleId = 0;
  let loadedFor = '';
  let knownSSIDs = [];
  let currentSSID = '';
  let currentSubnets = [];      // autocomplete suggestions for the subnet field
  let currentGatewayMAC = '';   // autocomplete suggestion for the MAC field
  // Live network context + per-rule decision from AutomationEvaluate().
  let preview = null;           // AutomationPreviewResponse
  // Full physical-interface list (same source as the per-tunnel egress
  // binding dropdown) — used to populate the "on interface" condition's
  // selector. Kept separate from `preview.interfaces`, which only lists the
  // currently-UP adapters and would pre-fill the active Wi-Fi (e.g. en0) and
  // hide every other NIC.
  let physicalInterfaces = [];
  let previewTimer = null;
  // Epoch of the current preview "session". Increased every time the modal
  // re-opens (and on close → null). refreshPreview carries the session id
  // and only writes to `preview` if its session is still active, so a slow
  // in-flight AutomationPreview from a previous open/close cycle can never
  // overwrite the current tunnel's state — the same principle loadGen uses
  // for settings, applied here so match indicators don't flicker between
  // stale responses and new ones. See memory EXP-100009308.
  let previewEpoch = 0;
  // Active AbortController for the in-flight preview request, if any.
  // Automatically cancels the previous one when refreshPreview is called
  // again before the request resolves. Combined with previewEpoch this
  // guarantees "single source of truth" for the live indicator state:
  // there is at most one pending fetch per session and it never lands
  // after its session ended.
  let previewAbort = null;
  // Combobox suggestions for the SSID field: every WiFi profile the OS has
  // saved (pre-filled), plus the current network in case it isn't saved yet.
  $: ssidSuggestions = [...new Set([...(knownSSIDs || []), ...(currentSSID ? [currentSSID] : [])])];
  // Physical interface names from the full enumeration (same list the
  // per-tunnel egress binding uses) — autocomplete suggestions for the
  // interface condition.
  $: interfaceSuggestions = [...new Set(physicalInterfaces.map(x => x.name))];
  let saveError = '';
  // loadGen tags each async load so a slow in-flight load(A) can't clobber
  // the rules after the user has already switched to load(B).
  let loadGen = 0;
  // Reload whenever the modal opens for a (possibly different) tunnel.
  $: if (open && tunnelName && loadedFor !== tunnelName) {
    load(tunnelName);
  }
  // Whenever the modal opens ensure the live-indicator poll is running:
  // onDestroy only fires once per component mount but close() nulls the
  // timer out, so simply re-opening the modal needs to start it again.
  // Without this re-open the indicators appear to "randomly change"
  // between opens because they reflect whichever preview poll window
  // happened to fire last.
  $: if (open && !previewTimer) {
    previewEpoch += 1;
    previewAbort?.abort?.();
    previewAbort = null;
    refreshPreview();
    previewTimer = setInterval(refreshPreview, 3000);
  }
  async function load(name) {
    const gen = ++loadGen;
    await flush();
    saveError = '';
    try {
      const s = await TunnelService.GetSettings();
      if (gen !== loadGen) return;
      const per = s?.automation?.per_tunnel_rules || {};
      rules = toList(per[name] || []);
      const defs = s?.automation?.default_state || {};
      defaultState = defs[name] === 'connect' ? 'connect' : 'disconnect';
      // Only mark "loaded" AFTER rules are actually populated. Setting
      // loadedFor earlier caused refreshPreview()'s guard to pass while
      // rules were still empty/stale, evaluating a wrong draft and
      // showing all badges as no-match/inactive until the load finished
      // — this was the direct cause of "every open looks different".
      loadedFor = name;
      // Durable automation opt-out also lives in the tunnel sidecar
      // (TunnelMeta.AutomationDisabled). Surface it here so the switch in
      // this editor and the hero toggle both reflect the same flag.
      try {
        const disabled = await getAutomationDisabled(TunnelService, name);
        if (gen !== loadGen) return;
        automationDisabled = disabled;
      } catch (_) { if (gen === loadGen) automationDisabled = false; }
    } catch (e) {
      if (gen === loadGen) {
        rules = [];
        defaultState = 'disconnect';
        loadedFor = name;
      }
      console.error('automation load:', e);
    }
    // The draft for THIS tunnel is now loaded (rules populated). Rate it
    // against the live network immediately so reopening the editor draws
    // the correct match / in-use badges on the first frame instead of
    // waiting for the 3 s poll; refreshPreview no-ops unless this load is
    // still the current tunnel/session.
    if (gen === loadGen) refreshPreview();
    try {
      const r = await TunnelService.GetKnownSSIDs();
      if (gen !== loadGen) return;
      knownSSIDs = r?.known || [];
      currentSSID = r?.current || '';
    } catch (_) {}
    try {
      const subs = (await TunnelService.GetCurrentSubnets()) || [];
      if (gen !== loadGen) return;
      currentSubnets = subs;
    } catch (_) { if (gen === loadGen) currentSubnets = []; }
    try {
      const mac = (await TunnelService.GetCurrentNetwork())?.gateway_mac || '';
      if (gen !== loadGen) return;
      currentGatewayMAC = mac;
    } catch (_) { if (gen === loadGen) currentGatewayMAC = ''; }
    // Full physical-interface list for the "on interface" selector. Uses the
    // same source as the per-tunnel egress binding dropdown so the two
    // selectors are driven by one enumeration. (preview.interfaces only lists
    // UP adapters and would pre-fill the active Wi-Fi, hiding other NICs.)
    try {
      const pis = await loadPhysicalInterfaces(TunnelService);
      if (gen !== loadGen) return;
      physicalInterfaces = pis;
    } catch (_) { if (gen === loadGen) physicalInterfaces = []; }
  }
  // Convert the persisted rule array into the editor model, PRESERVING the
  // stored order (position = priority — the engine evaluates top to bottom,
  // and rules may mix connect/disconnect actions). Conditions inside a rule
  // are always AND; legacy configs that used OR within a rule (match !==
  // 'all' with multiple conditions) are split into one rule per condition,
  // preserving the original semantics under the AND model. Legacy
  // none_match rules are dropped — the backend's Normalize already turned
  // the first one into the tunnel's Default State.
  function toList(rulesIn) {
    const out = [];
    for (const r of rulesIn || []) {
      const whens = Array.isArray(r.when) ? r.when : (r.when ? [r.when] : []);
      if (!whens.length) continue;
      if (whens.length === 1 && (whens[0]?.type || inferType(whens[0])) === 'none_match') continue;
      const doAction = r.do === 'disconnect' ? 'disconnect' : 'connect';
      const isLegacyOR = r.match !== 'all' && whens.length > 1;
      const makeRule = (w) => ({
        _gid: ++ruleId,
        do: doAction,
        conds: [{
          _id: ++condId,
          type: w?.type || inferType(w),
          ssid: w?.ssid || '',
          subnet: w?.subnet || '',
          gateway_mac: w?.gateway_mac || '',
          gateway_ip: w?.gateway_ip || '',
          interface_name: w?.interface_name || '',
          start: w?.start || '',
          end: w?.end || '',
          days: Array.isArray(w?.days) ? w.days.slice() : [],
          label: w?.label || '',
        }],
      });
      if (isLegacyOR) {
        for (const w of whens) out.push(makeRule(w));
      } else {
        const rule = { _gid: ++ruleId, do: doAction, conds: [] };
        for (const w of whens) rule.conds.push(makeRule(w).conds[0]);
        out.push(rule);
      }
    }
    return out;
  }
  // Infer the condition type of a rule that somehow lacks one (e.g. written
  // by an older tool or hand-edited). Keeps such rules intact on reload.
  function inferType(w) {
    if (w?.type) return w.type;
    if (w?.ssid) return 'ssid';
    if (w?.subnet) return 'subnet';
    if (w?.gateway_mac) return 'network';
    if (w?.gateway_ip) return 'gateway_ip';
    if (w?.interface_name) return 'interface';
    if (w?.start || w?.end || (w?.days && w.days.length)) return 'time';
    return 'none_match';
  }
  const MAX_CONDS = 50;
  // A fresh, blank condition row. Nothing is persisted until cleanedCond()
  // returns a complete object.
  function newCond() {
    return {
      _id: ++condId, type: 'network', ssid: '', subnet: '', gateway_mac: '',
      gateway_ip: '', interface_name: '', start: '', end: '', days: [], label: '',
    };
  }
  // A new rule card carries one blank condition so the user can start
  // editing immediately, and defaults to the connect action (one click on
  // the badge flips it). No save() here: a blank draft is not a config
  // change — it becomes persistable on the first input that completes it.
  function addRule() {
    const rule = { _gid: ++ruleId, do: 'connect', conds: [newCond()] };
    rules = [...rules, rule];
  }
  function removeRule(ruleIdx) {
    rules = rules.filter((_, i) => i !== ruleIdx);
    save();
  }
  function addCond(ruleIdx) {
    const rule = rules[ruleIdx];
    if (rule.conds.length >= MAX_CONDS) return;
    const nextRules = rules.map((r, i) => i === ruleIdx
      ? { ...r, conds: [...r.conds, newCond()] }
      : r);
    rules = nextRules;
  }
  function removeCond(ruleIdx, i) {
    const nextRules = rules.map((r, idx) => idx === ruleIdx
      ? { ...r, conds: r.conds.filter((_, j) => j !== i) }
      : r);
    rules = nextRules;
    save();
  }
  // Toggle a rule's action between connect and disconnect.
  function toggleRuleDo(rule) {
    rule.do = rule.do === 'connect' ? 'disconnect' : 'connect';
    rules = rules;
    save();
  }
  // Set the tunnel's Default State (what it converges to when no rule
  // matches). Persisted with the next save.
  function setDefaultState(v) {
    defaultState = v;
    save();
  }
  // Durable automation on/off toggle. The stored flag is
  // `automation_disabled` (true = automation switched OFF), but the control
  // the user sees is an "Automation on/off" checkbox, so the checkbox is
  // CHECKED while automation is ENABLED. It goes through the shared
  // read-modify-write helper so it writes the same flag, the same way, as
  // the hero toggle in the detail view.
  async function toggleAutomationDisabled() {
    if (!TunnelService || !tunnelName) return;
    const next = !automationDisabled;
    try {
      automationDisabled = await setAutomationDisabled(TunnelService, tunnelName, next);
    } catch (e) {
      console.error('toggle automation_disabled:', e);
    }
  }
  // Rule-level drag & drop: reorder rules — position IS priority. Live
  // reorder while dragging (same feel as condition rows); persisted by
  // onRuleDragEnd's save().
  let ruleDragIdx = null;
  function onRuleDragStart(e, i) {
    ruleDragIdx = i;
    e.dataTransfer.effectAllowed = 'move';
    try { e.dataTransfer.setData('text/plain', 'rule:' + i); } catch (_) {}
    const card = e.currentTarget.closest('.am-rule');
    if (card) {
      try { e.dataTransfer.setDragImage(card, 24, 20); } catch (_) {}
    }
  }
  function onRuleDragOver(e, i) {
    if (ruleDragIdx === null) return;
    e.preventDefault();
    e.dataTransfer.dropEffect = 'move';
    if (ruleDragIdx === i) return;
    const arr = [...rules];
    const [moved] = arr.splice(ruleDragIdx, 1);
    arr.splice(i, 0, moved);
    rules = arr;
    ruleDragIdx = i;
  }
  function onRuleDragEnd() {
    if (ruleDragIdx !== null) { ruleDragIdx = null; save(); }
  }
  // Lightweight format validation for user feedback (see engine-safe
  // comment in the original editor).
  function macHex(v) { return (v || '').replace(/[^0-9a-fA-F]/g, '').toLowerCase(); }
  function macInvalid(v) { const s = (v || '').trim(); return s !== '' && macHex(s).length !== 12; }
  function macCanon(v) {
    const h = macHex(v);
    if (h.length !== 12) return (v || '').trim();
    return h.match(/.{2}/g).join(':');
  }
  function onMacChange(c) {
    c.gateway_mac = macCanon(c.gateway_mac);
    rules = rules;
    save();
  }
  // Type switch from the condition's outer <select>. Switching families clears
  // every value field so no stale data carries across. The Wi-Fi SSID family
  // requires a concrete SSID (there is no "any Wi-Fi" — an empty rule set is the
  // default "all" state); the interface family defaults to the first detected NIC
  // (all network ports are listed, including Wi-Fi cards — gateway IP/MAC/subnet
  // already cover wired-network distinction).
  function onTypeChange(c, v) {
    c.type = v;
    c.ssid = ''; c.subnet = ''; c.gateway_mac = ''; c.gateway_ip = ''; c.interface_name = '';
    if (v === 'interface') { c.type = 'interface'; c.interface_name = (physicalInterfaces[0] && physicalInterfaces[0].name) || ''; }
    rules = rules;
    save();
    schedulePreviewRefresh();
  }
  // Toggle one weekday (0=Sunday … 6=Saturday) on a time condition row.
  function toggleDay(c, d) {
    const days = Array.isArray(c.days) ? c.days : [];
    c.days = days.includes(d) ? days.filter(x => x !== d) : [...days, d];
    rules = rules;
    save();
  }
  // Lightweight IPv4 check for user feedback; "" is allowed (incomplete).
  function gatewayIPInvalid(v) {
    const s = (v || '').trim();
    if (s === '') return false;
    const m = s.match(/^(\d{1,3})\.(\d{1,3})\.(\d{1,3})\.(\d{1,3})$/);
    if (!m) return true;
    return m.slice(1).some(o => Number(o) > 255);
  }
  function cidrInvalid(v) {
    const s = (v || '').trim();
    if (s === '') return false;
    const m = s.match(/^([^/]+)\/(\d{1,3})$/);
    if (!m) return true;
    const prefix = Number(m[2]);
    const ip = m[1];
    if (ip.includes(':')) return prefix < 0 || prefix > 128;
    const octets = ip.split('.');
    if (octets.length !== 4) return true;
    if (octets.some(o => o === '' || !/^\d+$/.test(o) || Number(o) > 255)) return true;
    return prefix < 0 || prefix > 32;
  }
  // Drag-to-reorder WITHIN a rule card (live reordering feel). Kept
  // separate from rule-level dragging (ruleDragIdx) so dragging a
  // condition never reorders rules and vice versa.
  let dragIndex = null;
  let dragRule = null;
  function onDragStart(e, ruleIdx, i) {
    dragIndex = i;
    dragRule = ruleIdx;
    e.dataTransfer.effectAllowed = 'move';
    try { e.dataTransfer.setData('text/plain', 'cond:' + i); } catch (_) {}
    const row = e.currentTarget.closest('.am-cond');
    if (row) {
      try { e.dataTransfer.setDragImage(row, 24, row.offsetHeight / 2); } catch (_) {}
    }
  }
  function onCondDragOver(e, ruleIdx, i) {
    if (dragIndex === null || dragRule !== ruleIdx) return;
    e.preventDefault();
    e.dataTransfer.dropEffect = 'move';
    if (dragIndex === i) return;
    const arr = [...rules[ruleIdx].conds];
    const [moved] = arr.splice(dragIndex, 1);
    arr.splice(i, 0, moved);
    const nextRules = rules.map((r, idx) => idx === ruleIdx ? { ...r, conds: arr } : r);
    rules = nextRules;
    dragIndex = i;
  }
  function onDragEnd() {
    if (dragIndex !== null) { dragIndex = null; dragRule = null; save(); }
  }
  // Scroll affordance for the rules area.
  let rulesEl;
  let canScrollUp = false;
  let canScrollDown = false;
  function updateScroll() {
    if (!rulesEl) return;
    canScrollUp = rulesEl.scrollTop > 2;
    canScrollDown = rulesEl.scrollTop + rulesEl.clientHeight < rulesEl.scrollHeight - 2;
  }
  afterUpdate(updateScroll);
  // cleanedCond returns the normalized persisted form of a condition row,
  // or null while incomplete. wifi / ethernet need no value. (none_match
  // is no longer creatable in this editor — the Default State control
  // replaces the "otherwise" fallback.)
  function cleanedCond(c) {
    const t = c.type;
    if (t === 'wifi') return { type: 'wifi', label: c.label || '' };
    if (t === 'ethernet') return { type: 'ethernet', label: c.label || '' };
    if (t === 'ssid') return c.ssid.trim() !== '' ? { type: 'ssid', ssid: c.ssid.trim(), label: c.label || '' } : null;
    if (t === 'subnet') return c.subnet.trim() !== '' ? { type: 'subnet', subnet: c.subnet.trim(), label: c.label || '' } : null;
    if (t === 'network') return c.gateway_mac.trim() !== '' ? { type: 'network', gateway_mac: macCanon(c.gateway_mac), label: c.label || '' } : null;
    if (t === 'gateway_ip') return c.gateway_ip.trim() !== '' ? { type: 'gateway_ip', gateway_ip: c.gateway_ip.trim(), label: c.label || '' } : null;
    if (t === 'interface') return c.interface_name.trim() !== '' ? { type: 'interface', interface_name: c.interface_name.trim(), label: c.label || '' } : null;
    if (t === 'time') {
      const has = (c.start || '').trim() !== '' || (c.end || '').trim() !== '' || (c.days || []).length > 0;
      return has ? { type: 'time', start: (c.start || '').trim(), end: (c.end || '').trim(), days: (c.days || []).slice(), label: c.label || '' } : null;
    }
    return null;
  }
  // buildRules assembles the persisted rule array — one entry per editor
  // rule card, IN UI ORDER (position = priority; the first complete rule
  // that matches the current network decides the outcome, so the user can
  // interleave connect/disconnect rules freely). A rule with no complete
  // conditions contributes no entry.
  function buildRules() {
    const out = [];
    for (const rule of rules) {
      const conds = rule.conds.map(cleanedCond).filter(Boolean);
      if (!conds.length) continue;
      const r = { when: conds, do: rule.do };
      r.match = 'all';
      out.push(r);
    }
    return out;
  }
  // Debounced, snapshot-based save (same design as before).
  let saveTimer = null;
  let pending = null;
  let saveChain = Promise.resolve();
  function save() {
    pending = { name: tunnelName, rules: buildRules(), def: defaultState };
    if (saveTimer) clearTimeout(saveTimer);
    saveTimer = setTimeout(runSave, 300);
  }

  // --- Live re-evaluation on edit -------------------------------------------------
  // The 3s poll catches network changes (SSID switched, VPN came up, …), but
  // edits to the rule/condition draft need immediate re-rating against the
  // CURRENT network — otherwise the user types a new SSID and stares at a
  // stale "no match" badge for up to 3 seconds.
  //
  // We trigger via a reactive statement on a JSON fingerprint of the rule
  // list + default state — that way EVERY change (new rule, removed rule,
  // condition type
  // switch, text input blur/input, day toggles, drag reorder) naturally
  // invalidates the fingerprint without us having to sprinkle calls through
  // every handler.
  //
  // Implementation notes:
  //   * SaveAutomationRules already short-circuits on diskDiffers so a
  //     debounce fire that produces no net change does zero I/O.
  //   * We don't overwrite saveError: that flag is for explicit "Save"
  //     button errors. Transient failures mid-edit silently fall back to
  //     the previous preview frame.
  //   * saveTimer (user-initiated) vs editTimer (live-preview) share the
  //     same buildRules+persist sink, so concurrent typesetting doesn't
  //     pile up disk writes.
  let editTimer = null;
  $: if (open && loadedFor === tunnelName) {
    // Touch nested fields so svelte considers them dependencies. The JSON
    // stringify is cheap for the small rule sets users write (~<30 rules)
    // and doubles as the fingerprint for "did the draft really change".
    // We only react once the draft for the CURRENT tunnel is loaded, so a
    // reopen never rates the previous tunnel's leftover cards.
    const _touchR = JSON.stringify(rules);
    const _touchD = defaultState;
    void _touchR; void _touchD;
    schedulePreviewRefresh();
  }
  // Re-rate the DRAFT (not the disk state) shortly after the user stops
  // typing. Persistence is handled separately by save()'s debounce; the
  // live indicators must follow the draft even before/independently of a
  // write landing, which is what removes the old "edit then wait for the
  // save round-trip before badges update" lag.
  function schedulePreviewRefresh() {
    if (!open) return;
    if (editTimer) clearTimeout(editTimer);
    editTimer = setTimeout(() => {
      editTimer = null;
      if (open) refreshPreview();
    }, 0);
  }
  function runSave() {
    saveTimer = null;
    const snap = pending;
    pending = null;
    if (!snap) return;
    saveChain = saveChain.then(() => persist(snap));
  }
  // Normalized fingerprint of the rules WE last wrote to disk. The
  // config_changed watcher fires for our own writes too (it polls mtime);
  // when the on-disk content equals what we just saved, it must NOT reload
  // and rebuild the cards (that would blur focus and re-trigger a rating
  // loop). Only a genuinely external edit differs and warrants a reload.
  let lastSavedFingerprint = '';
  async function persist(snap) {
    saveError = '';
    try {
      await TunnelService.SaveAutomationRules(snap.name, snap.rules, snap.def);
      lastSavedFingerprint = JSON.stringify(normRules(snap.rules));
      // No refreshPreview() here: the live indicators rate the in-memory
      // DRAFT (via AutomationEvaluate), which a disk write does not change.
      // Re-rating after the save would just repeat the same evaluation.
    } catch (e) {
      saveError = errText(e);
      console.error('automation save:', e);
    }
  }
  async function flush() {
    if (saveTimer) { clearTimeout(saveTimer); runSave(); }
    await saveChain;
  }
  async function close() {
    await flush();
    open = false;
    loadedFor = '';
    // Tear down the current preview session: abandon any in-flight request so
    // its result cannot overwrite a future session (session epoch + abort),
    // drop the interval poll (it will be recreated on the next open by the
    // `open && !previewTimer` reactive guard above), and blank the shown
    // preview so the very first render on reopen does not draw a stale
    // decision / winning-rule highlight from 20 minutes ago.
    previewEpoch += 1;
    if (previewAbort) { try { previewAbort.abort?.(); } catch (_) {} previewAbort = null; }
    if (previewTimer) { clearInterval(previewTimer); previewTimer = null; }
    if (editTimer) { clearTimeout(editTimer); editTimer = null; }
    if (saveTimer) { clearTimeout(saveTimer); saveTimer = null; pending = null; }
    preview = null;
  }
  // ---- Live match indicators -------------------------------------------
  // Marker semantics (same engine the helper enforces):
  //   - conditions inside one rule are AND; rules form ONE ordered list
  //     evaluated top-to-bottom, first match wins;
  //   - per-condition "match" badge = that condition's own result, judged
  //     individually — a rule only fires when ALL its conditions match;
  //   - "in use" badge + winning frame = the FIRST matching rule overall
  //     only, and only when its action actually executes; rules that also
  //     match but rank behind it stay match-only;
  //   - when no rule matches, the tunnel converges to its Default State
  //     (shown at the top of the dialog).
  // The GUI sends the editor's CURRENT DRAFT rules to AutomationEvaluate,
  // which rates them against the live network context with the same wifi
  // engine the helper enforces and returns per-rule / per-condition match
  // detail. Rating the DRAFT (not the last-saved disk state) means the
  // returned RuleDetails line up 1:1 with the cards by construction —
  // there is no longer a second, asynchronously-loaded "disk rules" array
  // to misalign with, which was the root cause of the badges/frame
  // lighting up the wrong rule or differing on every reopen. A 3 s poll
  // re-rates the same draft so network changes (SSID switch, VPN up/down)
  // are reflected in real time.
  //
  // UNIFIED refresh entry point (single source of truth per memory
  // EXP-100009308). Every caller — the open poll guard, load() completion,
  // the draft-fingerprint reactive, and the 3 s network poll — goes
  // through this function, which enforces:
  //   * no rating until the draft for the current tunnel is loaded
  //     (loadedFor === tunnelName) so a reopen never rates stale cards;
  //   * one in-flight request at a time (cancel the previous via AbortController);
  //   * response rejection when `previewEpoch` has advanced (closed/reopened);
  //   * response rejection when `tunnelName` changed between fire and resolution;
  //   * response rejection when the draft changed in flight (fingerprint mismatch);
  //   * transient failures keep the previous frame instead of blanking the
  //     indicators, so mid-session network hiccups don't cause "every open
  //     looks different".
  async function refreshPreview() {
    if (!open || !tunnelName) return;
    // Only rate once the DRAFT for this tunnel is loaded. Right after a
    // reopen (or tunnel switch) `rules` still holds the previous tunnel's
    // cards; rating those and mapping the details back onto the new cards
    // is exactly what lit up the wrong frame / showed stale badges. The
    // load() completion and the draft-fingerprint reactive both call us
    // again as soon as the real draft is in place.
    if (loadedFor !== tunnelName) return;
    const myEpoch = previewEpoch;
    const myTunnel = tunnelName;
    // Snapshot the draft we are about to rate, plus its fingerprint. The
    // returned RuleDetails line up 1:1 with THIS snapshot's rules (same
    // engine, caller-supplied rules); if the draft changes while the call
    // is in flight we discard the result and let the draft-fingerprint
    // reactive fire a fresh rating — cards and details can never land from
    // two different drafts.
    const draft = buildRules();
    const myDefault = defaultState;
    const fingerprint = JSON.stringify(draft) + '|' + myDefault;
    // Cancel any previous in-flight fetch: Wails calls that are already on
    // the wire don't have an HTTP abort, so for the AbortController guard
    // we still benefit from the epoch check below — no stale result ever
    // reaches `preview`. If a future call path brings a fetch that speaks
    // AbortSignal, this cancels it cleanly.
    if (previewAbort) { try { previewAbort.abort?.(); } catch (_) {} }
    previewAbort = new AbortController();
    try {
      const pv = await AutomationEvaluate(myTunnel, draft, myDefault);
      if (previewEpoch !== myEpoch) return;        // session changed
      if (tunnelName !== myTunnel) return;          // switched tunnel
      if (!open) return;
      // The draft was edited while this call was in flight: its details
      // describe an older draft, so drop it. The rules reactive statement
      // will re-rate the current draft on its own debounce.
      if (JSON.stringify(buildRules()) + '|' + defaultState !== fingerprint) return;
      // AutomationEvaluate always returns exactly one tunnel entry for the
      // requested tunnel (even an empty rule set yields an explicit
      // "unmanaged" decision), so a missing entry means the call predates
      // this tunnel — treat as no result and keep the previous frame.
      const t = (pv?.tunnels || []).find(x => x.name === myTunnel);
      preview = {
        on_wifi: pv?.on_wifi ?? false,
        ssid: pv?.ssid || '',
        gateway_mac: pv?.gateway_mac || '',
        gateway_ip: pv?.gateway_ip || '',
        physical_ips: pv?.physical_ips || [],
        subnets: pv?.subnets || [],
        decision: t ? t.decision : 'unmanaged',
        rules: t ? t.rules : [],
        active: !!t?.active,
        state: t?.state || '',
        interfaces: pv?.interfaces || [],
      };
    } catch (e) {
      // Log the error so a missing method (old binary) or a network
      // enumeration failure is visible in the dev console instead of
      // silently leaving badges blank.
      console.error('[automation] refreshPreview failed:', e);
      // Network enumeration can be temporarily unavailable mid-session;
      // keep the last frame so indicators don't flicker to empty/neutral
      // on a transient failure.
      if (previewEpoch !== myEpoch || tunnelName !== myTunnel) return;
    } finally {
      if (previewAbort && !previewAbort.signal.aborted) {
        previewAbort = null;
      }
    }
  }
  // Rules are evaluated top-to-bottom in list order; the first matching rule
  // decides the outcome, and when nothing matches the tunnel converges to
  // its Default State. We expose this so the UI can highlight the winning
  // rule and dim lower-priority rules that also match.
  $: winningRuleIndex = (() => {
    const rules = preview?.rules || [];
    for (let i = 0; i < rules.length; i++) {
      if (rules[i].matched) return i;
    }
    return -1;
  })();
  // The Default State row carries the SAME outer ring a winning rule card
  // gets — but the two mark different winners. A rule's ring means "that rule
  // matched and its action ran"; the Default row's ring means "NOTHING
  // matched, so the engine fell back to the Default State and executed it".
  // So the ring lights iff (a) no rule matched at all (winningRuleIndex < 0)
  // and (b) the final decision is really the selected default — i.e. no
  // manual latch swallowed it. A rule that matched and merely happens to do
  // the same action lights its OWN ring, never the default's, so the two
  // markers are mutually exclusive and always point at the real decision
  // source. This is NOT the tunnel's physical up/down state — that is a
  // tunnel-level fact shown in the status row below.
  $: defaultRuleWon = winningRuleIndex < 0 && !!defaultState && preview?.decision === defaultState;
  // The tunnel is under a manual latch right now: manual-off suppresses the
  // winning connect rule, manual-on suppresses the winning disconnect rule.
  // A manual action is the same as pausing automation by hand, so the
  // on/off column above switches to a "Manual control" banner with the
  // release button — there is no point showing the durable on/off switch
  // while a latch is overriding it anyway.
  // 当前隧道正带着手动闩锁：manual-off 抑制获胜的连接规则，manual-on 抑制获胜的
  // 断开规则。手动接管等同于手动暂停自动化，所以上方的 on/off 栏切换成「手动接管」
  // 横幅并附带释放按钮——闩锁本就在压制常驻开关，这时再显示开关没意义。
  $: manualLatched = preview?.decision === 'manual-off' || preview?.decision === 'manual-on';
  let restoringOverride = false;

  // Release the manual latch and hand the tunnel back to its rules. The shared
  // clear refreshes the one store every surface reads, so a restore here also
  // drops the list's indicator at once; this surface then re-rates immediately
  // (the preview decision flips from manual-* back to the rules' own verdict,
  // and waiting for the 3s poll would leave the banner claiming a latch that
  // was just released). Restore lives in this column because the user arrives
  // here precisely to ask "why are my rules not running?" — and a manual
  // action is one of the two answers (the other being the durable on/off
  // switch above).
  // 释放手动闩锁，把隧道交还给规则。共享清除会刷新所有表面共读的同一个 store，于是
  // 在此点恢复也会立刻撤下列表的提示；随后本面立即重新评估（preview 的 decision 会从
  // manual-* 翻回规则自身的结论，等 3s 轮询会让横幅继续声称一个刚刚释放的闩锁）。
  // 恢复入口就放在这一栏：用户来到这里正是为了问「我的规则为什么没跑？」——手动接管
  // 是两种答案之一（另一种就是上方的常驻开关）。
  async function restoreAutomation() {
    if (!TunnelService || !tunnelName || restoringOverride) return;
    restoringOverride = true;
    try {
      await clearManualOverride(tunnelName);
      refreshPreview();
    } catch (e) {
      console.warn('failed to restore automation for tunnel:', tunnelName, e);
    } finally {
      restoringOverride = false;
    }
  }
  // Register the bound service so the shared store can reach the backend, and
  // re-rate the moment THIS tunnel's latch entry changes in the shared store
  // (e.g. a restore performed on the tunnel list) — keeping the Manual control
  // banner in step with the list instead of up to 3s behind its own poll.
  // 注册被注入的服务，使共享 store 能触达后端；并在本隧道闩锁项于共享 store 中变化时
  // 立即重评（例如在通道列表上点了恢复），让「手动接管」横幅与列表同步，而非最多落后
  // 自身轮询 3s。
  $: if (TunnelService) setManualOverrideService(TunnelService);
  let prevOverrideToken = '';
  $: {
    const hit = ($manualOverrides || []).find(o => o && o.name === tunnelName);
    const token = hit ? hit.override : '';
    if (token !== prevOverrideToken) {
      prevOverrideToken = token;
      if (open && TunnelService) refreshPreview();
    }
  }
  // Human label for the live decision strip ("this network will connect /
  // disconnect / ..."), taken from the draft's evaluated decision.
  $: decisionLabelKey = ({
    // Order mirrors the backend switch in helper/automation_rules.go, where
    // "disabled" (automation switched off for this tunnel) outranks every
    // rule-derived token: without this entry the strip fell through to
    // decision_unmanaged ("no rule matches this network"), which reads as
    // "go fix your rules" when the real answer is "you turned automation off".
    disabled: 'automation.decision_disabled',
    connect: 'automation.decision_connect',
    disconnect: 'automation.decision_disconnect',
    'manual-off': 'automation.decision_manual_off',
    'manual-on': 'automation.decision_manual_on',
    blocked: 'automation.decision_blocked',
    'ssid-blind': 'automation.decision_ssid_blind',
    'network-unidentified': 'automation.decision_network_unidentified',
    backoff: 'automation.decision_backoff',
  })[decisionKey(preview)] || 'automation.decision_unmanaged';
  // Build one marker snapshot from the same ordered draft/details pair used
  // by the backend. Keying condition markers by the editor row id avoids
  // stale deep lookups when Svelte updates nested bind:value fields.
  // Preview RuleDetails align 1:1 with the PERSISTED draft rules (the ones
  // buildRules emits) in list order, so a single running counter maps cards
  // to details; draft (incomplete) cards are skipped when counting.
  $: markerSnapshot = (() => {
    const result = { rules: {}, conditions: {}, matchCounts: {} };
    const details = preview?.rules || [];
    let detailIndex = 0;
    let firstMatch = -1;
    for (let ruleIdx = 0; ruleIdx < rules.length; ruleIdx++) {
      const rule = rules[ruleIdx];
      const persisted = isPersistedRule(rule);
      const rd = persisted ? details[detailIndex++] : null;
      const globalIndex = detailIndex - 1;
      const ruleMatched = !!rd?.matched;
      if (ruleMatched && firstMatch < 0) firstMatch = globalIndex;
      const winning = ruleMatched && firstMatch === globalIndex;
      result.rules[rule._gid] = winning && actionExecuted(preview);
      for (let i = 0; i < rule.conds.length; i++) {
        const c = rule.conds[i];
        const idx = condIndexInRule(rule, i);
        const conditionMatched = !!rd?.conditions?.[idx]?.matched;
        // Two markers only, and both are about the CONDITION/rule itself —
        // never about whether the tunnel is physically up:
        //   - match   — does this condition fit the current network (a pure
        //               match test, nothing more);
        //   - in-use  — "in use": this condition's rule is the first match AND
        //               the engine actually ran its action (connect/disconnect).
        //               It is an "adopted" marker and says nothing about whether
        //               the tunnel is up (that has its own state chip).
        //
        // 只有两个标记，且描述的都是「条件/规则本身」，与隧道是否真正连通无关：
        //   - match   —— 该条件是否匹配当前网络（纯粹的匹配判断）；
        //   - in-use  —— 「使用中」：该条件所属规则是首个匹配，且引擎真的执行了
        //                其动作（connect/disconnect）。这是「被采纳」标记，
        //                不表示隧道已连通（连通状态有独立的状态徽章）。
        const inUse = conditionMatched && winning && actionExecuted(preview);
        result.conditions[c._id] = {
          match: conditionMatched,
          active: inUse,
        };
        // The summary rows and the badge above MUST NOT be able to disagree,
        // so the per-type tally is accumulated from this very `match`
        // boolean — the same one the badge renders — instead of being
        // recomputed from a second walk over the draft or over preview. Two
        // walks over two shapes is how the summary ended up saying "no rule
        // matches" next to a badge that said the opposite.
        // 汇总行与上面的徽标绝不允许互相矛盾，因此按类型累计的计数直接取自这个
        // `match` 布尔值——也就是徽标渲染的那一个——而不是再来一遍草稿或 preview
        // 的遍历。对两种数据结构各走一遍，正是此前汇总行显示「没有规则匹配」、
        // 旁边徽标却显示相反结论的原因。
        if (conditionMatched) {
          const t = condTypeNorm(c.type);
          result.matchCounts[t] = (result.matchCounts[t] || 0) + 1;
        }
      }
    }
    return result;
  })();
  // The decision is actually executed only for connect/disconnect. Under
  // the manual-off latch the winning connect rule is suppressed — nothing
  // runs, so nothing may be marked "in use" (match markers stay truthful).
  // The preview frame is passed in rather than read from the enclosing
  // scope: see decisionKey() for why that matters.
  // 决策真正被执行的只有 connect/disconnect。manual-off 闩锁下获胜的连接规则被
  // 压制——什么都没执行，所以任何东西都不能标成「使用中」（匹配标记保持真实）。
  // preview 这一帧由参数传入而非在作用域内直接读取：原因见 decisionKey()。
  function actionExecuted(pv) {
    const k = decisionKey(pv);
    return k === 'connect' || k === 'disconnect';
  }
  // Preview RuleDetails keep the persisted draft order, so the k-th
  // PERSISTED editor rule card maps to the k-th preview RuleDetail. Draft
  // rules — cards whose conditions are incomplete and therefore not
  // persisted — have no detail and must be SKIPPED when counting; mapping
  // naively by card index would shift every card after a draft onto the
  // wrong detail and light up the wrong rule frame even though the
  // engine's decision is correct.
  function isPersistedRule(rule) {
    return rule.conds.map(cleanedCond).filter(Boolean).length > 0;
  }
  function ruleDetailFor(ruleIdx) {
    const details = preview?.rules || [];
    let k = 0; // index among persisted rules
    for (let i = 0; i < rules.length; i++) {
      if (!isPersistedRule(rules[i])) continue;
      if (i === ruleIdx) return details[k] || null;
      k++;
    }
    return null;
  }
  // Index of a condition row within the PERSISTED rule's conditions array
  // (incomplete rows are filtered out, so the live indicator must skip them).
  function condIndexInRule(rule, i) {
    let idx = 0;
    for (let j = 0; j <= i; j++) {
      if (!cleanedCond(rule.conds[j])) continue;
      if (j === i) return idx;
      idx++;
    }
    return -1;
  }
  // Whether this condition row actually matches the current network,
  // taken from the backend per-condition result (each row is judged
  // individually even when the AND rule as a whole does not match).
  function condNetworkMatched(ruleIdx, i) {
    const rd = ruleDetailFor(ruleIdx);
    if (!rd) return false;
    const idx = condIndexInRule(rules[ruleIdx], i);
    if (idx < 0) return false;
    return !!rd.conditions?.[idx]?.matched;
  }
  onMount(() => {
    // The poll timer + first refresh are owned by the `open && !previewTimer`
    // reactive block above — it (re)creates them every time the modal opens,
    // which matches the component's mount-once / open-close-many lifecycle.
    cfgChangedUnsub = Events.On('config_changed', async () => {
      const name = tunnelName;
      const busy = () =>
        !open || tunnelName !== name || saveTimer !== null || pending || dragIndex !== null;
      if (!name || busy()) return;
      try {
        await saveChain;
        const s = await TunnelService.GetSettings();
        if (busy()) return;
        const disk = (s?.automation?.per_tunnel_rules || {})[name] || [];
        // Echo of our own save (the watcher fires on mtime, including writes
        // this editor made): cards already match disk, so don't reload.
        if (lastSavedFingerprint && JSON.stringify(normRules(disk)) === lastSavedFingerprint) return;
        if (!diskDiffers(disk)) return;
      } catch (e) {
        console.error('automation config_changed check:', e);
        return;
      }
      load(name);
    });
  });
  let cfgChangedUnsub = null;
  onDestroy(() => {
    previewEpoch += 1;
    if (previewAbort) { try { previewAbort.abort?.(); } catch (_) {} previewAbort = null; }
    if (previewTimer) { clearInterval(previewTimer); previewTimer = null; }
    if (editTimer) { clearTimeout(editTimer); editTimer = null; }
    if (saveTimer) { clearTimeout(saveTimer); saveTimer = null; pending = null; }
    preview = null;
    if (cfgChangedUnsub) cfgChangedUnsub();
  });
  // Disk comparison: normalize a disk rule array and the local editor state
  // into the same shape and compare, so an external edit triggers a reload.
  function normRules(rules) {
    return (rules || []).map(r => ({
      do: r.do,
      match: r.match === 'all' ? 'all' : '',
      when: (Array.isArray(r.when) ? r.when : (r.when ? [r.when] : [])).map(w => ({
        type: w?.type || inferType(w),
        ssid: (w?.ssid || '').trim(),
        subnet: (w?.subnet || '').trim(),
        mac: w?.gateway_mac ? macCanon(w?.gateway_mac) : '',
        gateway_ip: (w?.gateway_ip || '').trim(),
        interface_name: (w?.interface_name || '').trim(),
        start: (w?.start || '').trim(),
        end: (w?.end || '').trim(),
        days: Array.isArray(w?.days) ? w.days.slice() : [],
        label: w?.label || '',
      })),
    }));
  }
  function diskDiffers(disk) {
    return JSON.stringify(normRules(disk)) !== JSON.stringify(normRules(buildRules()));
  }
  // The preview frame is an ARGUMENT on purpose, not something this helper
  // reads from the enclosing scope. Svelte 5 derives a reactive statement's
  // dependency list from the identifiers that statement names DIRECTLY; a
  // helper reaching out for `preview` on its own is invisible to that scan.
  // Written as `decisionKey()` the statement below compiled to an effect
  // with an EMPTY dependency list, so it ran exactly once — at mount, while
  // preview was still empty — and the top strip stayed frozen on "no rule
  // matches this network" forever, even though the per-condition badges
  // below it (which read the same frame directly) updated correctly. Taking
  // the frame as a parameter puts `preview` back into the statement's own
  // dependency list, so the strip follows the engine again.
  // preview 这一帧刻意作为「参数」传入，而不是让本函数去外层作用域读。Svelte 5
  // 是依据响应式语句中「直接出现」的标识符来推导依赖列表的；辅助函数自己去取
  // preview 不在这次扫描范围内。写成 decisionKey() 时，下面的语句被编译成依赖
  // 列表为空的 effect，于是只在挂载时执行一次（那时 preview 还是空的），顶部条
  // 从此永远冻结在「无规则匹配当前网络」，而下方直读同一帧数据的条件徽标却是
  // 正确的。把这一帧作为参数传入，preview 就重新回到该语句自己的依赖列表里，
  // 顶部条才会重新跟随引擎结论。
  function decisionKey(pv) {
    if (!pv || !pv.decision) return 'unmanaged';
    // 'connect' | 'disconnect' | 'unmanaged' | 'manual-off' | 'manual-on'
    // | 'disabled' | 'blocked' | 'ssid-blind' | 'network-unidentified'
    // | 'backoff' — the last four are fail-closed / policy states the
    // engine enforces; they must be surfaced here so preview cannot claim a
    // connect the engine would refuse.
    return pv.decision;
  }
  // A condition's persisted type can be the legacy 'wifi' / 'ethernet'
  // spellings (kept verbatim from disk) even though the editor's dropdown
  // exposes them as 'ssid' / 'interface'. The match-count compares against
  // the dropdown vocabulary, so normalize the stored type before matching —
  // otherwise a matched Wi-Fi/SSID rule (type 'wifi') is never counted and the
  // live-matching row wrongly says "No rule matches".
  // 条件落盘的 type 可能是旧式的 'wifi' / 'ethernet'（从磁盘原样读入），而编辑器下拉框
  // 用的是 'ssid' / 'interface'。计数用下拉框那套词汇比较，因此先归一化：否则已匹配的
  // Wi-Fi/SSID 规则（type 为 'wifi'）永远计不上，living match 行会误报「没有规则匹配」。
  function condTypeNorm(type) {
    if (type === 'wifi') return 'ssid';
    if (type === 'ethernet') return 'interface';
    return type;
  }
  // How many conditions of this kind the engine reports as matching right
  // now. Counted from the backend's own per-condition verdict (preview.rules)
  // instead of from the editor's draft rows: each RuleDetail carries the
  // condition's evaluated TYPE and its matched flag, so this count can never
  // disagree with the badge on the rule card, and it depends on nothing but
  // the preview frame (a helper reading draft state here is invisible to the
  // reactive statement below, which then keeps showing a stale count).
  // 统计引擎判定为「匹配」的同类条件数量。计数取自后端自己的逐条件结论
  // （preview.rules），而非编辑器草稿行：每条 RuleDetail 都带条件的评估类型与
  // matched 标记，因此这个计数不可能与规则卡上的徽标矛盾，且只依赖 preview
  // 这一帧数据（此处若让辅助函数去读草稿状态，下面的响应式语句看不见该依赖，
  // 计数就会一直停在旧值上）。
  // Stable interface-kind labels, reused from the Routes view so the
  // automation board and the routing table speak the same short vocabulary
  // ("Wi-Fi", "Ethernet", …). Falls back to the raw kind when unknown.
  const IFACE_TYPE_KEYS = new Set([
    'wifi', 'ethernet', 'bridge', 'virtual', 'loopback', 'cellular', 'vpn',
  ]);
  function ifaceTypeLabel(type) {
    return type && IFACE_TYPE_KEYS.has(type) ? $t(`tools.route_iface_type_${type}`) : (type || '');
  }
  // A Wi-Fi adapter being up and its network name being readable are two
  // different facts: the interface list reports the link regardless of
  // whether the SSID is known, while `on_wifi` is derived from the SSID
  // text alone. The link is the broader of the two, so the header dot is
  // "on Wi-Fi" if either says so.
  // 「Wi-Fi 网卡连通」与「能读到网络名」是两个不同的事实：接口表只报链路、不关心
  // SSID 是否已知，而 on_wifi 仅由 SSID 文本推导。链路是两者中更宽的那个，因此
  // 顶部圆点只要有一方成立就判为「在 Wi-Fi 上」。
  $: wifiLinkUp = (preview?.interfaces || []).some(x => x?.is_wifi || x?.type === 'wifi');
  // The `count` on every row comes from markerSnapshot.matchCounts, which is
  // tallied from the very boolean each condition badge renders — see the
  // markerSnapshot builder. Reading `markerSnapshot` here (rather than
  // calling a helper that walks the draft again) also keeps the tally inside
  // this statement's dependency list, so it re-runs with the frame.
  // 每行的 count 取自 markerSnapshot.matchCounts，它由每个条件徽标所渲染的那个
  // 布尔值累计而来——见 markerSnapshot 的构建处。这里直接读 markerSnapshot（而不是
  // 调用某个辅助函数再去遍历一遍草稿），同时把该计数放进本语句的依赖列表里，
  // 使其随数据帧一起重算。
  $: networkFacts = [
    { key: 'interface', label: $t('automation.network_interface'), value: (preview?.interfaces || []).map(x => `${x.name}${x.type ? ' - ' + ifaceTypeLabel(x.type) : ''} (${x.active ? $t('automation.interface_in_use') : $t('automation.interface_not_in_use')})`).join(', ') || $t('automation.network_unavailable'), count: markerSnapshot?.matchCounts?.interface || 0 },
    { key: 'ssid', label: $t('automation.network_ssid'), value: preview?.ssid || (wifiLinkUp ? $t('automation.network_unavailable') : $t('automation.wifi_not_connected')), count: markerSnapshot?.matchCounts?.ssid || 0 },
    { key: 'gateway_mac', label: $t('automation.network_gateway_mac'), value: preview?.gateway_mac || $t('automation.network_unavailable'), count: markerSnapshot?.matchCounts?.network || 0 },
    { key: 'gateway_ip', label: $t('automation.network_gateway_ip'), value: preview?.gateway_ip || $t('automation.network_unavailable'), count: markerSnapshot?.matchCounts?.gateway_ip || 0 },
    { key: 'subnet', label: $t('automation.network_subnet'), value: (preview?.subnets || []).join(', ') || $t('automation.network_unavailable'), count: markerSnapshot?.matchCounts?.subnet || 0 },
  ];
</script>

<svelte:window on:keydown={(e) => e.key === 'Escape' && open && close()} />
{#if open}
  <div class="am-backdrop" on:click={close}>
    <div class="am-dialog" use:modalDrag={'.am-header'} on:click|stopPropagation role="dialog" aria-modal="true" tabindex="-1" aria-label={$t('automation.title')}>
      <div class="am-header">
        <div class="am-icon"><Icon name="wifi" size={18} strokeWidth={2} /></div>
        <div class="am-header-text">
          <h3>{$t('automation.title')}</h3>
          <p class="am-sub" title={tunnelName}>{tunnelName}</p>
        </div>
        <button class="am-close" on:click={close} aria-label="Close"><Icon name="x" size={16} strokeWidth={2} /></button>
      </div>
      {#if $moduleFlags.automation_extra}
      <details class="am-help">
        <summary>{$t('automation.help_summary')}</summary>
        <p class="am-hint">{$t('automation.hint')}</p>
      </details>
      {/if}
      <!-- Automation on/off. The stored flag is automation_disabled, so the
           checkbox is CHECKED while automation is ENABLED — otherwise the
           "on/off" label would read backwards. The trailing state text reuses
           the hero toggle's own vocabulary (tunnel.automation_on / _off) so
           the two controls say the same thing in the same words.
           While a manual latch holds, automation is paused by the user's own
           hand action — the same situation the durable off-switch would show —
           so this column switches to a "Manual control" banner carrying the
           release button instead of the checkbox. -->
      <div class="am-exclude" class:am-exclude-off={automationDisabled} class:am-exclude-manual={manualLatched}>
        {#if manualLatched}
          <div class="am-exclude-manual-row">
            <span class="am-exclude-text">{$t('tunnel.manual_control')}</span>
            <button class="am-restore" type="button" on:click={restoreAutomation} disabled={restoringOverride}
              title={$t('tunnel.restore_automation')}>{$t('tunnel.restore_automation')}</button>
          </div>
        {:else}
          <label class="am-exclude-row">
            <input type="checkbox" style="width:15px;height:15px;accent-color:var(--accent)" checked={!automationDisabled} on:change={toggleAutomationDisabled} />
            <span class="am-exclude-text">{$t('policy.automation_disabled')}</span>
            <span class="am-exclude-state">{automationDisabled ? $t('tunnel.automation_off') : $t('tunnel.automation_on')}</span>
          </label>
          <!-- The on/off hint belongs to the automation_extra family (the
               prose switch): mini keeps only the condition settings and the
               rule list, so the hint goes with the help text; the
               live-matching DATA panel has its own automation_live switch.
               开关提示隶属于 automation_extra 一族（说明文字开关）：mini 只
               保留条件设置与规则列表，该提示与帮助文字一同隐藏；实时匹配数据
               面板有独立的 automation_live 开关。 -->
          {#if $moduleFlags.automation_extra}
          <p class="am-exclude-hint">{$t('policy.automation_disabled_hint')}</p>
          {/if}
        {/if}
        <!-- No tunnel status row here on purpose: the connection state and the
             control source are tunnel-level facts that the detail page and the
             tunnel list already show, and repeating them inside the rule editor
             read as if they belonged to the rules being edited.
             刻意不在此处放隧道状态行：连接状态与控制来源是隧道级事实，详情页与通道列表
             已经显示过；在规则编辑器里再复述一遍，容易被误读成「属于正在编辑的规则」。 -->
      </div>
      {#if $moduleFlags.automation_live}
      <details class="am-info" open>
        <summary>{$t('automation.live_matching')}</summary>
        <SSIDPermissionBanner {TunnelService} />
        <!-- Live decision strip -->
        <div class="am-live">
          <span class="am-live-dot am-live-dot-{(wifiLinkUp || preview?.on_wifi) ? 'wifi' : 'wired'}"></span>
          <span class="am-live-label">{$t('automation.live_matching')}</span>
          <span class="am-live-decision">{$t(decisionLabelKey)}</span>
        </div>
        <div class="am-network-facts">
          {#each networkFacts as fact}
            <div class="am-network-fact">
              <span class="am-network-fact-label">{fact.label}</span>
              <span class="am-network-fact-value" title={fact.value}>{fact.value}</span>
              <span class="am-network-fact-separator">|</span>
              <span class:am-fact-match={fact.count > 0} class="am-network-fact-status">{fact.count > 0 ? $t('automation.network_rule_matches', { count: fact.count }) : $t('automation.network_no_rule')}</span>
            </div>
          {/each}
        </div>
      </details>
      {/if}
      <div class="am-default-state" class:am-default-won={defaultRuleWon}>
        <span class="am-default-label">{$t('automation.default_state')}</span>
        <div class="am-default-opts" role="radiogroup" aria-label={$t('automation.default_state')}>
          <button type="button" role="radio" class="am-default-opt am-default-opt-connect"
            class:on={defaultState === 'connect'}
            class:am-effective={defaultRuleWon && defaultState === 'connect'}
            title={defaultRuleWon && defaultState === 'connect' ? $t('automation.default_in_effect') : ''}
            aria-checked={defaultState === 'connect'} on:click={() => setDefaultState('connect')}>{$t('automation.action_connect')}</button>
          <button type="button" role="radio" class="am-default-opt am-default-opt-disconnect"
            class:on={defaultState === 'disconnect'}
            class:am-effective={defaultRuleWon && defaultState === 'disconnect'}
            title={defaultRuleWon && defaultState === 'disconnect' ? $t('automation.default_in_effect') : ''}
            aria-checked={defaultState === 'disconnect'} on:click={() => setDefaultState('disconnect')}>{$t('automation.action_disconnect')}</button>
        </div>
        <span class="am-default-hint">{$t('automation.default_when_no_match', { state: $t(defaultState === 'connect' ? 'automation.action_connect' : 'automation.action_disconnect') })}</span>
      </div>
      <div class="am-rules-wrap">
        <div class="am-fade am-fade-top" class:show={canScrollUp}>
          <span class="am-chevron am-chevron-up"><Icon name="chevron-down" size={15} strokeWidth={2.5} /></span>
        </div>
        <div class="am-rules" bind:this={rulesEl} on:scroll={updateScroll}>
          <div class="am-group">
            <div class="am-group-header">
              <span class="am-group-title">{$t('automation.rules_title')}</span>
              <button class="am-add-rule" on:click={addRule}>
                <Icon name="plus" size={12} strokeWidth={2.25} /> {$t('automation.add_rule')}
              </button>
            </div>
            {#if rules.length === 0}
              <div class="am-empty am-group-empty">{$t('automation.empty_rules')}</div>
            {:else}
              {#each rules as rule, ruleIdx (rule._gid)}
                <div class="am-rule" class:am-rule-won={markerSnapshot.rules[rule._gid] === true}
                  class:am-rule-dragging={ruleDragIdx === ruleIdx}
                  on:dragover={(e) => onRuleDragOver(e, ruleIdx)}
                  on:dragend={onRuleDragEnd}>
                  <div class="am-rule-head">
                    <span class="am-handle am-rule-handle" draggable="true" title={$t('automation.rule_drag_hint')}
                      on:dragstart={(e) => onRuleDragStart(e, ruleIdx)}>⋮⋮</span>
                    <span class="am-rule-num">{$t('automation.rule')} {ruleIdx + 1}</span>
                    <button type="button" class="am-do-badge am-do-{rule.do}" class:am-do-won={markerSnapshot.rules[rule._gid] === true} on:click={() => toggleRuleDo(rule)}
                      title={$t('automation.action_toggle_hint')}>{$t(rule.do === 'connect' ? 'automation.action_connect' : 'automation.action_disconnect')}</button>
                    <span class="am-match-badge" title={$t('automation.group_all_hint')}>{$t('automation.match_all')}</span>
                    <button class="am-remove-rule" on:click={() => removeRule(ruleIdx)} title={$t('automation.remove_rule')} aria-label="remove rule"><Icon name="x" size={13} strokeWidth={2} /></button>
                  </div>
                  {#if rule.conds.length === 0}
                    <div class="am-empty am-rule-empty">{$t('automation.rule_empty')}</div>
                  {:else}
                    {#each rule.conds as c, i (c._id)}
                      {@const marker = markerSnapshot.conditions[c._id] || { match: false, active: false }}
                      {@const net = marker.match}
                      {@const use = marker.active}
                      <div class="am-cond" class:am-dragging={dragIndex === i && dragRule === ruleIdx}
                        on:dragover={(e) => onCondDragOver(e, ruleIdx, i)}
                        on:dragend={onDragEnd}>
                        <span class="am-handle" draggable="true" title={$t('automation.drag_hint')}
                          on:dragstart={(e) => onDragStart(e, ruleIdx, i)}>⋮⋮</span>
                          <select class="am-type" value={c.type === 'wifi' ? 'ssid' : (c.type === 'ethernet' ? 'interface' : c.type)} on:change={(e) => onTypeChange(c, e.target.value)} aria-label={$t('automation.condition')}>
                          <option value="network">{$t('automation.cond_network')}</option>
                          <option value="subnet">{$t('automation.cond_subnet')}</option>
                          <option value="ssid">{$t('automation.cond_ssid')}</option>
                          <option value="gateway_ip">{$t('automation.cond_gateway_ip')}</option>
                          <option value="interface">{$t('automation.cond_interface')}</option>
                          <option value="time">{$t('automation.cond_time')}</option>
                        </select>
                        {#if c.type === 'network'}
                          <input
                            class="am-val" class:am-invalid={macInvalid(c.gateway_mac)}
                            list="am-mac-list"
                            placeholder={currentGatewayMAC || $t('automation.mac_placeholder')}
                            title={macInvalid(c.gateway_mac) ? $t('automation.mac_invalid') : ''}
                            bind:value={c.gateway_mac}
                            on:input={() => { save(); schedulePreviewRefresh(); }} on:change={() => { onMacChange(c); schedulePreviewRefresh(); }} />
                          <span class="am-cond-note">{$t('automation.cond_network_desc')}</span>
                        {:else if c.type === 'subnet'}
                          <input
                            class="am-val" class:am-invalid={cidrInvalid(c.subnet)}
                            list="am-subnet-list"
                            placeholder={currentSubnets[0] || '192.168.0.0/24'}
                            title={cidrInvalid(c.subnet) ? $t('automation.subnet_invalid') : ''}
                            bind:value={c.subnet}
                            on:input={() => { save(); schedulePreviewRefresh(); }} on:change={() => { save(); schedulePreviewRefresh(); }} />
                        {:else if c.type === 'ssid' || c.type === 'wifi'}
                          <input
                            class="am-val"
                            list="am-ssid-list"
                            placeholder={$t('automation.ssid_select_hint')}
                            bind:value={c.ssid}
                            on:input={() => { c.type = 'ssid'; save(); schedulePreviewRefresh(); }}
                            on:change={() => { c.type = 'ssid'; save(); schedulePreviewRefresh(); }}
                            aria-label={$t('automation.cond_ssid')} />
                        {:else if c.type === 'gateway_ip'}
                          <input
                            class="am-val" class:am-invalid={gatewayIPInvalid(c.gateway_ip)}
                            list="am-gwip-list"
                            placeholder={preview?.gateway_ip || '192.168.0.1'}
                            title={gatewayIPInvalid(c.gateway_ip) ? $t('automation.ip_invalid') : ''}
                            bind:value={c.gateway_ip}
                            on:input={() => { save(); schedulePreviewRefresh(); }}
                            on:change={() => { save(); schedulePreviewRefresh(); }}
                          />
                        {:else if c.type === 'interface' || c.type === 'ethernet'}
                          <select class="am-val" value={c.interface_name}
                            on:change={(e) => { c.type = 'interface'; c.interface_name = e.target.value; rules = rules; save(); schedulePreviewRefresh(); }}
                            aria-label={$t('automation.cond_interface')}>
                            {#each physicalInterfaces as i (i.index)}<option value={i.name}>{ifaceLabel(i, $t)}</option>{/each}
                          </select>
                        {:else if c.type === 'time'}
                          <div class="am-time">
                            <input type="time" class="am-clock" aria-label={$t('automation.time_start')} bind:value={c.start} on:change={() => { save(); schedulePreviewRefresh(); }} />
                            <span class="am-time-dash">–</span>
                            <input type="time" class="am-clock" aria-label={$t('automation.time_end')} bind:value={c.end} on:change={() => { save(); schedulePreviewRefresh(); }} />
                            <div class="am-days" role="group" aria-label={$t('automation.days_label')}>
                              {#each [0, 1, 2, 3, 4, 5, 6] as d}
                                <button type="button" class="am-day" class:on={(c.days || []).includes(d)} on:click={() => toggleDay(c, d)}>{$t('automation.day_' + d)}</button>
                              {/each}
                            </div>
                          </div>
                        {:else}
                          <span class="am-val am-val-none"></span>
                        {/if}
                        <span
                          class="am-live-cond"
                          title={net ? (use ? $t('automation.status_active') : $t('automation.status_shadowed')) : $t('automation.status_nomatch')}>
                          <span class="am-badge am-badge-match" class:am-yes={net} class:am-no={!net}>{$t(net ? 'automation.label_match' : 'automation.label_no_match')}</span>
                          <span class="am-badge am-badge-use" class:am-yes={use} class:am-no={!use}>{$t(use ? 'automation.label_in_use' : 'automation.label_not_in_use')}</span>
                        </span>
                        <button class="am-remove" on:click={() => removeCond(ruleIdx, i)} aria-label="remove condition"><Icon name="x" size={12} strokeWidth={2} /></button>
                      </div>
                    {/each}
                  {/if}
                  <button class="am-add-cond" on:click={() => addCond(ruleIdx)} disabled={rule.conds.length >= MAX_CONDS}>
                    <Icon name="plus" size={12} strokeWidth={2.25} /> {$t('automation.add_condition')}
                  </button>
                </div>
              {/each}
            {/if}
          </div>
          <p class="am-priority-note">{$t('automation.priority_note')}</p>
        </div>
        <div class="am-fade am-fade-bottom" class:show={canScrollDown}>
          <span class="am-chevron"><Icon name="chevron-down" size={15} strokeWidth={2.5} /></span>
        </div>
      </div>
      <datalist id="am-subnet-list">
        {#each currentSubnets as sn}<option value={sn}></option>{/each}
      </datalist>
      <datalist id="am-mac-list">
        {#if currentGatewayMAC}<option value={currentGatewayMAC}></option>{/if}
      </datalist>
      <datalist id="am-ssid-list">
        {#each ssidSuggestions as ssid}<option value={ssid}></option>{/each}
      </datalist>
      <datalist id="am-gwip-list">
        {#if preview?.gateway_ip}<option value={preview.gateway_ip}></option>{/if}
      </datalist>
      <datalist id="am-iface-list">
        {#each interfaceSuggestions as name}<option value={name}></option>{/each}
      </datalist>
      {#if saveError}<div class="am-error">{saveError}</div>{/if}
    </div>
  </div>
{/if}
<style>
  .am-backdrop {
    position: fixed; inset: 0; z-index: 1000;
    background: color-mix(in srgb, #000 45%, transparent);
    display: flex; align-items: center; justify-content: center;
    padding: 24px;
  }
  .am-dialog {
    width: 580px; max-width: calc(100vw - 48px); height: 600px; max-height: calc(100vh - 48px);
    display: flex; flex-direction: column;
    overflow-y: auto;
    resize: both;
    min-width: 480px;
    min-height: 420px;
    background: var(--bg-elevated, var(--bg-secondary));
    border: 1px solid var(--border);
    border-radius: 14px; padding: 20px;
    box-shadow: 0 16px 48px rgba(0,0,0,0.35);
  }
  .am-help, .am-info { flex-shrink: 0; }
  .am-help > summary, .am-info > summary {
    cursor: pointer; list-style: none; color: var(--text-primary);
    font: 600 12px/1.3 var(--font-sans); padding: 2px 0;
  }
  .am-help > summary::-webkit-details-marker, .am-info > summary::-webkit-details-marker { display: none; }
  .am-help > summary::after, .am-info > summary::after { content: '+'; float: right; color: var(--text-muted); font-size: 14px; }
  .am-help[open] > summary::after, .am-info[open] > summary::after { content: '−'; }
  .am-header { display: flex; align-items: center; gap: 12px; flex-shrink: 0; cursor: move; user-select: none; touch-action: none; }
  .am-icon {
    width: 36px; height: 36px; border-radius: 9px; flex-shrink: 0;
    display: flex; align-items: center; justify-content: center;
    background: color-mix(in srgb, var(--accent) 15%, transparent);
    color: var(--accent);
  }
  .am-header-text { flex: 1; min-width: 0; }
  .am-header-text h3 { margin: 0; font: 600 15px/1.2 var(--font-sans); color: var(--text-primary); }
  .am-sub { margin: 2px 0 0; font: 400 12px/1.2 var(--font-mono); color: var(--text-muted); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .am-close { background: transparent; border: 0; color: var(--text-muted); cursor: pointer; padding: 4px; border-radius: 6px; }
  .am-close:hover { background: var(--bg-hover); color: var(--text-primary); }
  .am-hint { margin: 8px 0 0; font: 400 12px/1.5 var(--font-sans); color: var(--text-secondary); flex-shrink: 0; }
  .am-live {
    flex-shrink: 0; display: flex; align-items: center; gap: 8px;
    margin: 10px 0 4px; padding: 7px 10px;
    background: color-mix(in srgb, var(--bg-primary) 70%, transparent);
    border: 1px solid var(--border); border-radius: 8px;
    font: 400 11px/1.3 var(--font-sans); color: var(--text-secondary);
  }
  .am-live-dot { width: 8px; height: 8px; border-radius: 50%; flex-shrink: 0; background: var(--text-muted); }
  .am-live-dot-connect { background: var(--green, #34c759); }
  .am-live-dot-disconnect { background: var(--orange, #ff9f0a); }
  .am-live-dot-manual-off { background: var(--orange, #ff9f0a); }
  .am-live-dot-unmanaged { background: var(--text-muted); }
  .am-live-dot-wifi { background: var(--green, #34c759); }
  .am-live-dot-wired { background: var(--accent); }
  .am-live-network { font-family: var(--font-mono); color: var(--text-primary); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .am-live-decision { margin-left: auto; color: var(--text-muted); white-space: nowrap; }
  .am-network-facts {
    display: grid; grid-template-columns: minmax(0, 1fr); gap: 4px;
    margin: 0 0 6px; padding: 6px 8px;
    background: color-mix(in srgb, var(--bg-primary) 55%, transparent);
    border: 1px solid var(--border); border-radius: 7px;
  }
  .am-network-fact { min-width: 0; display: flex; align-items: baseline; gap: 6px; }
  .am-network-fact-label { color: var(--text-muted); font: 500 10px/1.2 var(--font-sans); }
  .am-network-fact-value { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; color: var(--text-primary); font: 500 10px/1.2 var(--font-mono); }
  .am-network-fact-separator { color: var(--border); flex-shrink: 0; }
  .am-network-fact-status { color: var(--text-muted); font: 400 9px/1.2 var(--font-sans); white-space: nowrap; }
  .am-network-fact-status.am-fact-match { color: var(--green, #34c759); }
  .am-rules-wrap { flex: none; position: relative; display: flex; }
  .am-rules { flex: 1; display: flex; flex-direction: column; gap: 10px; margin: 2px 0 0; padding-right: 4px; }
  .am-fade {
    position: absolute; left: 0; right: 4px; height: 52px; pointer-events: none;
    opacity: 0; z-index: 2;
    display: flex; justify-content: center;
  }
  @media (prefers-reduced-motion: no-preference) {
    .am-fade { transition: opacity 140ms ease; }
    .am-chevron { animation: am-bob 1.4s ease-in-out infinite; }
  }
  .am-fade.show { opacity: 1; }
  .am-fade-top {
    top: 0; align-items: flex-start; padding-top: 2px;
    background: linear-gradient(to bottom,
      var(--bg-elevated, var(--bg-secondary)) 0%,
      color-mix(in srgb, var(--bg-elevated, var(--bg-secondary)) 88%, transparent) 45%,
      transparent 100%);
  }
  .am-fade-bottom {
    bottom: 0; align-items: flex-end; padding-bottom: 2px;
    background: linear-gradient(to top,
      var(--bg-elevated, var(--bg-secondary)) 0%,
      color-mix(in srgb, var(--bg-elevated, var(--bg-secondary)) 88%, transparent) 45%,
      transparent 100%);
  }
  .am-chevron { color: var(--accent); display: inline-flex; }
  .am-chevron-up { transform: rotate(180deg); }
  @keyframes am-bob {
    0%, 100% { transform: translateY(0); }
    50% { transform: translateY(3px); }
  }
  .am-chevron-up { animation: am-bob-up 1.4s ease-in-out infinite; }
  @keyframes am-bob-up {
    0%, 100% { transform: rotate(180deg) translateY(0); }
    50% { transform: rotate(180deg) translateY(3px); }
  }
  .am-group { display: flex; flex-direction: column; gap: 4px; }
  .am-group-header {
    display: flex; align-items: center; gap: 8px;
    padding: 1px 0 3px; border-bottom: 1px solid color-mix(in srgb, var(--border) 60%, transparent);
  }
  .am-group-dot { width: 8px; height: 8px; border-radius: 50%; flex-shrink: 0; background: var(--text-muted); }
  .am-group-dot-connect.am-group-matched { background: var(--green, #34c759); }
  .am-group-dot-disconnect.am-group-matched { background: var(--orange, #ff9f0a); }
  .am-group-title { font: 600 12px/1.2 var(--font-sans); color: var(--text-primary); }
  .am-add-rule {
    display: inline-flex; align-items: center; gap: 4px; margin-left: auto;
    font: 500 11px var(--font-sans); color: var(--accent);
    background: transparent; border: 1px dashed color-mix(in srgb, var(--accent) 45%, transparent);
    border-radius: 7px; padding: 4px 8px; cursor: pointer; flex-shrink: 0;
  }
  .am-add-rule:hover { background: color-mix(in srgb, var(--accent) 10%, transparent); }
  .am-rule {
    display: flex; flex-direction: column; gap: 3px;
    padding: 6px 7px 5px;
    background: color-mix(in srgb, var(--bg-primary) 55%, transparent);
    border: 1px solid var(--border); border-radius: 10px;
    transition: box-shadow 120ms ease, border-color 120ms ease;
  }
  .am-rule-won {
    border-color: color-mix(in srgb, var(--accent) 50%, var(--border));
    box-shadow: 0 0 0 1px color-mix(in srgb, var(--accent) 30%, transparent);
  }
  .am-rule-head { display: flex; align-items: center; gap: 8px; }
  .am-rule-num {
    font: 600 10px/1 var(--font-sans); color: var(--text-muted);
    text-transform: uppercase; letter-spacing: 0.05em; flex-shrink: 0;
  }
  .am-match-badge {
    margin-left: auto;
    display: inline-flex; align-items: center; flex-shrink: 0;
    font: 500 10px/1 var(--font-sans); color: var(--text-muted);
    padding: 4px 9px;
    background: color-mix(in srgb, var(--bg-primary) 65%, transparent);
    border: 1px solid var(--border);
    border-radius: 7px;
  }
  .am-remove-rule { background: transparent; border: 0; color: var(--text-muted); cursor: pointer; padding: 4px; border-radius: 6px; flex-shrink: 0; }
  .am-remove-rule:hover { background: color-mix(in srgb, var(--red, #ff3b30) 18%, transparent); color: var(--red, #ff3b30); }
  /* Default State control (no-rule-match fallback state) */
  .am-default-state {
    flex-shrink: 0; display: flex; align-items: center; gap: 8px; flex-wrap: wrap;
    margin: 8px 0 0; padding: 7px 10px;
    background: color-mix(in srgb, var(--bg-primary) 70%, transparent);
    border: 1px solid var(--border); border-radius: 8px;
  }
  .am-default-label { font: 600 11px/1.3 var(--font-sans); color: var(--text-primary); flex-shrink: 0; }
  .am-default-opts { display: inline-flex; border: 1px solid var(--border); border-radius: 7px; overflow: hidden; flex-shrink: 0; }
  .am-default-opt {
    font: 500 11px/1 var(--font-sans); color: var(--text-secondary);
    background: transparent; border: 0; padding: 5px 10px; cursor: pointer;
  }
  .am-default-opt + .am-default-opt { border-left: 1px solid var(--border); }
  .am-default-opt-connect.on { background: color-mix(in srgb, var(--green, #34c759) 22%, transparent); color: var(--green, #34c759); font-weight: 600; }
  .am-default-opt-disconnect.on { background: color-mix(in srgb, var(--orange, #ff9f0a) 22%, transparent); color: var(--orange, #ff9f0a); font-weight: 600; }
  /* Outer ring = "the Default State is what actually ran". It reuses the very
     frame a winning rule card gets (.am-rule-won), so the default and the
     rules speak one visual language. It lives on the ROW, not on the option
     buttons: the option row is a segmented control with overflow:hidden,
     which clips any outward box-shadow — that is why the marker could never
     be seen. It lights only on a true fallback (no rule matched), so a rule
     that matched and happens to do the same action lights its own card
     instead; the two rings are never on at once. */
  .am-default-state.am-default-won {
    border-color: color-mix(in srgb, var(--accent) 50%, var(--border));
    box-shadow: 0 0 0 1px color-mix(in srgb, var(--accent) 30%, transparent);
  }
  /* Inner ring on the option that is in effect, drawn in that option's OWN
     colour (green connect / orange disconnect). Inset, because the segmented
     control clips anything drawn outside the button. */
  .am-default-opt.am-effective {
    box-shadow: inset 0 0 0 2px color-mix(in srgb, currentColor 50%, transparent);
  }
  /* Restore button beside the manual-latch chip — the editor's own copy of
     the action the tunnel list and the detail page offer, so the release is
     reachable from the screen that explains WHY the rules are not
     running. Pinned to the row's right edge; wraps below on narrow
     dialogs. */
  .am-restore {
    flex-shrink: 0;
    margin-left: auto;
    font: 600 10px/1 var(--font-sans);
    padding: 4px 9px;
    border-radius: 999px;
    border: 1px solid color-mix(in srgb, var(--accent) 45%, transparent);
    background: color-mix(in srgb, var(--accent) 12%, transparent);
    color: var(--accent);
    cursor: pointer;
  }
  .am-restore:hover:not(:disabled) {
    background: color-mix(in srgb, var(--accent) 20%, transparent);
  }
  .am-restore:disabled {
    opacity: 0.55;
    cursor: default;
  }
  .am-default-hint { font: 400 10px/1.3 var(--font-sans); color: var(--text-muted); margin-left: auto; text-align: right; }
  /* Rule-level drag affordances */
  .am-rule-handle { flex-shrink: 0; cursor: grab; }
  .am-rule-dragging { opacity: 0.55; }
  .am-do-badge {
    flex-shrink: 0; font: 600 10px/1 var(--font-sans);
    padding: 4px 9px; border-radius: 7px; cursor: pointer; border: 1px solid var(--border);
  }
  .am-do-connect { color: var(--green, #34c759); background: color-mix(in srgb, var(--green, #34c759) 12%, transparent); }
  .am-do-disconnect { color: var(--orange, #ff9f0a); background: color-mix(in srgb, var(--orange, #ff9f0a) 12%, transparent); }
  /* Outer ring on the winning rule's action button, matching the badge's own
     color, to emphasize which rule is currently in effect. */
  .am-do-badge.am-do-won { box-shadow: 0 0 0 2px color-mix(in srgb, currentColor 45%, transparent); }
  .am-rule-or {
    text-align: center; font: 600 10px/1 var(--font-sans); color: var(--text-muted);
    letter-spacing: 0.08em; margin: 0; flex-shrink: 0;
  }
  .am-rule-empty { margin: 0 0 2px; }
  .am-rule .am-add-cond { align-self: flex-start; margin-top: 1px; }
  .am-group-empty { margin: 1px 0; }
  .am-empty { padding: 12px; text-align: center; font: 400 11px var(--font-sans); color: var(--text-muted); border: 1px dashed var(--border); border-radius: 8px; }
  .am-cond {
    display: flex; align-items: center; gap: 5px; flex-wrap: wrap;
    padding: 4px 4px 3px; border: 1px solid transparent; border-radius: 8px;
  }
  .am-cond.am-dragging { opacity: 0.35; }
  @media (prefers-reduced-motion: no-preference) {
    .am-cond { transition: opacity 120ms ease; }
  }
  .am-handle {
    cursor: grab; color: var(--text-muted); font: 700 12px/1 var(--font-sans);
    letter-spacing: -2px; padding: 0 2px; user-select: none; flex-shrink: 0;
  }
  .am-handle:active { cursor: grabbing; }
  .am-cond select, .am-cond input {
    font: 400 12px var(--font-sans); color: var(--text-primary);
    background: var(--bg-primary); border: 1px solid var(--border);
    border-radius: 6px; padding: 3px 6px;
  }
  .am-val { flex: 1; min-width: 120px; }
  .am-val-none { color: var(--text-muted); border: 0 !important; background: transparent !important; }
  .am-cond-note { display: block; margin-top: 4px; font-size: 11px; line-height: 1.4; color: var(--text-muted); }
  .am-time {
    flex: 1; min-width: 120px;
    display: flex; align-items: center; gap: 4px; flex-wrap: wrap;
  }
  .am-clock {
    font: 400 12px var(--font-sans); color: var(--text-primary);
    background: var(--bg-primary); border: 1px solid var(--border);
    border-radius: 7px; padding: 4px 5px;
  }
  .am-clock::-webkit-calendar-picker-indicator { opacity: 0.6; }
  .am-time-dash { color: var(--text-muted); font-size: 12px; }
  .am-days { display: inline-flex; gap: 3px; flex-wrap: wrap; }
  .am-day {
    appearance: none; -webkit-appearance: none;
    min-width: 20px; height: 20px; border-radius: 5px;
    font: 600 10px/1 var(--font-sans); color: var(--text-muted);
    background: color-mix(in srgb, var(--bg-primary) 70%, transparent);
    border: 1px solid var(--border); cursor: pointer; padding: 0 3px;
    transition: background 120ms ease, color 120ms ease;
  }
  .am-day:hover { color: var(--text-primary); }
  .am-day.on { background: var(--accent); color: #fff; border-color: var(--accent); }
  .am-cond input.am-invalid {
    border-color: var(--error-text, #ff453a);
    background: color-mix(in srgb, var(--error-text, #ff453a) 8%, var(--bg-primary));
  }
  .am-live-cond {
    flex-shrink: 0;
    display: inline-flex; align-items: center; gap: 3px;
  }
  .am-badge {
    font: 500 9px/1 var(--font-sans);
    padding: 2px 5px;
    border-radius: 4px;
    white-space: nowrap;
  }
  .am-badge.am-yes { color: #fff; background: var(--green, #34c759); }
  .am-badge.am-no { color: var(--text-muted); background: color-mix(in srgb, var(--text-muted) 14%, transparent); }
  /* The "in use" badge uses the accent colour so it reads differently from
     the green "match" badge when both are lit: match = the condition fits
     now; in-use = this condition's rule is the one the engine is applying.
     Neither depends on the tunnel being physically up.
     「使用中」徽章使用强调色，以便与绿色「匹配」徽章同时点亮时区分开来：
     match = 当前条件匹配；in-use = 该条件所属规则正是引擎正在应用的那条。
     两者都与隧道是否真正连通无关。 */
  .am-badge-use.am-yes { color: #fff; background: var(--accent); }
  .am-badge-match.am-no { color: #fff; background: color-mix(in srgb, var(--red, #ff3b30) 72%, transparent); }
  .am-remove { background: transparent; border: 0; color: var(--text-muted); cursor: pointer; padding: 4px; border-radius: 6px; flex-shrink: 0; }
  .am-remove:hover { background: color-mix(in srgb, var(--red, #ff3b30) 18%, transparent); color: var(--red, #ff3b30); }
  .am-priority-note { margin: 0; font: 400 10px/1.4 var(--font-sans); color: var(--text-muted); text-align: center; flex-shrink: 0; }
  .am-error { margin-top: 8px; font: 400 12px var(--font-sans); color: var(--error-text, #ff453a); flex-shrink: 0; }
  /* Automation on/off switch — sits at the top of the editor so the durable
     opt-out and the hero toggle are the same control, shown in the two places
     the user reaches for it (no second, out-of-sync copy). The checkbox is
     checked while automation is ENABLED; the trailing state text borrows the
     hero's wording so both controls read the same. */
  .am-exclude {
    flex-shrink: 0; display: flex; flex-direction: column; gap: 4px;
    margin: 6px 0 0; padding: 8px 10px;
    background: color-mix(in srgb, var(--bg-primary) 70%, transparent);
    border: 1px solid var(--border); border-radius: 8px;
  }
  .am-exclude-off { border-color: color-mix(in srgb, var(--text-muted) 55%, transparent); }
  .am-exclude-row { display: flex; align-items: center; gap: 8px; cursor: pointer; }
  .am-exclude-text { font: 600 12px/1.3 var(--font-sans); color: var(--text-primary); }
  .am-exclude-state {
    margin-left: auto; font: 400 11px/1.3 var(--font-sans);
    color: var(--text-muted);
  }
  .am-exclude-off .am-exclude-state { color: var(--text-muted); font-weight: 600; }
  .am-exclude-hint { font: 400 11px/1.45 var(--font-sans); color: var(--text-muted); margin: 2px 0 0; }
  /* Manual latch occupies the on/off column: automation is paused by the
     user's own hand action, so the column shows a "Manual control" banner
     with the release button instead of the durable switch. */
  .am-exclude-manual { background: color-mix(in srgb, var(--amber, #ff9f0a) 14%, transparent); border-color: color-mix(in srgb, var(--amber, #ff9f0a) 45%, transparent); }
  .am-exclude-manual-row { display: flex; align-items: center; gap: 8px; }
  .am-exclude-manual .am-exclude-text { color: color-mix(in srgb, var(--amber, #ff9f0a) 75%, var(--text-primary)); }
</style>
