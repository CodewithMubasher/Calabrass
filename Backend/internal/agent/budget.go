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
//
// The limits are enforced by the tools themselves, so they hold for every
// provider and every loop shape. A limit is a stop condition, not a pause: when
// it is reached the remaining tools are refused and the model is told to report
// what it actually completed. There is deliberately no "click continue" state,
// because nothing in the harness could resume one.
type Budget struct {
	MaxToolCalls       int // 40 in Chat mode, more in Crew
	MaxConsecutiveFail int // 3
	MaxTokensPerRun    int // 0 = no limit
}

// DefaultBudget returns sensible defaults per mode.
func DefaultBudget(mode string) Budget {
	b := Budget{
		MaxToolCalls:       40,
		MaxConsecutiveFail: 3,
		MaxTokensPerRun:    0,
	}
	if mode == ModeCrew {
		// Crew mode is where the long tasks live, so it is given the same
		// long leash the step budget gives it.
		b.MaxToolCalls = 120
	}
	return b
}

// signatureState is what one tool + argument combination has done so far. A
// signature is only compared within the current mutation epoch: once a
// state-changing tool succeeds, earlier "already did this" and "this keeps
// failing" conclusions are no longer evidence about the current state.
type signatureState struct {
	attempts  int
	failures  int
	successes int
	lastError string
	epoch     int
}

// RunState tracks the state of a single run for budget and loop enforcement.
//
// The guard is intentionally narrow. It stops a run from burning its whole
// budget on the same call, but it does not forbid retries: one repeat of a
// failed call is allowed (the failure may have been transient), and a repeat
// after a successful state change is allowed because the evidence changed.
type RunState struct {
	mu              sync.Mutex
	budget          Budget
	startTime       time.Time
	toolCalls       int
	succeededCalls  int
	failedCalls     int
	consecutiveFail int
	inFlight        map[string]int
	signatures      map[string]*signatureState
	mutationEpoch   int
	stopped         bool
	stopReason      string
	lastError       string
}

// NewRunState creates a new run state with the given budget.
func NewRunState(b Budget) *RunState {
	return &RunState{
		budget:     b,
		startTime:  time.Now(),
		inFlight:   make(map[string]int),
		signatures: make(map[string]*signatureState),
	}
}

// readOnlyTools do not change the workspace. Their identical repeats are only
// useful while nothing else has changed; a successful mutation resets that.
var readOnlyTools = map[string]bool{
	"read_file":  true,
	"list_dir":   true,
	"glob":       true,
	"grep":       true,
	"read_skill": true,
}

// readOnly reports whether a tool leaves the workspace unchanged.
func readOnly(toolName string) bool {
	return readOnlyTools[toolName]
}

// BeforeToolCall checks the budget and the loop guard before executing a tool.
// It marks an allowed call in flight, and RecordToolCall must follow every
// allowed call — including one that returns an error — to release it.
//
// Returns (allow, reason). If !allow, the tool must not run. The reason is
// written for the model: it says what happened and what to do instead.
func (rs *RunState) BeforeToolCall(toolName string, args map[string]any) (bool, string) {
	rs.mu.Lock()
	defer rs.mu.Unlock()

	if rs.stopped {
		return false, rs.stopReason
	}

	// Tool call cap. A call is counted when it is allowed, so refused calls do
	// not consume the budget.
	if rs.budget.MaxToolCalls > 0 && rs.toolCalls >= rs.budget.MaxToolCalls {
		rs.stop(fmt.Sprintf(
			"the run used its whole tool budget (%d calls) and is still incomplete. Do not start new work; report exactly what was completed, what was not, and what the user should do next.",
			rs.budget.MaxToolCalls))
		return false, rs.stopReason
	}

	argsHash := hashToolCall(toolName, args)

	// An identical call is already running in this step. Running it twice
	// would duplicate a side effect that has not finished yet.
	if rs.inFlight[argsHash] > 0 {
		return false, "an identical " + toolName + " call is already running; wait for its result instead of sending it again"
	}

	state := rs.signatures[argsHash]
	if state == nil {
		state = &signatureState{epoch: rs.mutationEpoch}
		rs.signatures[argsHash] = state
	} else if state.epoch != rs.mutationEpoch {
		// Something changed on disk since this signature was last seen. Keep
		// the attempt count for diagnostics, but the old failure/success
		// conclusions are stale.
		state.failures = 0
		state.successes = 0
		state.lastError = ""
		state.epoch = rs.mutationEpoch
	}

	// A read-only call that already answered the question is waste. Two
	// successes are allowed (a refresh after an unrelated change is cheap);
	// the third identical read in one epoch is refused.
	if readOnly(toolName) && state.successes >= 2 {
		return false, "this exact " + toolName + " call has already succeeded twice with no change in between; use the result you already have, or make a different call"
	}

	// One retry of a failed call is allowed: the failure may have been
	// transient. After two identical failures the call is not the answer.
	if state.failures >= 2 {
		reason := "this exact " + toolName + " call has already failed " +
			fmt.Sprintf("%d times", state.failures)
		if state.lastError != "" {
			reason += " (" + state.lastError + ")"
		}
		reason += "; change the arguments or the approach instead of repeating it"
		return false, reason
	}

	state.attempts++
	rs.toolCalls++
	rs.inFlight[argsHash]++
	return true, ""
}

