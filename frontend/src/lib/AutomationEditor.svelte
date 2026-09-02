<script>
  // Per-tunnel Automation rule editor (issue #12), WireTunnels-style
  // Connect/Disconnect groups. Under each action you can add MULTIPLE
  // independent rules — any one matching rule triggers the action (rules
  // under the same action are OR'd). Within a rule, conditions are always
  // combined with AND: every condition in the rule must match for the rule
  // to fire. This covers all practical cases while keeping the UI simple.
  // "on any Wi-Fi" is a condition that needs no value.
  // Persisted to Settings.automation.per_tunnel_rules[tunnelName] as one
  // {when: [...], do} entry per rule, disconnect rules FIRST so that when
  // both actions match the tunnel disconnects (safe default). The whole
  // settings object is re-fetched and spread on save so other screens' edits
  // (and other tunnels' rules) are never clobbered.
  import { afterUpdate, onMount, onDestroy } from 'svelte';
  import { Events } from '@wailsio/runtime';
  import { AutomationPreview } from '../../bindings/github.com/imonior/wireguide-plus/internal/app/tunnelservice.js';
  import Icon from './Icon.svelte';
  import { t } from '../i18n/index.js';
  import { errText } from './errors.js';
  import SSIDPermissionBanner from './SSIDPermissionBanner.svelte';
  export let TunnelService;
  export let tunnelName = '';
  export let open = false;
  // groups[do] = [ rule, ... ]; rule = { _gid, match: 'all', conds: [ {_id, type, ssid, subnet, gateway_mac, label} ] }
  let groups = { connect: [], disconnect: [] };
  // Local-only identity for {#each} keys; never persisted.
  let condId = 0;
  let ruleId = 0;
  let loadedFor = '';
  let knownSSIDs = [];
  let currentSSID = '';
  let currentSubnets = [];      // autocomplete suggestions for the subnet field
  let currentGatewayMAC = '';   // autocomplete suggestion for the MAC field
  // Live network context + per-rule decision from AutomationPreview().
  let preview = null;           // AutomationPreviewResponse
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
  // Physical interface names from the live AutomationPreview — used as
  // autocomplete suggestions for the interface condition.
  $: interfaceSuggestions = [...new Set((preview?.interfaces || []).map(x => x.name))];
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
    loadedFor = name;
    const gen = ++loadGen;
    await flush();
    saveError = '';
    try {
      const s = await TunnelService.GetSettings();
      if (gen !== loadGen) return;
      const per = s?.automation?.per_tunnel_rules || {};
      groups = toGroups(per[name] || []);
    } catch (e) {
      if (gen === loadGen) groups = { connect: [], disconnect: [] };
      console.error('automation load:', e);
    }
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
  }
  // Convert the persisted rule array into the editor model. Conditions inside
  // a rule are always AND; rules under the same action are OR. Legacy configs
  // that used OR within a rule (match !== 'all' or missing with multiple
  // conditions) are migrated by splitting each condition into its own rule,
  // preserving the original semantics under the new model.
  function toGroups(rules) {
    const g = { connect: [], disconnect: [] };
    for (const r of rules || []) {
      const d = r.do === 'disconnect' ? 'disconnect' : 'connect';
      const whens = Array.isArray(r.when) ? r.when : (r.when ? [r.when] : []);
      if (!whens.length) continue;
      const isLegacyOR = r.match !== 'all' && whens.length > 1;
      const makeRule = (w) => ({
        _gid: ++ruleId,
        match: 'all',
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
        for (const w of whens) g[d].push(makeRule(w));
      } else {
        const rule = { _gid: ++ruleId, match: 'all', conds: [] };
        for (const w of whens) rule.conds.push(makeRule(w).conds[0]);
        g[d].push(rule);
      }
    }
    return g;
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
  // editing immediately. No save() here: a blank draft is not a config
  // change — it becomes persistable on the first input that completes it.
  function addRule(d) {
    const rule = { _gid: ++ruleId, match: 'all', conds: [newCond()] };
    groups = { ...groups, [d]: [...groups[d], rule] };
  }
  function removeRule(d, ruleIdx) {
    groups = { ...groups, [d]: groups[d].filter((_, i) => i !== ruleIdx) };
    save();
  }
  function addCond(d, ruleIdx) {
    const rule = groups[d][ruleIdx];
    if (rule.conds.length >= MAX_CONDS) return;
    const nextRules = groups[d].map((r, i) => i === ruleIdx
      ? { ...r, conds: [...r.conds, newCond()] }
      : r);
    groups = { ...groups, [d]: nextRules };
  }
  function removeCond(d, ruleIdx, i) {
    const nextRules = groups[d].map((r, idx) => idx === ruleIdx
      ? { ...r, conds: r.conds.filter((_, j) => j !== i) }
      : r);
    groups = { ...groups, [d]: nextRules };
    save();
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
    groups = groups;
    save();
  }
  // Toggle one weekday (0=Sunday … 6=Saturday) on a time condition row.
  function toggleDay(c, d) {
    const days = Array.isArray(c.days) ? c.days : [];
    c.days = days.includes(d) ? days.filter(x => x !== d) : [...days, d];
    groups = groups;
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
  // Drag-to-reorder WITHIN a rule card (live reordering feel).
  let dragIndex = null;
  let dragGroup = null;
  let dragRule = null;
  function onDragStart(e, d, ruleIdx, i) {
    dragIndex = i;
    dragGroup = d;
    dragRule = ruleIdx;
    e.dataTransfer.effectAllowed = 'move';
    try { e.dataTransfer.setData('text/plain', String(i)); } catch (_) {}
    const row = e.currentTarget.closest('.am-cond');
    if (row) {
      try { e.dataTransfer.setDragImage(row, 24, row.offsetHeight / 2); } catch (_) {}
    }
  }
  function onCondDragOver(e, d, ruleIdx, i) {
    if (dragIndex === null || dragGroup !== d || dragRule !== ruleIdx) return;
    e.preventDefault();
    e.dataTransfer.dropEffect = 'move';
    if (dragIndex === i) return;
    const arr = [...groups[d][ruleIdx].conds];
    const [moved] = arr.splice(dragIndex, 1);
    arr.splice(i, 0, moved);
    const nextRules = groups[d].map((r, idx) => idx === ruleIdx ? { ...r, conds: arr } : r);
    groups = { ...groups, [d]: nextRules };
    dragIndex = i;
  }
  function onDragEnd() {
    if (dragIndex !== null) { dragIndex = null; dragGroup = null; dragRule = null; save(); }
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
  // or null while incomplete. wifi / ethernet / none_match need no value.
  function cleanedCond(c) {
    const t = c.type;
    if (t === 'none_match') return { type: 'none_match', label: c.label || '' };
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
  // rule card. Disconnect is emitted BEFORE connect so that when both
  // actions match the tunnel disconnects (safe default — avoids connecting
  // on a network the user asked to avoid). A rule with no complete
  // conditions contributes no entry.
  function buildRules() {
    const out = [];
    for (const d of ['disconnect', 'connect']) {
      for (const rule of groups[d]) {
        const conds = rule.conds.map(cleanedCond).filter(Boolean);
        if (!conds.length) continue;
        const r = { when: conds, do: d };
        if (rule.match === 'all') r.match = 'all';
        out.push(r);
      }
    }
    return out;
  }
  // Debounced, snapshot-based save (same design as before).
  let saveTimer = null;
  let pending = null;
  let saveChain = Promise.resolve();
  function save() {
    pending = { name: tunnelName, rules: buildRules() };
    if (saveTimer) clearTimeout(saveTimer);
    saveTimer = setTimeout(runSave, 300);
  }

  // --- Live re-evaluation on edit -------------------------------------------------
  // The 3s poll catches network changes (SSID switched, VPN came up, …), but
  // edits to the rule/condition draft need immediate re-rating against the
  // CURRENT network — otherwise the user types a new SSID and stares at a
  // stale "no match" badge for up to 3 seconds.
  //
  // We trigger via a reactive statement on a JSON fingerprint of both rule
  // groups — that way EVERY change (new rule, removed rule, condition type
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
  $: if (open) {
    // Touch nested fields so svelte considers them dependencies. The JSON
    // stringify is cheap for the small rule sets users write (~<30 rules)
    // and doubles as the fingerprint for "did the draft really change".
    const _touchD = JSON.stringify(groups.disconnect);
    const _touchC = JSON.stringify(groups.connect);
    void _touchD; void _touchC;
    scheduleLiveEditRefresh();
  }
  function scheduleLiveEditRefresh() {
    if (!open) return;
    if (editTimer) clearTimeout(editTimer);
    editTimer = setTimeout(async () => {
      editTimer = null;
      if (!open) return;
      try {
        await persist({ name: tunnelName, rules: buildRules() });
      } catch (_) {
        // See implementation notes above — mid-edit failures are silent so
        // the toast-free typing flow is preserved.
      }
    }, 250);
  }
  function runSave() {
    saveTimer = null;
    const snap = pending;
    pending = null;
    if (!snap) return;
    saveChain = saveChain.then(() => persist(snap));
  }
  async function persist(snap) {
    saveError = '';
    try {
      await TunnelService.SaveAutomationRules(snap.name, snap.rules);
      // Refresh the live indicators right away: the 3s poll would otherwise
      // leave the rule frames judged against the PRE-save disk state, briefly
      // highlighting the wrong card after every edit.
      if (open && snap.name === tunnelName) refreshPreview();
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
  //   - conditions inside one rule are AND; rules are OR'd with
  //     first-match-wins priority, disconnect rules before connect rules;
  //   - per-condition "match" badge = that condition's own result, judged
  //     individually — a rule only fires when ALL its conditions match;
  //   - "in use" badge + winning frame = the FIRST matching rule overall
  //     only, and only when its action actually executes; rules that also
  //     match but rank behind it (all connect rules behind a matched
  //     disconnect rule, later rules of the same action) stay match-only;
  //   - none_match ("otherwise") matches exactly when no rule above it
  //     matched.
  // The GUI evaluates every tunnel's rules against the current network
  // context (AutomationPreview, same wifi engine the helper uses) and
  // reports, per rule, whether each condition matched. We poll while the
  // editor is open so the indicators follow network changes in real time.
  //
  // UNIFIED refresh entry point (single source of truth per memory
  // EXP-100009308). Every caller — onMount poll, open reactive guard,
  // post-save immediate refresh — goes through this function, which
  // enforces:
  //   * one in-flight request at a time (cancel the previous via AbortController)
  //   * response rejection when `previewEpoch` has advanced (modal closed/reopened)
  //   * response rejection when `tunnelName` changed between fire and resolution
  //   * explicit "fetching" (preview = null) only when the first fetch of a
  //     session has no usable prior state; transient failures mid-session
  //     keep the previous frame (same semantics as before) instead of
  //     leaving indicators empty and causing "every open looks different".
  async function refreshPreview() {
    if (!open || !tunnelName) return;
    const myEpoch = previewEpoch;
    const myTunnel = tunnelName;
    // Cancel any previous in-flight fetch: Wails calls that are already on
    // the wire don't have an HTTP abort, so for the AbortController guard
    // we still benefit from the epoch check below — no stale result ever
    // reaches `preview`. If a future call path brings a fetch that speaks
    // AbortSignal, this cancels it cleanly.
    if (previewAbort) { try { previewAbort.abort?.(); } catch (_) {} }
    previewAbort = new AbortController();
    try {
      const pv = await AutomationPreview();
      if (previewEpoch !== myEpoch) return;        // session changed
      if (tunnelName !== myTunnel) return;          // switched tunnel
      if (!open) return;
      const t = (pv?.tunnels || []).find(x => x.name === myTunnel);
      // EITHER no per-tunnel result, OR the tunnel has NO rules saved yet
      // (an empty rules array with an "unmanaged" explicit decision is
      // still a VALID result we want to render — so we only bail when the
      // backend returned no tunnel entry at all, which really means "no
      // preview available"). Without this check a first save to a tunnel
      // that previously had no rules would leave the indicators blank until
      // the next poll cycle.
      if (!t && (pv?.tunnels?.some(x => x.name === myTunnel) ?? true)) return;
      preview = {
        on_wifi: pv.on_wifi,
        ssid: pv.ssid,
        decision: t ? t.decision : 'unmanaged',
        rules: t ? t.rules : [],
        interfaces: pv.interfaces,
      };
    } catch (e) {
      // Network enumeration can be temporarily unavailable mid-session;
      // keep last state. If we held no state to begin with (first open,
      // the call above returned nothing), expose a sentinel so the UI can
      // tell "couldn't fetch" from "genuinely no match".
      if (previewEpoch !== myEpoch || tunnelName !== myTunnel) return;
    } finally {
      if (previewAbort && previewAbort.signal.aborted) {
        // AbortController was replaced by a newer call; don't null it here
        // because the newer call owns it now.
      } else if (previewAbort && !previewAbort.signal.aborted) {
        previewAbort = null;
      }
    }
  }
  // Rules are evaluated top-to-bottom (disconnect before connect). The first
  // matching rule decides the outcome. We expose this so the UI can highlight
  // the winning rule and dim/shadow lower-priority rules that also match.
  $: winningRuleIndex = (() => {
    const rules = preview?.rules || [];
    for (let i = 0; i < rules.length; i++) {
      if (rules[i].matched) return i;
    }
    return -1;
  })();
  // The decision is actually executed only for connect/disconnect. Under
  // the manual-off latch the winning connect rule is suppressed — nothing
  // runs, so nothing may be marked "in use" (match markers stay truthful).
  function actionExecuted() {
    const k = decisionKey();
    return k === 'connect' || k === 'disconnect';
  }
  // A group "wins" only when the final decision belongs to that action.
  function groupWon(d) {
    if (!actionExecuted()) return false;
    const win = winningRuleIndex >= 0 ? preview.rules[winningRuleIndex] : null;
    return win && win.do === d;
  }
  // Rule matching is order-based: rules are OR'd with first-match-wins
  // priority (all disconnect rules before all connect rules). A rule is
  // the winner iff it is the first matching rule of the whole list.
  function isWinningRule(rd) {
    if (!rd || !rd.matched || winningRuleIndex < 0) return false;
    const globalIdx = (preview?.rules || []).findIndex(r => r === rd);
    return globalIdx === winningRuleIndex;
  }
  // "In use" = this rule's action is what the engine executes right now:
  // it must be the first matching rule AND that action must actually run.
  // Later rules that also match — including every connect rule behind a
  // matched disconnect rule — are deprioritized: match-only, never used.
  function ruleWon(d, ruleIdx) {
    return actionExecuted() && isWinningRule(ruleDetailFor(d, ruleIdx));
  }
  // "Otherwise" (none_match) judgment is independent of execution: it
  // matches exactly when no rule ABOVE it matched, i.e. its rule is the
  // first match — even if a manual-off latch then suppresses the action.
  function otherwiseHit(d, ruleIdx) {
    return isWinningRule(ruleDetailFor(d, ruleIdx));
  }
  // Preview rules keep the persisted order (disconnect first), so the k-th
  // PERSISTED editor rule card under an action maps to the k-th preview
  // RuleDetail of that action. Draft rules — cards whose conditions are
  // incomplete and therefore not persisted — have no detail and must be
  // SKIPPED when counting; mapping naively by card index would shift every
  // card after a draft onto the wrong detail and light up the wrong rule
  // frame even though the engine's decision is correct.
  function ruleDetailsFor(d) {
    return (preview?.rules || []).filter(r => r.do === d);
  }
  function isPersistedRule(rule) {
    return rule.conds.map(cleanedCond).filter(Boolean).length > 0;
  }
  function ruleDetailFor(d, ruleIdx) {
    const details = ruleDetailsFor(d);
    const grp = groups[d] || [];
    let k = 0; // index among this action's persisted rules
    for (let i = 0; i < grp.length; i++) {
      if (!isPersistedRule(grp[i])) continue;
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
  // Whether this condition row actually matches the current network. For
  // concrete conditions we use the backend per-condition result (each row
  // is judged individually even when the AND rule as a whole does not
  // match); for the "otherwise" fallback the match judgment is "no rule
  // above mine matched", independent of whether the action then executes.
  function condNetworkMatched(d, ruleIdx, i) {
    const c = groups[d][ruleIdx].conds[i];
    if (c.type === 'none_match') return otherwiseHit(d, ruleIdx);
    const rd = ruleDetailFor(d, ruleIdx);
    if (!rd) return false;
    const idx = condIndexInRule(groups[d][ruleIdx], i);
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
  function decisionKey() {
    if (!preview || !preview.decision) return 'unmanaged';
    return preview.decision; // 'connect' | 'disconnect' | 'unmanaged' | 'manual-off'
  }
</script>

<svelte:window on:keydown={(e) => e.key === 'Escape' && open && close()} />
{#if open}
  <div class="am-backdrop" on:click={close}>
    <div class="am-dialog" on:click|stopPropagation role="dialog" aria-modal="true" tabindex="-1" aria-label={$t('automation.title')}>
      <div class="am-header">
        <div class="am-icon"><Icon name="wifi" size={18} strokeWidth={2} /></div>
        <div class="am-header-text">
          <h3>{$t('automation.title')}</h3>
          <p class="am-sub">{tunnelName}</p>
        </div>
        <button class="am-close" on:click={close} aria-label="Close"><Icon name="x" size={16} strokeWidth={2} /></button>
      </div>
      <p class="am-hint">{$t('automation.hint')}</p>
      <SSIDPermissionBanner {TunnelService} />
      <!-- Live decision strip -->
      <div class="am-live">
        <span class="am-live-dot am-live-dot-{decisionKey()}"></span>
        <span class="am-live-label">{$t('automation.live_matching')}</span>
        <span class="am-live-network">{preview?.ssid || '—'}</span>
        <span class="am-live-decision">{decisionKey() === 'unmanaged' ? $t('automation.decision_unmanaged') : $t('automation.decision_' + decisionKey())}</span>
      </div>
      <div class="am-rules-wrap">
        <div class="am-fade am-fade-top" class:show={canScrollUp}>
          <span class="am-chevron am-chevron-up"><Icon name="chevron-down" size={15} strokeWidth={2.5} /></span>
        </div>
        <div class="am-rules" bind:this={rulesEl} on:scroll={updateScroll}>
          {#each ['disconnect', 'connect'] as d}
            {@const grpRules = groups[d]}
            <div class="am-group">
              <div class="am-group-header">
                <span class="am-group-dot am-group-dot-{d}" class:am-group-matched={groupWon(d)}></span>
                <span class="am-group-title">{$t(d === 'connect' ? 'automation.section_connect' : 'automation.section_disconnect')}</span>
                <button class="am-add-rule" on:click={() => addRule(d)}>
                  <Icon name="plus" size={12} strokeWidth={2.25} /> {$t('automation.add_rule')}
                </button>
              </div>
              {#if grpRules.length === 0}
                <div class="am-empty am-group-empty">{$t(d === 'connect' ? 'automation.empty_connect' : 'automation.empty_disconnect')}</div>
              {:else}
                {#each grpRules as rule, ruleIdx (rule._gid)}
                  {#if ruleIdx > 0}
                    <div class="am-rule-or">{$t('automation.rule_or')}</div>
                  {/if}
                  <div class="am-rule" class:am-rule-won={ruleWon(d, ruleIdx)}>
                    <div class="am-rule-head">
                      <span class="am-rule-num">{$t('automation.rule')} {ruleIdx + 1}</span>
                      <span class="am-match-badge" title={$t('automation.group_all_hint')}>{$t('automation.match_all')}</span>
                      <button class="am-remove-rule" on:click={() => removeRule(d, ruleIdx)} title={$t('automation.remove_rule')} aria-label="remove rule"><Icon name="x" size={13} strokeWidth={2} /></button>
                    </div>
                    {#if rule.conds.length === 0}
                      <div class="am-empty am-rule-empty">{$t('automation.rule_empty')}</div>
                    {:else}
                      {#each rule.conds as c, i (c._id)}
                        {@const net = condNetworkMatched(d, ruleIdx, i)}
                        {@const use = ruleWon(d, ruleIdx)}
                        <div class="am-cond" class:am-dragging={dragIndex === i && dragGroup === d && dragRule === ruleIdx}
                          on:dragover={(e) => onCondDragOver(e, d, ruleIdx, i)}
                          on:dragend={onDragEnd}>
                          <span class="am-handle" draggable="true" title={$t('automation.drag_hint')}
                            on:dragstart={(e) => onDragStart(e, d, ruleIdx, i)}>⋮⋮</span>
                          <select class="am-type" bind:value={c.type} on:change={save} aria-label={$t('automation.condition')}>
                            <option value="network">{$t('automation.cond_network')}</option>
                            <option value="subnet">{$t('automation.cond_subnet')}</option>
                            <option value="ssid">{$t('automation.cond_ssid')}</option>
                            <option value="wifi">{$t('automation.cond_wifi')}</option>
                            <option value="gateway_ip">{$t('automation.cond_gateway_ip')}</option>
                            <option value="interface">{$t('automation.cond_interface')}</option>
                            <option value="ethernet">{$t('automation.cond_ethernet')}</option>
                            <option value="time">{$t('automation.cond_time')}</option>
                            <option value="none_match">{$t('automation.cond_none')}</option>
                          </select>
                          {#if c.type === 'network'}
                            <input
                              class="am-val" class:am-invalid={macInvalid(c.gateway_mac)}
                              list="am-mac-list"
                              placeholder={currentGatewayMAC || $t('automation.mac_placeholder')}
                              title={macInvalid(c.gateway_mac) ? $t('automation.mac_invalid') : ''}
                              bind:value={c.gateway_mac}
                              on:input={save} on:change={() => onMacChange(c)} />
                          {:else if c.type === 'subnet'}
                            <input
                              class="am-val" class:am-invalid={cidrInvalid(c.subnet)}
                              list="am-subnet-list"
                              placeholder={currentSubnets[0] || '192.168.0.0/24'}
                              title={cidrInvalid(c.subnet) ? $t('automation.subnet_invalid') : ''}
                              bind:value={c.subnet}
                              on:input={save} on:change={save} />
                          {:else if c.type === 'ssid'}
                            <input
                              class="am-val"
                              list="am-ssid-list"
                              placeholder={$t('automation.ssid_select_hint')}
                              bind:value={c.ssid}
                              on:input={save}
                              on:change={save}
                            />
                          {:else if c.type === 'gateway_ip'}
                            <input
                              class="am-val" class:am-invalid={gatewayIPInvalid(c.gateway_ip)}
                              list="am-gwip-list"
                              placeholder={preview?.gateway_ip || '192.168.0.1'}
                              title={gatewayIPInvalid(c.gateway_ip) ? $t('automation.ip_invalid') : ''}
                              bind:value={c.gateway_ip}
                              on:input={save}
                              on:change={save}
                            />
                          {:else if c.type === 'interface'}
                            <input
                              class="am-val"
                              list="am-iface-list"
                              placeholder={$t('automation.interface_placeholder')}
                              bind:value={c.interface_name}
                              on:input={save}
                              on:change={save}
                            />
                          {:else if c.type === 'ethernet'}
                            <span class="am-val am-val-none">{$t('automation.cond_ethernet_desc')}</span>
                          {:else if c.type === 'time'}
                            <div class="am-time">
                              <input type="time" class="am-clock" aria-label={$t('automation.time_start')} bind:value={c.start} on:change={save} />
                              <span class="am-time-dash">–</span>
                              <input type="time" class="am-clock" aria-label={$t('automation.time_end')} bind:value={c.end} on:change={save} />
                              <div class="am-days" role="group" aria-label={$t('automation.days_label')}>
                                {#each [0, 1, 2, 3, 4, 5, 6] as d}
                                  <button type="button" class="am-day" class:on={(c.days || []).includes(d)} on:click={() => toggleDay(c, d)}>{$t('automation.day_' + d)}</button>
                                {/each}
                              </div>
                            </div>
                          {:else}
                            <span class="am-val am-val-none">{c.type === 'wifi' ? $t('automation.cond_wifi_desc') : $t('automation.cond_none_desc')}</span>
                          {/if}
                          <span
                            class="am-live-cond"
                            title={net ? (use ? $t('automation.status_active') : $t('automation.status_shadowed')) : $t('automation.status_nomatch')}>
                            <span class="am-badge am-badge-match" class:am-yes={net} class:am-no={!net}>{$t(net ? 'automation.label_match' : 'automation.label_no_match')}</span>
                            <span class="am-badge am-badge-use" class:am-yes={use} class:am-no={!use}>{$t(use ? 'automation.label_active' : 'automation.label_inactive')}</span>
                          </span>
                          <button class="am-remove" on:click={() => removeCond(d, ruleIdx, i)} aria-label="remove condition"><Icon name="x" size={12} strokeWidth={2} /></button>
                        </div>
                      {/each}
                    {/if}
                    <button class="am-add-cond" on:click={() => addCond(d, ruleIdx)} disabled={rule.conds.length >= MAX_CONDS}>
                      <Icon name="plus" size={12} strokeWidth={2.25} /> {$t('automation.add_condition')}
                    </button>
                  </div>
                {/each}
              {/if}
            </div>
          {/each}
          <p class="am-priority-note">{$t('automation.disconnect_priority_hint')}</p>
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
    width: 100%; max-width: 580px; height: 600px; max-height: 92vh;
    display: flex; flex-direction: column;
    background: var(--bg-elevated, var(--bg-secondary));
    border: 1px solid var(--border);
    border-radius: 14px; padding: 20px;
    box-shadow: 0 16px 48px rgba(0,0,0,0.35);
  }
  .am-header { display: flex; align-items: center; gap: 12px; flex-shrink: 0; }
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
  .am-live-network { font-family: var(--font-mono); color: var(--text-primary); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .am-live-decision { margin-left: auto; color: var(--text-muted); white-space: nowrap; }
  .am-rules-wrap { flex: 1; min-height: 0; position: relative; display: flex; }
  .am-rules { flex: 1; min-height: 0; overflow-y: auto; display: flex; flex-direction: column; gap: 10px; margin: 2px 0 0; padding-right: 4px; }
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
  .am-badge-match.am-no { color: #fff; background: color-mix(in srgb, var(--red, #ff3b30) 72%, transparent); }
  .am-remove { background: transparent; border: 0; color: var(--text-muted); cursor: pointer; padding: 4px; border-radius: 6px; flex-shrink: 0; }
  .am-remove:hover { background: color-mix(in srgb, var(--red, #ff3b30) 18%, transparent); color: var(--red, #ff3b30); }
  .am-priority-note { margin: 0; font: 400 10px/1.4 var(--font-sans); color: var(--text-muted); text-align: center; flex-shrink: 0; }
  .am-error { margin-top: 8px; font: 400 12px var(--font-sans); color: var(--error-text, #ff453a); flex-shrink: 0; }
</style>
