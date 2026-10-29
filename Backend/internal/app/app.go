// Package app wires the database, providers, workspaces, connectors and HTTP
// API together. Both the standalone server and the desktop shell start from
// here, so they never drift apart.
package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/CodewithMubasher/Calabrass/backend/internal/agent"
	"github.com/CodewithMubasher/Calabrass/backend/internal/ai"
	"github.com/CodewithMubasher/Calabrass/backend/internal/auth"
	"github.com/CodewithMubasher/Calabrass/backend/internal/config"
	"github.com/CodewithMubasher/Calabrass/backend/internal/connectors"
	"github.com/CodewithMubasher/Calabrass/backend/internal/host"
	"github.com/CodewithMubasher/Calabrass/backend/internal/httpapi"
	"github.com/CodewithMubasher/Calabrass/backend/internal/quota"
	"github.com/CodewithMubasher/Calabrass/backend/internal/secrets"
	"github.com/CodewithMubasher/Calabrass/backend/internal/store"
	"github.com/CodewithMubasher/Calabrass/backend/internal/version"
	"github.com/CodewithMubasher/Calabrass/backend/internal/webui"
)

// Options say how this host was started. The zero value is a plain server.
type Options struct {
	// Owner is what started the host (host.OwnerDaemon, OwnerDesktop,
	// OwnerServer), which decides whether it outlives the window that opened it.
	Owner string
	// OnShutdown is called when the owner asks the host to stop.
	OnShutdown func()
	// NoWebUI leaves the web interface out, for a host that only serves the API.
	NoWebUI bool
}

// App is a ready backend without an HTTP listener: the caller decides where it
// listens, or mounts the handler somewhere else.
type App struct {
	Config  config.Config
	Store   *store.Store
	Handler *httpapi.Server
	pool    *connectors.Pool
	// Host is what this host announces about itself. Its secret is what lets the
	// machine owner sign in from the terminal without a password.
	Host host.Info
	// KeyBackend says where provider keys are protected.
	KeyBackend string
}