// RecordToolCall records an allowed call and its outcome.
func (rs *RunState) RecordToolCall(toolName string, args map[string]any, err error) {
	rs.mu.Lock()
	defer rs.mu.Unlock()

	argsHash := hashToolCall(toolName, args)
	if rs.inFlight[argsHash] > 0 {
		rs.inFlight[argsHash]--
		if rs.inFlight[argsHash] == 0 {
			delete(rs.inFlight, argsHash)
		}
	}

	state := rs.signatures[argsHash]
	if state == nil {
		state = &signatureState{epoch: rs.mutationEpoch}
		rs.signatures[argsHash] = state
	}

	if err != nil {
		rs.failedCalls++
		rs.consecutiveFail++
		rs.lastError = err.Error()
		state.failures++
		state.lastError = err.Error()

		// A run that keeps failing is not making progress. Stop before it
		// spends the rest of its budget on the same wall.
		if rs.budget.MaxConsecutiveFail > 0 && rs.consecutiveFail >= rs.budget.MaxConsecutiveFail {
			rs.stop(fmt.Sprintf(
				"%d tool calls failed in a row; the last error was: %s. Stop repeating the failing operation and report the blocker with the real error.",
				rs.consecutiveFail, rs.lastError))
		}
		return
	}

	rs.succeededCalls++
	rs.consecutiveFail = 0
	state.successes++

	// A successful state-changing call makes every earlier read and every
	// earlier failure conclusion stale. This is what lets the agent read the
	// file again after editing it, or retry an operation whose precondition it
	// has just fixed.
	if !readOnly(toolName) {
		rs.mutationEpoch++
	}
}

// stop records a stop reason once, so the first cause is the one reported.
func (rs *RunState) stop(reason string) {
	if rs.stopped {
		return
	}
	rs.stopped = true
	rs.stopReason = reason
}

// StopReason reports whether the run was stopped by a limit and why. Callers
// use it to make the final answer honest about an incomplete task.
func (rs *RunState) StopReason() (string, bool) {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	return rs.stopReason, rs.stopped
}

// ToolCalls returns the current tool call count.
func (rs *RunState) ToolCalls() int {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	return rs.toolCalls
}

// FailedCalls returns how many allowed calls returned an error.
func (rs *RunState) FailedCalls() int {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	return rs.failedCalls
}

// SucceededCalls returns how many allowed calls returned without an error.
func (rs *RunState) SucceededCalls() int {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	return rs.succeededCalls
}

// ConsecutiveFail returns the current consecutive failure count.
func (rs *RunState) ConsecutiveFail() int {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	return rs.consecutiveFail
}

// SetMaxConsecutiveFail changes this run's failure cap. A worker is held to two
// failures; the ordinary modes keep the default. It is set before the run starts,
// and the wrapped tools see it because they hold this same state.
func (rs *RunState) SetMaxConsecutiveFail(n int) {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	rs.budget.MaxConsecutiveFail = n
}

// hashToolCall is a stable identity for one call: the tool's name plus a
// canonical encoding of its arguments. encoding/json sorts map keys, so the
// same arguments always produce the same bytes regardless of iteration order.
func hashToolCall(toolName string, args map[string]any) string {
	canonical, err := json.Marshal(args)
	if err != nil {
		// A value that cannot be marshalled (a function, a channel) still
		// has to produce a stable identity. Its formatted form is the best
		// available, and is only used for duplicate detection.
		canonical = []byte(fmt.Sprintf("%#v", args))
	}
	h := sha256.New()
	h.Write([]byte(toolName))
	h.Write([]byte{0})
	h.Write(canonical)
	return hex.EncodeToString(h.Sum(nil))
}

// ErrBudgetExceeded is returned when a budget or loop limit is hit.
var ErrBudgetExceeded = errors.New("budget limit reached")

// BudgetMiddleware wraps a goai.Tool to enforce budget limits and to add a
// structured explanation to tool errors.
//
// The tool's definition is copied, never rebuilt: Name, Description,
// InputSchema and the provider-defined fields are the model's contract, and
// rebuilding the tool from a map[string]any would replace that contract with an
// untyped one. Only Execute is replaced.
func BudgetMiddleware(tool goai.Tool, rs *RunState) goai.Tool {
	execute := tool.Execute
	if execute == nil {
		return tool
	}

	tool.Execute = func(ctx context.Context, input json.RawMessage) (string, error) {
		args := decodeToolArgs(input)
		allow, reason := rs.BeforeToolCall(tool.Name, args)
		if !allow {
			return "", fmt.Errorf("%w: %s", ErrBudgetExceeded, reason)
		}

		result, err := execute(ctx, input)
		rs.RecordToolCall(tool.Name, args, err)
		if err != nil {
			return "", ExplainToolError(tool.Name, err)
		}
		return result, nil
	}
	return tool
}

// decodeToolArgs reads the arguments for the loop guard's identity check. A
// malformed call is deliberately not rejected here: the tool's own typed
// decoder produces the useful error, and the guard still needs a stable
// identity for the malformed bytes.
func decodeToolArgs(input json.RawMessage) map[string]any {
	if len(input) == 0 {
		return nil
	}
	var args map[string]any
	if err := json.Unmarshal(input, &args); err != nil {
		return map[string]any{"__raw": string(input)}
	}
	return args
}
