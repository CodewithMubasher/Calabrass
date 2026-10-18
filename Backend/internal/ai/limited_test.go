package ai

import (
	"context"
	"encoding/json"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zendev-sh/goai"
)

// TestLimitedToolsCapsConcurrency is the promise of parallel tool calling: a
// step's calls run together, but never more than maxParallelTools at once.
func TestLimitedToolsCapsConcurrency(t *testing.T) {
	const count = 20

	var (
		running int64
		peak    int64
	)
	release := make(chan struct{})

	built := make([]goai.Tool, count)
	for i := range built {
		built[i] = goai.Tool{
			Name: "tool" + strconv.Itoa(i),
			Execute: func(ctx context.Context, input json.RawMessage) (string, error) {
				now := atomic.AddInt64(&running, 1)
				for {
					seen := atomic.LoadInt64(&peak)
					if now <= seen || atomic.CompareAndSwapInt64(&peak, seen, now) {
						break
					}
				}
				<-release // hold the slot until the test lets go
				atomic.AddInt64(&running, -1)
				return string(input), nil
			},
		}
	}

	limited := limitedTools(built, maxParallelTools)
	if len(limited) != count {
		t.Fatalf("got %d tools, want %d", len(limited), count)
	}

	var wg sync.WaitGroup
	for _, tool := range limited {
		wg.Add(1)
		go func(tool goai.Tool) {
			defer wg.Done()
			out, err := tool.Execute(context.Background(), json.RawMessage(`{"x":1}`))
			if err != nil {
				t.Errorf("%s: %v", tool.Name, err)
			}
			if out != `{"x":1}` {
				t.Errorf("%s returned %q, want the input echoed", tool.Name, out)
			}
		}(tool)
	}

	// Wait for the pool to fill. Exactly maxParallelTools calls can hold a slot
	// at once; if the cap were missing, peak would climb past it.
	deadline := time.Now().Add(5 * time.Second)
	for atomic.LoadInt64(&peak) < maxParallelTools && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}

	got := atomic.LoadInt64(&peak)
	if got > maxParallelTools {
		t.Errorf("peak concurrency = %d, want at most %d", got, maxParallelTools)
	}
	if got < 2 {
		t.Errorf("peak concurrency = %d: the calls did not run in parallel", got)
	}

	close(release)
	wg.Wait()

	if left := atomic.LoadInt64(&running); left != 0 {
		t.Errorf("%d calls still running after they were released", left)
	}
}

// TestLimitedToolsLeavesTheDefinitionAlone: the tool slice is shared between
// turns, so wrapping one turn's copy must not change the next turn's tools.
func TestLimitedToolsLeavesTheDefinitionAlone(t *testing.T) {
	calls := 0
	original := goai.Tool{
		Name:    "read_file",
		Execute: func(ctx context.Context, input json.RawMessage) (string, error) { calls++; return "ok", nil },
	}
	built := []goai.Tool{original}

	limited := limitedTools(built, maxParallelTools)
	if _, err := limited[0].Execute(context.Background(), nil); err != nil {
		t.Fatalf("limited execute: %v", err)
	}
	if calls != 1 {
		t.Fatalf("wrapped tool ran %d times, want 1", calls)
	}

	// The original still points at the unwrapped function, and wrapping it a
	// second time does not stack the semaphore on the first.
	again := limitedTools(built, maxParallelTools)
	if _, err := again[0].Execute(context.Background(), nil); err != nil {
		t.Fatalf("second limited execute: %v", err)
	}
	if calls != 2 {
		t.Errorf("original tool ran %d times after re-wrapping, want 2", calls)
	}
	if built[0].Execute == nil {
		t.Error("the original tool lost its Execute")
	}
}

// TestLimitedToolsKeepsANilExecute: a tool that cannot run is left as it is
// rather than wrapped around a nil call.
func TestLimitedToolsKeepsANilExecute(t *testing.T) {
	limited := limitedTools([]goai.Tool{{Name: "server_side"}}, maxParallelTools)
	if len(limited) != 1 || limited[0].Execute != nil {
		t.Errorf("a tool with no Execute should pass through untouched, got %#v", limited)
	}
}
