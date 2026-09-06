import { writable } from 'svelte/store';

// Global view of the app settings the UI needs outside the Settings
// dialog: AWG gating (badge + editor fields) and interface binding
// availability (egress dropdown). Kept in sync by Settings.svelte (on
// every change) and by App.svelte (on mount and when the Settings
// dialog closes).
//
// Interface binding reuses the pre-existing `pin_interface` switch — it
// is already the master toggle for "bind tunnel traffic to a physical
// interface", so no second setting is introduced.
// `loaded:false` until the persisted settings have actually been read —
// consumers MUST treat every gate as "unknown" while it is false. The
// AWG badge in particular used to render ON because this store defaulted
// enable_awg to true and nothing refreshed it at startup.
export const appSettings = writable({
  loaded: false,
  enable_awg: false,
  enable_wg_scripts: false,
  pin_interface: false,
});

export async function refreshAppSettings(TunnelService) {
  try {
    const s = await TunnelService.GetSettings();
    appSettings.set({
      loaded: true,
      // Strict: ON only when the persisted field is literally true. A
      // missing/undefined field means OFF — never assume a feature gate.
      enable_awg: s?.enable_awg === true,
      enable_wg_scripts: s?.enable_wg_scripts === true,
      pin_interface: s?.pin_interface === true,
    });
  } catch {
    // keep previous values — a transient IPC failure must not flip
    // feature gates the user explicitly configured.
  }
}
