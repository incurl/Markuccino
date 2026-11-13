// Valkyrie is a desktop application scaffold.
//
// It wires together Wails v3, an embedded PocketBase and a React
// frontend, and it provides a generic job queue that an application
// plugs its own backend into.
//
// The UI shell is deliberately plain: it is the starting point, not
// the product. Replace the contents of internal/runner with your own
// work and extend the frontend as needed.
//
// Persistence, file storage, migrations and access rules are provided
// by PocketBase, embedded as a Go library. PocketBase is never run as a
// standalone process: its REST and file API is mounted on the Wails
// asset server so the files are served from the UI's own origin.
package main

import (
	"embed"
	"fmt"
	"log"
	"net/http"
	"os"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/incurl/Markuccino/internal/bindings"
	"github.com/incurl/Markuccino/internal/console"
	"github.com/incurl/Markuccino/internal/jobs"
	"github.com/incurl/Markuccino/internal/pocketbase"
	"github.com/incurl/Markuccino/internal/project"
	"github.com/incurl/Markuccino/internal/runner"
	"github.com/wailsapp/wails/v3/pkg/application"
	wailsevents "github.com/wailsapp/wails/v3/pkg/events"
)

// Wails embeds the built frontend. Any file in frontend/dist is served
// to the webview.
//
//go:embed all:frontend/dist
var assets embed.FS

func init() {
	// Register the job update event so the binding generator produces a
	// strongly typed TS API for it.
	application.RegisterEvent[jobs.Job]("job:updated")

	// The debug console entry event, likewise.
	application.RegisterEvent[console.Entry]("console:entry")

	// The binder event carries a project id, so every view of the same
	// project refreshes when a document or an item changes.
	application.RegisterEvent[bindings.BinderEvent]("binder:updated")

	// The close handshake: the window is not destroyed until the
	// frontend has saved the editors.
	application.RegisterEvent[bindings.AppClosingEvent](bindings.EventAppClosing)
}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

// run bootstraps the backend and starts the Wails application.
func run() error {
	// --- PocketBase ------------------------------------------------
	pbService := pocketbase.NewService(dataDir())
	if err := pbService.Bootstrap(); err != nil {
		return fmt.Errorf("bootstrap pocketbase: %w", err)
	}

	cipher, err := pocketbase.NewCipher(pbService.DataDir())
	if err != nil {
		return fmt.Errorf("initialise settings encryption: %w", err)
	}
	settings := pocketbase.NewSettingsStore(pbService.App(), cipher)

	// The event bridge pushes job updates to the frontend. It is
	// created once the Wails application exists, so it is declared here
	// and assigned later.
	var bridge *bindings.EventBridge

	// The debug console records the application traffic for the panel
	// at the bottom of the window.
	consoleLog := console.New()

	// The job manager delegates the work to the registered runner. The
	// scaffold ships an echo runner, which is replaced by your own.
	jobManager := jobs.NewManager(
		pbService.App(),
		jobs.WithLog(consoleLog),
		jobs.WithRunnerFunc(func() jobs.Runner {
			return runner.NewEcho(settings, consoleLog)
		}),
	)

	// A job that was running when the application last closed was
	// never finished and never failed. Nothing would ever pick it up
	// again, and the user could not even delete it. The reconciliation
	// marks it as failed, so it appears in the history and can be
	// cleared.
	if reconciled, err := jobManager.Reconcile(); err != nil {
		return fmt.Errorf("reconcile the interrupted jobs: %w", err)
	} else if reconciled > 0 {
		consoleLog.Warn("jobs", fmt.Sprintf(
			"%d job(s) were interrupted by a restart and have been marked as failed", reconciled))
	}

	// The writing domain: projects, the binder and the Markdown bodies.
	// It is created before the Wails application because the service
	// needs it; the notifier is attached once the event bridge exists.
	projectStore := project.NewStore(pbService.App(), pbService.DataDir(), consoleLog)

	// The window does not exist yet, so the service is given a function
	// that it can call once the window does. RequestClose is the last
	// step of the save-then-close handshake.
	var closeWindow func()
	appService := bindings.NewAppService(pbService, settings, jobManager, consoleLog, projectStore, func() {
		if closeWindow != nil {
			closeWindow()
		}
	})

	// Mount PocketBase's own REST and file API alongside the static
	// frontend. This keeps everything on a single origin: no extra
	// port, no separate PocketBase process and no CORS handling.
	pbHandler, handlerErr := pbService.Handler()
	if handlerErr != nil {
		return fmt.Errorf("create pocketbase handler: %w", handlerErr)
	}

	// --- Wails application -----------------------------------------
	app := application.New(application.Options{
		Name: bindings.AppName,
		// The description lands in the about box of the platform, so
		// it is the one place the version is visible without a menu.
		Description: "A Wails v3 + PocketBase + React desktop application, version " + bindings.AppVersion,
		Services: []application.Service{
			application.NewService(appService),
		},
		Assets: application.AssetOptions{
			Handler: combineHandlers(pbHandler, application.AssetFileServerFS(assets)),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
		// The running jobs are stopped when the application closes, so
		// that a job is never killed in the middle of a write.
		//
		// The event bridge is stopped first: a job that is cancelled
		// emits its final status, and emitting into a webview that is
		// being destroyed is a native crash. Losing those last events is
		// harmless; the next start refetches.
		OnShutdown: func() {
			bridge.Stop()
			jobManager.ShutdownWithTimeout(jobs.ShutdownTimeout)
		},
		// PocketBase is closed after the windows are gone, so that no
		// binding call can race the database shutdown. Closing it
		// checkpoints the WAL and stops the cron ticker instead of
		// leaving both to the process exit.
		PostShutdown: func() {
			if err := pbService.Close(); err != nil {
				log.Printf("close pocketbase: %v", err)
			}
		},
	})

	// The event bridge pushes job updates to the frontend.
	bridge = bindings.NewEventBridge(app)
	jobManager.SetEmitter(bridge)
	consoleLog.SetEmitter(bridge)
	projectStore.SetNotifier(bridge.EmitBinderUpdated)

	// A virtual GPU's WebKitGTK/Mesa teardown is a known source of
	// native crashes on close. The override is documented, and an
	// explicit WEBKIT_DISABLE_DMABUF_RENDERER always wins.
	disableDMABUFOnVirtualGPU()

	window := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:  bindings.AppName,
		Width:  1440,
		Height: 900,
		Mac: application.MacWindow{
			InvisibleTitleBarHeight: 50,
			Backdrop:                application.MacBackdropTranslucent,
			TitleBar:                application.MacTitleBarHiddenInset,
		},
		BackgroundColour: application.NewRGB(6, 7, 15),
		URL:              "/",
	})

	// The close is a handshake: the first close request is cancelled so
	// the frontend can save its editors, and RequestClose closes the
	// window for real. Without this, any text typed in the last 900 ms
	// (the autosave pause) is lost.
	var closing atomic.Bool
	var flushRequested sync.Once
	closeWindow = func() {
		if closing.CompareAndSwap(false, true) {
			application.InvokeAsync(func() { window.Close() })
		}
	}
	window.RegisterHook(wailsevents.Common.WindowClosing, func(event *application.WindowEvent) {
		if closing.Load() {
			// The flush is done. Stop the event stream before the
			// webview is destroyed: an emit into a dying webview is a
			// native crash, not a recoverable error.
			bridge.Stop()
			return
		}
		event.Cancel()
		flushRequested.Do(func() {
			bridge.EmitAppClosing()
			// Never leave a window that cannot be closed: if the
			// frontend is wedged, close it after a short grace period.
			time.AfterFunc(3*time.Second, closeWindow)
		})
	})

	return app.Run()
}

