// Command win-control-mcp is a local MCP server that lets a model put things in
// front of the person it is helping: an app, a web search, a song.
//
// It speaks MCP over stdio, so any MCP client can run it and it needs no port,
// no configuration and no install. It is meant to be started by the client as a
// child process: the protocol goes out on stdout, everything else goes to
// stderr.
//
// Setting CALABRASS_WIN_CONTROL_DRY_RUN makes it report what it would do
// instead of doing it.
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/CodewithMubasher/Calabrass/mcp/win-control/internal/protocol"
	"github.com/CodewithMubasher/Calabrass/mcp/win-control/internal/tools"
)

const (
	name    = "win-control"
	version = "0.1.0"
)

func main() {
	// stderr, never stdout: stdout is the protocol, and a stray log line there
	// ends the conversation.
	logger := log.New(os.Stderr, "win-control-mcp: ", 0)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	registry := protocol.NewRegistry(name, version)
	registry.SetInstructions("These tools act on the computer you are running on, " +
		"in front of the person you are helping. Use them when they ask to see, " +
		"hear or open something rather than to be told about it.")
	registry.Add(tools.All()...)

	if err := registry.Serve(ctx, os.Stdin, os.Stdout, logger); err != nil {
		logger.Printf("stopped: %v", err)
		os.Exit(1)
	}
}
