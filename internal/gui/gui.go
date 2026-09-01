// Package gui contains the GUI-mode runtime for the WireGuide app.
//
// The package is split so each file has a single reason to change:
//   - gui.go              (this file)  — Run() entry, Wails app + window setup
//   - tray.go                           — system tray menu (event-driven rebuild)
//   - event_bridge.go                   — IPC event forwarding + subscription
//   - helper_lifecycle.go               — helper spawn, health check, auto-reconnect
package gui

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"runtime"
	"sync"
	"time"

	wgapp "github.com/imonior/wireguide-plus/internal/app"
	"github.com/imonior/wireguide-plus/internal/domain"
	"github.com/imonior/wireguide-plus/internal/ipc"
	"github.com/imonior/wireguide-plus/internal/logging"
	"github.com/imonior/wireguide-plus/internal/storage"
	"github.com/imonior/wireguide-plus/internal/update"
	"github.com/imonior/wireguide-plus/internal/wifi"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

// ReconnectEvent mirrors ipc.ReconnectStateDTO for Wails event emission.
type ReconnectEvent struct {
	Reconnecting bool `json:"reconnecting"`
	Attempt      int  `json:"attempt"`
	MaxAttempts  int  `json:"max_attempts"`
}

// HelperEvent notifies the frontend about helper process health changes.
type HelperEvent struct {
	Alive   bool   `json:"alive"`
	Message string `json:"message"`
}

// Register Wails event payload types. Called once per process from init.
func init() {
	application.RegisterEvent[domain.ConnectionStatus]("status")
	application.RegisterEvent[ReconnectEvent]("reconnect")
	application.RegisterEvent[map[string]any]("files-dropped")
	application.RegisterEvent[HelperEvent]("helper")
	application.RegisterEvent[struct{}]("helper_reset")
	application.RegisterEvent[update.UpdateInfo]("update-available")
	application.RegisterEvent[map[string]any]("update_progress")
	application.RegisterEvent[ipc.SettingsChangedPayload]("settings_changed")
	application.RegisterEvent[struct{}]("config_changed")
	application.RegisterEvent[struct{}]("tunnels_changed")
}

