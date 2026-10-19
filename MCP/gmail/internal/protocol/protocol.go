// Package protocol implements as much of the Model Context Protocol as a tool
// server needs: one JSON-RPC 2.0 message per line on stdin, answers on stdout,
// logs on stderr.
//
// It is written by hand rather than taken from an SDK for the same reason the
// win-control server gives: the protocol version. Clients are strict about the
// version they are answered with, so the server echoes back the version it was
// asked for instead of announcing a newer one of its own. That decision is
// worth a hundred lines and one dependency fewer — and the one dependency this
// server does carry is the Gmail API client, not a transport.
package protocol

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"time"
)

// supportedVersions are the protocol versions this server can speak, best
// first. The first is what it answers with when asked for something it has
// never heard of.
var supportedVersions = []string{"2025-03-26", "2024-11-05"}

// callTimeout is how long a tool may take before the call is given up on.
const callTimeout = 60 * time.Second

// maxMessage is the largest message accepted from a client.
const maxMessage = 32 << 20

// Tool is one thing the model can ask for.
type Tool struct {
	Name        string
	Description string
	// Schema is the JSON Schema for the arguments the tool takes.
	Schema map[string]any
	// Handler runs the call and returns what the model should read. An error is
	// reported to the model as a failed tool call, not as a broken conversation.
	Handler func(ctx context.Context, args map[string]any) (string, error)
}

// Registry holds the tools and speaks the protocol on their behalf.
type Registry struct {
	name         string
	version      string
	instructions string
	tools        []Tool
}

// NewRegistry names the server as the client will see it.
func NewRegistry(name, version string) *Registry {
	return &Registry{name: name, version: version}
}

// SetInstructions is the note the client shows the model beside the tools.
func (r *Registry) SetInstructions(text string) {
	r.instructions = text
}

// Add registers tools, in the order they should be listed.
func (r *Registry) Add(tools ...Tool) {
	r.tools = append(r.tools, tools...)
}

// Serve answers messages until the input ends.
func (r *Registry) Serve(ctx context.Context, in io.Reader, out io.Writer, logger *log.Logger) error {
	if logger == nil {
		logger = log.New(io.Discard, "", 0)
	}

	// Newline-delimited JSON, read as lines so that one unreadable message can
	// be skipped instead of ending the session.
	lines := bufio.NewScanner(in)
	lines.Buffer(make([]byte, 0, 64<<10), maxMessage)

	encoder := json.NewEncoder(out)

	for lines.Scan() {
		line := lines.Bytes()
		if len(line) == 0 {
			continue
		}

		var message incoming
		if err := json.Unmarshal(line, &message); err != nil {
			logger.Printf("ignoring a message that is not JSON: %v", err)
			continue
		}

		// A message without an id is a notification: nothing is answered.
		if len(message.ID) == 0 || string(message.ID) == "null" {
			continue
		}

		answer := answer{JSONRPC: "2.0", ID: message.ID}
		result, rpcErr := r.handle(ctx, message, logger)
		if rpcErr != nil {
			answer.Error = rpcErr
		} else {
			answer.Result = result
		}

		// Encode writes the newline that frames the message.
		if err := encoder.Encode(answer); err != nil {
			return err
		}
	}

	return lines.Err()
}

// incoming is a message from the client. Only what this server answers is read.
type incoming struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

// answer is a reply to the client. The id is echoed exactly as it arrived —
// clients may use a number or a string, and changing it loses the match.
type answer struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (r *Registry) handle(ctx context.Context, message incoming, logger *log.Logger) (json.RawMessage, *rpcError) {
	switch message.Method {
	case "initialize":
		return r.initialize(message.Params), nil
	case "tools/list":
		return r.listTools(), nil
	case "tools/call":
		return r.callTool(ctx, message.Params, logger)
	case "ping":
		// A ping is answered with an empty result, which is not the same as no
		// result at all.
		return json.RawMessage(`{}`), nil
	default:
		// Notifications this server has no interest in — "initialized", log
		// levels, cancellations — never reach here, because they carry no id.
		return nil, &rpcError{Code: -32601, Message: "method not found: " + message.Method}
	}
}

type initializeParams struct {
	ProtocolVersion string `json:"protocolVersion"`
}

func (r *Registry) initialize(raw json.RawMessage) json.RawMessage {
	var params initializeParams
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &params)
	}

	// Echo the version that was asked for, so a client that only knows an older
	// protocol is answered in the tongue it speaks.
	version := supportedVersions[0]
	for _, candidate := range supportedVersions {
		if params.ProtocolVersion == candidate {
			version = candidate
			break
		}
	}

	result := map[string]any{
		"protocolVersion": version,
		// Tools and nothing else: a client checks what is promised here before
		// it asks, so promising more than exists would be a lie worth failing on.
		"capabilities": map[string]any{
			"tools": map[string]any{"listChanged": false},
		},
		"serverInfo": map[string]any{"name": r.name, "version": r.version},
	}
	if r.instructions != "" {
		result["instructions"] = r.instructions
	}

	return encode(result)
}

func (r *Registry) listTools() json.RawMessage {
	listed := make([]any, 0, len(r.tools))
	for _, tool := range r.tools {
		listed = append(listed, map[string]any{
			"name":        tool.Name,
			"description": tool.Description,
			"inputSchema": tool.Schema,
		})
	}
	return encode(map[string]any{"tools": listed})
}

type callParams struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

func (r *Registry) callTool(ctx context.Context, raw json.RawMessage, logger *log.Logger) (json.RawMessage, *rpcError) {
	var params callParams
	if err := json.Unmarshal(raw, &params); err != nil {
		return nil, &rpcError{Code: -32602, Message: "invalid tools/call parameters"}
	}

	var tool *Tool
	for i := range r.tools {
		if r.tools[i].Name == params.Name {
			tool = &r.tools[i]
			break
		}
	}
	if tool == nil {
		return nil, &rpcError{Code: -32602, Message: "unknown tool: " + params.Name}
	}

	ctx, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()

	logger.Printf("calling %s", tool.Name)
	
	// Panic recovery to prevent server crash
	var text string
	var err error
	func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Printf("PANIC in tool %s: %v", tool.Name, r)
				err = fmt.Errorf("tool panicked: %v", r)
			}
		}()
		text, err = tool.Handler(ctx, params.Arguments)
	}()
	
	if err != nil {
		// A tool that could not do the thing is something the model should read
		// and work around, not a broken conversation.
		logger.Printf("%s failed: %v", tool.Name, err)
		return encode(map[string]any{
			"content": []any{map[string]any{"type": "text", "text": err.Error()}},
			"isError": true,
		}), nil
	}

	return encode(map[string]any{
		"content": []any{map[string]any{"type": "text", "text": text}},
	}), nil
}

// encode marshals a result that this package built itself, so a failure here is
// a mistake in the code rather than something a caller did.
func encode(value any) json.RawMessage {
	data, err := json.Marshal(value)
	if err != nil {
		// Unreachable for the shapes used above, and an empty object is still a
		// valid answer to any of the calls that get this far.
		return json.RawMessage(`{}`)
	}
	return data
}