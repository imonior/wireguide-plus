<script>
  import { tunnels, selectedTunnel, connectionStatus, refreshTunnels, refreshStatus } from '../stores/tunnels.js';
  import { appSettings } from '../stores/settings.js';
  import Icon from './Icon.svelte';
  import { t } from '../i18n/index.js';
  import { errText } from './errors.js';
  import { sanitizeTunnelName, validateTunnelName } from './tunnel-name.js';
  import { classifyIPCoverage, parseIP, stripPort } from './ip-utils.js';
  import { createEventDispatcher, tick, onDestroy } from 'svelte';
  import AutomationEditor from './AutomationEditor.svelte';
  import StatsDashboard from './StatsDashboard.svelte';

  export let TunnelService;
  const dispatch = createEventDispatcher();

  // Automation rule editor (issue #12) — replaces the old per-tunnel
  // Wi-Fi auto-connect UI, which is now handled by the general engine.
  let showAutomation = false;

  let detail = null;
  let loading = false;
  let error = '';

  // Track the last name we issued loadDetail for. The
  // selectedTunnel store emits a fresh object reference on every
  // status change (refreshTunnels and the per-tick is_connected
  // diff both call .set/.update), which without this gate would
  // trigger two RPCs every second. We only re-fetch when the
  // *name* actually changes.
  let lastLoadedName = '';
  $: if ($selectedTunnel && $selectedTunnel.name !== lastLoadedName) {
    // Flush any pending edit for the previous tunnel BEFORE we reset the
    // textarea state. Without this, a quick switch within the 800ms
    // debounce window would clobber notesValue with the new tunnel's
    // notes, and the deferred saveNotes would early-return on the
    // notesValue === notesSaved check, silently dropping the edit.
    if (lastLoadedName && notesValue !== notesSaved) {
      flushNotes(lastLoadedName, notesValue);
    }
    if (lastLoadedName && !sameProbeSlots(probeSlots, probeSlotsSaved)) {
      flushProbeSlots(lastLoadedName, probeSlots);
    }
    if (notesSaveTimer) {
      clearTimeout(notesSaveTimer);
      notesSaveTimer = null;
    }
    if (probeSlotSaveTimer) {
      clearTimeout(probeSlotSaveTimer);
      probeSlotSaveTimer = null;
    }
    lastLoadedName = $selectedTunnel.name;
    notesValue = $selectedTunnel.notes || '';
    notesSaved = notesValue;
    notesError = '';
    loadProbeSlots($selectedTunnel);
    loadDetail($selectedTunnel.name);
  }

  // Per-tunnel notes. Populated from TunnelInfo.notes in the store, edited
  // locally, persisted via SetTunnelNotes on blur or 800ms idle.
  // notesSaved is the last value we committed to disk — used to skip
  // no-op saves and to detect dirty state on tunnel switch.
  let notesValue = '';
  let notesSaved = '';
  let notesSaveTimer = null;
  let notesError = '';

  // Probe targets: four positional slots, matching storage.ProbeTargets.
  //
  //   [0] [1]  public-probe overrides — only shown (and only used) on a
  //            full tunnel, where they default to 8.8.8.8 / 223.5.5.5
  //   [2] [3]  free-form, offered on every tunnel type
  //
  // Always four entries so the markup can index them. An empty slot means
  // "not configured" — for [0]/[1] that is "use the built-in default", which
  // is why clearing a row does not delete it from the probe plan.
  const PROBE_SLOTS = 4;
  let probeSlots = ['', '', '', ''];
  // Last values confirmed on disk. The dirty check compares against this, so
  // a rejected save leaves the row dirty and the error visible instead of
  // silently pretending the value took.
  let probeSlotsSaved = ['', '', '', ''];
  // Address each slot resolved to ("" for a literal IP). Filled from the save
  // response (immediate) and refreshed from the probe stream (live), so a
  // hostname that later moves is shown as what it currently points at.
  let probeSlotResolved = ['', '', '', ''];
  let probeSlotSaveTimer = null;
  let probeSlotError = '';
  let probeInputs = [];
  // Slots whose value changed but whose probe row has not come back yet.
  // The helper now re-probes on save (Tunnel.ProbeNow), so this clears
  // within a second or two; it exists so that window reads as "measuring"
  // rather than as "the row silently ignored what I typed".
  let probePendingSlots = {};
  let probePendingTimer = null;
  // The long-form target explanation is collapsed by default — it is detail
  // you read once, and expanded it pushed the editable rows off the card.
  let hintOpen = false;

  // Drop a slot's "measuring" marker as soon as the probe stream reports a
  // row for it. Runs off the results themselves rather than a timer so the
  // marker disappears exactly when the number lands.
  function clearPendingSlots(results) {
    if (!Object.keys(probePendingSlots).length) return;
    let changed = false;
    const next = { ...probePendingSlots };
    for (const r of results || []) {
      const slot = r.slot ?? -1;
      if (slot >= 0 && next[slot]) {
        delete next[slot];
        changed = true;
      }
    }
    if (changed) probePendingSlots = next;
  }

  $: clearPendingSlots(probeResults);

  function markPendingSlots(prev, next) {
    const pending = {};
    for (let i = 0; i < PROBE_SLOTS; i++) {
      if (prev[i] !== next[i]) pending[i] = true;
    }
    probePendingSlots = pending;
    if (probePendingTimer) clearTimeout(probePendingTimer);
    // Backstop: a slot that is never probed (tunnel down, target dropped as
    // a duplicate of the endpoint) must not keep claiming to be measured.
    probePendingTimer = setTimeout(() => (probePendingSlots = {}), 20000);
  }

  // flushNotes is the single write path used by both the debounce/blur
  // saver and the cross-tunnel-switch flush. It patches BOTH stores —
  // selectedTunnel for the immediate UI, tunnels for the list — so
  // re-selecting the same tunnel before the next ListTunnels refresh
  // doesn't show stale (pre-edit) notes.
  async function flushNotes(name, value) {
    if (!name) return false;
    try {
      await TunnelService.SetTunnelNotes(name, value);
      tunnels.update(list => list.map(t => t.name === name ? { ...t, notes: value } : t));
      selectedTunnel.update(sel => sel && sel.name === name ? { ...sel, notes: value } : sel);
      // Only update local UI state if the user is still on this tunnel —
      // otherwise we'd overwrite the new tunnel's notesSaved with the
      // wrong value (this flush could be the cross-switch fire-and-forget).
      if ($selectedTunnel && $selectedTunnel.name === name) {
        notesSaved = value;
        notesError = '';
      }
      return true;
    } catch (e) {
      if ($selectedTunnel && $selectedTunnel.name === name) {
        notesError = errText(e);
      } else {
        console.error('flushNotes for', name, e);
      }
      return false;
    }
  }

  async function saveNotes() {
    if (!$selectedTunnel) return;
    if (notesValue === notesSaved) return;
    await flushNotes($selectedTunnel.name, notesValue);
  }

  function onNotesInput() {
    // Debounced auto-save. Blur still calls saveNotes immediately, so this
    // covers the case where the user stays in the textarea but stops typing.
    if (notesSaveTimer) clearTimeout(notesSaveTimer);
    notesSaveTimer = setTimeout(saveNotes, 800);
  }

  // What a row currently resolves to, for the text beside the box. The live
  // value from the probe stream wins — it is the more recent lookup, and a
  // hostname pointing somewhere new is exactly what the user needs to see —
  // with the save-time resolution filling the gap until the next cycle.
  function slotResolved(i) {
    return probeSlotLive[i] || probeSlotResolved[i] || '';
  }

  // --- Probe target slots -------------------------------------------------
  //
  // The whole list is written in one call rather than per row: the backend
  // validates every slot together (and resolves hostnames), and a partial
  // write would leave the sidecar describing a probe plan the user never
  // asked for.

  function sameProbeSlots(a, b) {
    return a.length === b.length && a.every((v, i) => v === b[i]);
  }

  function normalizeSlots(list) {
    const out = new Array(PROBE_SLOTS).fill('');
    for (let i = 0; i < PROBE_SLOTS; i++) out[i] = (list?.[i] || '').trim();
    return out;
  }

  // loadProbeSlots seeds the editor from the store. It understands the
  // pre-four-row shape too (a single `latency_probe_target`), mapping it to
  // slot 2 — the same migration the backend applies, so a tunnel that has
  // never been touched since the upgrade still shows its target.
  function loadProbeSlots(tun) {
    const stored = tun?.latency_probe_targets;
    if (Array.isArray(stored) && stored.length) {
      probeSlots = normalizeSlots(stored);
    } else {
      const legacy = (tun?.latency_probe_target || '').trim();
      probeSlots = ['', '', legacy, ''];
    }
    probeSlotsSaved = [...probeSlots];
    probeSlotResolved = ['', '', '', ''];
    probeSlotError = '';
    probePendingSlots = {};
    hintOpen = false;
  }

  async function flushProbeSlots(name, slots) {
    if (!name) return false;
    const payload = normalizeSlots(slots);
    const prevSaved = [...probeSlotsSaved];
    try {
      const resolved = await TunnelService.SetTunnelLatencyProbeTargets(name, payload);
      const resolvedList = normalizeSlots(Array.isArray(resolved) ? resolved : []);
      tunnels.update(list => list.map(t => t.name === name ? { ...t, latency_probe_targets: payload } : t));
      selectedTunnel.update(sel => sel && sel.name === name ? { ...sel, latency_probe_targets: payload } : sel);
      if ($selectedTunnel && $selectedTunnel.name === name) {
        probeSlotsSaved = [...payload];
        // Only adopt the normalized text if the user has not typed on since
        // the request went out — a slow round-trip must not overwrite what
        // they are in the middle of entering.
        if (sameProbeSlots(probeSlots, payload)) probeSlots = [...payload];
        // A literal IP resolves to "" — show the address itself so the row
        // does not look like resolution failed.
        probeSlotResolved = resolvedList.map((r, i) => r || payload[i]);
        probeSlotError = '';
        // What changed is now saved; the reading for it is on its way (the
        // helper re-probes on save). Mark the rows so the wait is visible.
        markPendingSlots(prevSaved, payload);
      }
      return true;
    } catch (e) {
      if ($selectedTunnel && $selectedTunnel.name === name) {
        // Rejected (out-of-range address on a split tunnel, unresolvable
        // hostname, ...). Keep the typed text on screen: reverting it would
        // erase the very value the message is about.
        probeSlotError = errText(e);
      } else {
        console.error('flushProbeSlots for', name, e);
      }
      return false;
    }
  }

  async function saveProbeSlots() {
    if (!$selectedTunnel) return;
    // Blocked locally: the address is not routed through this tunnel. The
    // backend enforces the same rule, so saving it would only produce an
    // error round-trip.
    if (Object.keys(probeSlotCoverage).length) return;
    if (sameProbeSlots(probeSlots, probeSlotsSaved)) return;
    await flushProbeSlots($selectedTunnel.name, probeSlots);
  }

  function onProbeSlotInput() {
    if (probeSlotSaveTimer) clearTimeout(probeSlotSaveTimer);
    probeSlotSaveTimer = setTimeout(saveProbeSlots, 600);
  }

  async function onProbeSlotKeydown(e, i) {
    if (e.key === 'Enter') {
      e.preventDefault();
      if (probeSlotSaveTimer) clearTimeout(probeSlotSaveTimer);
      await saveProbeSlots();
    } else if (e.key === 'Escape') {
      e.preventDefault();
      probeSlots = [...probeSlotsSaved];
      probeSlotError = '';
    }
  }

  // "Re-enter a valid value": clear the message and put the caret back in the
  // row that was refused, with its text selected so the next keystroke
  // replaces it outright.
  async function retryProbeSlots() {
    probeSlotError = '';
    // Prefer a row the local pre-check rejects; otherwise the first row that
    // differs from what is on disk — which is the row the backend refused.
    let offending = probeSlots.findIndex((v, i) => v && probeSlotCoverage[i]);
    if (offending < 0) offending = probeSlots.findIndex((v, i) => v !== probeSlotsSaved[i]);
    await tick();
    const el = probeInputs[offending >= 0 ? offending : 0];
    el?.focus();
    el?.select();
  }

  // Port half of an endpoint, or "" when there is none. Kept separate from
  // stripPort because the resolved address below needs to be reassembled
  // into a complete "ip:port" — and IPv6 ("[fd00::1]:51820", or a bare
  // address with many colons) must not be mistaken for host:port.
  function endpointPort(value) {
    const text = (value || '').trim();
    if (text.startsWith('[')) {
      const close = text.indexOf(']');
      if (close === -1 || text[close + 1] !== ':') return '';
      const port = text.slice(close + 2);
      return /^\d+$/.test(port) ? port : '';
    }
    const first = text.indexOf(':');
    if (first === -1 || first !== text.lastIndexOf(':')) return '';
    const port = text.slice(first + 1);
    return /^\d+$/.test(port) ? port : '';
  }

  // The address the endpoint NAME currently points at — the hero's "live IP".
  //
  // A DDNS peer is a name in the config but an address on the wire, and the
  // two drift the moment the peer moves (ISP reconnect, failover). Showing
  // the name alone hides the one thing worth knowing when a tunnel suddenly
  // misbehaves: which address it is actually dialling.
  //
  // Primary source is the UAPI connection address (status.endpoint): once the
  // link is up WireGuard reports the actual peer endpoint it handshook with, so
  // the hero shows the resolved IP from the first status tick — no waiting for
  // the next probe cycle. A fresh DNS lookup is avoided on purpose: the helper
  // also hands the probe the same UAPI address, so resolving it yields nothing
  // (it is already an IP) and used to leave the hero permanently blank.
  $: endpointLiveIP = (() => {
    const ep = ($selectedTunnel?.endpoint || '').trim();
    if (!ep) return '';
    // A literal address has nothing to resolve — printing it twice is noise.
    if (parseIP(stripPort(ep))) return '';

    // Best source: the address WireGuard is ACTUALLY talking to. When the link
    // is up, status.endpoint is read from the UAPI peer entry, i.e. the
    // endpoint chosen at handshake time — authoritative, and present from the
    // first status tick rather than after the next probe cycle.
    //
    // This deliberately does not rely on probing alone: the helper hands the
    // probe candidates `status.endpoint` too, so once connected the endpoint
    // row carries the resolved IP already, which resolves to nothing (it is an
    // address, not a name) and used to leave the hero permanently blank.
    if (isConnected) {
      const actual = (status?.endpoint || '').trim();
      const actualIP = actual ? stripPort(actual) : '';
      if (actualIP && parseIP(actualIP)) return actualIP;
    }

    // Fallback: the probe stream's own resolution of the endpoint name, for
    // the window before the link settles.
    const rows = probeResults;
    const byKind = rows.find(r => r.kind === 'endpoint' && r.resolved_ip);
    if (byKind) return byKind.resolved_ip;
    const byTarget = rows.find(r => r.target === ep && r.resolved_ip);
    return byTarget ? byTarget.resolved_ip : '';
  })();

  // "host:port (ip:port)" — the port rides along on the resolved side so the
  // parenthesised value is a complete endpoint, comparable and pasteable,
  // not a bare address the user has to mentally re-attach to a port.
  $: endpointLive = (() => {
    if (!endpointLiveIP) return '';
    const port = endpointPort($selectedTunnel?.endpoint || '');
    return port ? `${endpointLiveIP}:${port}` : endpointLiveIP;
  })();

  // Hover value for the whole endpoint: the pane can clip a long DDNS name,
  // and a clipped span is no help when the user is reading it off-screen.
  $: endpointTitle = endpointLive
    ? `${$selectedTunnel?.endpoint || ''} (${endpointLive})`
    : $selectedTunnel?.endpoint || '';

  // The hero endpoint is plain selectable text, not a copy button: the app
  // sets `user-select: none` globally (public/style.css), so the span opts
  // back in via CSS. Selecting beats a click-to-copy here because a user
  // copying an endpoint usually wants a piece of it — the host, or just the
  // resolved address in parentheses — not the whole string.
  //
  // Automatic latency probe target — single rule: probe the peer endpoint.
  //
  // Previous revisions preferred a /32 host from AllowedIPs, then 8.8.8.8
  // for full tunnels. Those hosts are frequently offline (a sleeping NAS,
  // a laptop) or measure something unrelated to this tunnel, whereas the
  // endpoint is reachable by definition whenever the tunnel is up. The
  // helper side implements exactly the same rule.
  //
  // Takes its input as a parameter (not via closure) so the `$:` below
  // actually re-runs: the compiler only tracks dependencies referenced in
  // the reactive statement itself, and a dep-less statement runs exactly
  // once — which froze this on the first tunnel's endpoint forever.
  function autoLatencyTarget(sel) {
    return stripPort(sel?.endpoint || '');
  }

  $: autoLatency = autoLatencyTarget($selectedTunnel);

  // Every row the helper actually pinged, in its own display order. The
  // headline number below is the best of them; the rows are what make it
  // readable: a silent public probe next to a healthy endpoint means "tunnel
  // up, ICMP filtered", which a single blended figure could never express.
  //
  // Health bands are deliberately coarse. This is a glanceable indicator,
  // not an SLA: green = snappy, amber = usable but slow, red = too slow or
  // not answering at all.
  const PROBE_OK_MS = 100;
  const PROBE_WARN_MS = 300;

  function probeHealth(r) {
    if (!r || !r.reachable) return 'bad';
    const ms = r.latency_ms || 0;
    if (ms <= PROBE_OK_MS) return 'ok';
    if (ms <= PROBE_WARN_MS) return 'warn';
    return 'bad';
  }

  function probeKindLabel(kind) {
    if (kind === 'public') return $t('tunnel.probe_kind_public');
    if (kind === 'endpoint') return $t('tunnel.probe_kind_endpoint');
    if (kind === 'custom') return $t('tunnel.probe_kind_custom');
    return '';
  }

  $: probeResults = status?.latency_probe_results || [];

  $: probeRows = probeResults.map((r, i) => {
    const resolved = r.resolved_ip || '';
    // "outside" — the address does not travel through this tunnel, so its
    // RTT describes the plain internet path. Flagged on the row itself, not
    // just in the editor: the marking has to be where the number is read.
    const outside = r.coverage === 'outside';
    return {
      key: `${r.kind || 'probe'}:${r.slot ?? -1}:${r.target}:${i}`,
      label: r.target,
      resolved,
      title: resolved ? `${r.target} → ${resolved}` : r.target,
      kindLabel: probeKindLabel(r.kind),
      state: probeHealth(r),
      outside,
      display: r.reachable ? `${Math.round(r.latency_ms)} ms` : '—',
    };
  });

  // Headline: the best reachable reading, i.e. the one the tunnel path is
  // actually capable of right now. "—" when nothing answered.
  $: probeHeadline = (() => {
    const reachable = probeResults.filter(r => r.reachable);
    if (!reachable.length) return null;
    return Math.round(Math.min(...reachable.map(r => r.latency_ms || 0)));
  })();

  // Live resolution per slot, straight off the probe stream: the row in the
  // editor shows what the address resolves to *now*, so a DDNS name that
  // moved (and may have moved out of AllowedIPs) is visible without
  // re-saving.
  $: probeSlotLive = (() => {
    const out = ['', '', '', ''];
    for (const r of probeResults) {
      const slot = r.slot ?? -1;
      if (slot >= 0 && slot < out.length) out[slot] = r.resolved_ip || r.target || '';
    }
    return out;
  })();

  // Coverage violations reported by the helper for the slots currently on
  // screen. The helper re-evaluates this every probe cycle, which is the
  // only way to notice that a target became unroutable after it was saved.
  $: probeSlotOutside = (() => {
    const out = {};
    for (const r of probeResults) {
      const slot = r.slot ?? -1;
      if (slot >= 0 && r.coverage === 'outside') out[slot] = true;
    }
    return out;
  })();

  // Probe target validation.
  //
  // A split tunnel only routes the addresses listed in AllowedIPs, so an
  // address outside that set would never enter the WireGuard interface —
  // its RTT describes the plain internet path, not the tunnel. Such a value
  // is rejected (not merely flagged): saving it would store a number that
  // cannot mean what the field claims. Full tunnels accept anything, since
  // they route everything anyway.
  //
  // Only literal IPs can be judged in the browser — a hostname needs a DNS
  // lookup, which the backend performs on save (resolveProbeTarget) and then
  // checks with the same coverage rule.
  function collectAllowedIPs(det) {
    const out = [];
    // Wails serializes Go structs with json snake_case tags, so the
    // WireGuardConfig that arrives here has `peers` / `allowed_ips`, NOT
    // the Go camelCase `Peers` / `AllowedIPs`. Reading the camelCase
    // names returned [] for every tunnel, which made every split-tunnel
    // probe target look "uncovered" and blocked the save (Bug B).
    for (const peer of det?.peers || []) {
      for (const ip of peer.allowed_ips || []) out.push(ip);
    }
    return out;
  }

  function isFullTunnel(det) {
    return (det?.peers || []).some(peer =>
      (peer.allowed_ips || []).some(ip => ip === '0.0.0.0/0' || ip === '::/0')
    );
  }

  // Full-tunnel status drives which rows the editor shows: slots 0/1 are
  // public-probe overrides that only mean anything when everything is routed
  // through the tunnel. On a split tunnel they are hidden — and left
  // untouched in the payload rather than blanked, so switching a tunnel
  // between full and split mode never destroys the values.
  //
  // Unknown config (still loading, or unreadable) counts as full: showing
  // the rows is the safer default, since hiding them from a full tunnel
  // would make its own public probes uneditable.
  $: fullTunnel = !detail || isFullTunnel(detail);

  // Which rows the editor renders. Split tunnels drop 0/1 entirely rather
  // than disabling them: a greyed-out box invites the user to try, and the
  // address would never be probed on that tunnel anyway.
  $: probeSlotIndexes = fullTunnel ? [0, 1, 2, 3] : [2, 3];

  // The compiled-in defaults for the public-probe slots, shown as
  // placeholders so an empty (cleared) row still says what will be probed.
  const PROBE_SLOT_DEFAULTS = ['8.8.8.8', '223.5.5.5', '', ''];
  function probeSlotDefault(i) {
    return PROBE_SLOT_DEFAULTS[i] || '';
  }

  // Local pre-check: a literal IP typed into a slot that this split tunnel
  // does not route is caught here so the offending row is marked as the user
  // types, without a round-trip. Hostnames cannot be judged in the browser
  // (no DNS), which is why the backend resolves on save and the helper
  // re-checks every cycle — that is what probeSlotOutside reports.
  //
  // Slots 0/1 are skipped on a split tunnel for the same reason the backend
  // skips them: they are hidden and unused there, so flagging their stored
  // defaults would paint a warning on a row the user cannot even see.
  $: probeSlotCoverage = (() => {
    const out = {};
    if (!detail || isFullTunnel(detail)) return out;
    const allowed = collectAllowedIPs(detail);
    const limit = PROBE_SLOTS;
    for (let i = 0; i < limit; i++) {
      if (i < 2) continue;
      const v = probeSlots[i];
      if (!v) continue;
      if (classifyIPCoverage(v, allowed) === 'uncovered') out[i] = true;
    }
    return out;
  })();

  // Rows that must be shown as invalid: locally detected (typed) plus
  // helper-reported (a target that became unroutable after it was saved).
  function slotInvalid(i) {
    return !!probeSlotCoverage[i] || !!probeSlotOutside[i];
  }

  // One message line, two sources: the local pre-check (typed value is not
  // routed here) and the backend's rejection (unresolvable hostname, or a
  // rule the frontend cannot evaluate). Either way the value was not saved,
  // so the message is paired with a "re-enter" affordance.
  $: probeSlotMessage = probeSlotError
    || (Object.keys(probeSlotCoverage).length ? $t('tunnel.latency_target_outside_error') : '');

  onDestroy(() => {
    if (notesSaveTimer) clearTimeout(notesSaveTimer);
    if (probeSlotSaveTimer) clearTimeout(probeSlotSaveTimer);
    if (probePendingTimer) clearTimeout(probePendingTimer);
    // Best-effort flush on unmount (e.g. user deselected the tunnel
    // while a debounce was still pending).
    if (lastLoadedName && notesValue !== notesSaved) {
      flushNotes(lastLoadedName, notesValue);
    }
    if (lastLoadedName && !sameProbeSlots(probeSlots, probeSlotsSaved)) {
      flushProbeSlots(lastLoadedName, probeSlots);
    }
  });

  // Single source of truth for "is this tunnel currently active?" —
  // combine the selected-tunnel flag with the live connection status so the
  // UI can't show a stale "connected" chip briefly after disconnect.
  // Only ESTABLISHED tunnels count as connected — see Manager.EstablishedTunnels.
  // `active_tunnels` also contains tunnels that are still dialling and may yet
  // fail (boot-time DNS miss, pending handshake); treating those as connected
  // is what made a failed attempt look like "connected, then dropped".
  $: activeNames = $connectionStatus?.active_tunnels || [];
  $: establishedNames = Array.isArray($connectionStatus?.established_tunnels)
    ? $connectionStatus.established_tunnels
    : activeNames;
  $: isEstablished = $selectedTunnel?.is_connected
    && establishedNames.includes($selectedTunnel?.name);
  $: isConnected = isEstablished;
  // "Connecting" also covers the multi-tunnel case: the primary may be another
  // tunnel, so the selected one can be mid-transition without owning
  // connectionStatus.state.
  $: isConnecting = !isConnected
    && (($connectionStatus?.state === 'connecting'
      && $connectionStatus?.tunnel_name === $selectedTunnel?.name)
      || (!$selectedTunnel?.is_connected && activeNames.includes($selectedTunnel?.name)));
  $: noHandshake = isConnected && !status?.last_handshake;
  // Use the primary status if it matches the selected tunnel (has full stats).
  // Otherwise fall back to the lightweight per-tunnel info from the tunnels array
  // (name + state + handshake only, no rx/tx/duration).
  $: status = (() => {
    if ($connectionStatus?.tunnel_name === $selectedTunnel?.name) {
      return $connectionStatus;
    }
    const tunnels = $connectionStatus?.tunnels || [];
    const match = tunnels.find(t => t.tunnel_name === $selectedTunnel?.name);
    return match || $connectionStatus;
  })();

  async function loadDetail(name) {
    try {
      detail = await TunnelService.GetTunnelDetail(name);
      error = '';
    } catch (e) {
      detail = null;
      // Surface the failure rather than silently leaving the panel
      // blank — most failures here are "tunnel was deleted" or
      // "config is corrupt" which the user can act on.
      error = errText(e);
    }
  }

  function connect() {
    dispatch('connect', {
      name: $selectedTunnel.name
    });
  }

  // Track consecutive "client closed" failures so we can swap the
  // raw error for a recovering-helper hint on the second attempt.
  let consecutiveClientClosed = 0;

  async function disconnect() {
    error = '';
    loading = true;
    try {
      await TunnelService.DisconnectTunnel($selectedTunnel.name);
      consecutiveClientClosed = 0;
      // Don't wait for event stream — refresh immediately.
      await refreshTunnels(TunnelService);
      await refreshStatus(TunnelService);
    } catch (e) {
      const raw = errText(e);
      if (/client closed|connection closed|broken pipe|EOF/i.test(raw)) {
        consecutiveClientClosed += 1;
        if (consecutiveClientClosed >= 2) {
          error = $t('tunnel.helper_recovering') || 'Helper recovering, please retry in a moment.';
        } else {
          error = raw;
        }
      } else {
        consecutiveClientClosed = 0;
        error = raw;
      }
    }
    loading = false;
  }

  let showDeleteConfirm = false;
  let deleteConfirmBtn = null;

  async function askDelete() {
    if (isConnected) {
      error = $t('confirm.disconnect_first');
      return;
    }
    showDeleteConfirm = true;
    // Auto-focus the confirm button so Enter confirms, Escape cancels,
    // and a stray Space press doesn't accidentally trigger the No button.
    await tick();
    deleteConfirmBtn?.focus();
  }

  async function confirmDelete() {
    showDeleteConfirm = false;
    try {
      await TunnelService.DeleteTunnel($selectedTunnel.name);
      selectedTunnel.set(null);
      dispatch('refresh');
    } catch (e) {
      error = errText(e);
    }
  }

  function cancelDelete() {
    showDeleteConfirm = false;
  }

  // Global ESC handler — closes whichever modal is open.
  function handleKeydown(e) {
    if (e.key !== 'Escape') return;
    if (showDeleteConfirm) cancelDelete();
    else if (renaming) cancelRename();
  }
  if (typeof window !== 'undefined') {
    window.addEventListener('keydown', handleKeydown);
    onDestroy(() => window.removeEventListener('keydown', handleKeydown));
  }

  function formatBytes(bytes) {
    if (bytes === 0) return '0 B';
    const k = 1024;
    const sizes = ['B', 'KB', 'MB', 'GB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + ' ' + sizes[i];
  }

  let renaming = false;
  let renameValue = '';
  let renameInput = null;
  // Esc cancels rename. The flow is: keydown handler calls cancelRename()
  // which sets `renameCancelled=true` and `renaming=false`. The Svelte
  // unmount fires the input's `on:blur` → commitRename(), which would
  // otherwise rename the tunnel to whatever was typed. The cancelled
  // flag short-circuits that blur-driven commit.
  let renameCancelled = false;

  async function startRename() {
    if (isConnected) {
      error = $t('confirm.disconnect_first');
      return;
    }
    renameValue = $selectedTunnel.name;
    renameCancelled = false;
    renaming = true;
    // Programmatic focus is more reliable than `autofocus` across Svelte
    // re-renders (and avoids the a11y warning).
    await tick();
    renameInput?.focus();
    renameInput?.select();
  }

  async function commitRename() {
    if (renameCancelled) {
      renameCancelled = false;
      return;
    }
    const oldName = $selectedTunnel.name;
    renaming = false;

    // Same treatment as an imported file name: unsupported characters are
    // replaced rather than rejected, so the user is never stuck guessing
    // which character the backend dislikes. They are told what the name
    // became (toast via the parent) instead of being met with a raw
    // English error from Go.
    const fix = sanitizeTunnelName(renameValue);
    if (!fix.name || fix.name === oldName) return;
    const invalid = validateTunnelName(fix.name);
    if (invalid) {
      error = $t(invalid.key, invalid.params);
      return;
    }
    error = '';
    if (fix.changed) {
      dispatch('notify', $t('name.auto_fixed', { from: fix.original, to: fix.name }));
    }
    try {
      await TunnelService.RenameTunnel(oldName, fix.name);
      selectedTunnel.set({ ...$selectedTunnel, name: fix.name });
      dispatch('refresh');
    } catch (e) {
      error = errText(e);
    }
  }

  function cancelRename() {
    renameCancelled = true;
    renaming = false;
  }
</script>

<div class="detail-panel">
  {#if !$selectedTunnel}
    <div class="no-selection">
      <p>{$t('tunnel.no_tunnels')}</p>
    </div>
  {:else}
    <!-- HERO STATUS CARD: big visual, gradient bg by state, large icon -->
    <div class="hero-card"
      class:hero-connected={isConnected && !noHandshake}
      class:hero-connecting={isConnecting}
      class:hero-warning={noHandshake}
      class:hero-idle={!isConnected && !isConnecting && !noHandshake}>
      <div class="hero-glow"></div>

      <div class="hero-icon">
        {#if isConnected && !noHandshake}
          <Icon name="shield" size={28} strokeWidth={2} />
        {:else if isConnecting}
          <Icon name="zap" size={28} strokeWidth={2} />
        {:else if noHandshake}
          <Icon name="triangle-alert" size={28} strokeWidth={2} />
        {:else}
          <Icon name="shield-off" size={28} strokeWidth={1.75} />
        {/if}
      </div>

      <div class="hero-body">
        <div class="hero-name-row">
          {#if renaming}
            <input
              class="rename-input"
              type="text"
              bind:value={renameValue}
              bind:this={renameInput}
              on:blur={commitRename}
              on:keydown={(e) => {
                if (e.key === 'Enter') commitRename();
                if (e.key === 'Escape') cancelRename();
              }}
            />
          {:else}
            <h2 class="hero-name" on:dblclick={startRename} title={$t('tunnel.rename_hint')}>{$selectedTunnel.name}</h2>
            <button class="btn-rename" on:click={startRename} title="Rename">
              <Icon name="pencil" size={12} strokeWidth={1.75} />
            </button>
          {/if}
        </div>
        <div class="hero-status-line">
          <span class="hero-dot"
            class:on={isConnected && !noHandshake}
            class:warning={noHandshake}
            class:connecting={isConnecting}></span>
          <span class="hero-state-text">
            {#if isConnected && noHandshake}
              {$t('app.no_handshake')}
            {:else if isConnected}
              {$t('app.connected')}
            {:else if isConnecting}
              {$t('app.connecting')}
            {:else}
              {$t('app.disconnected')}
            {/if}
          </span>
          {#if $selectedTunnel.endpoint}
            <span class="hero-sep">·</span>
            <!-- Selectable, not click-to-copy: users copy parts of this
                 (the host, or just the resolved address) as often as all of
                 it, and a button would only ever give them the whole string.
                 The title carries the full value in case the pane clips it. -->
            <span class="hero-endpoint" title={endpointTitle}>
              <span class="hero-endpoint-text">{$selectedTunnel.endpoint}{#if endpointLive}<span
                class="hero-endpoint-ip"
                title={endpointLiveIP}>({endpointLive})</span>{/if}</span>
            </span>
          {/if}
          {#if $selectedTunnel.protocol === 'amneziawg' && $appSettings.loaded}
            <span class="hero-sep">·</span>
            <!-- Single element / single style, two states — mirrors the
                 tunnel list badge exactly. -->
            <span
              class="awg-badge"
              class:awg-badge--off={!$appSettings.enable_awg}
              title={$appSettings.enable_awg ? $t('tunnel.awg_on_tip') : $t('tunnel.awg_off_tip')}>
              {$appSettings.enable_awg ? $t('tunnel.awg_on') : $t('tunnel.awg_off')}
            </span>
          {/if}
        </div>
        {#if isConnected && status.state === 'connected'}
          <div class="hero-meta">
            <span class="meta-item">
              <Icon name="clock" size={12} strokeWidth={2} />
              {$t('tunnel.handshake')}: {status.last_handshake || '—'}
            </span>
            <span class="meta-sep">·</span>
            <span class="meta-item">{$t('tunnel.duration')}: {status.duration || '—'}</span>
            {#if status.interface_name}
              <span class="meta-sep">·</span>
              <span class="meta-item">
                <Icon name="network" size={12} strokeWidth={2} />
                {$t('tunnel.interface')}: <span class="mono">{status.interface_name}</span>
              </span>
            {/if}
          </div>
        {/if}
      </div>
    </div>

    <!-- PRIMARY ACTION: big full-width button.
         These are MANUAL controls: pressing them is a user act, not a rule
         match, and it latches the tunnel against automation (connect →
         manual-on, disconnect → manual-off) until the user acts again or the
         app restarts. The caption says so out loud. Color coding: the button
         is brand-RED (the action you take), while GREEN is reserved for the
         live "connected" status — so a red button never reads as "already up". -->
    <div class="primary-action">
      {#if isConnected}
        <button class="btn-primary-large btn-disconnect-lg" on:click={disconnect} disabled={loading}
          title={$t('tunnel.manual_disconnect_tip')}>
          <span class="pa-line">
            <Icon name="lock" size={16} strokeWidth={2.25} />
            <span>{$t('tunnel.disconnect')}</span>
          </span>
        </button>
      {:else}
        <button class="btn-primary-large btn-connect-lg" on:click={connect} disabled={loading || isConnecting}
          title={$t('tunnel.manual_connect_tip')}>
          <span class="pa-line">
            {#if loading || isConnecting}
              <span class="spinner"></span>
              <span>{$t('app.connecting')}</span>
            {:else}
              <Icon name="zap" size={16} strokeWidth={2.25} />
              <span>{$t('tunnel.connect')}</span>
            {/if}
          </span>
        </button>
      {/if}
    </div>

    <!-- LATENCY ROW: probe display + target editor on ONE row.
         The headline number and the editable targets sit side by side so the
         value you read and the value you set are in the same place. The
         latency card also owns the per-target breakdown (same reading at two
         resolutions); a silent public probe next to a healthy endpoint reads
         as "tunnel up, ICMP filtered" rather than a disagreeing aggregate.
         The redundant endpoint card that used to live here was removed — the
         endpoint already shows (and is now copyable) in the hero above. -->
    {#if $selectedTunnel.endpoint}
      <div class="latency-row">
        <!-- LATENCY DISPLAY -->
        <div class="latency-card">
          <div class="latency-head">
            <div class="latency-head-label">
              <Icon name="activity" size={12} strokeWidth={2.5} />
              <span>{$t('tunnel.latency')}</span>
            </div>
            <div class="latency-head-value">
              {#if probeHeadline !== null}
                {probeHeadline}<span class="stat-unit">ms</span>
              {:else}
                —
              {/if}
            </div>
          </div>
          {#if probeRows.length}
            <div class="probe-panel">
              {#each probeRows as row (row.key)}
                <div class="probe-row" class:probe-row-outside={row.outside}>
                  <span class="probe-dot" class:dot-ok={row.state === 'ok'}
                    class:dot-warn={row.state === 'warn'} class:dot-bad={row.state === 'bad'}
                    class:dot-idle={row.state === 'idle'}></span>
                  <span class="probe-name mono" title={row.title}>{row.label}</span>
                  {#if row.resolved}
                    <span class="probe-resolved mono" title={row.resolved}>{row.resolved}</span>
                  {/if}
                  <span class="probe-kind" class:probe-kind-outside={row.outside}>
                    {row.outside ? $t('tunnel.probe_outside') : row.kindLabel}
                  </span>
                  <span class="probe-ms" class:ms-ok={row.state === 'ok'}
                    class:ms-warn={row.state === 'warn'} class:ms-bad={row.state === 'bad'}>
                    {row.display}
                  </span>
                </div>
              {/each}
              {#if status.latency_probe_state === 'unreachable'}
                <div class="probe-summary">{$t('tunnel.probe_unreachable')}</div>
              {/if}
            </div>
          {/if}
        </div>

        <!-- LATENCY TARGET EDITOR -->
        <div class="latency-target-col">
          <h3 class="section-label">{$t('tunnel.latency_target')}</h3>
          <div class="info-card endpoint-card probe-target-card">
            {#each probeSlotIndexes as i (i)}
              <div class="probe-slot" class:probe-slot-invalid={slotInvalid(i)}>
                <input
                  bind:this={probeInputs[i]}
                  class="latency-target-input probe-slot-input"
                  type="text"
                  spellcheck="false"
                  autocomplete="off"
                  placeholder={i < 2 ? probeSlotDefault(i) : $t('tunnel.latency_target_placeholder')}
                  bind:value={probeSlots[i]}
                  on:input={onProbeSlotInput}
                  on:keydown={(e) => onProbeSlotKeydown(e, i)} />
                {#if slotResolved(i)}
                  <!-- What the value resolves to, right now. Shown for
                       hostnames so a DDNS target that moved is visible
                       without re-saving; for a literal IP it is the
                       address itself, which reads as a confirmation
                       rather than a duplicate. -->
                  <span class="probe-slot-resolved mono" title={slotResolved(i)}>{slotResolved(i)}</span>
                {/if}
                {#if probePendingSlots[i]}
                  <!-- Saved, but not measured yet. The helper re-probes on
                       save, so this is a second or two — long enough that
                       showing nothing at all reads as "nothing happened". -->
                  <span class="probe-slot-pending" title={$t('tunnel.latency_probing')}>
                    {$t('tunnel.latency_probing')}
                  </span>
                {/if}
              </div>
            {/each}
            {#if probeSlotMessage}
              <!-- Rejected: the value was not saved, so say so and send
                   the caret back into the offending row. A red line alone
                   leaves the user staring at an unsaved, unchangeable
                   box. -->
              <span class="latency-target-error">
                {probeSlotMessage}
                <button class="latency-retry-btn" type="button" on:click={retryProbeSlots}>
                  {$t('tunnel.latency_target_retry')}
                </button>
              </span>
            {/if}
            <!-- One short line is always visible; the full explanation is
                 collapsed. Expanded it is taller than the four rows it
                 explains, which pushed the editable fields out of view —
                 and it is detail you read once, not a rule you re-check
                 every time you type an address. -->
            <div class="latency-target-foot">
              <span class="latency-target-hint">{$t('tunnel.latency_target_hint')}</span>
              <button
                class="hint-toggle"
                class:hint-toggle-open={hintOpen}
                type="button"
                aria-expanded={hintOpen}
                title={$t('tunnel.latency_target_help')}
                on:click={() => (hintOpen = !hintOpen)}>
                <Icon name="info" size={11} strokeWidth={2} />
                <span class="hint-toggle-text">{$t('tunnel.latency_target_help')}</span>
              </button>
            </div>
            {#if hintOpen}
              <p class="latency-target-details">{$t('tunnel.latency_target_details')}</p>
            {/if}
          </div>
        </div>
      </div>
    {/if}

    <!-- STATS HERO: counters + throughput graph on ONE row (connected only). -->
    {#if isConnected && status.state === 'connected'}
      <div class="stats-hero">
        <div class="stat-card stat-rx">
          <div class="stat-card-top">
            <div class="stat-icon"><Icon name="arrow-down" size={12} strokeWidth={2.5} /></div>
            <span class="stat-label">{$t('tunnel.rx')}</span>
          </div>
          <div class="stat-value stat-value-sm">{formatBytes(status.rx_bytes || 0)}</div>
        </div>
        <div class="stat-card stat-tx">
          <div class="stat-card-top">
            <div class="stat-icon"><Icon name="arrow-up" size={12} strokeWidth={2.5} /></div>
            <span class="stat-label">{$t('tunnel.tx')}</span>
          </div>
          <div class="stat-value stat-value-sm">{formatBytes(status.tx_bytes || 0)}</div>
        </div>
        <div class="stat-card stat-graph">
          <StatsDashboard compact />
        </div>
      </div>
    {/if}

    <!-- NOTES -->
    <div class="info-section">
      <h3 class="section-label">{$t('tunnel.notes')}</h3>
      <textarea
        id="tunnel-notes"
        class="notes-textarea"
        placeholder={$t('tunnel.notes_placeholder')}
        bind:value={notesValue}
        on:input={onNotesInput}
        on:blur={saveNotes}
        rows="2"></textarea>
      {#if notesError}
        <div class="notes-error">{notesError}</div>
      {/if}
    </div>

    {#if error}
      <div class="error-msg">{error}</div>
    {/if}

    <!-- SECONDARY ACTIONS: 4-up icon button grid -->
    <div class="secondary-actions">
      <button class="btn-icon-action" on:click={() => dispatch('edit', $selectedTunnel.name)}>
        <Icon name="file-pen" size={15} strokeWidth={1.75} />
        <span>{$t('tunnel.edit')}</span>
      </button>
      <button class="btn-icon-action" on:click={() => dispatch('export', $selectedTunnel.name)}>
        <Icon name="share" size={15} strokeWidth={1.75} />
        <span>{$t('tunnel.export')}</span>
      </button>
      <button class="btn-icon-action" on:click={() => showAutomation = true}>
        <Icon name="wifi" size={15} strokeWidth={1.75} />
        <span>{$t('automation.title')}</span>
      </button>
      <button class="btn-icon-action btn-icon-danger" on:click={askDelete}>
        <Icon name="trash-2" size={15} strokeWidth={1.75} />
        <span>{$t('tunnel.delete')}</span>
      </button>
    </div>
  {/if}
</div>

<AutomationEditor {TunnelService} tunnelName={$selectedTunnel?.name || ''} bind:open={showAutomation} />

{#if showDeleteConfirm}
  <div class="confirm-backdrop" on:click={cancelDelete}>
    <div class="confirm-dialog" on:click|stopPropagation role="dialog" aria-modal="true" tabindex="-1">
      <div class="dialog-header">
        <div class="dialog-icon-tile danger">
          <Icon name="triangle-alert" size={18} strokeWidth={2} />
        </div>
        <div class="dialog-header-text">
          <h3>{$t('confirm.delete_title')}</h3>
        </div>
      </div>
      <p class="dialog-message">{$t('confirm.delete_message', { name: $selectedTunnel.name })}</p>
      <div class="confirm-footer">
        <button class="dialog-btn dialog-btn-ghost" on:click={cancelDelete}>{$t('confirm.no')}</button>
        <button class="dialog-btn dialog-btn-danger" bind:this={deleteConfirmBtn} on:click={confirmDelete}>{$t('confirm.yes')}</button>
      </div>
    </div>
  </div>
{/if}

<style>
  /* ---------- Layout ---------- */
  .detail-panel {
    flex: 1;
    padding: 52px var(--space-7, 28px) var(--space-7, 28px);
    overflow-y: auto;
    max-width: 760px;
    margin: 0 auto;
    width: 100%;
    box-sizing: border-box;
  }
  .no-selection {
    display: flex;
    align-items: center;
    justify-content: center;
    height: 100%;
    color: var(--text-muted);
    font: var(--text-body);
  }

  /* ========== HERO STATUS CARD ==========
     Big visual element. Gradient background tinted by state.
     Large icon tile + tunnel name + state line. */
  .hero-card {
    position: relative;
    display: flex;
    align-items: center;
    gap: 16px;
    padding: 16px 18px;
    border-radius: 16px;
    background: var(--bg-card);
    border: 0.5px solid var(--border);
    margin-bottom: 10px;
    overflow: hidden;
    box-shadow: 0 1px 2px rgba(0,0,0,0.06);
  }
  @media (prefers-reduced-motion: no-preference) {
    .hero-card {
      transition: background 280ms ease, border-color 280ms ease, box-shadow 280ms ease;
    }
  }
  .hero-card.hero-connected {
    background:
      radial-gradient(120% 140% at 0% 0%, color-mix(in srgb, var(--green) 22%, var(--bg-card)) 0%, var(--bg-card) 70%);
    border-color: color-mix(in srgb, var(--green) 30%, var(--border));
    box-shadow: 0 4px 16px color-mix(in srgb, var(--green) 14%, transparent);
  }
  .hero-card.hero-connecting {
    background:
      radial-gradient(120% 140% at 0% 0%, color-mix(in srgb, var(--yellow) 22%, var(--bg-card)) 0%, var(--bg-card) 70%);
    border-color: color-mix(in srgb, var(--yellow) 30%, var(--border));
  }
  .hero-card.hero-warning {
    background:
      radial-gradient(120% 140% at 0% 0%, color-mix(in srgb, var(--orange, #FF9500) 22%, var(--bg-card)) 0%, var(--bg-card) 70%);
    border-color: color-mix(in srgb, var(--orange, #FF9500) 30%, var(--border));
  }

  /* Decorative glow blob in the top-right of connected state */
  .hero-glow {
    position: absolute;
    top: -40px;
    right: -40px;
    width: 140px;
    height: 140px;
    border-radius: 50%;
    pointer-events: none;
    filter: blur(40px);
    opacity: 0;
  }
  .hero-card.hero-connected .hero-glow {
    background: var(--green);
    opacity: 0.18;
  }
  .hero-card.hero-connecting .hero-glow {
    background: var(--yellow);
    opacity: 0.18;
  }
  .hero-card.hero-warning .hero-glow {
    background: var(--orange, #FF9500);
    opacity: 0.18;
  }

  /* Hero icon tile — 56x56 rounded square with state-colored bg/fg */
  .hero-icon {
    position: relative;
    display: flex;
    align-items: center;
    justify-content: center;
    width: 56px;
    height: 56px;
    border-radius: 14px;
    background: color-mix(in srgb, var(--text-muted) 14%, var(--bg-card));
    color: var(--text-muted);
    flex-shrink: 0;
    z-index: 1;
  }
  .hero-card.hero-connected .hero-icon {
    background: color-mix(in srgb, var(--green) 22%, transparent);
    color: var(--green);
  }
  .hero-card.hero-connecting .hero-icon {
    background: color-mix(in srgb, var(--yellow) 22%, transparent);
    color: var(--yellow);
  }
  .hero-card.hero-warning .hero-icon {
    background: color-mix(in srgb, var(--orange, #FF9500) 22%, transparent);
    color: var(--orange, #FF9500);
  }
  @keyframes hero-icon-pulse {
    0%, 100% { box-shadow: 0 0 0 0 color-mix(in srgb, var(--green) 55%, transparent); }
    55% { box-shadow: 0 0 0 10px color-mix(in srgb, var(--green) 0%, transparent); }
  }
  @media (prefers-reduced-motion: no-preference) {
    .hero-card.hero-connected .hero-icon {
      animation: hero-icon-pulse 2.6s ease-out infinite;
    }
  }

  .hero-body {
    flex: 1;
    min-width: 0;
    z-index: 1;
  }
  .hero-name-row {
    display: flex;
    align-items: center;
    gap: 6px;
  }
  .hero-name {
    margin: 0;
    font: 700 22px/28px var(--font-sans);
    color: var(--text-primary);
    letter-spacing: -0.02em;
    cursor: text;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .btn-rename {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    background: transparent;
    border: 0;
    color: var(--text-muted);
    cursor: pointer;
    padding: 4px;
    border-radius: 6px;
    opacity: 0.65;
  }
  .btn-rename:hover {
    background: rgba(255,255,255,0.06);
    opacity: 1;
  }
  .rename-input {
    font: 700 22px/28px var(--font-sans);
    letter-spacing: -0.02em;
    padding: 2px 8px;
    background: var(--bg-input);
    border: 1px solid var(--accent);
    border-radius: 6px;
    color: var(--text-primary);
    outline: none;
    flex: 1;
    max-width: 320px;
    box-shadow: 0 0 0 3px var(--accent-tint);
  }

  .hero-status-line {
    display: flex;
    align-items: center;
    gap: 6px;
    margin-top: 6px;
    font: 500 12px/16px var(--font-sans);
    color: var(--text-secondary);
    min-width: 0;
  }
  .hero-dot {
    width: 8px;
    height: 8px;
    border-radius: 50%;
    background: color-mix(in srgb, var(--text-muted) 55%, transparent);
    flex-shrink: 0;
  }
  .hero-dot.on {
    background: var(--green);
    box-shadow: 0 0 8px color-mix(in srgb, var(--green) 90%, transparent);
  }
  .hero-dot.warning { background: var(--orange, #FF9500); }
  @keyframes dot-blink {
    0%, 100% { opacity: 1; transform: scale(1); }
    50% { opacity: 0.5; transform: scale(0.85); }
  }
  @media (prefers-reduced-motion: no-preference) {
    .hero-dot.connecting {
      background: var(--yellow);
      animation: dot-blink 1.2s ease-in-out infinite;
    }
  }
  .hero-state-text {
    color: var(--text-primary);
    font-weight: 600;
    letter-spacing: -0.01em;
  }
  .hero-card.hero-connected .hero-state-text { color: var(--green); }
  .hero-card.hero-connecting .hero-state-text { color: var(--yellow); }
  .hero-card.hero-warning .hero-state-text { color: var(--orange, #FF9500); }
  .hero-sep { color: var(--text-muted); opacity: 0.6; }
  .hero-endpoint {
    display: inline-block;
    color: var(--text-secondary);
    font-family: var(--font-mono);
    font-size: 11px;
    /* The app turns selection off globally; this is one of the few values a
       user is expected to lift out of the UI, so it opts back in. Without
       this the text renders but cannot be highlighted at all. */
    user-select: text;
    -webkit-user-select: text;
    cursor: text;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    min-width: 0;
    max-width: 100%;
    vertical-align: bottom;
  }
  .hero-endpoint-text {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    min-width: 0;
  }
  /* The name's CURRENT address, in parentheses. Dimmer than the name
     because the name is what the config says and the address is a live
     measurement — but same monospace, so the two read as one value. */
  .hero-endpoint-ip {
    margin-left: 3px;
    font-family: var(--font-mono);
    opacity: 0.72;
  }
  .awg-badge {
    padding: 0 6px;
    border-radius: 999px;
    font: 600 9px/16px var(--font-sans);
    letter-spacing: 0.02em;
    /* No text-transform — preserves the AmneziaWG wordmark. */
    white-space: nowrap;
    /* Matches the tunnel-list badge geometry in both states. */
    min-width: 86px;
    text-align: center;
    color: var(--purple);
    background: color-mix(in srgb, var(--purple) 13%, transparent);
    border: 1px solid color-mix(in srgb, var(--purple) 32%, transparent);
  }
  /* AWG support disabled in Settings — same geometry, red palette. */
  .awg-badge--off {
    color: var(--danger, #d33);
    background: color-mix(in srgb, var(--danger, #d33) 13%, transparent);
    border-color: color-mix(in srgb, var(--danger, #d33) 32%, transparent);
  }

  /* "Handshake / Duration / Interface" used to be a 11px line under the
     latency card; it now lives in the hero and is enlarged so the live
     connection facts read at a glance. */
  .hero-meta {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: 8px;
    margin-top: 10px;
    font: 600 13px/18px var(--font-sans);
    color: var(--text-secondary);
  }
  .hero-meta .meta-item {
    display: inline-flex;
    align-items: center;
    gap: 5px;
  }
  .hero-meta .meta-item :global(svg) { opacity: 0.8; }
  .hero-meta .mono {
    font-family: var(--font-mono);
    font-size: 12px;
    color: var(--text-primary);
  }
  .hero-meta .meta-sep { opacity: 0.5; }

  /* ========== PRIMARY ACTION ==========
     Big full-width gradient button below the hero card. */
  /* Sticky so the manual Connect/Disconnect control never scrolls out of
     reach in a short window (hero + stats + info + notes can overflow).
     Solid panel bg covers the hero as it scrolls under; a soft downward
     shadow reads as a persistent action bar. */
  .primary-action {
    position: sticky;
    top: 0;
    z-index: 6;
    margin-bottom: 10px;
    padding: 4px 0 6px;
    background: var(--bg-primary);
    box-shadow: 0 10px 10px -10px color-mix(in srgb, #000 45%, transparent);
  }
  .btn-primary-large {
    width: 100%;
    min-height: 52px;
    padding: 8px 20px;
    border: 0;
    border-radius: 12px;
    font: 600 15px/22px var(--font-sans);
    letter-spacing: -0.01em;
    cursor: pointer;
    color: #fff;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: 1px;
    position: relative;
    overflow: hidden;
  }
  /* Single line: icon + the action itself ("Connect"). The full manual/
     enforcement explanation lives in the native title tooltip. */
  .btn-primary-large .pa-line {
    display: inline-flex;
    align-items: center;
    gap: 8px;
  }
  @media (prefers-reduced-motion: no-preference) {
    .btn-primary-large {
      transition: filter 180ms ease, transform 180ms ease, box-shadow 180ms ease;
    }
  }
  .btn-primary-large:disabled { opacity: 0.55; cursor: not-allowed; }
  /* Connect = green ("go"): a filled, saturated action button. Green is the
     universal "connect / start" cue and now also doubles as the connected
     STATE color, so the button and the live-status dot agree. A 4px deep-green
     strip on the left ties it to the app icon's medical green. */
  .btn-connect-lg {
    background: linear-gradient(180deg, color-mix(in srgb, #15803d 88%, #fff) 0%, #15803d 100%);
    color: #fff;
    box-shadow: 0 8px 24px color-mix(in srgb, #15803d 42%, transparent),
                0 2px 4px rgba(0,0,0,0.10);
  }
  .btn-connect-lg::before {
    content: '';
    position: absolute;
    left: 0; top: 0; bottom: 0;
    width: 4px;
    background: #047857;   /* deep green — matches app icon green */
    border-radius: 12px 0 0 12px;
  }
  .btn-connect-lg:hover:not(:disabled) {
    background: linear-gradient(180deg, color-mix(in srgb, #15803d 80%, #fff) 0%, color-mix(in srgb, #15803d 92%, #000) 100%);
    transform: translateY(-1px);
    box-shadow: 0 10px 28px color-mix(in srgb, #15803d 50%, transparent),
                0 2px 4px rgba(0,0,0,0.12);
  }
  .btn-connect-lg:active:not(:disabled) {
    background: color-mix(in srgb, #000 8%, #15803d);
    transform: translateY(0);
  }

  /* Disconnect = red ("stop"): a filled, saturated button that clearly reads as
     the inverse of the green Connect. Now BOTH buttons are filled and colorful,
     so the two manual actions are immediately distinguishable by hue — green =
     bring the tunnel up, red = tear it down. */
  .btn-disconnect-lg {
    background: linear-gradient(180deg, color-mix(in srgb, #dc2626 88%, #fff) 0%, #dc2626 100%);
    color: #fff;
    border: 1px solid color-mix(in srgb, #dc2626 60%, #000);
    box-shadow: 0 8px 24px color-mix(in srgb, #dc2626 38%, transparent),
                0 2px 4px rgba(0,0,0,0.10);
  }
  .btn-disconnect-lg .pa-line { color: #fff; }
  .btn-disconnect-lg:hover:not(:disabled) {
    background: linear-gradient(180deg, color-mix(in srgb, #dc2626 80%, #fff) 0%, color-mix(in srgb, #dc2626 92%, #000) 100%);
    border-color: color-mix(in srgb, #dc2626 70%, #000);
    color: #fff;
    transform: translateY(-1px);
  }
  .btn-disconnect-lg:hover:not(:disabled) .pa-line { color: #fff; }
  .btn-disconnect-lg:active:not(:disabled) {
    background: color-mix(in srgb, #000 8%, #dc2626);
    transform: translateY(0);
  }

  /* Spinner inside connect button when connecting */
  .spinner {
    width: 14px;
    height: 14px;
    border: 2px solid rgba(255,255,255,0.35);
    border-top-color: #fff;
    border-radius: 50%;
    animation: spin 0.7s linear infinite;
  }
  @keyframes spin { to { transform: rotate(360deg); } }

  /* ========== STATS HERO ==========
     One row: RX / TX / latency counters plus the throughput graph.
     The counters only need room for a few characters, so they are pinned
     narrow and the graph takes everything left over — the reverse of the
     old layout, where the graph was a full-width panel further down and
     pushed the rest of the detail below the fold. */
  .stats-hero {
    display: grid;
    /* Three cells now: the latency counter moved into the latency card
       below, where the per-target breakdown already lives. Keeping a
       separate aggregate card meant the same reading existed twice and the
       two could disagree. */
    grid-template-columns: minmax(0, 0.7fr) minmax(0, 0.7fr) minmax(0, 2.6fr);
    gap: 8px;
    margin-bottom: 6px;
  }
  .stat-card {
    padding: 10px 11px 9px;
    background: var(--bg-card);
    border: 0.5px solid var(--border);
    border-radius: 12px;
    display: flex;
    flex-direction: column;
    gap: 4px;
    min-width: 0;
  }
  .stat-card-top {
    display: flex;
    align-items: center;
    gap: 6px;
  }
  .stat-icon {
    width: 19px;
    height: 19px;
    display: flex;
    align-items: center;
    justify-content: center;
    border-radius: 5px;
    background: color-mix(in srgb, var(--text-muted) 14%, transparent);
    color: var(--text-muted);
    flex-shrink: 0;
  }
  .stat-card.stat-rx .stat-icon {
    background: color-mix(in srgb, var(--green) 22%, transparent);
    color: var(--green);
  }
  .stat-card.stat-tx .stat-icon {
    background: color-mix(in srgb, var(--accent) 22%, transparent);
    color: var(--accent);
  }
  /* Latency card: headline + per-target rows. One card, because the headline
     IS the best of the rows — splitting them let the two disagree on screen
     with nothing to say which was right. */
  .latency-card {
    padding: 9px 11px 8px;
    background: var(--bg-card);
    border: 0.5px solid var(--border);
    border-radius: 12px;
    margin-bottom: 6px;
  }
  /* Latency display + target editor on ONE row: the reading and the input
     that drives it share a line so they are obviously the same thing.
     The reading column is the narrower one: a headline RTT plus a few rows
     need far less room than the editor, whose values are IP/hostname strings
     that should not be clipped mid-domain. */
  .latency-row {
    display: grid;
    grid-template-columns: minmax(0, 1fr) minmax(0, 1.2fr);
    gap: 12px;
    margin-bottom: 6px;
    align-items: stretch;
  }
  .latency-row .latency-card { margin-bottom: 0; }
  .latency-target-col {
    display: flex;
    flex-direction: column;
    min-width: 0;
  }
  .latency-target-col .probe-target-card { flex: 1 1 auto; }
  .latency-head {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    gap: 8px;
  }
  .latency-head-label {
    display: flex;
    align-items: center;
    gap: 6px;
    font: 500 10px/13px var(--font-sans);
    color: var(--text-muted);
    text-transform: uppercase;
    letter-spacing: 0.08em;
  }
  .latency-head-label :global(svg) {
    color: var(--yellow);
  }
  .latency-head-value {
    font: 700 18px/22px var(--font-sans);
    color: var(--text-primary);
    font-feature-settings: "tnum";
    letter-spacing: -0.02em;
  }
  .stat-label {
    font: 500 10px/13px var(--font-sans);
    color: var(--text-muted);
    text-transform: uppercase;
    letter-spacing: 0.08em;
  }
  .stat-value {
    font: 700 22px/26px var(--font-sans);
    color: var(--text-primary);
    font-feature-settings: "tnum";
    letter-spacing: -0.02em;
  }
  /* Counters hold "1.2 GB" at most — they do not need the 22px headline
     size, and shrinking them is what frees the width for the graph. */
  .stat-value-sm {
    font: 700 15px/19px var(--font-sans);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .stat-unit {
    font: 500 12px/16px var(--font-sans);
    color: var(--text-muted);
    letter-spacing: 0;
    margin-left: 2px;
  }

  /* The graph card has no label row of its own — it is all canvas, so it
     stretches to the row height instead of padding a heading. */
  .stat-graph {
    padding: 6px;
  }
  /* Narrow panes: four columns get too tight, so the graph drops to its own
     full-width line rather than squeezing the counters into ellipses. */
  @media (max-width: 560px) {
    .stats-hero {
      grid-template-columns: 1fr 1fr 1fr;
    }
    .stat-graph {
      grid-column: 1 / -1;
      min-height: 60px;
    }
  }

  /* ========== PROBE ROWS ==========
     One compact row per probe target, colour-coded by health. They live
     inside .latency-card (which owns the card chrome) so they need no
     surface of their own — just a divider from the headline above. */
  .probe-panel {
    margin-top: 5px;
    padding-top: 5px;
    border-top: 0.5px solid var(--border);
    display: flex;
    flex-direction: column;
    gap: 2px;
  }
  .probe-row {
    display: flex;
    align-items: center;
    gap: 6px;
    font: 11px/16px var(--font-sans);
    color: var(--text-secondary);
    min-width: 0;
  }
  .probe-dot {
    width: 7px;
    height: 7px;
    border-radius: 50%;
    flex-shrink: 0;
    background: var(--text-muted);
  }
  .probe-dot.dot-ok { background: var(--green); }
  .probe-dot.dot-warn { background: var(--yellow, #f59e0b); }
  .probe-dot.dot-bad { background: var(--red, #ef4444); }
  .probe-dot.dot-idle { background: var(--text-muted); opacity: 0.45; }
  /* Probe rows are addresses, and addresses get copied: the resolved IP of
     a DDNS target, the endpoint actually being measured. Opt them in — the
     global opt-out would otherwise leave them unselectable. */
  .probe-name,
  .probe-resolved,
  .probe-slot-resolved {
    user-select: text;
    -webkit-user-select: text;
    cursor: text;
  }
  .probe-name {
    color: var(--text-primary);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    flex-shrink: 0;
    max-width: 42%;
  }
  .probe-resolved {
    color: var(--text-muted);
    font-size: 10px;
    /* Without this a flex item refuses to shrink below its content width and
       pushes the row past the card edge — exactly what happens now that the
       latency column is the narrow one. */
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .probe-kind {
    color: var(--text-muted);
    font-size: 10px;
    padding: 0 4px;
    border-radius: 4px;
    background: color-mix(in srgb, var(--text-muted) 12%, transparent);
    flex-shrink: 0;
  }
  .probe-ms {
    margin-left: auto;
    font-variant-numeric: tabular-nums;
    color: var(--text-muted);
    flex-shrink: 0;
  }
  .probe-ms.ms-ok { color: var(--green); }
  .probe-ms.ms-warn { color: var(--yellow, #f59e0b); }
  .probe-ms.ms-bad { color: var(--red, #ef4444); }
  .probe-summary {
    margin-top: 2px;
    padding-top: 4px;
    border-top: 0.5px solid var(--border);
    font: 10px/14px var(--font-sans);
    color: var(--yellow, #f59e0b);
  }
  /* A row whose address is not routed through this tunnel: the number is
     still shown (it is a real measurement) but the row is marked, because
     reading it as "this tunnel's latency" would be wrong. */
  .probe-row-outside .probe-name,
  .probe-row-outside .probe-ms {
    color: var(--text-muted);
  }
  .probe-kind-outside {
    color: var(--red, #ef4444);
    background: color-mix(in srgb, var(--red, #ef4444) 14%, transparent);
  }

  .meta-item {
    display: inline-flex;
    align-items: center;
    gap: 4px;
  }
  .meta-sep { opacity: 0.5; }

  /* ========== INFO SECTION ==========
     Card with rows + hairline dividers (iOS Settings style). */
  .info-section {
    margin-bottom: 12px;
  }
  .section-label {
    margin: 0 0 6px 4px;
    font: 500 10px/13px var(--font-sans);
    color: var(--text-muted);
    text-transform: uppercase;
    letter-spacing: 0.08em;
  }
  .info-card {
    background: var(--bg-card);
    border: 0.5px solid var(--border);
    border-radius: 12px;
    overflow: hidden;
  }
  .endpoint-card {
    min-height: 50px;
    padding: 11px 14px;
    box-sizing: border-box;
    display: flex;
    justify-content: center;
    flex-direction: column;
  }
  .latency-target-input {
    width: 100%;
    min-width: 0;
    box-sizing: border-box;
    height: 26px;
    padding: 0 8px;
    background: color-mix(in srgb, var(--bg-primary) 78%, transparent);
    border: 0.5px solid var(--border);
    border-radius: 7px;
    color: var(--text-primary);
    font: 11px/16px var(--font-mono);
    outline: none;
  }
  @media (prefers-reduced-motion: no-preference) {
    .latency-target-input {
      transition: border-color 140ms ease, box-shadow 140ms ease, background 140ms ease;
    }
  }
  .latency-target-input:focus-visible {
    border-color: var(--accent);
    box-shadow: 0 0 0 3px var(--accent-tint);
    background: var(--bg-primary);
  }
  .latency-target-input::placeholder {
    color: var(--text-muted);
    font-family: var(--font-sans);
  }
  /* --- Four-slot probe target editor ---
     Rows are positional and always visible (no edit/read modes): with four
     of them, a click-to-edit affordance per row would cost more attention
     than the fields are worth, and the resolved address has to sit next to
     the box at all times anyway. */
  .probe-target-card {
    gap: 4px;
    justify-content: flex-start;
  }
  .probe-slot {
    display: flex;
    align-items: center;
    gap: 6px;
    min-width: 0;
  }
  .probe-slot-input {
    flex: 1 1 auto;
  }
  .probe-slot-resolved {
    flex: 0 1 auto;
    max-width: 45%;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-size: 10px;
    color: var(--text-muted);
  }
  /* Not routed through this tunnel: the row is marked where the value is
     typed, and the marking survives a reload because it is driven by the
     probe stream as well as the local pre-check. */
  .probe-slot-invalid .probe-slot-input {
    border-color: var(--red, #ef4444);
    box-shadow: 0 0 0 2px color-mix(in srgb, var(--red, #ef4444) 18%, transparent);
  }
  .probe-slot-invalid .probe-slot-resolved {
    color: var(--red, #ef4444);
  }
  .latency-target-error {
    display: block;
    color: var(--red);
    font: 10px/14px var(--font-sans);
  }
  /* Nothing was saved, so the only way forward is a different value —
     this button puts the caret back in the field with the bad text
     selected, rather than leaving the user to find the pencil again. */
  .latency-retry-btn {
    margin-left: 6px;
    padding: 0 6px;
    font: 10px/14px var(--font-sans);
    color: var(--red);
    background: color-mix(in srgb, var(--red) 12%, transparent);
    border: 0.5px solid color-mix(in srgb, var(--red) 35%, transparent);
    border-radius: 4px;
    cursor: pointer;
  }
  .latency-retry-btn:hover {
    background: color-mix(in srgb, var(--red) 22%, transparent);
  }
  .probe-slot-pending {
    flex-shrink: 0;
    padding: 0 5px;
    border-radius: 999px;
    font: 9px/15px var(--font-sans);
    color: var(--text-muted);
    background: color-mix(in srgb, var(--text-muted) 12%, transparent);
    white-space: nowrap;
  }
  .latency-target-foot {
    display: flex;
    align-items: baseline;
    gap: 6px;
    margin-top: 4px;
  }
  .latency-target-hint {
    flex: 1 1 auto;
    min-width: 0;
    font: 10px/14px var(--font-sans);
    opacity: 0.65;
  }
  /* Collapsed by default — see the markup comment. Opacity, not display:
     it stays a real control for keyboard/screen-reader users either way. */
  .hint-toggle {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    flex-shrink: 0;
    padding: 0 6px;
    height: 16px;
    border: 0.5px solid var(--border);
    border-radius: 999px;
    background: transparent;
    color: var(--text-muted);
    font: 10px/14px var(--font-sans);
    cursor: pointer;
  }
  .hint-toggle:hover {
    color: var(--text-secondary);
    border-color: color-mix(in srgb, var(--accent) 40%, transparent);
  }
  .hint-toggle-open {
    color: var(--accent);
    border-color: color-mix(in srgb, var(--accent) 45%, transparent);
  }
  .hint-toggle :global(svg) { flex-shrink: 0; }
  .latency-target-details {
    margin: 6px 0 0;
    font: 10px/15px var(--font-sans);
    color: var(--text-muted);
    opacity: 0.9;
  }
  /* Narrow panes: the latency row stacks instead of squeezing the two cards
     into ellipses. */
  @media (max-width: 520px) {
    .latency-row {
      grid-template-columns: 1fr;
    }
  }

  /* ========== NOTES ========== */
  .notes-textarea {
    width: 100%;
    box-sizing: border-box;
    padding: 10px 14px;
    background: var(--bg-card);
    border: 0.5px solid var(--border);
    border-radius: 12px;
    color: var(--text-primary);
    font: 13px/18px var(--font-sans);
    line-height: 1.5;
    resize: vertical;
    min-height: 56px;
    max-height: 200px;
    outline: none;
  }
  @media (prefers-reduced-motion: no-preference) {
    .notes-textarea {
      transition: border-color 140ms ease, box-shadow 140ms ease;
    }
  }
  .notes-textarea:focus-visible {
    border-color: var(--accent);
    box-shadow: 0 0 0 3px var(--accent-tint);
  }
  .notes-textarea::placeholder { color: var(--text-muted); }
  .notes-error {
    margin: 6px 0 0 4px;
    font: 11px/15px var(--font-sans);
    color: var(--red);
  }

  /* ========== ERROR ========== */
  .error-msg {
    padding: 10px 14px;
    margin-bottom: 14px;
    background: var(--error-bg);
    border: 0.5px solid var(--red);
    border-radius: 10px;
    color: var(--error-text);
    font: 13px/18px var(--font-sans);
  }

  /* ========== SECONDARY ACTIONS ==========
     4-column icon-button grid at the bottom. */
  .secondary-actions {
    display: grid;
    grid-template-columns: repeat(4, 1fr);
    gap: 8px;
    margin-top: 4px;
  }
  .btn-icon-action {
    display: inline-flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: 4px;
    height: 64px;
    padding: 0 8px;
    background: var(--bg-card);
    border: 0.5px solid var(--border);
    border-radius: 12px;
    color: var(--text-primary);
    font: 500 11px/14px var(--font-sans);
    cursor: pointer;
    position: relative;
  }
  @media (prefers-reduced-motion: no-preference) {
    .btn-icon-action {
      transition: background 140ms ease, border-color 140ms ease, transform 140ms ease;
    }
  }
  .btn-icon-action:hover {
    background: var(--bg-hover);
    border-color: color-mix(in srgb, var(--accent) 35%, var(--border));
    transform: translateY(-1px);
  }
  .btn-icon-action:active { transform: translateY(0); background: var(--bg-active); }
  .btn-icon-action.btn-icon-danger { color: var(--red); }
  .btn-icon-action.btn-icon-danger:hover {
    background: color-mix(in srgb, var(--red) 8%, var(--bg-card));
    border-color: color-mix(in srgb, var(--red) 40%, var(--border));
    color: var(--red);
  }

  /* ========== Modal dialog (Wi-Fi auto-connect + Delete confirm) ========== */
  .confirm-backdrop {
    position: fixed;
    inset: 0;
    background: var(--overlay-bg);
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 400;
  }
  .confirm-dialog {
    background: var(--bg-primary);
    border: 0.5px solid var(--border);
    border-radius: 14px;
    padding: 22px 26px 18px;
    width: 480px;
    box-shadow: var(--shadow-lg);
  }

  /* Shared dialog header: icon tile + title + optional subtitle */
  .dialog-header {
    display: flex;
    align-items: center;
    gap: 12px;
    margin-bottom: 10px;
  }
  .dialog-icon-tile {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 36px;
    height: 36px;
    border-radius: 10px;
    background: color-mix(in srgb, var(--accent) 16%, transparent);
    color: var(--accent);
    flex-shrink: 0;
  }
  .dialog-icon-tile.danger {
    background: color-mix(in srgb, var(--red) 16%, transparent);
    color: var(--red);
  }
  .dialog-header-text {
    flex: 1;
    min-width: 0;
  }
  .confirm-dialog h3 {
    margin: 0;
    color: var(--text-primary);
    font: 700 15px/20px var(--font-sans);
    letter-spacing: -0.01em;
  }
  .dialog-message {
    margin: 0 0 18px;
    font: 13px/19px var(--font-sans);
    color: var(--text-secondary);
  }
  .confirm-footer {
    display: flex;
    gap: 8px;
    justify-content: flex-end;
    margin-top: 14px;
  }

  /* Dialog buttons — 32px height, gradient primary / red danger / ghost cancel */
  .dialog-btn {
    height: 32px;
    min-width: 76px;
    padding: 0 16px;
    border: 0;
    border-radius: 9px;
    font: 600 13px/18px var(--font-sans);
    letter-spacing: -0.005em;
    cursor: pointer;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    gap: 5px;
    color: var(--text-primary);
  }
  @media (prefers-reduced-motion: no-preference) {
    .dialog-btn {
      transition: filter 140ms ease, background-color 140ms ease,
                  border-color 140ms ease, transform 140ms ease,
                  box-shadow 140ms ease;
    }
  }
  .dialog-btn:disabled { opacity: 0.4; cursor: not-allowed; }

  .dialog-btn-danger {
    background: var(--red);
    color: #fff;
    box-shadow:
      0 1px 3px color-mix(in srgb, var(--red) 26%, transparent),
      0 1px 2px rgba(0,0,0,0.08);
  }
  .dialog-btn-danger:hover:not(:disabled) {
    background: color-mix(in srgb, #fff 8%, var(--red));
    transform: translateY(-1px);
    box-shadow:
      0 4px 10px color-mix(in srgb, var(--red) 30%, transparent),
      0 1px 2px rgba(0,0,0,0.10);
  }
  .dialog-btn-danger:active:not(:disabled) {
    background: color-mix(in srgb, #000 8%, var(--red));
    transform: translateY(0);
  }

  .dialog-btn-ghost {
    background: var(--bg-card);
    border: 0.5px solid var(--border);
    color: var(--text-primary);
  }
  .dialog-btn-ghost:hover { background: var(--bg-hover); }
  .dialog-btn-ghost:active { background: var(--bg-active); }
</style>
