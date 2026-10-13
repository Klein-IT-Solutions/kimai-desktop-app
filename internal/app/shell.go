package app

import (
	_ "embed"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
	"github.com/wailsapp/wails/v3/pkg/services/notifications"

	"github.com/klein-it-solutions/kimai-desktop-app/internal/config"
	"github.com/klein-it-solutions/kimai-desktop-app/internal/desktop"
	"github.com/klein-it-solutions/kimai-desktop-app/internal/hotkey"
)

//go:embed assets/tray-idle.png
var trayIdleIcon []byte

//go:embed assets/tray-running.png
var trayRunningIcon []byte

// EventNavigate asks the frontend to show a view ("settings").
const EventNavigate = "navigate"

// EventShellChanged is emitted with a ShellStatus when background settings change.
const EventShellChanged = "shell:changed"

// Command-line flags handled by the running instance.
const (
	FlagHidden = "--hidden" // start in the tray
	FlagToggle = "--toggle" // show/hide the window
	FlagShow   = "--show"   // show the window
	FlagTimer  = "--timer"  // start/stop the timer
)

// ShellStatus describes background features for the settings UI.
type ShellStatus struct {
	TrayAvailable      bool          `json:"trayAvailable"`
	Autostart          bool          `json:"autostart"`
	AutostartSupported bool          `json:"autostartSupported"`
	Notifications      bool          `json:"notifications"`
	Hotkey             hotkey.Status `json:"hotkey"`
	ToggleCommand      string        `json:"toggleCommand"`
	TimerCommand       string        `json:"timerCommand"`
}

// Shell runs the app in the background: tray icon, close-to-tray, global
// hotkey, notifications and command-line actions from a second instance.
// It is not bound to the frontend; ShellService exposes the UI methods.
type Shell struct {
	store    *config.Store
	tracker  *TrackerService
	notifier *notifications.NotificationService

	app    *application.App
	window *application.WebviewWindow
	tray   *application.SystemTray

	menu       *application.Menu
	statusItem *application.MenuItem
	detailItem *application.MenuItem
	timerItem  *application.MenuItem
	windowItem *application.MenuItem

	hotkeys   *hotkey.Manager
	quitting  atomic.Bool
	shownOnce atomic.Bool

	mu        sync.Mutex
	state     TrackerState
	lastLabel string
}

// NewShell creates the shell. Call Setup before app.Run.
func NewShell(store *config.Store, tracker *TrackerService, notifier *notifications.NotificationService) *Shell {
	s := &Shell{store: store, tracker: tracker, notifier: notifier}
	s.hotkeys = hotkey.NewManager(
		hotkey.Shortcut{ID: "toggle-window", Description: "Kimai Tracker ein-/ausblenden"},
		func() { application.InvokeAsync(s.ToggleWindow) },
		nil,
		func(hotkey.Status) { s.emitStatus() },
	)
	return s
}

// Setup creates the tray icon and wires window, tracker and hotkey events.
func (s *Shell) Setup(app *application.App, window *application.WebviewWindow) {
	s.app, s.window = app, window

	// Closing the window (button, Alt+F4, compositor) hides it to the tray.
	window.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		if s.quitting.Load() {
			return
		}
		e.Cancel()
		s.CloseWindow()
	})
	// Window events do not fire reliably for Hide() on every platform; the
	// window actions below refresh the menu themselves as well.
	window.OnWindowEvent(events.Common.WindowShow, func(*application.WindowEvent) { s.refreshMenuSoon() })
	window.OnWindowEvent(events.Common.WindowHide, func(*application.WindowEvent) { s.refreshMenuSoon() })

	s.buildTray()

	app.Event.On(EventTrackerChanged, func(e *application.CustomEvent) {
		if st, ok := e.Data.(TrackerState); ok {
			s.onTrackerChanged(st)
		}
	})

	app.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(*application.ApplicationEvent) {
		s.state = s.tracker.State()
		s.refreshMenu()
		go s.hotkeys.Register(s.store.Get().Hotkey)
		go s.tick()
	})
}

// Shutdown releases the hotkey.
func (s *Shell) Shutdown() { s.hotkeys.Stop() }

func (s *Shell) buildTray() {
	s.tray = s.app.SystemTray.New()
	s.tray.SetIcon(trayIdleIcon)
	s.tray.SetLabel("Kimai Tracker")
	s.tray.SetTooltip("Kimai Tracker")
	s.tray.OnClick(func() { s.ToggleWindow() })

	m := s.app.NewMenu()
	s.statusItem = m.Add("Gestoppt").SetEnabled(false)
	s.detailItem = m.Add("").SetEnabled(false).SetHidden(true)
	s.timerItem = m.Add("Timer starten").OnClick(func(*application.Context) { go s.ToggleTimer() })
	m.AddSeparator()
	s.windowItem = m.Add("Fenster anzeigen").OnClick(func(*application.Context) { s.ToggleWindow() })
	m.Add("Einstellungen …").OnClick(func(*application.Context) {
		s.ShowWindow()
		s.app.Event.Emit(EventNavigate, "settings")
	})
	m.AddSeparator()
	m.Add("Beenden").OnClick(func(*application.Context) { s.Quit() })
	s.menu = m
	s.tray.SetMenu(m)
}

