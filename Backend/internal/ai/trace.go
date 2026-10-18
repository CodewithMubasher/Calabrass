package ai

import (
	"strings"

	"github.com/zendev-sh/goai"
	"github.com/zendev-sh/goai/provider"
)

// Trace is the ordered record of one assistant turn: the steps the model took
// and, inside each step, what it thought, said, and ran. The UI renders it
// inline, so the parts stay in the order they happened.
type Trace struct {
	Steps []TraceStep `json:"steps"`
}

// TraceStep is one model call, plus the results of the tools that call asked
// for. The number is 1-based and matches the loop's step counter.
type TraceStep struct {
	Number       int         `json:"number"`
	FinishReason string      `json:"finishReason,omitempty"`
	Parts        []TracePart `json:"parts"`
}

// TracePart is one element of a step: a stretch of reasoning, a stretch of
// narration, or one tool call.
type TracePart struct {
	// Kind is PartReasoning, PartText or PartTool.
	Kind string `json:"kind"`
	// Text is the content of a reasoning or text part.
	Text string `json:"text,omitempty"`
	// Tool call fields, set when Kind is PartTool.
	CallID string `json:"callId,omitempty"`
	Name   string `json:"name,omitempty"`
	// Server names the connector a tool came from, when it came from one. It
	// is empty for the harness's own tools. It is what lets a timeline show
	// "Excel" beside a spreadsheet glyph rather than a bare wrench.
	Server string `json:"server,omitempty"`
	// Input is the model's arguments as raw JSON.
	Input string `json:"input,omitempty"`
	// Status is StatusPending, StatusRunning, StatusOK, StatusError or
	// StatusSkipped.
	Status string `json:"status,omitempty"`
	// Output is what the tool returned, and Error is what went wrong instead.
	Output string `json:"output,omitempty"`
	Error  string `json:"error,omitempty"`
	// DurationMs is how long the call took.
	DurationMs int64 `json:"durationMs,omitempty"`
}

// Trace part kinds.
const (
	PartReasoning = "reasoning"
	PartText      = "text"
	PartTool      = "tool"
)

// Trace part statuses. A tool part is created pending, flips to running once
// the step closes and the harness starts executing it, then lands on ok, error
// or skipped with the result.
const (
	StatusPending = "pending"
	StatusRunning = "running"
	StatusOK      = "ok"
	StatusError   = "error"
	StatusSkipped = "skipped"
)

// ToolOutcome is what one tool call returned, once it has run.
type ToolOutcome struct {
	CallID     string
	Output     string
	Error      string
	Status     string
	DurationMs int64
}

// AnswerText is the model's closing message: the text of the last step that has
// any. Text in earlier steps is narration around tool calls, which the trace
// shows, so it is not part of the answer.
func (t Trace) AnswerText() string {
	for i := len(t.Steps) - 1; i >= 0; i-- {
		var b strings.Builder
		for _, part := range t.Steps[i].Parts {
			if part.Kind == PartText {
				b.WriteString(part.Text)
			}
		}
		if strings.TrimSpace(b.String()) != "" {
			return b.String()
		}
	}
	return ""
}

// Clone copies the trace deep enough that the copy can be marshalled while the
// builder keeps working.
func (t Trace) Clone() Trace {
	out := Trace{Steps: make([]TraceStep, len(t.Steps))}
	for i, step := range t.Steps {
		step.Parts = append([]TracePart(nil), step.Parts...)
		out.Steps[i] = step
	}
	return out
}

// findTool locates the tool part for one call.
func (s *TraceStep) findTool(callID string) (int, bool) {
	for i := range s.Parts {
		if s.Parts[i].Kind == PartTool && s.Parts[i].CallID == callID {
			return i, true
		}
	}
	return 0, false
}

// traceBuilder assembles a Trace from the chunk stream. Every mutation happens
// on the goroutine that consumes the stream, so no locking is needed.
type traceBuilder struct {
	trace Trace
	// open reports whether the current step has taken anything yet. The first
	// part after a tool round opens the next step instead of extending the
	// previous one.
	open bool
	// origins maps a tool name to the connector that offers it, so a tool part
	// can say which MCP server a call belongs to. It is nil when the turn had
	// no connectors.
	origins map[string]string
}

// step returns the step being written, opening a new one when the previous
// step has already closed.
func (b *traceBuilder) step() *TraceStep {
	if !b.open {
		b.trace.Steps = append(b.trace.Steps, TraceStep{Number: len(b.trace.Steps) + 1})
		b.open = true
	}
	return &b.trace.Steps[len(b.trace.Steps)-1]
}

// append adds a reasoning or text delta, extending the part already open when
// it is of the same kind.
func (b *traceBuilder) append(kind, text string) {
	if text == "" {
		return
	}
	step := b.step()
	if n := len(step.Parts); n > 0 && step.Parts[n-1].Kind == kind {
		step.Parts[n-1].Text += text
		return
	}
	step.Parts = append(step.Parts, TracePart{Kind: kind, Text: text})
}

