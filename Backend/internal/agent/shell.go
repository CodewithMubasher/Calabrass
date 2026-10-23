package agent

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/CodewithMubasher/Calabrass/backend/internal/proc"
	"github.com/zendev-sh/goai"
)

// --- run_command -------------------------------------------------------------

const (
	// shellTimeout bounds a command that does not ask for a budget of its own.
	shellTimeout = 120 * time.Second
	// shellMaxTimeout is the most a command may ask for.
	shellMaxTimeout = 900 * time.Second
	// shellOutputBytes caps what one command returns (total JSON size budget).
	shellOutputBytes = 32 << 10
	// outputHeadBytes is how many bytes to keep from the start of output.
	outputHeadBytes = 2 << 10
	// outputTailBytes is how many bytes to keep from the end of output. Build
	// and test failures live at the end, so the tail is the larger half.
	outputTailBytes = 8 << 10
)

// runCommandArgs is the wire shape of run_command: the JSON Schema is generated
// from these tags.
type runCommandArgs struct {
	Command        string `json:"command" jsonschema:"description=The command line to run; it runs with this machine's shell (PowerShell on Windows, sh elsewhere)"`
	Cwd            string `json:"cwd,omitempty" jsonschema:"description=Working directory relative to workspace root (e.g., 'Frontend/'). Defaults to workspace root. Must not escape the workspace."`
	TimeoutSeconds int    `json:"timeout_seconds,omitempty" jsonschema:"description=Seconds to allow before the command is killed; defaults to 120 and at most 900"`
}

// shellToolResult is the structured result returned to the model.
type shellToolResult struct {
	ExitCode   int    `json:"exit_code"`
	Cwd        string `json:"cwd"`
	DurationMs int64  `json:"duration_ms"`
	TimedOut   bool   `json:"timed_out,omitempty"`
	Output     string `json:"output"`
}

func shellTool(ws *Workspace) goai.Tool {
	return goai.NewTool("run_command",
		"Run one command line in the project directory and return a structured result. "+
			"Use it to build, test, install and inspect the project with tools of your own; "+
			"for reading code prefer read_file, glob and grep, which cost far less. "+
			"Set cwd to the project folder that owns the config file (e.g., 'Frontend/' for package.json). "+
			"A command that fails is not a failed call: its output, exit code, and cwd come back for you to read. "+
			"Commands may change state; do not run two that touch the same files in parallel.",
		func(ctx context.Context, args runCommandArgs) (string, error) {
			return ws.runCommand(ctx, args)
		})
}

// runCommand runs one command line in the workspace and returns a structured result.
func (w *Workspace) runCommand(ctx context.Context, args runCommandArgs) (string, error) {
	command := strings.TrimSpace(args.Command)
	if command == "" {
		return "", errors.New("no command to run")
	}

	timeout := shellTimeout
	if args.TimeoutSeconds > 0 {
		timeout = time.Duration(args.TimeoutSeconds) * time.Second
		if timeout > shellMaxTimeout {
			timeout = shellMaxTimeout
		}
	}

	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	name, argv := shellFor(command)
	cmd := exec.CommandContext(runCtx, name, argv...)

	// Resolve cwd: default to workspace root, validate it's inside workspace
	cwd := w.Root()
	if args.Cwd != "" {
		requested := filepath.Join(w.Root(), args.Cwd)
		absReq, err := filepath.Abs(requested)
		if err != nil {
			return "", fmt.Errorf("invalid cwd: %w", err)
		}
		absRoot, err := filepath.Abs(w.Root())
		if err != nil {
			return "", fmt.Errorf("workspace root: %w", err)
		}
		// Ensure requested cwd is inside workspace root
		rel, err := filepath.Rel(absRoot, absReq)
		if err != nil || strings.HasPrefix(rel, "..") {
			return "", errors.New("cwd must be inside the workspace root")
		}
		cwd = absReq
	}
	cmd.Dir = cwd

	// Force non-interactive, no progress, no profile
	env := os.Environ()
	env = append(env,
		"CI=1",
		"POWERSHELL_TELEMETRY_OPTOUT=1",
		"NO_COLOR=1",
		"TERM=dumb",
	)
	cmd.Env = env

	proc.Hide(cmd)

	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	started := time.Now()
	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("could not start %s: %w", name, err)
	}
	waitErr := cmd.Wait()
	durationMs := time.Since(started).Milliseconds()

	// The turn was abandoned: a command killed along with it is not worth reporting.
	if ctx.Err() != nil {
		return "", ctx.Err()
	}

	exitCode := 0
	switch {
	case runCtx.Err() != nil:
		exitCode = -1 // timeout or cancellation
	case waitErr != nil:
		var exit *exec.ExitError
		if errors.As(waitErr, &exit) {
			exitCode = exit.ExitCode()
		} else {
			exitCode = -1
		}
	}

	rawOutput := out.String()
	cleanOutput := stripAnsiAndClixml(rawOutput)
	trimmedOutput := trimOutput(cleanOutput)

	timedOut := runCtx.Err() == context.DeadlineExceeded
	if timedOut {
		// The command may have done part of its work before it was killed.
		// Saying so is what stops the model from assuming a clean failure.
		trimmedOutput += "\n[Hint] The command hit its timeout and was stopped. It may have completed part of its work; inspect the current state before retrying, or raise timeout_seconds."
	}

	// Hint injection for known error patterns
	hint := detectHint(trimmedOutput, cwd, w.Root())
	if hint != "" {
		trimmedOutput = trimmedOutput + "\n\n[Hint] " + hint
	}

	result := shellToolResult{
		ExitCode:   exitCode,
		Cwd:        cwd,
		DurationMs: durationMs,
		TimedOut:   timedOut,
		Output:     trimmedOutput,
	}

	jsonResult, err := json.Marshal(result)
	if err != nil {
		return "", fmt.Errorf("marshal result: %w", err)
	}
	return string(jsonResult), nil
}