// tick keeps the running time in the tray menu current (minute resolution).
func (s *Shell) tick() {
	for range time.Tick(5 * time.Second) {
		s.mu.Lock()
		running := s.state.Running
		s.mu.Unlock()
		if running {
			s.refreshMenu()
		}
	}
}

func (s *Shell) onTrackerChanged(st TrackerState) {
	s.mu.Lock()
	prev := s.state
	s.state = st
	s.mu.Unlock()
	s.refreshMenu()

	switch {
	case st.Running && (!prev.Running || prev.TimesheetID != st.TimesheetID):
		body := st.Project.Name + " · " + st.Activity.Name
		if st.Customer.Name != "" {
			body += "\n" + st.Customer.Name
		}
		s.notify("Timer gestartet", body)
	case !st.Running && prev.Running:
		body := prev.Project.Name
		if d := elapsedSince(prev.Begin); d > 0 {
			body = formatHours(d) + " · " + body
		}
		s.notify("Timer gestoppt", body)
	}
}

// refreshMenu updates tray icon and menu labels; cheap when nothing changed.
func (s *Shell) refreshMenu() {
	if s.tray == nil {
		return
	}
	s.mu.Lock()
	st := s.state
	s.mu.Unlock()
	visible := s.window != nil && s.window.IsVisible()

	status, detail, timer := "Gestoppt", "", "Timer starten"
	if st.Running {
		status = "Läuft · " + formatHours(elapsedSince(st.Begin))
		if st.Project.Name != "" {
			status += " – " + st.Project.Name
		}
		detail = strings.Trim(st.Customer.Name+" · "+st.Activity.Name, " ·")
		timer = "Timer stoppen"
	} else if st.Project.ID != 0 && st.Activity.ID != 0 {
		timer = "Timer starten – " + st.Project.Name
	}
	windowLabel := "Fenster anzeigen"
	if visible {
		windowLabel = "Fenster ausblenden"
	}
	key := strings.Join([]string{status, detail, timer, windowLabel}, "|")

	s.mu.Lock()
	changed := key != s.lastLabel
	s.lastLabel = key
	s.mu.Unlock()
	if !changed {
		return
	}
	application.InvokeAsync(func() {
		if st.Running {
			s.tray.SetIcon(trayRunningIcon)
		} else {
			s.tray.SetIcon(trayIdleIcon)
		}
		s.tray.SetLabel(status)
		s.statusItem.SetLabel(status)
		s.detailItem.SetLabel(detail).SetHidden(detail == "")
		s.timerItem.SetLabel(timer)
		s.windowItem.SetLabel(windowLabel)
		s.tray.SetMenu(s.menu)
	})
}

// ToggleWindow shows the window, or hides it when it is visible and focused.
func (s *Shell) ToggleWindow() {
	if s.window == nil {
		return
	}
	if s.window.IsVisible() && !s.window.IsMinimised() && s.window.IsFocused() {
		s.window.Hide()
		s.refreshMenuSoon()
		return
	}
	s.ShowWindow()
}

// refreshMenuSoon updates the menu once the window state has settled.
func (s *Shell) refreshMenuSoon() {
	go func() {
		time.Sleep(150 * time.Millisecond)
		s.refreshMenu()
	}()
}

// ShowWindow brings the window to the front.
func (s *Shell) ShowWindow() {
	if s.window == nil {
		return
	}
	if s.window.IsMinimised() {
		s.window.UnMinimise()
	}
	// A window created hidden (autostart) is not centred by the toolkit;
	// GTK ignores positioning before the window is mapped, so centre after
	// showing. (On Wayland the compositor decides the position anyway.)
	center := !s.shownOnce.Swap(true) && !s.window.IsVisible()
	s.window.Show()
	s.window.Focus()
	if center {
		go func() {
			time.Sleep(50 * time.Millisecond)
			application.InvokeAsync(s.window.Center)
		}()
	}
	s.refreshMenuSoon()
}

// CloseWindow hides the window to the tray, or minimises it when no tray
// is available (otherwise the app could not be reached anymore).
func (s *Shell) CloseWindow() {
	if s.window == nil {
		return
	}
	if desktop.TrayAvailable() {
		s.window.Hide()
	} else {
		s.window.Minimise()
	}
	s.refreshMenuSoon()
}

// Quit exits the app. A running timer keeps running in Kimai.
func (s *Shell) Quit() {
	s.quitting.Store(true)
	s.hotkeys.Stop()
	s.app.Quit()
}

