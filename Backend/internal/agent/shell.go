package agent

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os/exec"
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
	// shellTimeout bounds a command that does not ask for a budget of its own. A
	// build or a test run that hangs must not hold the turn open for ever.
	shellTimeout = 120 * time.Second
	// shellMaxTimeout is the most a command may ask for.
	shellMaxTimeout = 900 * time.Second
	// shellOutputBytes caps what one command returns, so a chatty build cannot
	// fill the context by itself.
	shellOutputBytes = 32 << 10
)

// runCommandArgs is the wire shape of run_command: the JSON Schema is generated
// from these tags.
type runCommandArgs struct {
	Command        string `json:"command" jsonschema:"description=The command line to run; this machine's shell is PowerShell"`
	TimeoutSeconds int    `json:"timeout_seconds,omitempty" jsonschema:"description=Seconds to allow before the command is killed; defaults to 120 and at most 900"`
}

func shellTool(ws *Workspace) goai.Tool {
	return goai.NewTool("run_command",
		"Run one command line in the project directory and return what it printed. "+
			"Use it to build, test, install and inspect the project with tools of your own; "+
			"for reading code prefer read_file, glob and grep, which cost far less. "+
			"A command that fails is not a failed call: its output and exit code come back for you to read.",
		func(ctx context.Context, args runCommandArgs) (string, error) {
			return ws.runCommand(ctx, args)
		})
}

// runCommand runs one command line in the workspace and reports what it printed
// and how it ended.
//
// The file tools are confined to the project by resolving every path against it.
// A command line cannot be confined that way, because what it starts inherits
// the right to go wherever the user can: the command runs on their machine, with
// their account, and this program does not pretend otherwise. What is enforced
// here is where it starts and how long it may take.
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
	cmd.Dir = w.Root()
	// The shell runs without a console window of its own: the desktop app has no
	// console to lend it, and a terminal flashing over the app for every command
	// the agent runs is not something the user asked to watch.
	proc.Hide(cmd)

	// One buffer for both streams, so the report reads in the order the command
	// wrote it rather than as two piles to be compared.
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("could not start %s: %w", name, err)
	}
	waitErr := cmd.Wait()

	// The turn was abandoned: a command killed along with it is not worth
	// reporting, and the answer it belonged to is on its way out.
	if ctx.Err() != nil {
		return "", ctx.Err()
	}

	status := "exit code 0"
	switch {
	case runCtx.Err() != nil:
		status = "killed after " + timeout.String()
	case waitErr != nil:
		var exit *exec.ExitError
		if errors.As(waitErr, &exit) {
			status = fmt.Sprintf("exit code %d", exit.ExitCode())
		} else {
			status = waitErr.Error()
		}
	}

	return reportCommand(out.String(), status), nil
}

// reportCommand is the text the model reads: what the command printed, and how
// it ended. The ending is always stated, because a command that printed nothing
// and succeeded looks exactly like one that printed nothing and failed.
func reportCommand(output, status string) string {
	var b strings.Builder

	if trimmed := strings.TrimRight(output, " \t\r\n"); trimmed == "" {
		b.WriteString("(no output)\n")
	} else {
		b.WriteString(elideMiddle(trimmed, shellOutputBytes))
		b.WriteString("\n")
	}

	b.WriteString("[" + status + "]")
	return b.String()
}

// shellFor is the command line that runs command on this machine: PowerShell on
// Windows, /bin/sh anywhere else.
func shellFor(command string) (string, []string) {
	if runtime.GOOS != "windows" {
		return "/bin/sh", []string{"-c", command}
	}

	// Output is read back as bytes and shown as UTF-8, so the shell is told to
	// write it that way; without this a non-ASCII path comes back mangled.
	script := "$OutputEncoding = [Console]::OutputEncoding = [Text.Encoding]::UTF8; " + command

	// The script goes over encoded rather than as an argument: a command line is
	// its own little language, and the quoting the shell would apply to its own
	// source is not the quoting this program would use to hand it across.
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

// elideMiddle shortens long output by dropping its middle, keeping the start,
// where a command says what it is doing, and the end, where it says how it went.
// Both cuts land on a rune boundary, so the report stays valid UTF-8.
func elideMiddle(text string, max int) string {
	if len(text) <= max {
		return text
	}

	head := runeBoundary(text, max/3)
	start := len(text) - (max - max/3)
	for start < len(text) && !utf8.RuneStart(text[start]) {
		start++
	}

	return text[:head] +
		fmt.Sprintf("\n… [%d bytes of output left out] …\n", start-head) +
		text[start:]
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
