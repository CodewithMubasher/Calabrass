// Command desktop is the Wails shell for Calabrass. It is a face of the host,
// not the host itself: it looks for the host that is already running on this
// machine and attaches to it, starts a background host when there is none (so
// scheduled tasks keep running after the window closes), and only hosts the
// backend in-process as a last resort. It embeds the built React frontend.
package main

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/CodewithMubasher/Calabrass/backend/internal/app"
	"github.com/CodewithMubasher/Calabrass/backend/internal/config"
	"github.com/CodewithMubasher/Calabrass/backend/internal/host"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

const appName = "Calabrass"

// The built frontend. `pnpm build:desktop` writes it here before Wails compiles
// the Go side; the placeholder .gitkeep keeps `go build ./...` happy on a fresh
// checkout.
//
//go:embed all:dist
var assets embed.FS

// The quota file is duplicated into assets by the build instructions so the
// installed app still has limits. A test keeps the copy honest.
//
//go:embed assets/model_free_tier_quotas.md
var embeddedQuotas []byte

type desktop struct {
	log     *slog.Logger
	cfg     config.Config
	dataDir string
	prefs   *prefs

	// How this window relates to the host.
	attached host.Info // the host we are a guest of; zero when we host in-process
	spawned  bool      // we started that host, so closing the window is our question to ask

	// Only when the host is in this process (the last resort).
	backend *app.App
	server  *http.Server
	cancel  context.CancelFunc

	ctx      context.Context // the window's, set at startup
	startErr error

	// stopHome ends Home Base (approval watching, run notices, the hotkey).
	stopHome func()
}

func main() {
	cfg, logPath := resolveConfig()
	log, closeLog := newLogger(logPath)
	defer closeLog()

	dataDir := config.DataDir()
	d := &desktop{log: log, cfg: cfg, dataDir: dataDir, prefs: loadPrefs(dataDir)}
	d.startErr = d.start()
	defer d.stop()

	err := wails.Run(&options.App{
		Title: appName,
		// The restore size stays inside a 1366x768 screen's 720px work area.
		// Starting maximized lets Windows size the window to that work area, so
		// the bottom is never hidden under the taskbar.
		Width:            1200,
		Height:           680,
		MinWidth:         960,
		MinHeight:        560,
		WindowStartState: options.Maximised,
		BackgroundColour: &options.RGBA{R: 0, G: 0, B: 0, A: 1},
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		OnStartup: func(ctx context.Context) {
			d.ctx = ctx
			if d.startErr == nil {
				// Home Base is an extra: the window is already usable without it.
				d.stopHome = d.startHomeBase(ctx)
				return
			}
			_, _ = runtime.MessageDialog(ctx, runtime.MessageDialogOptions{
				Type:    runtime.ErrorDialog,
				Title:   appName,
				Message: d.startErr.Error(),
			})
			runtime.Quit(ctx)
		},
		// Closing the window is where a person decides whether the host they
		// started keeps running behind it.
		OnBeforeClose: func(ctx context.Context) (prevent bool) {
			d.beforeClose(ctx)
			return false
		},
		// Dropping a folder on the window opens a chat in it (see homebase.go).
		DragAndDrop: &options.DragAndDrop{EnableFileDrop: true},
		OnShutdown: func(ctx context.Context) {
			if d.stopHome != nil {
				d.stopHome()
				d.stopHome = nil
			}
			d.stop()
		},
		SingleInstanceLock: &options.SingleInstanceLock{
			UniqueId: "calabrass-desktop",
			// Opening Calabrass a second time brings the window back instead
			// of doing nothing, which matters once the host runs without one.
			OnSecondInstanceLaunch: func(options.SecondInstanceData) {
				if d.ctx != nil {
					runtime.WindowUnminimise(d.ctx)
					runtime.Show(d.ctx)
				}
			},
		},
		Windows: &windows.Options{
			WebviewIsTransparent: false,
			WindowIsTranslucent:  false,
		},
	})
	if err != nil {
		log.Error("desktop shell failed", "error", err)
		os.Exit(1)
	}
}

