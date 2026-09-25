package helper

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/imonior/wireguide-plus/internal/ipc"
	"github.com/imonior/wireguide-plus/internal/network"
	"github.com/imonior/wireguide-plus/internal/storage"
	"github.com/imonior/wireguide-plus/internal/wifi"
)

// loadUserSettings reads the user's settings.json directly. Reading
// fresh on every SSID transition (instead of caching + IPC sync from
// the GUI) means rule edits made in Settings take effect on the next
// network change without any explicit push, and there's no "in-memory
// state diverged from disk" failure mode.
func (h *Helper) loadUserSettings() (*storage.Settings, error) {
	if h.userAppSupport == "" {
		return nil, fmt.Errorf("user app-support dir not derived")
	}
	path := filepath.Join(h.userAppSupport, "config.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return storage.DefaultSettings(), nil
		}
		return nil, err
	}
	s := storage.DefaultSettings()
	if err := json.Unmarshal(data, s); err != nil {
		return nil, err
	}
	return s, nil
}

// loadTunnelBinding reads the per-tunnel physical-egress binding from the
// meta sidecar in the user dir (the helper keeps no TunnelStore). Returns
// (0, "") when absent or unreadable — every failure mode means "auto", the
// conservative default. Mirrors the authoritative-check pattern of
// loadUserSettings so CLI/automation connects get the same binding as
// GUI-initiated ones.
func (h *Helper) loadTunnelBinding(name string) (int, string) {
	if h.userAppSupport == "" {
		return 0, ""
	}
	if err := storage.ValidateTunnelName(name); err != nil {
		return 0, ""
	}
	path := filepath.Join(h.userAppSupport, "tunnels", name+".meta.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, ""
	}
	var meta struct {
		BindIfIndex int    `json:"bind_if_index"`
		BindIfName  string `json:"bind_if_name"`
	}
	if json.Unmarshal(data, &meta) != nil {
		return 0, ""
	}
	return meta.BindIfIndex, meta.BindIfName
}

// currentNetworkContext builds the NetworkContext automation rules are
// evaluated against — the single source for both the live engine
// (reevaluateAutomation) and the read-only preview, so `wireguideplus ctl
// automation` always shows exactly what the engine would act on.
//
// SSID staleness: on macOS the SSID only arrives via GUI reports (the
// root helper can't read it). If the GUI has exited and the machine then
// moved networks, that value is stale. A gateway MAC different from the
// one stamped at report time proves the network changed since the report,
// so the SSID is treated as unknown — SSID rules stop matching rather
// than misfiring on the old network's name, while subnet/MAC rules keep
// working off fresh data. An empty stamp (no report yet, or gateway
// unknown at report time) never invalidates.
func (h *Helper) currentNetworkContext() wifi.NetworkContext {
	ssid := ""
	if h.wifiMon != nil {
		ssid = h.wifiMon.LastSSID()
	}
	gw := wifi.GatewayMAC()
	if ssid != "" && gw != "" {
		h.wifiMu.Lock()
		stamp := h.ssidStampGW
		h.wifiMu.Unlock()
		if stamp != "" && stamp != gw {
			slog.Debug("SSID considered stale: gateway changed since GUI report",
				"ssid", ssid, "stamped_gw", stamp, "current_gw", gw)
			ssid = ""
		}
	}
	return wifi.NetworkContext{
		SSID:        ssid,
		PhysicalIPs: wifi.PhysicalInterfaceIPs(),
		GatewayMAC:  gw,
		GatewayIP:   wifi.GatewayIP(),
		Interfaces:  wifi.PhysicalInterfaces(),
	}
}

// handleSSIDChange is one trigger for Automation re-evaluation: the
// Wi-Fi monitor fires it on every SSID transition. It only posts a
// coalesced request — the decision logic (and the network-context probe)
// lives in reevaluateAutomation, run by automationEvalLoop, which every
// trigger (SSID change, network change, poll, post-connect) shares.
func (h *Helper) handleSSIDChange(oldSSID, newSSID string) {
	h.requestAutomationEval("ssid-change")
}

