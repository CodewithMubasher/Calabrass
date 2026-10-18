// Command excel-mcp is a local MCP server that lets a model make and change
// Microsoft Excel workbooks: a budget, a report, a list, a chart — a file the
// person can open in Excel when the conversation is over.
//
// It speaks MCP over stdio, so any MCP client can run it and it needs no port
// and no install. It is meant to be started by the client as a child process:
// the protocol goes out on stdout, everything else goes to stderr.
//
// Two settings, both optional:
//
//	-root <folder>   where relative paths land. Defaults to Documents\Calabrass.
//	CALABRASS_EXCEL_ROOT, CALABRASS_EXCEL_ALLOW_OUTSIDE
//	                 the same, from the environment.
//
// Excel files are read and written with github.com/xuri/excelize, so the server
// is one self-contained binary with no Office install behind it.
package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/CodewithMubasher/Calabrass/mcp/excel/internal/protocol"
	"github.com/CodewithMubasher/Calabrass/mcp/excel/internal/tools"
	"github.com/CodewithMubasher/Calabrass/mcp/excel/internal/workbook"
)

const (
	name    = "excel"
	version = "0.1.0"
)

func main() {
	// stderr, never stdout: stdout is the protocol, and a stray log line there
	// ends the conversation.
	logger := log.New(os.Stderr, "excel-mcp: ", 0)

	root := flag.String("root", "", "folder relative paths are resolved against (default Documents\\Calabrass)")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	folder, err := workbook.NewRoot(*root)
	if err != nil {
		logger.Printf("no folder to work in: %v", err)
		os.Exit(1)
	}
	logger.Printf("working in %s", folder.Dir())

	registry := protocol.NewRegistry(name, version)
	registry.SetInstructions(tools.Instructions(folder))
	registry.Add(tools.All(folder)...)

	if err := registry.Serve(ctx, os.Stdin, os.Stdout, logger); err != nil {
		logger.Printf("stopped: %v", err)
		os.Exit(1)
	}
}
