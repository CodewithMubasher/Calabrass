// Package connectors runs the MCP servers the agent is allowed to use.
//
// A connector is a command line. The agent reads the enabled ones, starts each
// as a child process, speaks MCP to it over stdio, and turns the tools it offers
// into tools the model can call.
//
// The processes are kept alive between turns, for two reasons. Starting a server
// costs a process launch and a handshake, which is not worth paying on every
// message. And a converted tool is a call back into the server it came from, so
// a server stopped at the end of a turn would leave the conversation holding
// tools that no longer lead anywhere.
package connectors

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	goai "github.com/zendev-sh/goai"
	"github.com/zendev-sh/goai/mcp"

	"github.com/CodewithMubasher/Calabrass/backend/internal/store"
)

const (
	// How this server introduces itself to a connector.
	clientName    = "calabrass"
	clientVersion = "0.1.0"

	// idleTimeout is how long a connector may go unused before its process is
	// stopped. Long enough that a conversation never pays for it twice.
	idleTimeout = 15 * time.Minute

	// retryAfter is how long a connector that would not start is left alone
	// before it is tried again. Without it, a mistyped command would spend a
	// minute's wait at the start of every turn.
	retryAfter = 30 * time.Second
)

// Pool is the set of connectors that are running, started when they are first
// needed and stopped when they have been idle.
type Pool struct {
	store *store.Store
	// root is the checkout the relative commands are resolved against, so a
	// connector can be written as a path inside the project.
	root string
	log  *slog.Logger

	// ctx is the lifetime of every child process. The SDK starts a connector with
	// exec.CommandContext, so the context handed to Connect is the one the process
	// lives and dies with — it cannot be a turn's context, or the connector would
	// be killed at the end of the turn that started it.
	ctx    context.Context
	cancel context.CancelFunc

	mu      sync.Mutex
	running map[string]*server
	// broken remembers the connectors that would not start, so the next turn is
	// not made to wait for the same failure again.
	broken map[string]failure
}

// server is one connector's process, and what it offers.
type server struct {
	record store.ConnectorRecord
	client *mcp.Client
	tools  []mcp.Tool
	// instructions are the server's own words about what it is for, from the MCP
	// handshake. They are what the prompt can tell the model about a connector
	// without this program knowing anything about it in particular.
	instructions string
	usedLast     time.Time
}

// failure is a connector that could not be started, and the command it was
// written with — a new command is a new attempt.
type failure struct {
	command string
	at      time.Time
	err     error
}

// ToolInfo is a tool as the Connectors page shows it.
type ToolInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// Brief is a connector as the agent should be told about it: what it is called,
// what it says it is for, and the tools it is actually offering.
type Brief struct {
	Name         string   `json:"name"`
	Instructions string   `json:"instructions"`
	Tools        []string `json:"tools"`
}

// New builds a pool. Nothing is started until a turn asks for tools.
func New(st *store.Store, root string, log *slog.Logger) *Pool {
	if log == nil {
		log = slog.Default()
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Pool{
		store:   st,
		root:    root,
		log:     log,
		ctx:     ctx,
		cancel:  cancel,
		running: map[string]*server{},
		broken:  map[string]failure{},
	}
}

// Turn is everything a turn takes from the connectors: the tools to offer the
// model, and, for each connector, the note that says what it is for.
//
// It is one call rather than two because the note describes exactly the tools
// being handed over — a connector whose every tool lost its name to another one
// has nothing to say for itself.
//
// A connector that will not start is logged and left out: one broken server must
// not take the whole turn down with it.
func (p *Pool) Turn(ctx context.Context) ([]Brief, []goai.Tool) {
	if p.store == nil {
		return nil, nil
	}

	p.prune()

	records, err := p.store.ListConnectors(ctx)
	if err != nil {
		p.log.Error("could not read the connectors", "error", err)
		return nil, nil
	}

	var (
		briefs []Brief
		tools  []goai.Tool
		seen   = map[string]bool{}
	)
	for _, record := range records {
		if !record.Enabled {
			continue
		}

		running, err := p.ensure(record)
		if err != nil {
			p.log.Warn("connector is not usable", "connector", record.Name, "error", err)
			continue
		}

		kept := make([]goai.Tool, 0, len(running.tools))
		for _, tool := range mcp.ConvertTools(running.client, running.tools) {
			if seen[tool.Name] {
				p.log.Warn("a connector tool was skipped: another connector already offers that name",
					"tool", tool.Name, "connector", record.Name)
				continue
			}
			seen[tool.Name] = true
			kept = append(kept, inUse(p, record.ID, tool))
		}
		if len(kept) == 0 {
			continue
		}

		tools = append(tools, kept...)
		briefs = append(briefs, Brief{
			Name:         record.Name,
			Instructions: running.instructions,
			Tools:        toolNames(kept),
		})
	}
	return briefs, tools
}

func toolNames(tools []goai.Tool) []string {
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		names = append(names, tool.Name)
	}
	return names
}

// Inspect reports what one connector offers. It starts the connector if it is
// not already running, which is how the Connectors page can show its tools.
func (p *Pool) Inspect(ctx context.Context, record store.ConnectorRecord) ([]ToolInfo, error) {
	if p.store == nil {
		return nil, errors.New("connectors are not available")
	}

	running, err := p.ensure(record)
	if err != nil {
		return nil, err
	}

	info := make([]ToolInfo, 0, len(running.tools))
	for _, tool := range running.tools {
		info = append(info, ToolInfo{Name: tool.Name, Description: tool.Description})
	}
	return info, nil
}

// Forget forgets a connector: a process that is running is stopped, and a
// failure it was carrying is dropped. The next turn starts it again, which is
// what an edited command, or a connector turned off, needs.
func (p *Pool) Forget(id string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.stop(id)
	delete(p.broken, id)
}

