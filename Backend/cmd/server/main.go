// Command server is the Calabrass host: the one process per machine that owns
// the data, the keys, the schedules and the approvals. Every face (the desktop
// app, the terminal, the web page) is a guest of it.
//
//	calabrass-server            run the host in this terminal
//	calabrass-server --daemon   run it as a background host that outlives the
//	                            window or terminal that started it
//	calabrass-server status     say whether a host is running, and where
//	calabrass-server stop       ask the running host to stop
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/CodewithMubasher/Calabrass/backend/internal/app"
	"github.com/CodewithMubasher/Calabrass/backend/internal/config"
	"github.com/CodewithMubasher/Calabrass/backend/internal/host"
	"github.com/CodewithMubasher/Calabrass/backend/internal/version"
)

func main() {
	args := os.Args[1:]
	if len(args) > 0 {
		switch args[0] {
		case "status":
			os.Exit(runStatus())
		case "stop":
			os.Exit(runStop())
		case "version", "--version", "-v":
			fmt.Println(version.Version)
			return
		}
	}
	daemon := false
	for _, a := range args {
		if a == "--daemon" || a == "-daemon" {
			daemon = true
		}
	}
	os.Exit(runHost(daemon))
}

func runHost(daemon bool) int {
	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(log)

	cfg := config.Load()
	dataDir := config.DataDir()

	// The agent can run commands, so reaching the API is as good as reaching a
	// shell. It listens beyond this machine only when a pairing code is set.
	if err := cfg.ValidateBind(); err != nil {
		log.Error("refusing to start", "error", err)
		fmt.Fprintln(os.Stderr, "calabrass:", err)
		return 2
	}

	// One host per machine. If one is already running, this is a guest: say so
	// and leave. Starting a second would fight it for the database.
	if existing, ok := host.Discover(context.Background(), dataDir); ok {
		log.Info("a Calabrass host is already running; use that one",
			"addr", existing.Addr, "pid", existing.PID, "owner", existing.Owner)
		return 0
	}

	owner := host.OwnerServer
	if daemon {
		owner = host.OwnerDaemon
	}

	stop := make(chan struct{}, 1)
	requestStop := func() {
		select {
		case stop <- struct{}{}:
		default:
		}
	}

	application, err := app.New(context.Background(), cfg, log, app.Options{Owner: owner, OnShutdown: requestStop})
	if err != nil {
		log.Error("could not start the backend", "error", err)
		return 1
	}
	defer application.Close()

	listener, err := net.Listen("tcp", cfg.Addr)
	if err != nil {
		log.Error("could not listen", "addr", cfg.Addr, "error", err)
		return 1
	}

	// Announce the host only once it can answer, so a face that finds the file
	// can rely on it.
	if err := host.Claim(context.Background(), dataDir, application.Host); err != nil {
		_ = listener.Close()
		if running, ok := host.IsAlreadyRunning(err); ok {
			log.Info("a Calabrass host is already running; use that one", "addr", running.Info.Addr)
			return 0
		}
		log.Error("could not announce the host", "error", err)
		return 1
	}
	defer host.Remove(dataDir, application.Host.PID)

	// The scheduled tasks run here: the loop looks for what has come due, runs
	// it as a turn of its own, and stops with the host.
	schedulerCtx, stopScheduler := context.WithCancel(context.Background())
	defer stopScheduler()
	go application.Handler.RunScheduler(schedulerCtx)

	srv := &http.Server{
		Handler:           application.Handler,
		ReadHeaderTimeout: 10 * time.Second,
		// No WriteTimeout: responses are long-lived SSE streams.
		IdleTimeout: 120 * time.Second,
	}

	go func() {
		log.Info("calabrass host listening",
			"addr", cfg.Addr, "owner", owner, "version", version.Version,
			"db", cfg.DatabasePath, "keys", application.KeyBackend)
		if err := srv.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("server failed", "error", err)
			requestStop()
		}
	}()

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	select {
	case <-signals:
	case <-stop:
	}

	log.Info("shutting down")
	stopScheduler()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Error("graceful shutdown failed", "error", err)
	}
	return 0
}

// runStatus prints what is running, for people and for scripts.
func runStatus() int {
	info, ok := host.Discover(context.Background(), config.DataDir())
	if !ok {
		fmt.Println("Calabrass is not running.")
		return 1
	}
	out, _ := json.MarshalIndent(info.Public(), "", "  ")
	fmt.Println(string(out))
	return 0
}

// runStop asks the running host to stop, proving it may with the host secret.
func runStop() int {
	info, ok := host.Discover(context.Background(), config.DataDir())
	if !ok {
		fmt.Println("Calabrass is not running.")
		return 0
	}
	if err := host.Stop(context.Background(), info); err != nil {
		fmt.Fprintln(os.Stderr, "calabrass:", err)
		return 1
	}
	fmt.Println("Calabrass stopped.")
	return 0
}