const (
	// startupRuleCheckWindow is how long after helper start a manual
	// connect gets an immediate rule re-check.
	startupRuleCheckWindow = 60 * time.Second
	// postConnectRuleCheckDelay is how long after a connect we wait before
	// re-evaluating the rules.
	postConnectRuleCheckDelay = 3 * time.Second

	// settleRecheckDelay is how long after an evaluation that acted (or
	// skipped a tunnel as SSID-blind) a follow-up re-evaluation runs. The
	// coalesced engine only re-runs on the next event, and a mid-flap
	// context (SSID blanked while its network is otherwise back) may never
	// be followed by one: macOS has no 30s poll, and an SSID that returned
	// to its old value fires no transition. The recheck re-samples the
	// context so the engine converges without depending on an event.
	settleRecheckDelay = 8 * time.Second

	// evalReasonSettle tags the follow-up recheck. A settle recheck that
	// still finds things blind or acts again schedules no further recheck
	// on purpose: the normal triggers remain the safety net and a
	// self-perpetuating 8s timer would replace the quiet fail-closed skip
	// with a steady evaluation loop.
	evalReasonSettle = "settle-recheck"

	// evalReasonStartup tags the one-shot evaluation the helper runs
	// shortly after start (login / boot / LaunchDaemon restart). It is a
	// log label only — startup evaluations obey exactly the same rules as
	// every other trigger, including the unidentified-network skip below.
	evalReasonStartup = "startup"
)

// scheduleRuleCheck re-runs the automation rules shortly after a manual
// (RPC) connect made during the helper's startup window. The startup
// evaluation already decides each tunnel's intended state from the rules;
// a connect that arrives afterwards (e.g. a GUI restore of the last
// session) would otherwise stay up until the 30s poll notices and tears
// it down — looking like the app "connects first, then obeys the rules".
// A quick post-connect eval makes the rule decision authoritative within
// a few seconds. Outside the startup window a deliberate manual connect
// is left alone and the regular poll remains the fallback.
func (h *Helper) scheduleRuleCheck() {
	if time.Since(h.startedAt) > startupRuleCheckWindow {
		return
	}
	go func() {
		select {
		case <-h.done:
			return
		case <-time.After(postConnectRuleCheckDelay):
		}
		h.requestAutomationEval("post-connect")
	}()
}

// reconcileAction maps an evaluated desired state plus the tunnel's
// actual reality to the ONE action the engine takes:
//
//	"connect"         → bring the tunnel up (rule/default says connect, it
//	                    is down, and the manual-off latch is not set)
//	"disconnect"      → tear the tunnel down (rule/default says disconnect
//	                    and it is up — regardless of who brought it up)
//	"skip-manual-off" → the policy wants connect but the user switched the
//	                    tunnel off by hand; the latch wins until they
//	                    reconnect manually or the app restarts
//	"skip-manual-on"  → the policy wants disconnect but the user switched
//	                    the tunnel on by hand; the latch wins until they
//	                    disconnect manually or the app restarts
//	""                → leave the tunnel alone (policy agrees with reality,
//	                    the tunnel is unmanaged, or a wanted disconnect has
//	                    nothing to tear down)
//
// The two manual latches are mutually exclusive (a tunnel is at most one of
// off/on): manual-off suppresses connect, manual-on suppresses disconnect.
// Pure and side-effect free so the reconcile invariants (idempotency,
// manual-latch supremacy) are testable without a live tunnel manager.
func reconcileAction(state wifi.DesiredState, active, manualOff, manualOn bool) string {
	switch state {
	case wifi.StateConnect:
		if manualOff {
			return "skip-manual-off"
		}
		if !active {
			return "connect"
		}
	case wifi.StateDisconnect:
		if manualOn {
			return "skip-manual-on"
		}
		if active {
			return "disconnect"
		}
	}
	return ""
}

