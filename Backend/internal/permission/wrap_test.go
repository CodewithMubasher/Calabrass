package permission

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"

	goai "github.com/zendev-sh/goai"
)

type cmdArgs struct {
	Command string `json:"command"`
}

// counting returns a tool that records how many times it really ran.
func counting(name string, ran *atomic.Int32) goai.Tool {
	return goai.NewTool(name, "test tool", func(ctx context.Context, a cmdArgs) (string, error) {
		ran.Add(1)
		return "ran", nil
	})
}

func call(t goai.Tool, command string) (string, error) {
	raw, _ := json.Marshal(cmdArgs{Command: command})
	return t.Execute(context.Background(), raw)
}

func TestGateRefusesWithoutRunningTheTool(t *testing.T) {
	var ran atomic.Int32
	tools := Gate([]goai.Tool{counting("run_command", &ran)},
		&Session{Mode: Unattended, Scope: DefaultScope(), Workspace: ws})

	_, err := call(tools[0], "npm install")
	var denied *Denied
	if !errors.As(err, &denied) {
		t.Fatalf("want a denial, got %v", err)
	}
	if ran.Load() != 0 {
		t.Fatal("a refused command was executed anyway")
	}
}

func TestGateRunsPreApprovedAndObservingCommands(t *testing.T) {
	var ran atomic.Int32
	s := &Session{Mode: Unattended, Scope: ParseScope(`{"commands":["npm test"]}`), Workspace: ws}
	tools := Gate([]goai.Tool{counting("run_command", &ran)}, s)

	if out, err := call(tools[0], "npm test"); err != nil || out != "ran" {
		t.Fatalf("pre-approved command: %q %v", out, err)
	}
	if out, err := call(tools[0], "git status"); err != nil || out != "ran" {
		t.Fatalf("read-only command: %q %v", out, err)
	}
	if _, err := call(tools[0], "npm test && rm -rf /"); err == nil {
		t.Fatal("a chained command slipped through a pre-approved pattern")
	}
	if ran.Load() != 2 {
		t.Fatalf("ran %d times, want 2", ran.Load())
	}
}

func TestGateWaitsForAnApproval(t *testing.T) {
	var ran atomic.Int32
	b := NewBroker()
	tools := Gate([]goai.Tool{counting("run_command", &ran)}, interactive(b))

	done := make(chan error, 1)
	go func() { _, err := call(tools[0], "npm test"); done <- err }()
	req := waitPending(t, b, "u1")
	if ran.Load() != 0 {
		t.Fatal("the tool ran before anyone answered")
	}
	_ = b.Decide("u1", req.ID, AllowOnce)
	if err := <-done; err != nil || ran.Load() != 1 {
		t.Fatalf("approved run: err=%v ran=%d", err, ran.Load())
	}
}

func TestGateLeavesFileToolsAlone(t *testing.T) {
	var ran atomic.Int32
	tools := Gate([]goai.Tool{counting("write_file", &ran), counting("read_file", &ran)},
		&Session{Mode: Unattended, Scope: DefaultScope(), Workspace: ws})
	for _, tool := range tools {
		if _, err := call(tool, ""); err != nil {
			t.Fatalf("%s: %v", tool.Name, err)
		}
	}
	if ran.Load() != 2 {
		t.Fatal("file tools were held up")
	}
}

func TestEveryToolIsGatedIncludingUnknownOnes(t *testing.T) {
	var ran atomic.Int32
	tools := Gate([]goai.Tool{counting("send_email", &ran), counting("mystery_tool", &ran)},
		&Session{Mode: Unattended, Scope: DefaultScope(), Workspace: ws})
	for _, tool := range tools {
		if _, err := call(tool, ""); err == nil {
			t.Fatalf("%s ran unattended", tool.Name)
		}
	}
	if ran.Load() != 0 {
		t.Fatal("an unknown or outward-facing tool ran with nobody to ask")
	}
}
