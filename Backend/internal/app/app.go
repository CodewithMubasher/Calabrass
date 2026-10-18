// Package app wires the database, providers, workspaces, connectors and HTTP
// API together. Both the standalone server and the desktop shell start from
// here, so they never drift apart.
package app

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"

	"github.com/CodewithMubasher/Calabrass/backend/internal/agent"
	"github.com/CodewithMubasher/Calabrass/backend/internal/ai"
	"github.com/CodewithMubasher/Calabrass/backend/internal/config"
	"github.com/CodewithMubasher/Calabrass/backend/internal/connectors"
	"github.com/CodewithMubasher/Calabrass/backend/internal/httpapi"
	"github.com/CodewithMubasher/Calabrass/backend/internal/quota"
	"github.com/CodewithMubasher/Calabrass/backend/internal/store"
)

// App is a ready backend without an HTTP listener: the caller decides where it
// listens, or mounts the handler somewhere else.
type App struct {
	Config  config.Config
	Store   *store.Store
	Handler *httpapi.Server
	pool    *connectors.Pool
}

// New opens the store and builds everything a turn, a schedule or the usage
// page needs. A missing workspace folder, provider or quota file is logged and
// tolerated; only a database that cannot be opened is fatal.
func New(ctx context.Context, cfg config.Config, log *slog.Logger) (*App, error) {
	st, err := store.Open(cfg.DatabasePath)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	// Providers are stored in the database, so the registry reads them on
	// demand: built-in providers (Google, Groq) keep their credential in the
	// environment unless a stored override exists, and custom providers carry
	// their own endpoint and key.
	registry := ai.NewRegistry(cfg.GeminiKey, cfg.GroqKey, cfg.ProviderPriority,
		func(ctx context.Context) ([]ai.ProviderRecord, error) {
			rows, sourceErr := st.ListProviders(ctx)
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
					Custom:  row.Custom,
				})
			}
			return records, nil
		})

	if len(registry.ProviderOrder(ctx)) == 0 {
		log.Warn("no usable AI provider; set GROQ_API_KEY or GOOGLE_GENERATIVE_AI_API_KEY in Backend/.env, or add a provider on the Providers page")
	}

	// The harness works inside one workspace folder at a time, and every
	// workspace lives inside a single library directory: <project>/workspace
	// unless it is configured elsewhere. A missing library is not fatal: chat
	// still works, it just has no tools.
	libraryDir := cfg.WorkspacesDir
	if strings.TrimSpace(libraryDir) == "" {
		libraryDir = filepath.Join(agent.DiscoverRoot(""), "workspace")
	}

	var (
		library   *agent.Library
		workspace *agent.Workspace
	)
	library, err = agent.NewLibrary(libraryDir)
	if err != nil {
		log.Warn("agent tools are off: no usable workspaces folder", "dir", libraryDir, "error", err)
		library = nil
	} else {
		// The default workspace is the one a chat works in until it picks its
		// own, so it exists from the first boot.
		workspace, err = library.Ensure(agent.DefaultWorkspaceName)
		if err != nil {
			log.Warn("agent tools are off: the default workspace is unusable", "error", err)
			workspace = nil
		} else {
			log.Info("agent workspace", "root", workspace.Root(), "library", library.Root())
		}
		if _, err := st.EnsureWorkspace(ctx, store.WorkspaceRecord{
			Name:        "Default",
			Description: "Where chats work until they pick a workspace of their own.",
			Dir:         agent.DefaultWorkspaceName,
			Builtin:     true,
		}); err != nil {
			log.Warn("could not record the default workspace", "error", err)
		}
	}

	// The MCP servers the agent may start. They are started when a turn first
	// needs them and stopped when they have been idle, or when this process
	// ends — the connector's own process lives inside this one.
	pool := connectors.New(st, agent.DiscoverRoot(""), log)

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

	handler := httpapi.NewServer(httpapi.Deps{
		Store:         st,
		Registry:      registry,
		SystemPrompt:  cfg.SystemPrompt,
		Library:       library,
		Workspace:     workspace,
		MistralKey:    cfg.MistralKey,
		VoiceProvider: cfg.VoiceProvider,
		VoiceModel:    cfg.VoiceModel,
		Connectors:    pool,
		Quotas:        quotas,
		Logger:        log,
	})

	return &App{
		Config:  cfg,
		Store:   st,
		Handler: handler,
		pool:    pool,
	}, nil
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
