package gui

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa
#import <Cocoa/Cocoa.h>
#import <objc/runtime.h>

// Associated-object keys keep the controller reachable through [statusItem target].
// Wails reads that getter back in systemTrayDestroy / showMenu / systemTraySetCachedMenu;
// without this bridge those paths would receive nil once we forward target/action
// to the button.
// 关联对象键用于让 [statusItem target] 仍能返回 controller。
// Wails 在 systemTrayDestroy / showMenu / systemTraySetCachedMenu 里会回调这个 getter；
// 若把 target/action 转发到 button 而不做桥接，这些路径会拿到 nil。
static char wgTrayTargetKey = 0;

// Original target getter IMP, used only as a fallback when no controller was stored.
// 原始 target getter 的 IMP，仅在尚未存入 controller 时作为回退。
static IMP wgOrigTargetGetter = NULL;

// Fix state: 0=pending, 1=installed, 2=not needed (old macOS), 3=failed.
// 修复状态：0=待安装，1=已安装，2=无需安装（旧版 macOS），3=安装失败。
static int wgTrayFixState = 0;

// wgSwizzledSetTarget intercepts Wails' [statusItem setTarget:controller].
// It stores the controller for the getter bridge and attaches it to the button
// instead of the status item: on macOS 27 a status item that owns target/action
// makes MenuBarAgent send the action directly with NO synthesized mouse event,
// which leaves [NSApp currentEvent] non-mouse and breaks left-click handling.
// The button path restores the synthesized leftMouseDown that every monitor
// (Wails' pre-click monitor and our double-click detector) relies on.
// 拦截 Wails 的 [statusItem setTarget:controller]：把 controller 存入关联对象供 getter 桥接，
// 并把 target 挂到 button 而非 statusItem。macOS 27 上若 statusItem 自带 target/action，
// MenuBarAgent 会直接 sendAction 而不合成鼠标事件，导致 [NSApp currentEvent] 不是鼠标事件、
// 左键判定全废。改挂 button 后恢复合成的 leftMouseDown，所有监听鼠标事件的 monitor 重新生效。
static void wgSwizzledSetTarget(id self, SEL _cmd, id target) {
	objc_setAssociatedObject(self, &wgTrayTargetKey, target, OBJC_ASSOCIATION_RETAIN_NONATOMIC);
	NSButton *button = [(NSStatusItem *)self button];
	if (button != nil) {
		[button setTarget:target];
	}
	NSLog(@"[WGTrayFix] setTarget -> button (%@)", NSStringFromClass([target class]));
}

// wgSwizzledTarget returns the bridged controller first; the original getter is
// only consulted before Wails has installed one. This keeps Wails' own cleanup
// and menu-caching code working while the status item's real ivar stays nil.
// 优先返回桥接的 controller；仅在 Wails 尚未安装时回退到原始 getter。
// 这样 Wails 自身的清理与菜单缓存代码照常工作，同时 statusItem 的真实 ivar 保持为 nil。
static id wgSwizzledTarget(id self, SEL _cmd) {
	id stored = objc_getAssociatedObject(self, &wgTrayTargetKey);
	if (stored != nil) {
		return stored;
	}
	if (wgOrigTargetGetter != NULL) {
		return ((id (*)(id, SEL))wgOrigTargetGetter)(self, _cmd);
	}
	return nil;
}

// wgSwizzledSetAction mirrors setTarget for the action selector. The action
// getter is deliberately NOT swizzled: leaving it nil avoids any chance that
// MenuBarAgent treats the status item as "has target/action" via the getter.
// setAction 的转发逻辑与 setTarget 一致。刻意不 swizzle action getter：
// 让它保持 nil，避免 MenuBarAgent 通过 getter 误判 statusItem「自带 target/action」。
static void wgSwizzledSetAction(id self, SEL _cmd, SEL action) {
	NSButton *button = [(NSStatusItem *)self button];
	if (button != nil) {
		[button setAction:action];
	}
	NSLog(@"[WGTrayFix] setAction -> button (%@)", NSStringFromSelector(action));
}