// Run starts the GUI process. Blocks until the Wails app exits.
func Run(assetsHandler http.Handler, dataDir string) error {
	// 0. Install the slog handler that broadcasts to the LogViewer. Do this
	// first so every subsequent log call (path init, helper spawn, etc.) is
	// captured. The Wails app isn't built yet; bindAppToLogHandler wires
	// the app reference later once application.New returns.
	installGUILogHandler()
	// Expose the level mutator to the Wails-bound service layer so
	// Settings changes can reach us without an import cycle.
	wgapp.SetGUILogLevelSetter(setGUILogLevel)

	// Pre-render the tray icon variants now that main.go has populated
	// customTrayIconPNG via SetTrayIconPNG. Windows/Linux get the full
	// app-icon set; macOS gets its own 64px app-icon set (the monochrome
	// W template built in tray.go's init() stays as the fallback when no
	// source PNG was embedded).
	buildWindowsTrayIcons()
	buildMacTrayIcons()

	// 1. Local storage
	paths, err := storage.GetPaths()
	if err != nil {
		return fmt.Errorf("paths: %w", err)
	}
	if err := paths.EnsureDirs(); err != nil {
		return fmt.Errorf("create dirs: %w", err)
	}
	setGUILogFile(paths.LogsDir)
	tunnelStore := storage.NewTunnelStore(paths.TunnelsDir)
	settingsStore := storage.NewSettingsStore(paths.ConfigDir)
	historyStore := storage.NewHistoryStore(paths.ConfigDir)
	// Sweep up sessions left open by a previous crash before the UI has a
	// chance to read them. Mark "app_quit" so they aren't shown as still-
	// active in the timeline.
	historyStore.CloseOpenSessions("app_quit")

	// Apply persisted log level to the GUI side immediately (helper-side
	// gets it after ensureHelper + the SaveSettings path), and seed the
	// update checker's proxy setting so the first scheduled check already
	// honors the user's saved proxy without waiting for a settings save.
	if s, err := settingsStore.Load(); err == nil && s != nil {
		if s.LogLevel != "" {
			setGUILogLevel(s.LogLevel)
		}
		update.SetProxy(s.ProxyMode, s.ProxyURL)
		// Sweep daily log files older than the configured retention so
		// the logs folder can't grow without bound. 0 in settings means
		// the default (7 days).
		retention := logging.DefaultRetentionDays
		if s.LogRetentionDays > 0 {
			retention = s.LogRetentionDays
		}
		for _, prefix := range []string{"wireguideplus", "helper"} {
			n, err := logging.CleanupOldLogs(paths.LogsDir, prefix, retention)
			if err != nil {
				slog.Warn("log cleanup failed", "prefix", prefix, "error", err)
			} else if n > 0 {
				slog.Info("removed old log files", "prefix", prefix, "removed", n, "retention_days", retention)
			}
		}
	}

	// 2. Helper process (spawn if needed).
	// If the user cancels the admin prompt, retry up to 3 times with a
	// user-visible dialog explaining why the helper is required. If the
	// user still cancels (or elevation never succeeds), keep running with
	// a nil client instead of exiting: the window and tray stay up, the
	// health monitor probes for a helper on every tick, and tunnel
	// operations surface an error until the helper comes back. Previously
	// this returned an error and the whole GUI process exited — which
	// read to the user as "the main window auto-closes and the tray icon
	// disappears ~30s after launch" (one 30s helper probe + two retries).
	var initialClient *ipc.Client
	for attempt := 0; attempt < 3; attempt++ {
		helperCtx, helperCancel := context.WithTimeout(context.Background(), 30*time.Second)
		var err error
		initialClient, err = ensureHelper(helperCtx, dataDir)
		helperCancel()
		if err == nil {
			break
		}
		slog.Warn("helper connection failed", "attempt", attempt+1, "error", err)
		if attempt < 2 {
			// Native per-OS retry dialog (retry_prompt_*.go) — the Wails
			// app isn't running yet, so Wails dialogs are unavailable.
			// On retry, ensureHelper first probes for an already-running
			// helper, so a UAC prompt the user answered while this dialog
			// was up connects instantly instead of re-prompting.
			if !askHelperRetry() {
				slog.Warn("user cancelled helper setup; continuing without helper")
				break
			}
			continue
		}
		slog.Error("helper connection failed after 3 attempts; continuing with limited functionality")
	}
	clients := ipc.NewClientHolder(initialClient)

	// 3. Wails service
	tunnelService := wgapp.NewTunnelService(tunnelStore, settingsStore, historyStore, clients)
	// Fresh app session: release every manual-off latch so automation rules
	// resume ("manual off only lasts until the app is reopened").
	tunnelService.ClearManualOffAll()

	// 4. Wails app
	app := application.New(application.Options{
		Name:        "WireGuide Plus",
		Description: "WireGuide Plus - multi-tunnel automated WireGuard VPN client",
		Services: []application.Service{
			application.NewService(tunnelService),
		},
		Assets: application.AssetOptions{
			Handler: assetsHandler,
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: false,
		},
	})
	tunnelService.SetApp(app)
	bindAppToLogHandler(app)

	// Trigger CLLocationManager authorization so this .app bundle appears in
	// System Settings → Location Services. Must run in the GUI process (not
	// the helper) because Location Services tracks the calling bundle ID.
	// cwRequestLocationAuthorization dispatches to the main thread internally.
	wifi.RequestLocationAuthorization()

	// On macOS 14+ the helper (root LaunchDaemon) cannot read SSID via
	// CoreWLAN because Location Services permission is bundle-scoped. Poll
	// here in the GUI process (which holds permission) and forward changes
	// to the helper via MethodReportSSID so auto-connect rules fire correctly.
	// The reporter is wired into the same shutdown lifecycle as the helper
	// health monitor (declared further down) — both stop cleanly on app quit.

	// Register the log event shape so Wails knows how to marshal it.
	application.RegisterEvent[ipc.LogEntry]("log")

	// 5. Main window
	//
	// Default size: 1200 × 740 — width:height ≈ 1.62, the golden ratio.
	// Picked over the previous 900 × 680 (ratio 1.32) because that felt
	// squat after the notes section pushed the detail pane taller. The
	// extra width gives the 3-pane layout (sidebar 200 + list 240 +
	// detail flex) ~760 px for the detail pane, which is enough for the
	// stats grid + info rows + notes textarea + actions row to breathe.
	//
	// Min size pins the smallest pretty-looking shape (≈1.44 ratio) while
	// still leaving the detail pane wide enough that nothing wraps weirdly.
	win := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:          "WireGuide Plus",
		Width:          1200,
		Height:         740,
		MinWidth:       920,
		MinHeight:      640,
		EnableFileDrop: true,
		Mac: application.MacWindow{
			InvisibleTitleBarHeight: 50,
			Backdrop:                application.MacBackdropTranslucent,
			TitleBar:                application.MacTitleBarHiddenInset,
		},
		BackgroundColour: application.NewRGB(30, 30, 30), // matches --bg-primary dark (#1E1E1E)
		URL:              "/",
	})

	// macOS standard: close button hides the window instead of destroying it.
	// The app stays alive in the tray; "Show Window" brings it back.
	//
	// CRITICAL: Must use RegisterHook, NOT OnWindowEvent. Wails registers
	// its own OnWindowEvent(WindowClosing) listener that calls markAsDestroyed
	// + close. Listeners run in separate goroutines, so Cancel() from our
	// listener races with Wails' listener — the window gets destroyed despite
	// Cancel. Hooks run sequentially BEFORE listeners, so Cancel() here
	// reliably prevents Wails' default close/destroy behavior.
	// Linux must intercept the native delete event itself. Wails maps
	// Linux.WindowDeleteEvent to Common.WindowClosing asynchronously; hooking
	// only the mapped event allows the default Common.WindowClosing listener to
	// destroy the window before close-to-tray can cancel it. Windows and macOS
	// expose a cancellable Common.WindowClosing event directly.
	closingEvent := events.Common.WindowClosing
	if runtime.GOOS == "linux" {
		closingEvent = events.Linux.WindowDeleteEvent
	}
	win.RegisterHook(closingEvent, func(event *application.WindowEvent) {
		event.Cancel()
		// Close = go to background on every platform: hide the window,
		// keep the tray icon alive, and let the tray's left-click (or the
		// popup's Open Window) bring the main window back. Minimising
		// instead (the old Windows-only branch) made Close behave exactly
		// like the minimise button — the window stayed on the taskbar —
		// which users reported as surprising for a tray-first app.
		win.Hide()
		hideDock() // no-op outside macOS
	})

	// Wire the window reference so showDock() can retry showing it.
	dockWindow = win

	// "Start minimized": launch straight into the tray when the setting is
	// on. The tray menu's Show Window restores the main window, so the app
	// stays fully reachable even when the user never sees the window.
	if s, err := settingsStore.Load(); err == nil && s != nil && s.StartMinimized {
		// "Start minimized" hides to the tray on every platform — the same
		// model as close-to-tray. The tray's left-click / Show Window
		// restores the main window, so it stays fully reachable.
		win.Hide()
		hideDock() // no-op outside macOS
	}

	// Native file drop forwarded to frontend
	win.OnWindowEvent(events.Common.WindowFilesDropped, func(event *application.WindowEvent) {
		files := event.Context().DroppedFiles()
		app.Event.Emit("files-dropped", map[string]any{"files": files})
	})

	// 6. System tray. macOS uses non-template coloured icons (the
	// connected badge must stay green; template images are alpha-only),
	// with the W glyph colour following the menu bar's actual rendered
	// appearance via the KVO observer the tray manager installs. Never
	// call SetTemplateIcon: Wails's sticky isTemplateIcon flag would
	// render every later SetIcon monochrome.
	tray := app.SystemTray.New()
	if runtime.GOOS == "darwin" {
		// App-icon variant when the embedded source was built (macOS now
		// mirrors the app icon). Falls back to the white W template
		// (historical default); the appearance observer swaps that for
		// the black set only if the menu bar is really light.
		if len(trayOffIconMac) > 0 {
			tray.SetIcon(trayOffIconMac)
		} else {
			tray.SetIcon(trayOffIconDark)
		}
	} else {
		tray.SetLabel("WireGuide Plus")
		// Windows and Linux need an explicit icon at startup. Without it,
		// Linux StatusNotifier hosts display their own fallback icon and
		// Windows falls back to Wails' embedded template. setIconState only
		// runs on connection transitions, so it cannot initialise the icon.
		if (runtime.GOOS == "windows" || runtime.GOOS == "linux") && len(trayOffIconWindows) > 0 {
			tray.SetIcon(trayOffIconWindows)
		}
		// Windows convention: left-click is the primary action and shows
		// the main window; the context menu remains on right-click.
		// Registered only here: on macOS any click opens the NSStatusItem
		// menu natively, and an OnClick handler would fight it. On Linux,
		// Wails' StatusNotifier fires the click handler for the dbusmenu
		// "opened" event too, so registering it would raise the window on
		// every right-click; Linux users get the "Show Window" menu item.
		if runtime.GOOS == "windows" {
			tray.OnClick(showDock)
		}
	}
	tray.SetTooltip("WireGuide Plus")

	// 7. Shutdown coordination (declared upfront so closures can reference it)
	var (
		shutdownOnce sync.Once
		doShutdown   func()
	)
	doShutdown = func() {
		shutdownOnce.Do(func() {
			slog.Info("shutting down GUI + helper")
			// Close any in-flight history sessions BEFORE the helper goes
			// away — snapshotActiveStats needs the helper alive to fetch
			// last-known rx/tx counters.
			tunnelService.CloseHistorySessions("app_quit")
			// Flush any deferred history writes synchronously. The
			// debounced writer would otherwise lose the last 100ms of
			// records when the GUI process exits.
			historyStore.Flush()
			c := clients.Get()
			if c != nil {
				// Bounded timeouts so a hung helper can't keep the GUI
				// alive forever — user clicked Quit, they expect the app
				// to die. 2s is generous for a local Unix-socket RPC that
				// just kicks off async teardown.
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				if err := c.CallWithContext(ctx, ipc.MethodDisconnect, nil, nil); err != nil {
					slog.Warn("shutdown: Disconnect RPC failed", "error", err)
				}
				cancel()
				ctx, cancel = context.WithTimeout(context.Background(), 2*time.Second)
				if err := c.CallWithContext(ctx, ipc.MethodShutdown, nil, nil); err != nil {
					slog.Warn("shutdown: Shutdown RPC failed", "error", err)
				}
				cancel()
			}
			// Close in a goroutine with a short delay so the helper has
			// time to process the shutdown command without blocking the
			// macOS main thread (AppKit requires the main thread to stay
			// responsive during termination).
			go func() {
				time.Sleep(50 * time.Millisecond)
				clients.Close()
			}()
		})
	}

	trayMgr := newTrayManager(app, win, tray, tunnelService, doShutdown)
	trayMgr.initialBuild()
	// Report the connection situation 10s after launch — enough time for
	// the user to approve any permission prompts and for the automation
	// rules to finish auto-connecting.
	trayMgr.scheduleStatusNotification()

	if runtime.GOOS == "darwin" {
		app.Event.OnApplicationEvent(events.Mac.ApplicationWillTerminate, func(_ *application.ApplicationEvent) {
			doShutdown()
			tray.Destroy()
		})
	}

	// 8. IPC event bridge + helper health monitor.
	// The bridge owns the subscription and re-subscribes when the helper
	// process restarts. The health monitor swaps the client in the holder.
	// Pass the tray's cheap icon-update hook — NOT the full menu rebuild —
	// so the 1 Hz status stream doesn't trigger IPC round-trips on every event.
	bridge := newEventBridge(app, clients, trayMgr.setIconState, tunnelService.ReconcileHistoryFromStatus, trayMgr.quitApp)
	bridge.start()

	// Push the persisted log level to the helper now that the event
	// subscription is live — ensures DEBUG from Settings takes effect
	// on helper-side records immediately after app launch, not only
	// after the user opens and saves Settings.
	if s, err := settingsStore.Load(); err == nil && s != nil && s.LogLevel != "" {
		if c := clients.Get(); c != nil {
			_ = c.Call(ipc.MethodSetLogLevel, ipc.SetLogLevelRequest{Level: s.LogLevel}, nil)
		}
	}

	healthDone := make(chan struct{})
	var healthWg sync.WaitGroup
	healthWg.Add(1)
	startHelperHealthMonitor(app, clients, dataDir, bridge, healthDone, &healthWg)
	// SSID reporter shares the same shutdown channel + WaitGroup so app
	// quit waits for it to exit before returning, instead of leaking the
	// goroutine until process death.
	healthWg.Add(1)
	startSSIDReporter(clients, healthDone, &healthWg)

	// Watch config.json + the tunnels dir so external changes (the CLI)
	// reflect in a running GUI. Shares the shutdown channel/WaitGroup.
	healthWg.Add(1)
	startConfigWatcher(app, configFilePath(paths.ConfigDir), paths.TunnelsDir, healthDone, &healthWg)

	// Update scheduler — periodic GitHub Releases check. Lives in the GUI
	// process (not the helper) because update notifications are a pure UI
	// concern and the helper runs as root with minimum network surface by
	// design. Dev builds short-circuit inside Scheduler.Start, so this is
	// inert until a release-tagged binary runs it.
	updateStateStore, err := update.NewStateStore(paths.ConfigDir)
	if err != nil {
		slog.Warn("update scheduler: cannot create state store", "error", err)
	} else {
		schedulerCtx, schedulerCancel := context.WithCancel(context.Background())
		scheduler := update.NewScheduler(updateStateStore, func(info *update.UpdateInfo) {
			if info == nil {
				return
			}
			app.Event.Emit("update-available", *info)
		}, func() bool {
			// Settings toggle gate: respected at every tick boundary so
			// flipping the toggle off (or back on) takes effect without
			// restarting the app.
			s, err := settingsStore.Load()
			if err != nil || s == nil {
				return true // legacy / unreadable settings → default on
			}
			return s.AutoUpdateCheckEnabled()
		})
		scheduler.Start(schedulerCtx)
		tunnelService.SetUpdateScheduler(scheduler, updateStateStore)

		// Re-check when the window regains focus IF the cached check is
		// older than `focusRecheckThreshold` (the scheduler's Kick handles
		// the staleness gate). Covers the laptop-closed-for-a-week case.
		win.OnWindowEvent(events.Common.WindowFocus, func(_ *application.WindowEvent) {
			scheduler.Kick(false)
		})

		// Cancel scheduler on shutdown so its goroutine exits before
		// app.Run returns.
		go func() {
			<-healthDone
			schedulerCancel()
		}()
	}

	// 9. Run (blocks)
	err = app.Run()
	close(healthDone)
	healthWg.Wait()
	return err
}
