// Command desktop is the Wails shell for Calabrass. It runs the same backend
// the web app talks to, on loopback, and embeds the built React frontend.
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
	"time"

	"github.com/CodewithMubasher/Calabrass/backend/internal/app"
	"github.com/CodewithMubasher/Calabrass/backend/internal/config"
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
	log      *slog.Logger
	cfg      config.Config
	backend  *app.App
	server   *http.Server
	cancel   context.CancelFunc
	startErr error
}

func main() {
	cfg, logPath := resolveConfig()
	log, closeLog := newLogger(logPath)
	defer closeLog()

	d := &desktop{log: log, cfg: cfg}
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
			if d.startErr == nil {
				return
			}
			_, _ = runtime.MessageDialog(ctx, runtime.MessageDialogOptions{
				Type:    runtime.ErrorDialog,
				Title:   appName,
				Message: d.startErr.Error(),
			})
			runtime.Quit(ctx)
		},
		OnShutdown: func(ctx context.Context) {
			d.stop()
		},
		SingleInstanceLock: &options.SingleInstanceLock{
			UniqueId: "calabrass-desktop",
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

	// The quota file travels with the app, so the installed build has limits
	// without the checkout it was built from.
	quotaPath := filepath.Join(dataDir, "model_free_tier_quotas.md")
	if err := os.WriteFile(quotaPath, embeddedQuotas, 0o644); err == nil {
		cfg.QuotasPath = quotaPath
	}
	return cfg, filepath.Join(dataDir, "desktop.log")
}

func newLogger(path string) (*slog.Logger, func()) {
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return slog.New(slog.NewTextHandler(os.Stdout, nil)), func() {}
	}
	return slog.New(slog.NewTextHandler(io.MultiWriter(os.Stdout, file), nil)), func() {
		_ = file.Close()
	}
}

// start brings the API up before the webview loads. If another Calabrass
// backend already owns the port — the web dev server, usually — it reuses that
// one instead of failing, so both windows share the same data.
func (d *desktop) start() error {
	cfg := d.cfg

	listener, err := net.Listen("tcp", cfg.Addr)
	if err != nil {
		if backendHealthy(cfg.Addr) {
			d.log.Info("using the backend already running", "addr", cfg.Addr)
			return nil
		}
		return fmt.Errorf("Calabrass could not start its local API on %s.\n\nClose the other process using that port and try again.\n\n%v", cfg.Addr, err)
	}

	application, err := app.New(context.Background(), cfg, d.log)
	if err != nil {
		_ = listener.Close()
		return err
	}
	d.backend = application

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

	d.log.Info("calabrass desktop API listening", "addr", cfg.Addr, "db", cfg.DatabasePath)
	go func() {
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			d.log.Error("desktop API failed", "error", err)
		}
	}()
	return nil
}

// stop shuts the API and its scheduler down. It is safe to call twice: Wails
// calls it at shutdown, and main defers it for the case Wails never started.
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
		d.backend.Close()
		d.backend = nil
	}
}

// backendHealthy asks the process already on addr whether it is Calabrass.
func backendHealthy(addr string) bool {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}

	client := &http.Client{Timeout: time.Second}
	res, err := client.Get("http://" + net.JoinHostPort(host, port) + "/api/health")
	if err != nil {
		return false
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		return false
	}

	var health struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(res.Body).Decode(&health); err != nil {
		return false
	}
	return health.Status == "ok"
}
