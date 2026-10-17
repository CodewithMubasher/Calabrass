package protocol

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
)

// exchange runs messages through the server and reads back everything it
// answered. It is deliberately the real path — real messages on a pipe, real
// answers read off the wire — because the shape of the wire is the promise this
// package makes.
func exchange(t *testing.T, lines ...string) []map[string]any {
	t.Helper()

	registry := NewRegistry("test", "1.0")
	registry.SetInstructions("say what you mean")
	registry.Add(
		Tool{
			Name:        "echo",
			Description: "says it back",
			Schema:      Object(map[string]any{"text": String("what to say")}, "text"),
			Handler: func(ctx context.Context, args map[string]any) (string, error) {
				text, err := RequiredString(args, "text", 100)
				if err != nil {
					return "", err
				}
				return "said " + text, nil
			},
		},
		Tool{
			Name:        "broken",
			Description: "never works",
			Schema:      Object(nil),
			Handler: func(context.Context, map[string]any) (string, error) {
				return "", errors.New("it could not be done")
			},
		},
	)

	reader, writer := io.Pipe()
	var out bytes.Buffer
	served := make(chan error, 1)
	go func() { served <- registry.Serve(context.Background(), reader, &out, nil) }()

	go func() {
		for _, line := range lines {
			_, _ = io.WriteString(writer, line+"\n")
		}
		_ = writer.Close()
	}()

	if err := <-served; err != nil {
		t.Fatalf("serve: %v", err)
	}

	var answers []map[string]any
	decoder := json.NewDecoder(&out)
	for {
		var message map[string]any
		if err := decoder.Decode(&message); err != nil {
			break
		}
		answers = append(answers, message)
	}
	return answers
}

func resultOf(t *testing.T, message map[string]any) map[string]any {
	t.Helper()
	result, ok := message["result"].(map[string]any)
	if !ok {
		t.Fatalf("no result in %v", message)
	}
	return result
}

func TestInitializeEchoesTheVersionAndPromisesTools(t *testing.T) {
	answers := exchange(t, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","clientInfo":{"name":"test","version":"1"}}}`)
	if len(answers) != 1 {
		t.Fatalf("got %d answers, want 1", len(answers))
	}

	result := resultOf(t, answers[0])
	if got := result["protocolVersion"]; got != "2025-03-26" {
		t.Errorf("protocolVersion = %v, want the version that was asked for", got)
	}
	capabilities, _ := result["capabilities"].(map[string]any)
	if _, ok := capabilities["tools"]; !ok {
		t.Error("capabilities do not include tools, and a client checks this before asking for them")
	}
	server, _ := result["serverInfo"].(map[string]any)
	if server["name"] != "test" {
		t.Errorf("serverInfo.name = %v, want test", server["name"])
	}
	if result["instructions"] != "say what you mean" {
		t.Errorf("instructions = %v, want the note that was set", result["instructions"])
	}
}

func TestInitializeFallsBackForAVersionItDoesNotKnow(t *testing.T) {
	answers := exchange(t, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"1999-01-01"}}`)
	if got := resultOf(t, answers[0])["protocolVersion"]; got != "2025-03-26" {
		t.Errorf("protocolVersion = %v, want the newest this server speaks", got)
	}
}

func TestNotificationsAreNotAnswered(t *testing.T) {
	answers := exchange(t,
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26"}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
	)
	if len(answers) != 1 {
		t.Fatalf("got %d answers, want 1: a message without an id has nothing to answer to", len(answers))
	}
}

func TestToolsAreListedWithTheirSchema(t *testing.T) {
	answers := exchange(t, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)

	result := resultOf(t, answers[0])
	listed, _ := result["tools"].([]any)
	if len(listed) != 2 {
		t.Fatalf("got %d tools, want 2", len(listed))
	}

	first, _ := listed[0].(map[string]any)
	if first["name"] != "echo" {
		t.Errorf("name = %v, want echo", first["name"])
	}
	if first["description"] != "says it back" {
		t.Errorf("description = %v", first["description"])
	}
	schema, _ := first["inputSchema"].(map[string]any)
	if schema["type"] != "object" {
		t.Errorf("inputSchema.type = %v, want object", schema["type"])
	}
	properties, _ := schema["properties"].(map[string]any)
	if _, ok := properties["text"]; !ok {
		t.Error("the schema does not describe the text argument")
	}
	required, _ := schema["required"].([]any)
	if len(required) != 1 || required[0] != "text" {
		t.Errorf("required = %v, want [text]", schema["required"])
	}
}

func TestCallingAToolAnswersWithItsText(t *testing.T) {
	answers := exchange(t, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"echo","arguments":{"text":"hello"}}}`)

	result := resultOf(t, answers[0])
	content, _ := result["content"].([]any)
	if len(content) != 1 {
		t.Fatalf("got %d content blocks, want 1", len(content))
	}
	block, _ := content[0].(map[string]any)
	if block["type"] != "text" {
		t.Errorf("content type = %v, want text", block["type"])
	}
	if block["text"] != "said hello" {
		t.Errorf("text = %v, want said hello", block["text"])
	}
	if result["isError"] == true {
		t.Error("a call that worked is not an error")
	}
}

