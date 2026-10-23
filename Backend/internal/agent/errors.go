package agent

import (
	"context"
	"errors"
	"os"
	"strings"
)

// ToolErrorKind is the category of a tool failure. It exists so the model gets
// a recovery instruction that fits the failure, instead of a bare error string
// that invites the same call again.
type ToolErrorKind string

const (
	// ErrInvalidArguments means the call did not match the tool's schema.
	ErrInvalidArguments ToolErrorKind = "invalid_arguments"
	// ErrMissingPath means the file or directory does not exist.
	ErrMissingPath ToolErrorKind = "missing_path"
	// ErrStaleContent means the file changed since the model read it.
	ErrStaleContent ToolErrorKind = "stale_content"
	// ErrEditMismatch means the text an edit named is not in the file.
	ErrEditMismatch ToolErrorKind = "edit_mismatch"
	// ErrAlreadyApplied means the requested change appears to be in place.
	ErrAlreadyApplied ToolErrorKind = "already_applied"
	// ErrPermission means the OS or the workspace refused the operation.
	ErrPermission ToolErrorKind = "permission_denied"
	// ErrTimeout means the operation was stopped by its own deadline.
	ErrTimeout ToolErrorKind = "timeout"
	// ErrCancelled means the turn was cancelled.
	ErrCancelled ToolErrorKind = "cancelled"
	// ErrInternal means the harness or tool has a bug, not the call.
	ErrInternal ToolErrorKind = "internal_error"
	// ErrUnknown is a failure no rule recognised.
	ErrUnknown ToolErrorKind = "unknown_error"
)

// ToolError is a tool failure with a category and a recovery hint. Its Error
// string is what the model reads, so the hint is part of the message rather
// than hidden metadata.
type ToolError struct {
	Kind    ToolErrorKind
	Tool    string
	Message string
	Hint    string
	Err     error
}

func (e *ToolError) Error() string {
	var b strings.Builder
	b.WriteString(string(e.Kind))
	if e.Tool != "" {
		b.WriteString(" (")
		b.WriteString(e.Tool)
		b.WriteString(")")
	}
	b.WriteString(": ")
	b.WriteString(strings.TrimSpace(e.Message))
	if e.Hint != "" {
		b.WriteString("\nHint: ")
		b.WriteString(e.Hint)
	}
	return b.String()
}

func (e *ToolError) Unwrap() error { return e.Err }

// ExplainToolError classifies err and returns a *ToolError that tells the model
// what kind of failure it hit and what to do next. An error that is already
// classified is returned unchanged.
func ExplainToolError(toolName string, err error) error {
	if err == nil {
		return nil
	}
	var classified *ToolError
	if errors.As(err, &classified) {
		return err
	}

	kind, hint := classifyToolError(err)
	return &ToolError{
		Kind:    kind,
		Tool:    toolName,
		Message: err.Error(),
		Hint:    hint,
		Err:     err,
	}
}

// classifyToolError maps an error's text onto a category and a hint. The tools
// write specific, human-readable errors, and those messages are the most
// reliable signal available without changing every call site.
func classifyToolError(err error) (ToolErrorKind, string) {
	switch {
	case errors.Is(err, context.Canceled):
		return ErrCancelled, "The turn was cancelled. Stop, and report what was already completed; do not restart the task automatically."
	case errors.Is(err, context.DeadlineExceeded):
		return ErrTimeout, "The operation hit its deadline. It may have changed something before it was stopped: inspect the current state before deciding whether to retry, and use a longer timeout or a smaller command if a retry is correct."
	case errors.Is(err, os.ErrPermission):
		return ErrPermission, "The operation was refused by the filesystem or policy. Do not work around permissions; ask the user for access, or report the blocker."
	case errors.Is(err, os.ErrNotExist):
		return ErrMissingPath, "The path does not exist. Find the real path with glob or list_dir, then retry with that path."
	}

	text := strings.ToLower(err.Error())

	switch {
	case containsAny(text, "already applied", "already exists", "already been applied", "leave the file unchanged"):
		return ErrAlreadyApplied, "The requested change appears to be in place. Read the file and stop editing if the goal is satisfied; do not apply it a second time."
	case containsAny(text, "is required", "must be", "cannot be", "invalid", "bad pattern", "no edits", "over the limit", "not a ", "unknown", "unsupported", "unmarshal", "cannot unmarshal", "json:"):
		return ErrInvalidArguments, "The call's arguments were rejected. Re-read the tool's parameters, fix the one that is wrong, and send a corrected call. Do not resend the same arguments."
	case containsAny(text, "no such file", "no such directory", "does not exist", "not found", "could not be found"):
		return ErrMissingPath, "The path does not exist. Find the real path with glob or list_dir, then retry with that path."
	case containsAny(text, "is not in", "not in the file", "oldstring", "line ", "read the file again", "appears", "not \""):
		return ErrStaleContent, "The file is not the version you read. Read it again, then reconstruct the edit from the current lines. Do not repeat the stale edit."
	case containsAny(text, "permission denied", "access is denied", "operation not permitted"):
		return ErrPermission, "The operation was refused by the filesystem or policy. Do not work around permissions; ask the user for access, or report the blocker."
	case containsAny(text, "timed out", "timeout", "deadline exceeded"):
		return ErrTimeout, "The operation hit its deadline. It may have changed something before it was stopped: inspect the current state before deciding whether to retry."
	case containsAny(text, "context canceled", "cancelled", "canceled"):
		return ErrCancelled, "The turn was cancelled. Stop, and report what was already completed; do not restart the task automatically."
	case containsAny(text, "internal", "panic", "nil pointer", "marshal", "could not be opened"):
		return ErrInternal, "This looks like a harness or tool bug rather than a mistake in the call. Report it with the exact message, and use a different tool or a narrower operation."
	}
	return ErrUnknown, "Read the error and change the call or approach. Do not repeat the identical call; if the cause is not recoverable, stop and report the blocker."
}

func containsAny(text string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(text, needle) {
			return true
		}
	}
	return false
}

// ErrAlreadyAppliedTool is a convenience error for an edit whose replacement is
// already present. It carries the category so the classifier leaves it alone.
func ErrAlreadyAppliedTool(message string) error {
	return &ToolError{
		Kind:    ErrAlreadyApplied,
		Message: message,
		Hint:    "The goal may already be satisfied. Read the file, confirm the change, and stop editing instead of applying it twice.",
	}
}
