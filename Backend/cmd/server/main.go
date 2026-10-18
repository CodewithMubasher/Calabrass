package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/CodewithMubasher/Calabrass/backend/internal/app"
	"github.com/CodewithMubasher/Calabrass/backend/internal/config"
)

func main() {
	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(log)

	cfg := config.Load()

	application, err := app.New(context.Background(), cfg, log)
	if err != nil {
		log.Error("could not start the backend", "error", err)
		os.Exit(1)
	}
	defer application.Close()

	// The scheduled tasks run here: the loop looks for what has come due, runs
	// it as a turn of its own, and stops with the process.
	schedulerCtx, stopScheduler := context.WithCancel(context.Background())
	defer stopScheduler()
	go application.Handler.RunScheduler(schedulerCtx)

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           application.Handler,
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
