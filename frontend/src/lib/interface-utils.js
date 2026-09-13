// Shared physical-interface listing + label logic, used by BOTH:
//   * EgressBinding.svelte      — per-tunnel "bind physical egress" dropdown
//   * AutomationEditor.svelte    — automation "on interface" condition dropdown
// so the two selectors show the SAME set of adapters with the SAME labels.
//
// The canonical source is TunnelService.ListPhysicalInterfaces() — the
// platform-specific full enumeration (macOS: networksetup hardware ports,
// Windows: net.Interfaces, Linux: sysfs). This deliberately differs from the
// live AutomationPreview.interfaces, which only lists interfaces that are
// currently UP with a routable address — fine for the live status board, but
// wrong as the editable selector (a laptop on Wi-Fi would only ever offer
// "en0", pre-filling it and hiding every other NIC).

// Fetch the full physical-interface list. Never throws: on failure it logs
// and returns [] so the calling UI degrades to an empty selector rather than
// crashing the modal.
export async function loadPhysicalInterfaces(TunnelService) {
  if (!TunnelService || typeof TunnelService.ListPhysicalInterfaces !== 'function') {
    return [];
  }
  try {
    return (await TunnelService.ListPhysicalInterfaces()) || [];
  } catch (e) {
    console.error('[interface-utils] ListPhysicalInterfaces failed:', e);
    return [];
  }
}

// Render one adapter the same way the per-tunnel binding dropdown does:
//   friendly name · hardware (when different) · (#index) · down-marker
// `tFunc` is the component's reactive translator ($t) so the "down" marker
// stays localized when the UI language changes.
export function ifaceLabel(ifc, tFunc) {
  if (!ifc) return '';
  const generic = ifc.friendly || ifc.name || '';
  const hw = ifc.hardware && ifc.hardware !== generic ? ` · ${ifc.hardware}` : '';
  const idx = ifc.index > 0 ? ` (#${ifc.index})` : '';
  const state = !ifc.is_up && typeof tFunc === 'function' ? ` · ${tFunc('fields.bind_down')}` : '';
  return `${generic}${hw}${idx}${state}`;
}
