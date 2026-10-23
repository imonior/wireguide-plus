import { writable } from 'svelte/store';

// Single source of truth for manual-override latches across EVERY surface —
// tunnel list, detail page, automation editor. A restore anywhere clears the
// backend latch and refreshes this store, so the override indicator vanishes
// on all surfaces at once (fully synchronized). No surface keeps a private
// copy that could disagree with the others.
// 所有表面（通道列表、详情页、自动化编辑器）手动接管闩锁的单一真相源。任何一处点
// 恢复都会清除后端闩锁并刷新本 store，于是接管提示会在所有地方同时消失（完全同步）。
// 没有任何表面保留会与其它面不一致的私有副本。
export const manualOverrides = writable([]);

let svc = null;

// The list / editor / detail page all receive the same bound TunnelService
// prop; whichever mounts first registers it here. Idempotent.
// 列表 / 编辑器 / 详情页都拿到同一个被注入的 TunnelService；谁先挂载就在这里注册它。幂等。
export function setManualOverrideService(service) {
  if (service) svc = service;
}

// Re-read the backend latch set into the shared store. Called on every status
// change (a manual connect/disconnect moves the active/established sets) and
// right after a restore, so every surface reflects the same truth.
// 把后端闩锁集合重新读入共享 store。在每次状态变化时（手动连接/断开会改变
// active/established 集合）以及刚点完恢复之后调用，使每个表面都反映同一真相。
export async function refreshManualOverrides() {
  if (!svc) return;
  try {
    const list = await svc.GetManualOverrides();
    manualOverrides.set(Array.isArray(list) ? list : []);
  } catch (_) {
    // A failed read must never blank the list: showing nothing is the
    // pre-feature behaviour, not a broken one.
    // 读取失败绝不能让列表出错：显示「无闩锁」是本特性出现之前的行为，不算坏掉。
    manualOverrides.set([]);
  }
}

// Release the manual latch (off or on — mutually exclusive) and refresh the
// shared store so EVERY surface drops its indicator simultaneously. Returns
// true on success.
// 释放手动闩锁（off 或 on，二者互斥）并刷新共享 store，于是所有表面同时撤下提示。
// 成功返回 true。
export async function clearManualOverride(name) {
  if (!svc || !name) return false;
  try {
    await svc.ClearManualOverride(name);
    await refreshManualOverrides(); // sync every surface immediately
    return true;
  } catch (e) {
    console.warn('failed to restore automation for tunnel:', name, e);
    return false;
  }
}
