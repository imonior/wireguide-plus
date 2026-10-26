import { writable, derived, get } from 'svelte/store';
import { TunnelService } from '../../bindings/github.com/imonior/wireguide-plus/internal/app';

// Shared UI view-state, mirrored from persisted Settings. App.onMount
// seeds these from GetSettings; the controls (Settings toggle, tunnel
// list header) update them live and persist via the helpers below.

// compactList mirrors Settings.compact_list — persisted from Settings.svelte.
export const compactList = writable(false);

// The full set of optional GUI modules the user can show/hide. The keys
// here are the authoritative list; moduleFlags below resolves each to a
// visible boolean based on the active UIMode.
const MODULE_KEYS = [
  'nav_history', 'nav_dns_leak', 'nav_route', 'nav_log',
  'automation_extra',
  'tunnel_latency', 'tunnel_traffic', 'tunnel_notes', 'tunnel_endpoint',
  'tunnel_hero_meta', 'tunnel_hero_note', 'tunnel_endpoint_hints',
  'tunnel_actions', 'create_hints',
  'settings_hints', 'settings_proxy',
];

// Modules the mini preset keeps VISIBLE. Everything else optional is hidden
// in mini; the tunnel detail's management buttons (edit / export /
// automation / delete) are essential actions rather than verbose detail, so
// mini shows them and only the custom preset can turn them off.
//
// mini 预设下仍然可见的模块。其余可选模块在 mini 下全部隐藏；隧道详情底部的
// 管理按钮（编辑 / 导出 / 自动化 / 删除）是必备操作而非冗详信息，因此 mini
// 保留显示，只有「自定义」预设能将其关闭。
const MINI_KEEP = { tunnel_actions: true };

// uiMode mirrors Settings.ui_mode — the module-visibility preset:
// "all" (default) | "mini" | "custom". An older stored value "normal" is
// treated as "all" everywhere (see resolveMode). Read by App.svelte
// (sidebar collapse + optional nav items) and TunnelDetail / AutomationEditor
// / Settings via moduleFlags.
export const uiMode = writable('all');

// moduleVis mirrors Settings.ui_visibility — the per-module show/hide map,
// used only when uiMode == "custom". Every entry defaults to true.
export const moduleVis = writable({
  nav_history: true, nav_dns_leak: true, nav_route: true, nav_log: true,
  automation_extra: true,
  tunnel_latency: true, tunnel_traffic: true, tunnel_notes: true,
  tunnel_endpoint: true, tunnel_hero_meta: true,
  tunnel_hero_note: true, tunnel_endpoint_hints: true,
  tunnel_actions: true, create_hints: true,
  settings_hints: true, settings_proxy: true,
});

// resolveMode normalises the stored value: "normal" (legacy) and "" both
// collapse to "all".
function resolveMode(m) {
  if (m === 'mini' || m === 'custom') return m;
  return 'all';
}

// moduleFlags is the resolved visibility map: true = module visible.
//   mini   → every optional module hidden except MINI_KEEP.
//   all    → every optional module visible.
//   custom → per moduleVis flag.
// Components read it as $moduleFlags.<key>.
export const moduleFlags = derived([uiMode, moduleVis], ([$mode, $vis]) => {
  const mode = resolveMode($mode);
  const out = {};
  for (const k of MODULE_KEYS) {
    if (mode === 'mini') out[k] = !!MINI_KEEP[k];
    else if (mode === 'all') out[k] = true;
    else out[k] = !!$vis[k];
  }
  out.__mode = mode; // convenient for templates that branch on the preset
  return out;
});

// isModuleVisible is the imperative sibling of $moduleFlags, for use in
// plain JS (event handlers, guards) rather than template expressions.
export function isModuleVisible(name) {
  const f = get(moduleFlags);
  return name === '__mode' ? f.__mode : !!f[name];
}

// Tunnel-list ordering, controlled from the list header.
export const listSort = writable('name_asc');       // "name_asc" | "name_desc"
export const listActiveOnTop = writable(true);

// Draggable width (px) of the tunnel-list column.
export const LIST_PANE_MIN = 190;
export const LIST_PANE_MAX = 460;
export const LIST_PANE_DEFAULT = 240;
// The mini preset keeps the list column deliberately narrow so the detail
// surface gets the room: a smaller cap and a smaller default. The clamp is
// non-destructive — the stored width is kept as-is, so switching back to
// all/custom restores whatever the user had there.
//
// mini 预设下通道列表列保持窄小，把空间让给详情界面：更小的上限与默认值。
// 钳制是非破坏的——存储值原样保留，切回全部/自定义时恢复此前的宽度。
export const LIST_PANE_MINI_MAX = 220;
export const LIST_PANE_MINI_DEFAULT = 200;
export const listPaneWidth = writable(LIST_PANE_DEFAULT);

// effectiveListPaneWidth is what the UI actually renders: the stored width,
// clamped to the mini cap while the mini preset is active.
//
// effectiveListPaneWidth 是界面实际渲染的宽度：存储宽度在 mini 预设生效期间
// 钳制到 mini 上限。
export const effectiveListPaneWidth = derived(
  [listPaneWidth, uiMode],
  ([$w, $mode]) => ($mode === 'mini' ? Math.min($w, LIST_PANE_MINI_MAX) : $w)
);

// paneMaxForMode / paneDefaultForMode keep the drag, keyboard and reset
// controls honest about the active preset's limits.
//
// paneMaxForMode / paneDefaultForMode 让拖拽、键盘与重置控件遵循当前预设的
// 宽度边界。
export function paneMaxForMode(mode) {
  return mode === 'mini' ? LIST_PANE_MINI_MAX : LIST_PANE_MAX;
}
export function paneDefaultForMode(mode) {
  return mode === 'mini' ? LIST_PANE_MINI_DEFAULT : LIST_PANE_DEFAULT;
}

let saveTimer = null;

// saveListPrefs persists the list-ordering prefs. It re-fetches the
// freshest settings and overlays only its own fields (the same
// spread-fresh pattern TunnelDetail uses for automation rules) so it never
// clobbers edits another screen made to settings.json. Debounced.
export function saveListPrefs() {
  if (saveTimer) clearTimeout(saveTimer);
  saveTimer = setTimeout(async () => {
    saveTimer = null;
    try {
      const fresh = await TunnelService.GetSettings();
      await TunnelService.SaveSettings({
        ...fresh,
        list_sort: get(listSort),
        list_active_on_top: get(listActiveOnTop),
        list_pane_width: Math.round(get(listPaneWidth)),
      });
    } catch (e) {
      console.warn('saveListPrefs failed (will retry on next change):', e);
    }
  }, 300);
}