// ssidBlind reports whether one tunnel's policy is UNDECIDABLE this
// round: the rule set conditions on the SSID while the SSID is unknown
// (a Wi-Fi uplink is present but no report backs it — see
// NetworkContext.SSIDUndecidable). Evaluating anyway is what let a
// same-SSID static→DHCP switch misconnect a tunnel: the ssid-conditioned
// disconnect rules could not match an empty SSID, so the Default State
// of "connect" won the round and brought the tunnel up. Fail closed:
// take no action on such a tunnel until the SSID is known again — a
// refused decision costs seconds, a wrong one costs an unwanted tunnel
// that (on macOS, with no poll) can persist until the next event.
func ssidBlind(rules []wifi.Rule, ctx wifi.NetworkContext) bool {
	return ctx.SSIDUndecidable() && wifi.RulesReferenceSSID(rules)
}

// reevaluateAutomation drives every tunnel that has Automation rules
// toward its desired state for the current network context (SSID +
// physical-interface subnets). This runs entirely inside the helper, so
// rules keep firing whether or not a GUI is alive.
//
// Semantics (issue #12): a rule can connect OR disconnect its tunnel
// regardless of how it was brought up. A tunnel with neither rules nor
// an explicit Default State is never touched; a default-only policy
// always converges on its Default State. On an UNIDENTIFIED network the
// whole evaluation is skipped (see below) — no condition, no action. A
// tunnel whose rules depend on an SSID that is currently unknown is
// skipped individually (see ssidBlind) — the Default State never
// overrides conditions the context cannot judge.
//
// Concurrency: triggers must NOT call this directly — they post through
// requestAutomationEval, and automationEvalLoop is the only caller. A
// mid-burst trigger therefore never queues a stale duplicate evaluation;
// the NetworkContext is sampled fresh at the top of each run. reevalMu
// remains as defence-in-depth serialisation.
func (h *Helper) reevaluateAutomation(reason string) {
	h.reevalMu.Lock()
	defer h.reevalMu.Unlock()

	settings, err := h.loadUserSettings()
	if err != nil {
		slog.Debug("automation: cannot load settings", "error", err)
		return
	}
	settings.EnsureAutomation()
	auto := settings.Automation
	// Both maps count: a tunnel with rules has a policy, and so does a
	// tunnel with only a Default State ("always converge to it"). Only a
	// tunnel in NEITHER map is policy-free.
	if auto == nil || (len(auto.PerTunnel) == 0 && len(auto.Defaults) == 0) {
		return
	}

	ctx := h.currentNetworkContext()

	// An UNIDENTIFIED network (no SSID reported AND no physical-interface
	// address yet) means no condition can be judged: SSID rules have
	// nothing to match against, subnet/MAC rules have no interfaces to
	// inspect. Acting anyway would blind-connect tunnels onto a network
	// we haven't seen — then tear them down seconds later when the SSID
	// report arrives — or wrongly tear down a crash-recovered tunnel.
	// So: skip the entire evaluation, in BOTH directions, and wait. The
	// SSID report, route-change events and the poll each re-post an eval
	// request, so the real decision runs the moment the network is
	// identified — a Default State of "connect" therefore still takes
	// effect at autostart, only a few seconds later.
	if ctx.SSID == "" && len(ctx.PhysicalIPs) == 0 {
		slog.Debug("automation: network unidentified — skipping evaluation",
			"reason", reason)
		return
	}

	// Context line: every evaluation states WHAT it judged against. This
	// is the first thing anyone diagnosing "the tunnel didn't come up"
	// needs, and it used to be missing entirely.
	ipStrs := make([]string, 0, len(ctx.PhysicalIPs))
	for _, ip := range ctx.PhysicalIPs {
		ipStrs = append(ipStrs, ip.String())
	}
	ifaces := make([]string, 0, len(ctx.Interfaces))
	for _, inf := range ctx.Interfaces {
		ifaces = append(ifaces, inf.Name)
	}
	slog.Info("automation: evaluating",
		"category", "network",
		"reason", reason,
		"tunnels", len(auto.PolicyTunnelNames()),
		"ssid", ctx.SSID,
		"gateway_mac", ctx.GatewayMAC,
		"gateway_ip", ctx.GatewayIP,
		"physical_ips", strings.Join(ipStrs, ","),
		"interfaces", strings.Join(ifaces, ","))

	// A tunnel the user has manually switched off (UI or tray menu) must
	// not be silently reconnected by its rules until they reconnect it by
	// hand once or the app restarts — the manual off wins over automation.
	manualOff := make(map[string]bool, len(settings.ManualOffTunnels))
	for _, n := range settings.ManualOffTunnels {
		manualOff[n] = true
	}
	// The mirror latch: a tunnel the user has manually switched ON must not
	// be silently torn down by its rules until they disconnect it by hand
	// once or the app restarts — the manual on wins over automation.
	manualOn := make(map[string]bool, len(settings.ManualOnTunnels))
	for _, n := range settings.ManualOnTunnels {
		manualOn[n] = true
	}

	active := make(map[string]bool)
	for _, n := range h.manager.ActiveTunnels() {
		active[n] = true
	}

	// acted: this round carried out a connect/disconnect; blindSkipped:
	// this round skipped at least one tunnel because its rules depend on
	// an SSID that is currently unknown. Either means a decision is in
	// flight or deferred on context data that may land without firing any
	// event — re-sample it shortly (see settleRecheckDelay).
	var acted, blindSkipped bool

	for _, name := range auto.PolicyTunnelNames() {
		// A tunnel the user exempted from automation is never touched: its
		// rules stay stored and previewable, but the engine leaves it alone
		// — the durable counterpart to the per-session manual latches.
		if h.automationDisabled(name) {
			slog.Debug("automation: tunnel exempted (automation disabled)",
				"category", "network", "tunnel", name, "reason", reason)
			continue
		}
		// An unresolved address conflict parks the tunnel until the user
		// answers the dialog (stop automation / keep trying) — retrying into
		// the same "another adapter holds my address" wall is exactly the
		// loop the pause exists to break.
		if h.isAutoConnectPaused(name) {
			slog.Debug("automation: connect skipped (paused on address conflict, awaiting user decision)",
				"category", "network", "tunnel", name, "reason", reason)
			continue
		}
		// The situation a back-off reacted to is over as soon as the tunnel
		// is up, the user has taken over (either latch), or the network
		// changed — let the next attempt through.
		if active[name] || manualOff[name] || manualOn[name] || reason == "ssid-change" {
			h.clearAutoConnectBackoff(name)
		}
		rules := auto.PerTunnel[name]
		// SSID-blind fail-closed gate (see ssidBlind): the rules depend on
		// an SSID this context cannot supply, so neither a rule match nor
		// the Default State fallback is trustworthy this round. Skip the
		// tunnel entirely rather than let the fallback act on half a
		// picture — this is precisely the round that used to misconnect
		// tunnels during a same-SSID static→DHCP switch.
		if ssidBlind(rules, ctx) {
			blindSkipped = true
			slog.Info("automation: skip (SSID unknown while rules depend on it — no default fallback on half a context)",
				"category", "network",
				"tunnel", name, "reason", reason,
				"rules", len(rules),
				"default", auto.Defaults[name],
				"active", active[name])
			continue
		}
		state := wifi.EvaluatePolicy(rules, auto.Defaults[name], ctx)
		switch reconcileAction(state, active[name], manualOff[name], manualOn[name]) {
		case "connect":
			// A tunnel stuck in a retry loop (its address held elsewhere,
			// an unreachable endpoint) must not be re-hammered every poll;
			// skip until its back-off window elapses. Logged at Debug so the
			// steady-state poll stays quiet — the failure itself was already
			// reported at Warn when it happened.
			if h.autoConnectBackingOff(name) {
				slog.Debug("automation: connect deferred (back-off after repeated failures)",
					"category", "network", "tunnel", name, "reason", reason)
				continue
			}
			acted = true
			h.automationConnect(name, reason, ctx.SSID)
		case "disconnect":
			acted = true
			slog.Info("automation: rule disconnect",
				"category", "network",
				"tunnel", name, "reason", reason, "ssid", ctx.SSID)
			h.disconnectAutoManaged(name)
		case "skip-manual-off":
			slog.Info("automation: skip connect (manually switched off)",
				"category", "network",
				"tunnel", name, "reason", reason, "ssid", ctx.SSID)
		case "skip-manual-on":
			slog.Info("automation: skip disconnect (manually switched on)",
				"category", "network",
				"tunnel", name, "reason", reason, "ssid", ctx.SSID)
		default:
			// No action — log it anyway. Without this line an evaluation
			// that leaves everything alone is invisible in the log viewer,
			// and "why didn't my tunnel connect?" has no answer.
			slog.Info("automation: no action",
				"category", "network",
				"tunnel", name,
				"reason", reason,
				"decision", decisionLabel(state, manualOff[name], manualOn[name]),
				"rules", len(rules),
				"default", auto.Defaults[name],
				"active", active[name],
				"ssid", ctx.SSID)
		}
	}

	// One-shot settle recheck: after an action or a blind skip, re-sample
	// the context shortly so the engine converges even when the healing
	// state change fires no event (the macOS case: no poll, and an SSID
	// returning to its old value transitions nothing). Deliberately not
	// scheduled from a settle recheck itself — see evalReasonSettle.
	if (acted || blindSkipped) && reason != evalReasonSettle {
		go func() {
			select {
			case <-h.done:
				return
			case <-time.After(settleRecheckDelay):
			}
			h.requestAutomationEval(evalReasonSettle)
		}()
	}
}