// stripAnsiAndClixml removes ANSI escape sequences and PowerShell CLIXML noise.
func stripAnsiAndClixml(s string) string {
	// Remove ANSI escape sequences (CSI sequences)
	ansiRe := regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)
	s = ansiRe.ReplaceAllString(s, "")

	// Remove OSC sequences (e.g., title changes)
	oscRe := regexp.MustCompile(`\x1b\][^\x07]*\x07`)
	s = oscRe.ReplaceAllString(s, "")

	// Remove PowerShell CLIXML fragments (#< CLIXML ...)
	clixmlRe := regexp.MustCompile(`#< CLIXML[^>]*>`)
	s = clixmlRe.ReplaceAllString(s, "")

	// Remove standalone #< CLIXML lines
	lines := strings.Split(s, "\n")
	filtered := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#< CLIXML") {
			continue
		}
		filtered = append(filtered, line)
	}
	s = strings.Join(filtered, "\n")

	return s
}

// trimOutput keeps head + tail of output, drops the middle.
func trimOutput(s string) string {
	if len(s) <= outputHeadBytes+outputTailBytes {
		return strings.TrimRight(s, " \t\r\n")
	}

	head := runeBoundary(s, outputHeadBytes)
	tailStart := len(s) - outputTailBytes
	for tailStart < len(s) && !utf8.RuneStart(s[tailStart]) {
		tailStart++
	}

	return strings.TrimRight(s[:head], " \t\r\n") +
		"\n… [" + fmt.Sprintf("%d bytes omitted", tailStart-head) + "] …\n" +
		strings.TrimRight(s[tailStart:], " \t\r\n")
}

// detectHint returns a helpful hint for known error patterns, or empty string.
func detectHint(output, cwd, workspaceRoot string) string {
	lower := strings.ToLower(output)

	// Missing components.json (shadcn/ui)
	if strings.Contains(lower, "components.json") && strings.Contains(lower, "not found") {
		// Find Frontend/ relative to workspace
		rel, _ := filepath.Rel(workspaceRoot, cwd)
		if rel == "." || rel == "" {
			return "components.json not found. Run from the project folder containing components.json (e.g., Frontend/)."
		}
	}

	// pnpm not found / lockfile mismatch
	if strings.Contains(lower, "pnpm") && (strings.Contains(lower, "not found") || strings.Contains(lower, "command not found")) {
		return "pnpm not found in PATH. Ensure pnpm is installed or use the package manager indicated by the lockfile."
	}

	// npm/pnpm/yarn lockfile mismatch
	if strings.Contains(lower, "lockfile") && strings.Contains(lower, "mismatch") {
		return "Lockfile mismatch. Regenerate lockfile with your package manager (pnpm install / npm install / yarn install)."
	}

	// Go module not found
	if strings.Contains(lower, "go.mod") && strings.Contains(lower, "not found") {
		return "go.mod not found. Run from the module root (the folder containing go.mod)."
	}

	// Permission denied
	if strings.Contains(lower, "permission denied") || strings.Contains(lower, "access is denied") {
		return "Permission denied. Check file permissions or run the command from an allowed directory."
	}

	// Command not found
	if strings.Contains(lower, "command not found") || strings.Contains(lower, "is not recognized") {
		return "Command not found in PATH. Ensure the tool is installed and available."
	}

	return ""
}

// shellFor is the command line that runs command on this machine: PowerShell on
// Windows, /bin/sh anywhere else.
func shellFor(command string) (string, []string) {
	if runtime.GOOS != "windows" {
		return "/bin/sh", []string{"-c", command}
	}

	// Output is read back as bytes and shown as UTF-8, so the shell is told to
	// write it that way; without this a non-ASCII path comes back mangled.
	script := "$OutputEncoding = [Console]::OutputEncoding = [Text.Encoding]::UTF8; " +
		"$ProgressPreference = 'SilentlyContinue'; " +
		command

	return "powershell.exe", []string{
		"-NoProfile", "-NonInteractive", "-EncodedCommand", encodeScript(script),
	}
}

// encodeScript is a script as PowerShell's -EncodedCommand wants it: base64 over
// UTF-16LE, which is the encoding PowerShell's own parser reads.
func encodeScript(script string) string {
	units := utf16.Encode([]rune(script))
	raw := make([]byte, 0, len(units)*2)
	for _, unit := range units {
		raw = append(raw, byte(unit), byte(unit>>8))
	}
	return base64.StdEncoding.EncodeToString(raw)
}

// runeBoundary is cut moved back to the start of a rune.
func runeBoundary(text string, cut int) int {
	if cut <= 0 {
		return 0
	}
	if cut >= len(text) {
		return len(text)
	}
	for cut > 0 && !utf8.RuneStart(text[cut]) {
		cut--
	}
	return cut
}

// elideMiddle is kept for backwards compatibility with tests.
// It delegates to trimOutput with the old shellOutputBytes cap.
func elideMiddle(text string, max int) string {
	if len(text) <= max {
		return text
	}
	// Use similar logic but with the provided max
	head := runeBoundary(text, max/3)
	start := len(text) - (max - max/3)
	for start < len(text) && !utf8.RuneStart(text[start]) {
		start++
	}
	return text[:head] +
		fmt.Sprintf("\n… [%d bytes of output left out] …\n", start-head) +
		text[start:]
}
