package main

import (
	"embed"
	"log"
	"os"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
	"github.com/wailsapp/wails/v3/pkg/services/notifications"

	"github.com/klein-it-solutions/kimai-desktop-app/internal/app"
	"github.com/klein-it-solutions/kimai-desktop-app/internal/config"
	"github.com/klein-it-solutions/kimai-desktop-app/internal/desktop"
)

//go:embed all:frontend/dist
var assets embed.FS

// License texts shown in Settings → Über (also for the portable Windows .exe).
//
//go:embed LICENSE
var licenseText string

//go:embed THIRD_PARTY_NOTICES.md
var noticesText string

// Window size including the transparent margin that holds the CSS drop shadow.
// The visible card is 360×480 plus 2×16 px (frontend/src/app.css, --shadow-gutter).
const (
	windowWidth  = 392
	windowHeight = 512
)

func init() {
	application.RegisterEvent[bool]("theme:changed")
	application.RegisterEvent[app.ConnectionResult](app.EventConnectionChanged)
	application.RegisterEvent[app.TrackerState](app.EventTrackerChanged)
	application.RegisterEvent[app.PendingStatus](app.EventPendingChanged)
	application.RegisterEvent[app.ShellStatus](app.EventShellChanged)
	application.RegisterEvent[string](app.EventNavigate)
	application.RegisterEvent[app.VoiceState](app.EventVoiceState)
	application.RegisterEvent[float64](app.EventVoiceLevel)
	application.RegisterEvent[string](app.EventVoiceResult)
	application.RegisterEvent[app.DownloadProgress](app.EventVoiceDownload)
	application.RegisterEvent[app.AssistantDownload](app.EventAssistantDownload)
}

// openConfig loads the configuration. A corrupt file is moved aside so the
// app still starts (with defaults) instead of failing.
func openConfig() *config.Store {
	path, err := config.DefaultPath()
	if err != nil {
		log.Fatal(err)
	}
	store, err := config.Open(path)
	if err == nil {
		return store
	}
	log.Printf("config: %v – moving it to %s.broken and starting with defaults", err, path)
	_ = os.Rename(path, path+".broken")
	store, err = config.Open(path)
	if err != nil {
		log.Fatal(err)
	}
	return store
}

// openCatalog opens the list cache (~/.cache) and UI state (~/.config).
func openCatalog() *app.Catalog {
	cachePath, err := config.DefaultCachePath("cache.json")
	if err != nil {
		log.Fatal(err)
	}
	statePath, err := config.DefaultStatePath("state.json")
	if err != nil {
		log.Fatal(err)
	}
	return app.NewCatalog(cachePath, statePath)
}

// openOutbox opens the entries saved while Kimai was unreachable (~/.config).
func openOutbox() *app.Outbox {
	path, err := config.DefaultStatePath("pending.json")
	if err != nil {
		log.Fatal(err)
	}
	return app.NewOutbox(path)
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--check-keyring" {
		os.Exit(checkKeyring())
	}

	var mainWindow *application.WebviewWindow

	secretsPath, err := config.DefaultSecretsPath()
	if err != nil {
		log.Fatal(err)
	}
	store := openConfig()
	conn := app.NewConnection(store, config.Keyring{}, &config.FileSecrets{Path: secretsPath})
	settings := app.NewSettingsService(conn)
	tracker := app.NewTrackerService(conn, settings, openCatalog(), openOutbox())
	notifier := notifications.New()
	shell := app.NewShell(store, tracker, notifier)

	a := application.New(application.Options{
		Name:        "Kimai Tracker",
		Description: "Minimalist desktop time tracker for Kimai",
		Services: []application.Service{
			application.NewService(app.NewAppService(licenseText, noticesText)),
			application.NewService(settings),
			application.NewService(tracker),
			application.NewService(app.NewEntriesService(tracker)),
			application.NewService(app.NewShellService(shell)),
			application.NewService(app.NewVoiceService(store, conn, shell)),
			application.NewService(app.NewAssistantService(store)),
			application.NewService(notifier),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		Linux: application.LinuxOptions{
			ApplicationID: "solutions.klein_it.KimaiTracker",
			ProgramName:   "kimai-tracker",
			// The app lives in the tray; closing the window must not quit it.
			DisableQuitOnLastWindowClosed: true,
		},
		// A second launch is forwarded to the running instance: it shows the
		// window or runs --toggle / --timer (used for desktop shortcuts, e.g.
		// on Wayland where apps cannot grab global keys).
		SingleInstance: &application.SingleInstanceOptions{
			UniqueID:               "solutions.klein-it.kimai-tracker",
			OnSecondInstanceLaunch: func(d application.SecondInstanceData) { shell.HandleArgs(d.Args) },
		},
		Mac: application.MacOptions{
			// The app lives in the tray; closing the window must not quit it.
			ApplicationShouldTerminateAfterLastWindowClosed: false,
		},
	})

	mainWindow = a.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:            "main",
		Title:           "Kimai Tracker",
		Width:           windowWidth,
		Height:          windowHeight,
		MinWidth:        windowWidth,
		MinHeight:       windowHeight,
		DisableResize:   true,
		Frameless:       true,
		BackgroundType:  application.BackgroundTypeTransparent,
		InitialPosition: application.WindowCentered,
		Hidden:          app.StartHidden(os.Args[1:]),
		Mac: application.MacWindow{
			Backdrop: application.MacBackdropTranslucent,
			TitleBar: application.MacTitleBarHiddenInset,
		},
		Windows: application.WindowsWindow{
			// The rounded window and its shadow are drawn by the page inside a
			// transparent margin; a native frame shadow would outline that margin.
			DisableFramelessWindowDecorations: true,
		},
		Linux: application.LinuxWindow{
			WindowIsTranslucent: true,
		},
		URL: "/",
	})

	a.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(*application.ApplicationEvent) {
		application.InvokeSync(desktop.MakeFramelessWindowsTransparent)
	})
	shell.Setup(a, mainWindow)
	defer shell.Shutdown()

	a.Event.OnApplicationEvent(events.Common.ThemeChanged, func(*application.ApplicationEvent) {
		a.Event.Emit("theme:changed", a.Env.IsDarkMode())
	})

	if err := a.Run(); err != nil {
		log.Fatal(err)
	}
}
