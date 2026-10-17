package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/CodewithMubasher/Calabrass/backend/internal/agent"
	"github.com/CodewithMubasher/Calabrass/backend/internal/ai"
	"github.com/CodewithMubasher/Calabrass/backend/internal/config"
	"github.com/CodewithMubasher/Calabrass/backend/internal/connectors"
	"github.com/CodewithMubasher/Calabrass/backend/internal/httpapi"
	"github.com/CodewithMubasher/Calabrass/backend/internal/store"
)

func main() {
	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(log)

	cfg := config.Load()

	st, err := store.Open(cfg.DatabasePath)
	if err != nil {
		log.Error("could not open database", "error", err)
		os.Exit(1)
	}
	defer func() { _ = st.Close() }()

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

	if len(registry.ProviderOrder(context.Background())) == 0 {
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
		if _, err := st.EnsureWorkspace(context.Background(), store.WorkspaceRecord{
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
	defer pool.Close()

	handler := httpapi.New(httpapi.Deps{
		Store:         st,
		Registry:      registry,
		SystemPrompt:  cfg.SystemPrompt,
		Library:       library,
		Workspace:     workspace,
		MistralKey:    cfg.MistralKey,
		VoiceProvider: cfg.VoiceProvider,
		VoiceModel:    cfg.VoiceModel,
		Connectors:    pool,
		Logger:        log,
	})

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		// No WriteTimeout: responses are long-lived SSE streams.
		IdleTimeout: 120 * time.Second,
	}

	go func() {
		log.Info("calabrass backend listening", "addr", cfg.Addr, "db", cfg.DatabasePath)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("server failed", "error", err)
			os.Exit(1)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	log.Info("shutting down")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Error("graceful shutdown failed", "error", err)
	}
}
