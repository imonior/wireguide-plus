package helper

import (
	"github.com/imonior/wireguide-plus/internal/storage"
	"github.com/imonior/wireguide-plus/internal/wifi"
)

// TunnelDecision is the single, authoritative output of the automation
// decision layer. Every consumer — the live engine (reevaluateAutomation),
// the reconnect monitor (reconnectPolicyBlocked) and the read-only preview
// (handleAutomationPreview / the CLI) — must derive its behaviour from one
// of these, never re-implementing the gates by hand. EvaluateTunnelDecision
// is PURE: it only reads its inputs (settings + network context + the
// in-memory latch/pause/backoff/policy state supplied by the Helper) and
// performs NO side effects — no connect, no firewall, no DNS. Callers
// translate Action into real operations.
type TunnelDecision struct {
	Name      string
	HasPolicy bool
	Desired   wifi.DesiredState // connect / disconnect / unmanaged
	Action    string            // "", connect, disconnect, skip-manual-off,
	// skip-manual-on, blocked, deferred, unresolved
	Reason       string
	Blocked      bool
	BlockReason  string
	MatchedRule  *wifi.Rule
	ManualOff    bool
	ManualOn     bool
	Exempted     bool // automation disabled (sidecar flag)
	Paused       bool // address-conflict pause
	SSIDBlind    bool
	NetworkKnown bool
	BackingOff   bool
}

// DecisionEnv bundles every input the decision needs, so the gate logic can
// live in a pure, side-effect-free function (evaluateTunnelDecision) that is
// unit-testable without a live Helper. EvaluateTunnelDecision builds one
// from the Helper's runtime state and delegates to it.
type DecisionEnv struct {
	Name         string
	HasPolicy    bool
	Rules        []wifi.Rule
	Default      wifi.Action
	Ctx          wifi.NetworkContext
	NetworkKnown bool
	SSIDBlind    bool
	Exempted     bool
	Paused       bool
	ManualOff    bool
	ManualOn     bool
	Active       bool
	BackingOff   bool
	PolicyBlock  string // "" = not blocked; otherwise the block reason
	DNSBlock     bool
}

// evaluateTunnelDecision is the convergent, pure core. It applies every gate
// in one place so the engine, reconnect monitor and preview cannot drift
// apart, and returns what the automation layer believes should happen to one
// tunnel right now. It never acts.
//
// Gate order (authoritative for all three callers):
//
//	unidentified network → unresolved (no condition is judgeable)
//	exempted (automation disabled) → nothing
//	paused (address-conflict) → nothing
//	ssid-blind → unresolved (fail-closed: never act on half a context)
//	evaluate policy + Default State → Desired
//	reconcile(Desired, Active, latch) → connect / disconnect / skip-* / nothing
//	wanted connect + policy block → blocked
//	wanted connect + DNS-path block → blocked
//	wanted connect + back-off → deferred
func evaluateTunnelDecision(env DecisionEnv) TunnelDecision {
	d := TunnelDecision{
		Name:         env.Name,
		HasPolicy:    env.HasPolicy,
		NetworkKnown: env.NetworkKnown,
		SSIDBlind:    env.SSIDBlind,
		Exempted:     env.Exempted,
		Paused:       env.Paused,
		ManualOff:    env.ManualOff,
		ManualOn:     env.ManualOn,
		BackingOff:   env.BackingOff,
	}

	if !env.NetworkKnown {
		d.Action = "unresolved"
		d.Reason = "network-unidentified"
		return d
	}
	if env.Exempted {
		d.Action = ""
		d.Reason = "automation-disabled"
		return d
	}
	if env.Paused {
		d.Action = ""
		d.Reason = "address-conflict-paused"
		return d
	}
	if env.SSIDBlind {
		d.Action = "unresolved"
		d.Reason = "ssid-blind"
		return d
	}

	state, details := wifi.EvaluatePolicyDetail(env.Rules, env.Default, env.Ctx)
	d.Desired = state
	if state != wifi.StateUnmanaged && len(details) > 0 {
		for i := range details {
			if details[i].Matched && i < len(env.Rules) {
				r := env.Rules[i]
				d.MatchedRule = &r
				break
			}
		}
	}

	base := reconcileAction(state, env.Active, env.ManualOff, env.ManualOn)
	switch base {
	case "connect":
		if env.PolicyBlock != "" {
			d.Action = "blocked"
			d.Blocked = true
			d.BlockReason = env.PolicyBlock
			return d
		}
		if env.DNSBlock {
			d.Action = "blocked"
			d.Blocked = true
			d.BlockReason = "dns"
			return d
		}
		if env.BackingOff {
			d.Action = "deferred"
			d.Reason = "back-off"
			return d
		}
		d.Action = "connect"
	case "disconnect":
		d.Action = "disconnect"
	case "skip-manual-off":
		d.Action = "skip-manual-off"
		d.Reason = "manual-off"
	case "skip-manual-on":
		d.Action = "skip-manual-on"
		d.Reason = "manual-on"
	default:
		d.Action = ""
	}
	return d
}