// reconnectAllowed is the stale-connection monitor's policy gate, as a
// pure decision over already-collected inputs (see reconnectPolicyBlocked
// for the probing half). The monitor's blind "it was up, put it back"
// restore must not contradict the automation engine:
//
//   - paused/exempted (address-conflict pause, "stop automation"): the
//     tunnel is the user's to decide, never the monitor's;
//   - no policy at all: legacy behaviour — the monitor restores what it
//     watched, there is no rule to contradict;
//   - manual-on latch: the user's deliberate connect outranks the policy
//     (mirror of reconcileAction's skip-manual-on);
//   - UNIDENTIFIED network: no condition can be judged, so the engine
//     takes no action — and a reconnect IS an action. The route monitor /
//     SSID report / poll re-post an evaluation once the network is known;
//     that, not the monitor, is what brings the tunnel back then;
//   - SSID-blind (ssidBlind): the policy depends on an SSID the context
//     cannot supply, so its verdict is half a picture and the engine
//     would skip this tunnel this round — the monitor must not act on the
//     same half picture the engine refuses to act on;
//   - policy says disconnect: the tunnel being DOWN is the desired state
//     (a stale-handshake detection or a wake restore must not undo it);
//   - otherwise (connect/unmanaged with a known network): allow —
//     reconnecting converges on, or does not fight, the policy.
func reconnectAllowed(state wifi.DesiredState, hasPolicy, networkKnown, ssidBlind, manualOn, paused, exempted bool) bool {
	if paused || exempted {
		return false
	}
	if !hasPolicy || manualOn {
		return true
	}
	if !networkKnown || ssidBlind {
		return false
	}
	return state != wifi.StateDisconnect
}