func TestAFailedToolIsSomethingTheModelReads(t *testing.T) {
	answers := exchange(t, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"broken","arguments":{}}}`)

	result := resultOf(t, answers[0])
	if result["isError"] != true {
		t.Fatalf("isError = %v, want true", result["isError"])
	}
	content, _ := result["content"].([]any)
	block, _ := content[0].(map[string]any)
	text, _ := block["text"].(string)
	if !strings.Contains(text, "it could not be done") {
		t.Errorf("text = %q, want the reason it failed", text)
	}
}

func TestAnUnknownToolIsAProtocolError(t *testing.T) {
	answers := exchange(t, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"nothing"}}`)

	rpcErr, ok := answers[0]["error"].(map[string]any)
	if !ok {
		t.Fatalf("got %v, want an error", answers[0])
	}
	if rpcErr["code"] != float64(-32602) {
		t.Errorf("code = %v, want -32602", rpcErr["code"])
	}
	if _, both := answers[0]["result"]; both {
		t.Error("an answer is either a result or an error, never both")
	}
}

func TestTheIdIsEchoedExactlyAsItArrived(t *testing.T) {
	answers := exchange(t,
		`{"jsonrpc":"2.0","id":"seven","method":"ping"}`,
		`{"jsonrpc":"2.0","id":7,"method":"ping"}`,
	)
	if len(answers) != 2 {
		t.Fatalf("got %d answers, want 2", len(answers))
	}
	if answers[0]["id"] != "seven" {
		t.Errorf("id = %v, want the string seven", answers[0]["id"])
	}
	if answers[1]["id"] != float64(7) {
		t.Errorf("id = %v, want the number 7", answers[1]["id"])
	}
	if result := resultOf(t, answers[0]); len(result) != 0 {
		t.Errorf("ping result = %v, want an empty object", result)
	}
}

func TestAnUnknownMethodIsRefusedAndTheSessionCarriesOn(t *testing.T) {
	answers := exchange(t,
		`{"jsonrpc":"2.0","id":1,"method":"nothing/at/all"}`,
		`{"jsonrpc":"2.0","id":2,"method":"ping"}`,
	)
	if len(answers) != 2 {
		t.Fatalf("got %d answers, want 2", len(answers))
	}
	if _, ok := answers[0]["error"]; !ok {
		t.Errorf("got %v, want an error for the unknown method", answers[0])
	}
	if _, ok := answers[1]["result"]; !ok {
		t.Errorf("got %v, want the session to still be there", answers[1])
	}
}

func TestAnUnreadableMessageIsSkippedRatherThanFatal(t *testing.T) {
	answers := exchange(t,
		`this is not json`,
		`{"jsonrpc":"2.0","id":1,"method":"ping"}`,
	)
	if len(answers) != 1 {
		t.Fatalf("got %d answers, want 1", len(answers))
	}
}