// ToggleTimer starts the timer with the last selection or stops it. Used by
// the tray, the --timer flag and (later) shortcuts; errors are notified.
func (s *Shell) ToggleTimer() {
	st := s.tracker.State()
	var err error
	if st.Running {
		_, err = s.tracker.Stop()
	} else {
		if st.Project.ID == 0 || st.Activity.ID == 0 {
			application.InvokeAsync(s.ShowWindow)
			return
		}
		_, err = s.tracker.Start(StartRequest{Customer: st.Customer, Project: st.Project, Activity: st.Activity, Description: st.Description})
	}
	if err != nil {
		s.notifyAlways("Kimai Tracker", err.Error())
	}
}

// HandleArgs runs the action requested by a second instance (or the first
// launch). Unknown or missing flags show the window.
func (s *Shell) HandleArgs(args []string) {
	action := FlagShow
	for _, a := range args {
		switch a {
		case FlagToggle, FlagShow, FlagTimer:
			action = a
		}
	}
	switch action {
	case FlagToggle:
		application.InvokeAsync(s.ToggleWindow)
	case FlagTimer:
		go s.ToggleTimer()
	default:
		application.InvokeAsync(s.ShowWindow)
	}
}

// StartHidden reports whether the window should start hidden: requested via
// --hidden and a tray icon is available to bring it back.
func StartHidden(args []string) bool {
	for _, a := range args {
		if a == FlagHidden {
			return desktop.TrayAvailable()
		}
	}
	return false
}

func (s *Shell) notify(title, body string) {
	if !s.store.Get().Notifications {
		return
	}
	s.notifyAlways(title, body)
}

func (s *Shell) notifyAlways(title, body string) {
	if s.notifier == nil {
		return
	}
	go func() {
		_ = s.notifier.SendNotification(notifications.NotificationOptions{
			ID:    "kimai-tracker-timer",
			Title: title,
			Body:  body,
		})
	}()
}

// Status returns the state of the background features.
func (s *Shell) Status() ShellStatus {
	cfg := s.store.Get()
	return ShellStatus{
		TrayAvailable:      desktop.TrayAvailable(),
		Autostart:          desktop.AutostartEnabled(),
		AutostartSupported: autostartSupported(),
		Notifications:      cfg.Notifications,
		Hotkey:             s.hotkeys.Status(),
		ToggleCommand:      "kimai-tracker " + FlagToggle,
		TimerCommand:       "kimai-tracker " + FlagTimer,
	}
}

func (s *Shell) emitStatus() {
	if s.app != nil {
		s.app.Event.Emit(EventShellChanged, s.Status())
	}
}

func elapsedSince(begin string) time.Duration {
	t, err := time.Parse(time.RFC3339, begin)
	if err != nil {
		return 0
	}
	return time.Since(t)
}

// formatHours renders a duration as "H:MM h".
func formatHours(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	m := int(d.Minutes())
	return fmt.Sprintf("%d:%02d h", m/60, m%60)
}

// ShellService exposes background settings to the frontend.
type ShellService struct{ shell *Shell }

// NewShellService creates the bound service.
func NewShellService(shell *Shell) *ShellService { return &ShellService{shell: shell} }

// Status returns tray, autostart, notification and hotkey state.
func (s *ShellService) Status() ShellStatus { return s.shell.Status() }

// SetAutostart enables or disables starting at login.
func (s *ShellService) SetAutostart(enabled bool) (ShellStatus, error) {
	if err := desktop.SetAutostart(enabled); err != nil {
		return s.shell.Status(), fmt.Errorf("Autostart konnte nicht geändert werden: %v", err)
	}
	return s.shell.Status(), nil
}

// SetNotifications enables or disables desktop notifications.
func (s *ShellService) SetNotifications(enabled bool) (ShellStatus, error) {
	if err := s.shell.store.Update(func(c *config.Config) { c.Notifications = enabled }); err != nil {
		return s.shell.Status(), err
	}
	return s.shell.Status(), nil
}

// SetHotkey changes the global shortcut ("" disables it).
func (s *ShellService) SetHotkey(accelerator string) (ShellStatus, error) {
	accelerator = strings.TrimSpace(accelerator)
	if accelerator != "" {
		acc, err := hotkey.Parse(accelerator)
		if err != nil {
			return s.shell.Status(), fmt.Errorf("Ungültiges Tastenkürzel: %v", err)
		}
		accelerator = acc.String()
	}
	if err := s.shell.store.Update(func(c *config.Config) { c.Hotkey = accelerator }); err != nil {
		return s.shell.Status(), err
	}
	s.shell.hotkeys.Register(accelerator)
	return s.shell.Status(), nil
}

// TestNotification sends a sample notification.
func (s *ShellService) TestNotification() {
	s.shell.notifyAlways("Kimai Tracker", "Benachrichtigungen funktionieren.")
}

// CloseWindow hides the window to the tray (or minimises it without tray).
func (s *ShellService) CloseWindow() { s.shell.CloseWindow() }

// Quit exits the app.
func (s *ShellService) Quit() { s.shell.Quit() }

func autostartSupported() bool { return runtime.GOOS == "linux" || runtime.GOOS == "windows" }