// reconnectPolicyBlocked answers the monitor's question for one tunnel:
// may automation put it back up right now? It mirrors reevaluateAutomation
// exactly — fresh settings read, both policy maps count, the same manual
// latches, the same unidentified-network skip, the same SSID-blind
// gate — because the two paths
// acting on DIFFERENT verdicts is what lets a stale-handshake reconnect
// connect a tunnel whose rule says disconnect (reconfiguring the IP
// address flaps en0, the flap reads as a dead connection, and the blind
// restore outran the rules). Any doubt it cannot resolve (settings
// unreadable) fails OPEN to the monitor's legacy behaviour: the 30s
// engine evaluation is seconds away and remains the authority.
func (h *Helper) reconnectPolicyBlocked(name string) bool {
	if name == "" {
		return false
	}
	exempted := h.automationDisabled(name)
	paused := h.isAutoConnectPaused(name)
	settings, err := h.loadUserSettings()
	if err != nil {
		slog.Debug("automation: reconnect gate cannot load settings", "tunnel", name, "error", err)
		return exempted || paused
	}
	settings.EnsureAutomation()
	auto := settings.Automation
	var rules []wifi.Rule
	var def wifi.Action
	hasPolicy := false
	if auto != nil {
		rules = auto.PerTunnel[name]
		def = auto.Defaults[name]
		hasPolicy = len(rules) > 0 || def != ""
	}
	manualOn := false
	for _, n := range settings.ManualOnTunnels {
		if n == name {
			manualOn = true
			break
		}
	}
	ctx := h.currentNetworkContext()
	networkKnown := ctx.SSID != "" || len(ctx.PhysicalIPs) > 0
	blind := hasPolicy && ssidBlind(rules, ctx)
	state := wifi.StateUnmanaged
	if hasPolicy && !blind {
		state = wifi.EvaluatePolicy(rules, def, ctx)
	}
	if !reconnectAllowed(state, hasPolicy, networkKnown, blind, manualOn, paused, exempted) {
		slog.Info("automation: reconnect blocked by policy",
			"category", "network",
			"tunnel", name,
			"decision", decisionLabel(state, false, manualOn),
			"network_known", networkKnown,
			"ssid_blind", blind,
			"ssid", ctx.SSID,
			"exempted", exempted,
			"paused", paused)
		return true
	}
	return false
}