// toolStart records a tool call the model has begun writing.
func (b *traceBuilder) toolStart(callID, name string) {
	b.toolPart(callID, name, "")
}

// toolInput records a tool call the model has finished writing.
func (b *traceBuilder) toolInput(callID, name, input string) {
	b.toolPart(callID, name, input)
}

func (b *traceBuilder) toolPart(callID, name, input string) {
	if callID == "" {
		return
	}
	step := b.step()
	if i, ok := step.findTool(callID); ok {
		if name != "" {
			step.Parts[i].Name = name
		}
		if server := b.serverOf(name); server != "" {
			step.Parts[i].Server = server
		}
		if input != "" {
			step.Parts[i].Input = input
		}
		return
	}
	step.Parts = append(step.Parts, TracePart{
		Kind:   PartTool,
		CallID: callID,
		Name:   name,
		Server: b.serverOf(name),
		Input:  input,
		Status: StatusPending,
	})
}

// serverOf names the connector a tool came from, when the turn said which. It
// is how the timeline tells an MCP server's tools apart from the harness's own.
func (b *traceBuilder) serverOf(name string) string {
	if b.origins == nil || name == "" {
		return ""
	}
	return b.origins[name]
}

// stepFinish closes the current step: the model has stopped talking and its
// tools are about to run.
func (b *traceBuilder) stepFinish(finishReason string) {
	if !b.open {
		return
	}
	step := &b.trace.Steps[len(b.trace.Steps)-1]
	if finishReason != "" {
		step.FinishReason = finishReason
	}
	for i := range step.Parts {
		if step.Parts[i].Kind == PartTool && step.Parts[i].Status == StatusPending {
			step.Parts[i].Status = StatusRunning
		}
	}
	b.open = false
}

// toolResults folds the outcome of each call back into the part that asked for
// it. Results are matched by call ID because the loop reports them for the step
// it just ran, which may not be the last one.
func (b *traceBuilder) toolResults(outcomes []ToolOutcome) {
	for _, outcome := range outcomes {
		for i := range b.trace.Steps {
			step := &b.trace.Steps[i]
			if j, ok := step.findTool(outcome.CallID); ok {
				applyOutcome(&step.Parts[j], outcome)
			}
		}
	}
}

// applyFinal fills in tool outcomes from the loop's own record of the turn, for
// any call the live stream never reported. Parts that already landed are left
// alone, so they keep the timing the hook gave them.
func (b *traceBuilder) applyFinal(steps []goai.StepResult) {
	var outcomes []ToolOutcome
	for _, step := range steps {
		for _, res := range step.ToolResults {
			outcome := ToolOutcome{CallID: res.ToolCallID, Output: res.Output}
			if res.Error != nil || res.IsError {
				outcome.Output = ""
				outcome.Error = res.Output
			}
			outcomes = append(outcomes, outcome)
		}
	}
	for _, outcome := range outcomes {
		for i := range b.trace.Steps {
			step := &b.trace.Steps[i]
			j, ok := step.findTool(outcome.CallID)
			if !ok {
				continue
			}
			switch step.Parts[j].Status {
			case StatusOK, StatusError, StatusSkipped:
				// Already reported live.
			default:
				applyOutcome(&step.Parts[j], outcome)
			}
		}
	}
}

// applyOutcome records what a tool call returned on its part.
func applyOutcome(part *TracePart, outcome ToolOutcome) {
	part.Output = outcome.Output
	part.Error = outcome.Error
	if outcome.DurationMs > 0 {
		part.DurationMs = outcome.DurationMs
	}
	switch {
	case outcome.Status != "":
		part.Status = outcome.Status
	case outcome.Error != "":
		part.Status = StatusError
	default:
		part.Status = StatusOK
	}
}

// fromContent fills an empty step from the provider's own ordered snapshot.
// Text and reasoning parts normally arrive as deltas; this covers providers
// that report the step but stream nothing, and any step whose deltas were
// dropped.
func (b *traceBuilder) fromContent(parts []provider.Part) {
	if !b.open || len(parts) == 0 {
		return
	}
	if step := &b.trace.Steps[len(b.trace.Steps)-1]; len(step.Parts) > 0 {
		return
	}
	for _, part := range parts {
		switch part.Type {
		case provider.PartReasoning:
			b.append(PartReasoning, part.Text)
		case provider.PartText:
			b.append(PartText, part.Text)
		case provider.PartToolCall:
			b.toolPart(part.ToolCallID, part.ToolName, string(part.ToolInput))
		}
	}
}

// stepSource reads the tag goai puts on its own step boundaries.
func stepSource(metadata map[string]any) string {
	if metadata == nil {
		return ""
	}
	source, _ := metadata["stepSource"].(string)
	return source
}
