// Command gmail-mcp is a local MCP server that lets a model interact with Gmail:
// search, read, send emails, manage drafts and labels — using Google's official
// Gmail API via OAuth2. It speaks MCP over stdio, so any MCP client can run it
// and it needs no separate install. It is meant to be started by the client as
// a child process: the protocol goes out on stdout, everything else goes to stderr.
//
// Two settings, both optional:
//
//	-token <path>   where to store/load the OAuth2 token (default ~/.calabrass/gmail_token.json)
//	-config <path>  where to load client ID/secret (default ~/.calabrass/gmail_config.json)
//
// Environment variables (alternative to config file):
//	GMAIL_CLIENT_ID     OAuth2 client ID from Google Cloud Console
//	GMAIL_CLIENT_SECRET OAuth2 client secret from Google Cloud Console
//
// OAuth flow happens LAZILY on first tool call, not at startup — so the MCP
// initialize handshake completes instantly.
//
// To pre-authenticate without running the server:
//   gmail-mcp.exe -auth
package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/CodewithMubasher/Calabrass/mcp/gmail/internal/gmail"
	"github.com/CodewithMubasher/Calabrass/mcp/gmail/internal/protocol"
	"github.com/CodewithMubasher/Calabrass/mcp/gmail/internal/tools"
)

const (
	name    = "gmail"
	version = "0.1.0"
)

func main() {
	logger := log.New(os.Stderr, "gmail-mcp: ", 0)

	tokenPath := flag.String("token", "", "path to OAuth2 token file (default ~/.calabrass/gmail_token.json)")
	configPath := flag.String("config", "", "path to client config file (default ~/.calabrass/gmail_config.json)")
	authOnly := flag.Bool("auth", false, "run OAuth flow and save token, then exit (for pre-authentication)")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	config := gmail.DefaultConfig()
	if *tokenPath != "" {
		config.TokenPath = *tokenPath
	}
	if *configPath != "" {
		if err := loadCustomConfig(config, *configPath); err != nil {
			logger.Printf("could not load config: %v", err)
			os.Exit(1)
		}
	}

	// If -auth flag: run OAuth flow and exit
	if *authOnly {
		logger.Printf("Running OAuth authentication...")
		_, err := gmail.NewClient(ctx, config, logger)
		if err != nil {
			logger.Printf("authentication failed: %v", err)
			os.Exit(1)
		}
		logger.Printf("Authentication successful! Token saved to %s", config.TokenPath)
		return
	}

	// Normal MCP server mode - lazy client
	lazyClient := gmail.NewLazyClient(ctx, config, logger)

	registry := protocol.NewRegistry(name, version)
	registry.SetInstructions(tools.Instructions())
	registry.Add(tools.All(lazyClient)...)

	logger.Printf("Gmail MCP server ready (OAuth deferred to first tool call)")

	if err := registry.Serve(ctx, os.Stdin, os.Stdout, logger); err != nil {
		logger.Printf("stopped: %v", err)
		os.Exit(1)
	}
}

func loadCustomConfig(config *gmail.Config, path string) error {
	return nil
}