// decisionLabel renders an evaluated desired state (plus the manual latches)
// as the word the log viewer and the CLI preview both use, so the same
// vocabulary appears everywhere.
func decisionLabel(state wifi.DesiredState, manualOff, manualOn bool) string {
	switch state {
	case wifi.StateConnect:
		if manualOff {
			return "manual-off"
		}
		return "connect"
	case wifi.StateDisconnect:
		if manualOn {
			return "manual-on"
		}
		return "disconnect"
	}
	return "unmanaged"
}

// automationDisabled reports whether the tunnel is exempted from automation
// by its sidecar flag. A read error or a missing store means "not disabled":
// automation keeps its normal authority, and the user's opt-out is the
// explicit exception rather than the default.
func (h *Helper) automationDisabled(name string) bool {
	if h.userTunnelStore == nil || name == "" {
		return false
	}
	meta, err := h.userTunnelStore.LoadMeta(name)
	if err != nil || meta == nil {
		return false
	}
	return meta.AutomationDisabled
}

// autoConnectBackoff tracks consecutive automation-connect failures for one
// tunnel so a tunnel that cannot come up (its address already held by
// another WireGuard client, a bad endpoint, ...) is not re-attempted on
// every poll tick. Each attempt tears down and recreates a Wintun adapter;
// the churn is what turned "TS453Dmini is connected" into a 30s bubble loop.
type autoConnectBackoff struct {
	failures  int
	nextRetry time.Time
}

// automationBackoffDelay is the pause after n consecutive failures:
// 30s, 1m, 2m, then 5m (capped). Kept as an explicit table so the schedule
// is obvious and testable.
func automationBackoffDelay(failures int) time.Duration {
	switch {
	case failures <= 1:
		return 30 * time.Second
	case failures == 2:
		return time.Minute
	case failures == 3:
		return 2 * time.Minute
	default:
		return 5 * time.Minute
	}
}

// autoConnectBackingOff reports whether the tunnel is inside its back-off
// window. Caller holds reevalMu (indirectly), the map is guarded by autoFailMu.
func (h *Helper) autoConnectBackingOff(name string) bool {
	h.autoFailMu.Lock()
	defer h.autoFailMu.Unlock()
	b, ok := h.autoConnectBackoff[name]
	if !ok {
		return false
	}
	return time.Now().Before(b.nextRetry)
}

// recordAutoConnectFailure increments the tunnel's failure streak and pushes
// its next retry out along automationBackoffDelay.
func (h *Helper) recordAutoConnectFailure(name string) {
	h.autoFailMu.Lock()
	defer h.autoFailMu.Unlock()
	b := h.autoConnectBackoff[name]
	b.failures++
	delay := automationBackoffDelay(b.failures)
	b.nextRetry = time.Now().Add(delay)
	h.autoConnectBackoff[name] = b
	slog.Info("automation: connect failure — backing off",
		"category", "network", "tunnel", name,
		"consecutive_failures", b.failures,
		"retry_in", delay.String())
}