// EvaluateTunnelDecision is the single, authoritative decision entry point.
// It gathers the Helper's runtime state into a DecisionEnv and delegates to
// the pure evaluateTunnelDecision. Callers must translate d.Action into side
// effects; this method performs none.
func (h *Helper) EvaluateTunnelDecision(name string, s *storage.Settings, ctx wifi.NetworkContext) TunnelDecision {
	auto := s.Automation
	var rules []wifi.Rule
	var def wifi.Action
	if auto != nil {
		rules = auto.PerTunnel[name]
		def = auto.Defaults[name]
	}
	hasPolicy := len(rules) > 0 || def != ""

	manualOff := false
	manualOn := false
	for _, n := range s.ManualOffTunnels {
		if n == name {
			manualOff = true
			break
		}
	}
	for _, n := range s.ManualOnTunnels {
		if n == name {
			manualOn = true
			break
		}
	}

	active := make(map[string]bool)
	for _, n := range h.manager.ActiveTunnels() {
		active[n] = true
	}

	env := DecisionEnv{
		Name:         name,
		HasPolicy:    hasPolicy,
		Rules:        rules,
		Default:      def,
		Ctx:          ctx,
		NetworkKnown: ctx.SSID != "" || len(ctx.PhysicalIPs) > 0,
		SSIDBlind:    hasPolicy && ssidBlind(rules, ctx),
		Exempted:     h.automationDisabled(name),
		Paused:       h.isAutoConnectPaused(name),
		ManualOff:    manualOff,
		ManualOn:     manualOn,
		Active:       active[name],
		BackingOff:   h.autoConnectBackingOff(name),
	}
	if block := h.policyBlockFor(name); block != nil {
		env.PolicyBlock = block.Reason
	}
	if h.tunnelClaimsDNSPath(name) {
		if blockers := h.dnsPathBlockers(name); len(blockers) > 0 {
			env.DNSBlock = true
		}
	}
	return evaluateTunnelDecision(env)
}

// DecisionConnectAllowed answers the reconnect monitor's question from a
// TunnelDecision, preserving the historical reconnect truth table exactly:
// a reconnect is allowed when the tunnel has no policy or the user latched it
// on (the monitor's legacy "restore what it watched" behaviour), and blocked
// when the decision is exempted/paused/network-unknown/ssid-blind or wants
// disconnect. Policy/DNS blocks are deliberately NOT consulted here — the
// historical reconnect path never did, and convergence must not silently
// change reconnect behaviour.
func DecisionConnectAllowed(d TunnelDecision) bool {
	if d.Paused || d.Exempted {
		return false
	}
	if !d.HasPolicy || d.ManualOn {
		return true
	}
	if !d.NetworkKnown || d.SSIDBlind {
		return false
	}
	return d.Desired != wifi.StateDisconnect
}
