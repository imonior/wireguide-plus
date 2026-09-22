// Shared read-modify-write helpers for the durable per-tunnel automation
// switch (TunnelMeta.AutomationDisabled, exposed over IPC as
// `automation_disabled`).
//
// Three places touch this flag:
//   1. the hero toggle in TunnelDetail,
//   2. the "Automation on/off" checkbox in AutomationEditor,
//   3. the pass-through save in TunnelPolicies.
//
// They must not drift apart, because SetTunnelPolicies replaces the WHOLE
// sidecar: a write that only sends `automation_disabled` would zero every
// other policy field (traffic protect, domains, DNS path, keep-idle
// override), and a write that omits it would reset the flag itself back to
// false — silently re-enabling automation for a tunnel the user had stopped.
// Both helpers therefore do a full read-modify-write, and both let errors
// propagate so each call site keeps its own degradation policy (the hero
// shows an inline error, the editor logs, the policies panel falls back to
// "automation on").

// getAutomationDisabled reports whether automation is currently switched OFF
// for this tunnel. Throws on transport failure — callers decide whether to
// degrade to "automation on" (the safe default: the engine keeps its normal
// authority) or surface an error.
export async function getAutomationDisabled(service, name) {
  if (!service || !name) return false;
  const p = await service.GetTunnelPolicies(name);
  return !!p?.automation_disabled;
}

// setAutomationDisabled flips the flag to `next` and returns the value that
// was actually persisted, so callers can update their local state from the
// source of truth instead of assuming the write stuck.
export async function setAutomationDisabled(service, name, next) {
  if (!service || !name) return false;
  const p = (await service.GetTunnelPolicies(name)) || {};
  p.automation_disabled = !!next;
  await service.SetTunnelPolicies(name, p);
  return !!next;
}