// clearAutoConnectBackoff drops the tunnel's failure streak so the next poll
// may try again immediately. Called on success, on a manual action, and on a
// network change — any of which means the situation we backed off from is
// over.
func (h *Helper) clearAutoConnectBackoff(name string) {
	h.autoFailMu.Lock()
	defer h.autoFailMu.Unlock()
	if _, ok := h.autoConnectBackoff[name]; ok {
		delete(h.autoConnectBackoff, name)
		slog.Debug("automation: connect back-off cleared", "tunnel", name)
	}
}

// handleAutomationPreview is a read-only dry-run of the Automation
// engine: it reports the current network context and each rule-bearing
// tunnel's evaluated decision, without connecting or disconnecting
// anything. Backs `wireguideplus ctl automation` and answers "why did this
// tunnel (dis)connect?".
func (h *Helper) handleAutomationPreview(_ json.RawMessage) (interface{}, error) {
	settings, err := h.loadUserSettings()
	if err != nil {
		return nil, err
	}
	settings.EnsureAutomation()
	auto := settings.Automation

	ctx := h.currentNetworkContext()

	ipStrs := make([]string, 0, len(ctx.PhysicalIPs))
	for _, ip := range ctx.PhysicalIPs {
		ipStrs = append(ipStrs, ip.String())
	}

	active := make(map[string]bool)
	for _, n := range h.manager.ActiveTunnels() {
		active[n] = true
	}

	manualOff := make(map[string]bool, len(settings.ManualOffTunnels))
	for _, n := range settings.ManualOffTunnels {
		manualOff[n] = true
	}
	manualOn := make(map[string]bool, len(settings.ManualOnTunnels))
	for _, n := range settings.ManualOnTunnels {
		manualOn[n] = true
	}

	resp := ipc.AutomationPreviewResponse{
		SSID:        ctx.SSID,
		PhysicalIPs: ipStrs,
		GatewayMAC:  ctx.GatewayMAC,
		GatewayIP:   ctx.GatewayIP,
		Interfaces:  ctx.Interfaces,
	}
	if auto != nil {
		for _, name := range auto.PolicyTunnelNames() {
			rules := auto.PerTunnel[name]
			// The engine skips an exempted tunnel outright, so the preview
			// must say the same thing rather than show a decision that will
			// never be acted on.
			if h.automationDisabled(name) {
				resp.Tunnels = append(resp.Tunnels, ipc.AutomationTunnelDecision{
					Name:      name,
					RuleCount: len(rules),
					Decision:  "disabled",
					Active:    active[name],
					ManualOff: manualOff[name],
					ManualOn:  manualOn[name],
				})
				continue
			}
			decision := "unmanaged"
			switch wifi.EvaluatePolicy(rules, auto.Defaults[name], ctx) {
			case wifi.StateConnect:
				if manualOff[name] {
					decision = "manual-off" // suppressed by the manual-off latch
				} else {
					decision = "connect"
				}
			case wifi.StateDisconnect:
				if manualOn[name] {
					decision = "manual-on" // suppressed by the manual-on latch
				} else {
					decision = "disconnect"
				}
			}
			resp.Tunnels = append(resp.Tunnels, ipc.AutomationTunnelDecision{
				Name:      name,
				RuleCount: len(rules),
				Decision:  decision,
				Active:    active[name],
				ManualOff: manualOff[name],
				ManualOn:  manualOn[name],
			})
		}
	}
	return resp, nil
}