// resolveConfig points the desktop at the per-user data folder. The database,
// the workspaces and the log all live there, never inside the checkout the app
// was built from: a clean, a fresh clone or an update must not be able to move
// or delete what the user made with the app.
//
// It deliberately does not change the working directory: Wails runs the app
// from the project root while generating bindings and expects wails.json there.
func resolveConfig() (config.Config, string) {
	dataDir := config.DataDir()
	_ = os.MkdirAll(dataDir, 0o755)

	cfg := config.LoadFrom(filepath.Join(dataDir, ".env"))
	if os.Getenv("CALABRASS_DB") == "" {
		cfg.DatabasePath = filepath.Join(dataDir, "calabrass.db")
	}
	if cfg.WorkspacesDir == "" {
		cfg.WorkspacesDir = filepath.Join(dataDir, "workspace")
	}

	cfg.QuotasPath = installQuotas(dataDir, embeddedQuotas)
	return cfg, filepath.Join(dataDir, "desktop.log")
}

func newLogger(path string) (*slog.Logger, func()) {
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return slog.New(slog.NewTextHandler(os.Stdout, nil)), func() {}
	}
	// The file writes first, and stdout only joins when there is a console to
	// write to. An installed app is a GUI process with none, so os.Stdout is an
	// invalid handle, every write to it fails, and io.MultiWriter stops at the
	// first writer that fails — which left the log empty for every real run and
	// a whole turn's history unreadable.
	writers := []io.Writer{file}
	if info, statErr := os.Stdout.Stat(); statErr == nil && info.Mode()&os.ModeCharDevice != 0 {
		writers = append(writers, os.Stdout)
	}
	return slog.New(slog.NewTextHandler(io.MultiWriter(writers...), nil)), func() {
		_ = file.Close()
	}
}

// installQuotas puts the free-tier limits where the backend reads them, and
// returns that path.
//
// Two copies, on purpose. model_free_tier_quotas.default.md is the one that
// ships with this build and is refreshed on every start, so an update brings
// new limits. model_free_tier_quotas.md is the person's own: it is created from
// the shipped one only when it does not exist, and is never overwritten, so a
// hand edit survives every launch and every update. The backend reads the
// person's copy.
func installQuotas(dataDir string, shipped []byte) string {
	defaultPath := filepath.Join(dataDir, "model_free_tier_quotas.default.md")
	userPath := filepath.Join(dataDir, "model_free_tier_quotas.md")

	if err := os.WriteFile(defaultPath, shipped, 0o644); err != nil {
		return userPath
	}
	if _, err := os.Stat(userPath); errors.Is(err, os.ErrNotExist) {
		if err := os.WriteFile(userPath, shipped, 0o644); err != nil {
			return defaultPath
		}
	}
	return userPath
}

// start makes sure a host is running and this window is its guest.
//
//  1. A host is already running (the CLI started one, or a background host from
//     an earlier session): attach to it.
//  2. None is, and the background host's executable is installed beside the
//     app: start it detached, so it keeps running, schedules and all, after this
//     window closes, and attach to that.
//  3. Otherwise host the backend in this process, as every earlier version did.
//     It ends with the window.
func (d *desktop) start() error {
	cfg := d.cfg

	if err := cfg.ValidateBind(); err != nil {
		return err
	}

	if info, ok := host.Discover(context.Background(), d.dataDir); ok {
		d.attach(info, false)
		return nil
	}
	// A host from before hosts announced themselves: still a host, so share it.
	if host.Healthy(context.Background(), cfg.Addr) {
		d.log.Info("using the backend already running", "addr", cfg.Addr)
		return nil
	}

	if d.prefs.backgroundAllowed() {
		if exe, err := os.Executable(); err == nil {
			if bin, found := host.FindBinary(filepath.Dir(exe)); found {
				info, spawnErr := host.Spawn(context.Background(), bin, d.dataDir, 20*time.Second)
				if spawnErr == nil {
					d.attach(info, true)
					return nil
				}
				d.log.Warn("could not start the background host; hosting in this window instead", "error", spawnErr)
			}
		}
	}

	return d.hostInProcess()
}

func (d *desktop) attach(info host.Info, spawned bool) {
	d.attached = info
	d.spawned = spawned
	d.log.Info("attached to the Calabrass host",
		"addr", info.Addr, "pid", info.PID, "owner", info.Owner, "version", info.Version, "startedByThisWindow", spawned)
}