// disableDMABUFOnVirtualGPU turns off WebKitGTK's DMA-BUF renderer on a
// virtual GPU.
//
// WebKitGTK's DMA-BUF path goes through the host's virtio/SVGA driver,
// and the surface teardown can crash natively when the webview closes
// (Mesa reports "context mismatch in svga_surface_destroy"). A writing
// application does not need GPU compositing, so the renderer is disabled
// on the machine types where the crash is observed. An explicit
// WEBKIT_DISABLE_DMABUF_RENDERER always wins.
func disableDMABUFOnVirtualGPU() {
	if runtime.GOOS != "linux" {
		return
	}
	if os.Getenv("WEBKIT_DISABLE_DMABUF_RENDERER") != "" {
		return
	}

	markers := []string{
		"/sys/class/dmi/id/sys_vendor",
		"/sys/class/dmi/id/product_name",
		"/sys/class/dmi/id/board_vendor",
	}
	vendors := []string{"vmware", "virtualbox", "innotek", "qemu", "kvm", "xen"}
	for _, path := range markers {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		value := strings.ToLower(string(data))
		for _, vendor := range vendors {
			if strings.Contains(value, vendor) {
				_ = os.Setenv("WEBKIT_DISABLE_DMABUF_RENDERER", "1")
				return
			}
		}
	}
}

// combineHandlers routes /api/* to the PocketBase handler and
// everything else to the static frontend.
func combineHandlers(apiHandler, staticHandler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			apiHandler.ServeHTTP(w, r)
			return
		}
		staticHandler.ServeHTTP(w, r)
	})
}

// dataDir returns the directory holding the database and the uploaded
// files.
//
// The VALKYRIE_DATA_DIR environment variable overrides the default,
// which lets the app run against an isolated database during development
// and testing.
func dataDir() string {
	if dir := os.Getenv("VALKYRIE_DATA_DIR"); dir != "" {
		return dir
	}
	return pocketbase.DefaultDataDir()
}