// automationConnect brings up a tunnel a rule matched and records it in
// the auto-managed map. Caller holds reevalMu.
func (h *Helper) automationConnect(name, reason, ssid string) {
	if h.userTunnelStore == nil {
		slog.Warn("automation: tunnel store unavailable, cannot connect", "tunnel", name)
		return
	}
	cfg, err := h.userTunnelStore.Load(name)
	if err != nil {
		slog.Warn("automation: cannot load tunnel config", "tunnel", name, "error", err)
		return
	}

	// Policy validation sits BETWEEN the desired action and the tunnel
	// operation (principle 32): the automation engine decides WHAT it
	// wants; the policy layer decides whether that is allowed. Automation
	// is blocked by default on a true tie and never prompts — it logs and
	// notifies the tray instead (principle 25).
	if block := h.policyBlockFor(name); block != nil {
		slog.Warn("automation: connect blocked by policy",
			"category", "policy",
			"tunnel", name, "reason", block.Reason, "summary", block.Summary)
		h.server.Broadcast(ipc.EventPolicyBlocked, *block)
		return
	}

	// The DNS resolve path is exclusive among CONNECTED tunnels, which is a
	// runtime fact the configuration-only analyzer cannot see. A manual
	// connect gets parked and asks the user; automation has nobody to
	// answer, so it never waits — it skips and notifies, same as any other
	// policy block (principle 25).
	if h.tunnelClaimsDNSPath(name) {
		if blockers := h.dnsPathBlockers(name); len(blockers) > 0 {
			block := ipc.PolicyBlockedPayload{
				Tunnel: name,
				Reason: "dns",
				Summary: fmt.Sprintf("the system's DNS resolve path is already carried by %s — only one connected tunnel can carry it",
					strings.Join(blockers, ", ")),
			}
			slog.Warn("automation: connect blocked by DNS resolve path",
				"category", "policy", "tunnel", name, "blockers", blockers)
			h.server.Broadcast(ipc.EventPolicyBlocked, block)
			return
		}
	}

	slog.Info("automation: rule connect",
		"category", "network",
		"tunnel", name, "reason", reason, "ssid", ssid)
	h.connectMu.Lock()
	err = h.doConnectHeld(cfg)
	if err == nil {
		// Same firewall follow-up a manual connect does — otherwise a
		// headless automation connect never enforces the tunnel's DNS resolve path
		// DNS policy (issue #12).
		h.applyPostConnectFirewall(cfg)
	}
	h.connectMu.Unlock()
	if err != nil {
		slog.Warn("automation connect failed", "tunnel", name, "error", err)
		var acErr *network.AddressConflictError
		if errors.As(err, &acErr) {
			// The address is held by a different adapter: further attempts
			// die at the same step. Park the tunnel on a pause (and raise the
			// dialog) instead of feeding the backoff loop.
			h.pauseForAddressConflict(name, acErr.Address, acErr.Holder, "down")
			return
		}
		h.recordAutoConnectFailure(name)
		return
	}
	h.clearAutoConnectBackoff(name)
	h.wifiMu.Lock()
	h.autoConnectedBy[name] = ssid
	h.wifiMu.Unlock()
	// Notify GUI so it runs the same post-connect refresh as a manual connect.
	h.server.Broadcast(ipc.EventAutoConnect, ipc.AutoConnectPayload{TunnelName: name})
}

// disconnectAutoManaged tears down a tunnel that the wifi-rule
// engine auto-connected, then clears every cache that referenced it
// (activeCfgs, autoConnectedBy, in-flight retry). Without each of
// these cleanups the helper's various recovery paths would
// resurrect the tunnel: the reconnect monitor would fire its
// pending retry; manager.Disconnect()'s legacy "all tunnels" path
// would re-Connect from a stale activeCfgs entry; and the next
// SSID change handler would try to disconnect a tunnel already
// gone.
func (h *Helper) disconnectAutoManaged(name string) {
	if h.monitor != nil {
		h.monitor.CancelRetryFor(name)
	}
	if err := h.manager.DisconnectTunnel(name); err != nil {
		slog.Warn("automation disconnect failed", "tunnel", name, "error", err)
	}
	// If this tunnel carried the DNS resolve path policy, drop its enforcement.
	h.clearDNSPathIfOwner(name)
	h.mu.Lock()
	delete(h.activeCfgs, name)
	h.mu.Unlock()
	h.wifiMu.Lock()
	delete(h.autoConnectedBy, name)
	h.wifiMu.Unlock()
	// Prune the latency cache exactly as handleDisconnect does — otherwise
	// the status broadcast keeps reporting the dead tunnel's last RTT.
	h.latencyMu.Lock()
	delete(h.latencyByTunnel, name)
	delete(h.latencyProbeByTunnel, name)
	h.latencyMu.Unlock()
	h.maybeArmShutdownAfterTeardown("rule-driven disconnect, no GUI attached")
}