// wgInstallTrayTargetActionFix installs the swizzle on macOS 27+ only.
// Older systems keep the stock behaviour and never reach our replacements,
// so the known-good macOS 15 path is untouched. The swizzle is process-wide
// on NSStatusItem, which is safe because the app owns exactly one status item.
// 仅在 macOS 27+ 安装 swizzle。旧系统保持原生行为、完全不进入替换实现，
// 因此已验证正常的 macOS 15 路径零改动。swizzle 作用于进程内整个 NSStatusItem 类，
// 本应用只有一个状态栏项，因此安全。
static int wgInstallTrayTargetActionFix(void) {
	if (wgTrayFixState != 0) {
		return wgTrayFixState;
	}
	NSOperatingSystemVersion v = [[NSProcessInfo processInfo] operatingSystemVersion];
	if (v.majorVersion < 27) {
		wgTrayFixState = 2;
		return wgTrayFixState;
	}
	Class cls = objc_getClass("NSStatusItem");
	if (cls == Nil) {
		NSLog(@"[WGTrayFix] install FAILED: NSStatusItem class missing");
		wgTrayFixState = 3;
		return wgTrayFixState;
	}
	Method setTargetM = class_getInstanceMethod(cls, @selector(setTarget:));
	Method targetM = class_getInstanceMethod(cls, @selector(target));
	Method setActionM = class_getInstanceMethod(cls, @selector(setAction:));
	if (setTargetM == NULL || targetM == NULL || setActionM == NULL) {
		NSLog(@"[WGTrayFix] install FAILED: NSStatusItem methods missing");
		wgTrayFixState = 3;
		return wgTrayFixState;
	}
	wgOrigTargetGetter = method_getImplementation(targetM);

	// class_addMethod first: if the method is inherited, adding an override on
	// NSStatusItem avoids mutating the superclass implementation shared by others.
	// 先尝试 class_addMethod：若方法是继承来的，在 NSStatusItem 上加覆盖实现，
	// 避免改动父类实现而波及其他类。
	if (!class_addMethod(cls, @selector(setTarget:), (IMP)wgSwizzledSetTarget, method_getTypeEncoding(setTargetM))) {
		method_setImplementation(setTargetM, (IMP)wgSwizzledSetTarget);
	}
	if (!class_addMethod(cls, @selector(target), (IMP)wgSwizzledTarget, method_getTypeEncoding(targetM))) {
		method_setImplementation(targetM, (IMP)wgSwizzledTarget);
	}
	if (!class_addMethod(cls, @selector(setAction:), (IMP)wgSwizzledSetAction, method_getTypeEncoding(setActionM))) {
		method_setImplementation(setActionM, (IMP)wgSwizzledSetAction);
	}
	wgTrayFixState = 1;
	NSLog(@"[WGTrayFix] installed on macOS %ld.%ld", (long)v.majorVersion, (long)v.minorVersion);
	return wgTrayFixState;
}
*/
import "C"

// install state mirrored from the C side; used for startup diagnostics.
// 与 C 侧对应的安装状态，用于启动诊断日志。
const (
	trayFixPending   = 0
	trayFixInstalled = 1
	trayFixNotNeeded = 2
	trayFixFailed    = 3
)

var trayTargetFixState int

// init runs before application.New / app.Run, so the swizzle is in place before
// Wails creates the NSStatusItem in systemTrayNew.
// init 早于 application.New / app.Run 执行，确保 Wails 在 systemTrayNew 创建
// NSStatusItem 之前 swizzle 已就位。
func init() {
	trayTargetFixState = int(C.wgInstallTrayTargetActionFix())
}

// trayTargetFixLabel renders the install state for structured logs.
// 把安装状态转成可读标签，供结构化日志使用。
func trayTargetFixLabel() string {
	switch trayTargetFixState {
	case trayFixInstalled:
		return "installed"
	case trayFixNotNeeded:
		return "not-needed"
	case trayFixFailed:
		return "FAILED"
	default:
		return "pending"
	}
}