// hostInProcess is the fallback: the backend lives in this process and ends
// with it. It still announces itself, so the terminal attaches instead of
// starting a second one.
func (d *desktop) hostInProcess() error {
	cfg := d.cfg

	listener, err := net.Listen("tcp", cfg.Addr)
	if err != nil {
		if host.Healthy(context.Background(), cfg.Addr) {
			d.log.Info("using the backend already running", "addr", cfg.Addr)
			return nil
		}
		return fmt.Errorf("Calabrass could not start its local API on %s.\n\nClose the other process using that port and try again.\n\n%v", cfg.Addr, err)
	}

	application, err := app.New(context.Background(), cfg, d.log, app.Options{
		Owner: host.OwnerDesktop,
		// `calabrass-server stop` closes the window, which ends the host.
		OnShutdown: func() {
			if d.ctx != nil {
				runtime.Quit(d.ctx)
			}
		},
	})
	if err != nil {
		_ = listener.Close()
		return err
	}
	d.backend = application

	if err := host.Claim(context.Background(), d.dataDir, application.Host); err != nil {
		d.log.Warn("could not announce this window's host to other faces", "error", err)
	}

	schedulerCtx, cancel := context.WithCancel(context.Background())
	d.cancel = cancel
	go application.Handler.RunScheduler(schedulerCtx)

	server := &http.Server{
		Handler:           application.Handler,
		ReadHeaderTimeout: 10 * time.Second,
		// No WriteTimeout: chat replies are long-lived SSE streams.
		IdleTimeout: 120 * time.Second,
	}
	d.server = server

	d.log.Info("calabrass desktop API listening", "addr", cfg.Addr, "db", cfg.DatabasePath, "keys", application.KeyBackend)
	go func() {
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			d.log.Error("desktop API failed", "error", err)
		}
	}()
	return nil
}

// beforeClose asks, once, whether a host this window started should keep
// running. The answer is remembered; delete desktop.json to be asked again.
func (d *desktop) beforeClose(ctx context.Context) {
	if d.attached.PID == 0 || !d.spawned || d.attached.Owner != host.OwnerDaemon {
		return // not our host to decide about
	}
	switch d.prefs.OnClose {
	case "keep":
		return
	case "quit":
		d.stopBackgroundHost()
		return
	}

	const keep, quit = "Keep running", "Quit completely"
	choice, err := runtime.MessageDialog(ctx, runtime.MessageDialogOptions{
		Type:  runtime.QuestionDialog,
		Title: appName,
		Message: "Keep Calabrass running in the background?\n\n" +
			"Scheduled tasks only happen while it is running. You can open Calabrass again at any time, " +
			"and the `calabrass-backend stop` command (also in the Start menu) stops it.\n\n" +
			"Your choice is remembered. Delete desktop.json in Calabrass's data folder to be asked again.",
		Buttons:       []string{keep, quit},
		DefaultButton: keep,
		CancelButton:  keep,
	})
	if err != nil || choice == keep || choice == "" {
		d.prefs.OnClose = "keep"
		d.prefs.save(d.dataDir)
		return
	}
	d.prefs.OnClose = "quit"
	d.prefs.save(d.dataDir)
	d.stopBackgroundHost()
}

func (d *desktop) stopBackgroundHost() {
	if d.attached.PID == 0 {
		return
	}
	if err := host.Stop(context.Background(), d.attached); err != nil {
		d.log.Warn("could not stop the background host", "error", err)
	}
}

// stop ends what this window owns. A host that runs on its own is left alone:
// the window was only its guest. It is safe to call twice: Wails calls it at
// shutdown, and main defers it for the case Wails never started.
func (d *desktop) stop() {
	if d.cancel != nil {
		d.cancel()
		d.cancel = nil
	}
	if d.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_ = d.server.Shutdown(ctx)
		cancel()
		d.server = nil
	}
	if d.backend != nil {
		host.Remove(d.dataDir, d.backend.Host.PID)
		d.backend.Close()
		d.backend = nil
	}
}

// --- preferences -------------------------------------------------------------

// prefs are the few things the window remembers about itself.
type prefs struct {
	// OnClose is what to do with a host this window started: "" asks, "keep"
	// leaves it running, "quit" stops it.
	OnClose string `json:"onClose,omitempty"`
}

func loadPrefs(dataDir string) *prefs {
	p := &prefs{}
	if raw, err := os.ReadFile(filepath.Join(dataDir, "desktop.json")); err == nil {
		_ = json.Unmarshal(raw, p)
	}
	return p
}

func (p *prefs) save(dataDir string) {
	if raw, err := json.MarshalIndent(p, "", "  "); err == nil {
		_ = os.WriteFile(filepath.Join(dataDir, "desktop.json"), raw, 0o600)
	}
}

// backgroundAllowed is false when the person turned the background host off,
// with CALABRASS_BACKGROUND=off, and the window should host in-process instead.
func (p *prefs) backgroundAllowed() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("CALABRASS_BACKGROUND"))) {
	case "off", "0", "false", "no":
		return false
	}
	return true
}