// Close stops every connector this pool started.
func (p *Pool) Close() {
	p.mu.Lock()
	for id := range p.running {
		p.stop(id)
	}
	p.mu.Unlock()
	p.cancel()
}

// ensure returns the running server for a connector, starting it if it is not
// running, and starting it again if its command has changed.
func (p *Pool) ensure(record store.ConnectorRecord) (*server, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if broken, failed := p.broken[record.ID]; failed {
		if broken.command == record.Command && time.Since(broken.at) < retryAfter {
			return nil, broken.err
		}
		delete(p.broken, record.ID)
	}

	if running, ok := p.running[record.ID]; ok {
		if running.record.Command == record.Command {
			running.usedLast = time.Now()
			return running, nil
		}
		// The command was edited: the old process is answering for a command
		// that is no longer written down.
		p.stop(record.ID)
	}

	running, err := p.start(record)
	if err != nil {
		p.broken[record.ID] = failure{command: record.Command, at: time.Now(), err: err}
		return nil, err
	}
	p.running[record.ID] = running
	return running, nil
}

func (p *Pool) start(record store.ConnectorRecord) (*server, error) {
	program, args, err := p.resolve(record.Command)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", record.Name, err)
	}

	options := []mcp.StdioOption{}
	if p.root != "" {
		options = append(options, mcp.WithStdioDir(p.root))
	}
	client := mcp.NewClient(clientName, clientVersion,
		mcp.WithTransport(mcp.NewStdioTransport(program, args, options...)))

	// p.ctx, not the turn's: see the field's comment. A server that never answers
	// is given up on by the client's own request timeout.
	if err := client.Connect(p.ctx); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("%s would not start from %q: %w", record.Name, record.Command, err)
	}
	listed, err := client.ListTools(p.ctx, nil)
	if err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("%s started but would not list its tools: %w", record.Name, err)
	}

	p.log.Info("connector started", "connector", record.Name, "tools", len(listed.Tools))
	return &server{
		record:       record,
		client:       client,
		tools:        listed.Tools,
		instructions: client.Instructions(),
		usedLast:     time.Now(),
	}, nil
}

// prune stops the connectors that have gone unused. It is called before a turn
// reads the connectors, so the cost of tidying up is paid where it is invisible.
func (p *Pool) prune() {
	p.mu.Lock()
	defer p.mu.Unlock()

	for id, running := range p.running {
		if time.Since(running.usedLast) > idleTimeout {
			p.log.Info("stopping an idle connector", "connector", running.record.Name)
			p.stop(id)
		}
	}
}

// touch marks a connector as still in use, so that the idle prune cannot stop it
// out from under a turn that is calling it right now.
func (p *Pool) touch(id string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if running, ok := p.running[id]; ok {
		running.usedLast = time.Now()
	}
}

// stop closes a connector's process. The caller holds the lock.
func (p *Pool) stop(id string) {
	running, ok := p.running[id]
	if !ok {
		return
	}
	delete(p.running, id)

	if err := running.client.Close(); err != nil {
		p.log.Warn("a connector did not stop cleanly", "connector", running.record.Name, "error", err)
	}
}

// inUse wraps a converted tool so that every call keeps its connector alive.
func inUse(p *Pool, id string, tool goai.Tool) goai.Tool {
	call := tool.Execute
	tool.Execute = func(ctx context.Context, input json.RawMessage) (string, error) {
		p.touch(id)
		return call(ctx, input)
	}
	return tool
}

// resolve turns a connector's command into a program and its arguments.
//
// A path that exists as it stands wins over splitting the line, because this
// project lives under a folder with a space in its name and a path with a space
// in it is the one thing a naive split cannot survive. When arguments are needed
// the line is split on the quotes the caller wrote, so a path with spaces goes
// in quotes.
func (p *Pool) resolve(command string) (string, []string, error) {
	command = strings.TrimSpace(command)
	if command == "" {
		return "", nil, errors.New("no command to run")
	}

	if path, ok := p.existing(command); ok {
		return path, nil, nil
	}

	fields, err := splitCommand(command)
	if err != nil {
		return "", nil, err
	}
	if len(fields) == 0 {
		return "", nil, errors.New("no command to run")
	}

	program := fields[0]
	if path, ok := p.existing(program); ok {
		program = path
	}
	return program, fields[1:], nil
}

// existing reports whether a name is a file, as written or inside the project.
func (p *Pool) existing(name string) (string, bool) {
	if name == "" {
		return "", false
	}

	candidates := []string{name}
	if !filepath.IsAbs(name) && p.root != "" {
		candidates = append(candidates, filepath.Join(p.root, name))
	}

	for _, candidate := range candidates {
		info, err := os.Stat(candidate)
		if err != nil || info.IsDir() {
			continue
		}
		if absolute, err := filepath.Abs(candidate); err == nil {
			return absolute, true
		}
		return candidate, true
	}
	return "", false
}

// splitCommand splits a command line into a program and its arguments, keeping
// single- and double-quoted runs together.
func splitCommand(line string) ([]string, error) {
	var (
		fields  []string
		current strings.Builder
		quote   rune
	)

	flush := func() {
		if current.Len() > 0 {
			fields = append(fields, current.String())
			current.Reset()
		}
	}

	for _, r := range line {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
				continue
			}
			current.WriteRune(r)
		case r == '"' || r == '\'':
			quote = r
		case r == ' ' || r == '\t':
			flush()
		default:
			current.WriteRune(r)
		}
	}

	if quote != 0 {
		return nil, fmt.Errorf("the command ends inside a %q", string(quote))
	}
	flush()
	return fields, nil
}