// New opens the store and builds everything a turn, a schedule or the usage
// page needs. A missing workspace folder, provider or quota file is logged and
// tolerated; only a database that cannot be opened is fatal.
func New(ctx context.Context, cfg config.Config, log *slog.Logger, opts ...Options) (*App, error) {
	var opt Options
	if len(opts) > 0 {
		opt = opts[0]
	}

	st, err := store.Open(cfg.DatabasePath)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	// Provider keys are sealed under a master key the operating system holds, so
	// the database file alone is not enough to read them. A database from an
	// older build has its keys sealed here, once.
	keyBackend := "none: provider keys are stored unprotected"
	if sealer, sealErr := secrets.New(filepath.Dir(cfg.DatabasePath)); sealErr != nil {
		log.Error("provider keys are NOT protected: the system key store could not be used", "error", sealErr)
	} else {
		st.SetSealer(sealer)
		keyBackend = sealer.Backend()
		if n, migrateErr := st.SealLegacySecrets(ctx); migrateErr != nil {
			log.Error("could not protect the provider keys already saved", "error", migrateErr)
		} else if n > 0 {
			log.Info("protected provider keys that were saved as plain text", "count", n, "keystore", keyBackend)
		}
	}

	hostInfo := host.Info{
		Addr:      cfg.Addr,
		PID:       os.Getpid(),
		Version:   version.Version,
		Owner:     opt.Owner,
		StartedAt: time.Now(),
		Secret:    host.NewSecret(),
	}
	if hostInfo.Owner == "" {
		hostInfo.Owner = host.OwnerServer
	}

	var webHandler http.Handler
	if !opt.NoWebUI {
		if site, ok := webui.Embedded(); ok {
			webHandler = webui.Handler(site)
		}
	}

	// Accounts live in the same database as the chats. Nothing is created here:
	// the first account is whoever signs up on the login page, and it owns its
	// own chats, providers, workspaces and settings. A database from an older
	// build still starts, and is signed into once the schema catches up.
	accounts := auth.New(st)

	// Providers live in the database, so the registry reads them on demand:
	// every row carries its own endpoint and key, and a fresh account starts
	// with none. The registry is scoped per account: a row on one account is
	// invisible to another.
	registry := ai.NewRegistry(
		func(ctx context.Context, userID string) ([]ai.ProviderRecord, error) {
			rows, sourceErr := st.ListProviders(ctx, userID)
			if sourceErr != nil {
				return nil, sourceErr
			}
			records := make([]ai.ProviderRecord, 0, len(rows))
			for _, row := range rows {
				records = append(records, ai.ProviderRecord{
					ID:      row.ID,
					Name:    row.Name,
					BaseURL: row.BaseURL,
					APIKey:  row.APIKey,
					Enabled: row.Enabled,
				})
			}
			return records, nil
		})

	// The harness works inside one workspace folder at a time, and every
	// workspace lives inside a single library directory: the workspace folder in
	// the user's data directory. It is never the project the app was built from,
	// where a clean would delete the files someone was working on. A missing
	// library is not fatal: chat still works, it just has no tools.
	libraryDir := cfg.WorkspacesDir
	if strings.TrimSpace(libraryDir) == "" {
		libraryDir = filepath.Join(config.DataDir(), "workspace")
	}

	// The folder is only the container an account's workspaces live under.
	// Nothing is made inside it here: an account gets its own workspace the
	// first time it opens one, so a fresh install holds no user data at all.
	var library *agent.Library
	library, err = agent.NewLibrary(libraryDir)
	if err != nil {
		log.Warn("agent tools are off: no usable workspaces folder", "dir", libraryDir, "error", err)
		library = nil
	} else {
		log.Info("agent workspaces", "library", library.Root())
	}

	// The MCP servers the agent may start. They are started when a turn first
	// needs them and stopped when they have been idle, or when this process
	// ends — the connector's own process lives inside this one. mcpRoot is both
	// what a connector's command is resolved against and where the servers that
	// ship with the app are looked for.
	mcpRoot := mcpRoot()
	pool := connectors.New(st, mcpRoot, log)

	// The free-tier limits are optional: usage still works without them, there
	// is just nothing to measure against. CALABRASS_QUOTAS lets the desktop
	// shell point at the copy it ships in its data folder.
	quotaPath := strings.TrimSpace(cfg.QuotasPath)
	if quotaPath == "" {
		quotaPath = filepath.Join(agent.DiscoverRoot(""), "model_free_tier_quotas.md")
	}
	quotas, quotaErr := quota.Load(quotaPath)
	if quotaErr != nil {
		log.Warn("usage limits are off: could not read the quota file", "file", quotaPath, "error", quotaErr)
		quotas = nil
	} else {
		log.Info("usage limits loaded", "file", quotaPath)
	}

	// Skills are files in ~/.calabrass/skills. The shipped ones are written on
	// first run so a worker has a standard to follow; a skill the person has
	// already edited or removed is left exactly as it is.
	if err := agent.EnsureDefaultSkills(); err != nil {
		log.Warn("could not install the shipped skills", "error", err)
	}

	handler := httpapi.NewServer(httpapi.Deps{
		Store:         st,
		Registry:      registry,
		SystemPrompt:  cfg.SystemPrompt,
		Auth:          accounts,
		Library:       library,
		MistralKey:    cfg.MistralKey,
		VoiceProvider: cfg.VoiceProvider,
		VoiceModel:    cfg.VoiceModel,
		Connectors:    pool,
		Bundled:       connectors.BundledAt(mcpRoot),
		Quotas:        quotas,
		Logger:        log,
		ListenAddr:    cfg.Addr,
		PairingCode:   cfg.PairingCode,
		HostInfo:      hostInfo,
		WebUI:         webHandler,
		KeyBackend:    keyBackend,
		OnShutdown:    opt.OnShutdown,
	})

	return &App{
		Config:     cfg,
		Store:      st,
		Handler:    handler,
		pool:       pool,
		Host:       hostInfo,
		KeyBackend: keyBackend,
	}, nil
}

// mcpRoot is the folder a connector's command is resolved against, and the
// folder the MCP servers that ship with the app are looked for in. An installed
// app keeps them in an MCP folder beside the executable, so that folder wins
// when it is there; running from the source tree there is no such folder, and
// the project root is used instead. It matters that the answer does not depend
// on the working directory: a shortcut, a file association and a terminal all
// start the app from somewhere different.
func mcpRoot() string {
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		if info, err := os.Stat(filepath.Join(dir, "MCP")); err == nil && info.IsDir() {
			return dir
		}
	}
	return agent.DiscoverRoot("")
}

// Close releases the connector processes and the database.
func (a *App) Close() {
	if a == nil {
		return
	}
	if a.pool != nil {
		a.pool.Close()
	}
	if a.Store != nil {
		_ = a.Store.Close()
	}
}
