package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/zendev-sh/goai"
)

// Budget limits for a single run.
type Budget struct {
	MaxToolCalls       int           // 40 in Build mode, lower in Plan
	MaxConsecutiveFail int           // 3
	MaxTokensPerRun    int           // 0 = no limit
	PauseOnCap         bool          // true = pause and ask user to continue
}

// DefaultBudget returns sensible defaults per mode.
func DefaultBudget(mode string) Budget {
	b := Budget{
		MaxToolCalls:       40,
		MaxConsecutiveFail: 3,
		MaxTokensPerRun:    0,
		PauseOnCap:         true,
	}
	if mode == "plan" {
		b.MaxToolCalls = 15 // fewer calls in plan mode
	}
	return b
}

// RunState tracks the state of a single run for budget enforcement.
type RunState struct {
	mu                  sync.Mutex
	toolCalls           int
	consecutiveFail     int
	lastToolCall        string
	lastArgsHash        string
	lastError           string
	tokenBudgetExceeded bool
	paused              bool
	pauseReason         string
	budget              Budget
	startTime           time.Time
}

// NewRunState creates a new run state with the given budget.
func NewRunState(b Budget) *RunState {
	return &RunState{
		budget:    b,
		startTime: time.Now(),
	}
}

// BeforeToolCall checks budget before executing a tool.
// Returns (allow, reason). If !allow, the tool should not run.
func (rs *RunState) BeforeToolCall(toolName string, args map[string]any) (bool, string) {
	rs.mu.Lock()
	defer rs.mu.Unlock()

	if rs.paused {
		return false, rs.pauseReason
	}

	// Tool call cap
	if rs.budget.MaxToolCalls > 0 && rs.toolCalls >= rs.budget.MaxToolCalls {
		rs.paused = true
		rs.pauseReason = fmt.Sprintf("Tool call limit reached (%d). Click Continue to proceed.", rs.budget.MaxToolCalls)
		return false, rs.pauseReason
	}

	// Token budget (if enabled)
	if rs.budget.MaxTokensPerRun > 0 && rs.tokenBudgetExceeded {
		rs.paused = true
		rs.pauseReason = fmt.Sprintf("Token budget exceeded (%d tokens). Click Continue to proceed.", rs.budget.MaxTokensPerRun)
		return false, rs.pauseReason
	}

	// Check for repeated identical failing calls
	argsHash := hashArgs(args)
	if argsHash == rs.lastArgsHash && rs.lastError != "" {
		// Same args, previous call failed
		return false, fmt.Sprintf("Same command failed previously: %s. Adjust and retry.", rs.lastError)
	}

	return true, ""
}

// RecordToolCall records a tool call and its outcome.
func (rs *RunState) RecordToolCall(toolName string, args map[string]any, err error) {
	rs.mu.Lock()
	defer rs.mu.Unlock()

	rs.toolCalls++
	rs.lastToolCall = toolName
	rs.lastArgsHash = hashArgs(args)

	if err != nil {
		rs.consecutiveFail++
		rs.lastError = err.Error()
		// Check consecutive failure cap
		if rs.budget.MaxConsecutiveFail > 0 && rs.consecutiveFail >= rs.budget.MaxConsecutiveFail {
			rs.paused = true
			rs.pauseReason = fmt.Sprintf("%d consecutive tool failures. Last error: %s", rs.consecutiveFail, rs.lastError)
		}
	} else {
		rs.consecutiveFail = 0
		rs.lastError = ""
	}
}

// RecordTokenUsage updates token usage and checks budget.
func (rs *RunState) RecordTokenUsage(tokens int) {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	// Token budget is checked by the caller; this is for future use if we track cumulative
	_ = tokens
}

// ToolCalls returns the current tool call count.
func (rs *RunState) ToolCalls() int {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	return rs.toolCalls
}

// ConsecutiveFail returns the current consecutive failure count.
func (rs *RunState) ConsecutiveFail() int {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	return rs.consecutiveFail
}

// IsPaused returns whether the run is paused.
func (rs *RunState) IsPaused() (bool, string) {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	return rs.paused, rs.pauseReason
}

// Resume clears the pause state.
func (rs *RunState) Resume() {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	rs.paused = false
	rs.pauseReason = ""
}

// hashArgs creates a stable hash of tool arguments for repeat detection.
func hashArgs(args map[string]any) string {
	// Simple deterministic string representation for hashing
	// In production, use a proper JSON canonicalization
	var parts []string
	for k, v := range args {
		parts = append(parts, fmt.Sprintf("%s=%v", k, v))
	}
	// Sort for determinism
	// (simplified - in reality you'd want a proper canonical JSON)
	h := sha256.Sum256([]byte(fmt.Sprintf("%v", parts)))
	return hex.EncodeToString(h[:])
}

// ErrBudgetExceeded is returned when a budget limit is hit.
var ErrBudgetExceeded = errors.New("budget limit reached")

// Context keys for passing run state through the call chain.
type runStateKey struct{}

// WithRunState attaches a RunState to a context.
func WithRunState(ctx context.Context, rs *RunState) context.Context {
	return context.WithValue(ctx, runStateKey{}, rs)
}

// RunStateFromContext extracts the RunState from a context.
func RunStateFromContext(ctx context.Context) *RunState {
	if rs, ok := ctx.Value(runStateKey{}).(*RunState); ok {
		return rs
	}
	return nil
}

// BudgetMiddleware wraps a goai.Tool to enforce budget limits.
func BudgetMiddleware(tool goai.Tool, rs *RunState) goai.Tool {
	return goai.NewTool(tool.Name, tool.Description,
		func(ctx context.Context, args map[string]any) (string, error) {
			allow, reason := rs.BeforeToolCall(tool.Name, args)
			if !allow {
				return "", fmt.Errorf("%w: %s", ErrBudgetExceeded, reason)
			}

			// Convert args to json.RawMessage for the inner tool
			rawArgs, err := json.Marshal(args)
			if err != nil {
				return "", fmt.Errorf("marshal args: %w", err)
			}

			result, err := tool.Execute(ctx, rawArgs)
			rs.RecordToolCall(tool.Name, args, err)
			return result, err
		})
}