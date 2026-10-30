// Package headless is `cal run`: one task, no screen, for scripts, pipelines
// and CI.
//
//	cal run "fix the failing test" --allow "go test *"
//	git diff | cal run "write a commit message for this"
//	cal run --json "summarise the open TODOs" > result.ndjson
//
// What it prints is for programs: the answer goes to stdout, so it pipes; what
// the agent is doing goes to stderr; and the exit code says how it ended.
//
// Nobody is there to approve anything, so a run works inside a scope written on
// the command line: it may read and edit files in the folder, and run only the
// commands named with --allow. Anything else is refused and reported, never
// guessed at, and the run exits with ExitBlocked so a pipeline can tell "the
// agent could not finish without permission" from "the agent failed".
package headless

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"calabrass-tui/internal/api"
)

// Exit codes. They are part of the interface: a pipeline branches on them.
const (
	ExitOK      = 0 // the task ran to the end
	ExitFailed  = 1 // the agent or the model failed, or ran out of steps
	ExitUsage   = 2 // the command line was wrong
	ExitBlocked = 3 // it finished what it could, but something it needed was refused
	ExitTimeout = 4 // --timeout passed first
	ExitSetup   = 5 // no host to talk to, cannot sign in, or no model is set up
)

// MaxInput is the most text taken from a pipe. A runaway `cat bigfile` should
// fail loudly, not be sent to a model provider.
const MaxInput = 512 << 10

// Options are what a run needs.
type Options struct {
	Prompt  string
	Dir     string
	Allow   []string
	JSON    bool
	Quiet   bool
	Timeout time.Duration
	Model   string // "provider/model", or empty for the default
}

// Usage is the help text for `cal run`.
const Usage = `Usage: cal run [flags] [--] <task...>

Runs one task with the agent and exits. The answer goes to stdout, progress to
stderr. Text piped in on stdin is added to the task.

Flags:
  -C dir            Work in this folder (default: the current folder)
  --allow pattern   A command the run may execute; repeatable. "npm test" allows
                    that command, "go *" allows any go command. With none, the
                    run can read and edit files in the folder and run nothing.
  --json            Print one JSON object per line instead of plain text
  -q, --quiet       No progress on stderr
  --timeout dur     Stop after this long (default 15m; 0 for no limit)
  --model p/m       Use this provider/model instead of the default

For a machine nobody sits at (CI, a container), bring the provider in the
environment: CALABRASS_PROVIDER=openai CALABRASS_API_KEY=... CALABRASS_MODEL=...

Exit codes:
  0  finished           3  finished, but something it needed was refused
  1  failed             4  timed out
  2  bad command line   5  no host, not signed in, or no model set up

Nothing run this way can reach outside the folder or act on the outside world
(push, publish, send); those need a person to approve them in the app.`

// stringList is a repeatable flag.
type stringList []string

func (s *stringList) String() string     { return strings.Join(*s, ",") }
func (s *stringList) Set(v string) error { *s = append(*s, v); return nil }

// Parse reads the arguments after "run". stdin is added to the task when it is a
// pipe (piped true) and not a terminal.
func Parse(args []string, stdin io.Reader, piped bool) (Options, error) {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	var opts Options
	var allow stringList
	fs.StringVar(&opts.Dir, "C", "", "")
	fs.Var(&allow, "allow", "")
	fs.BoolVar(&opts.JSON, "json", false, "")
	fs.BoolVar(&opts.Quiet, "q", false, "")
	fs.BoolVar(&opts.Quiet, "quiet", false, "")
	fs.DurationVar(&opts.Timeout, "timeout", 15*time.Minute, "")
	fs.StringVar(&opts.Model, "model", "", "")
	if err := fs.Parse(reorder(args)); err != nil {
		return opts, fmt.Errorf("%v\n\n%s", err, Usage)
	}
	for _, a := range allow {
		for _, part := range strings.Split(a, ",") {
			if part = strings.ToLower(strings.TrimSpace(part)); part != "" {
				opts.Allow = append(opts.Allow, part)
			}
		}
	}

	prompt := strings.TrimSpace(strings.Join(fs.Args(), " "))
	if piped && stdin != nil {
		data, err := io.ReadAll(io.LimitReader(stdin, MaxInput+1))
		if err != nil {
			return opts, fmt.Errorf("could not read stdin: %w", err)
		}
		if len(data) > MaxInput {
			return opts, fmt.Errorf("the piped input is larger than %d KB; narrow it first", MaxInput>>10)
		}
		if piece := strings.TrimSpace(string(data)); piece != "" {
			if prompt == "" {
				prompt = piece
			} else {
				prompt = prompt + "\n\n---\n" + piece
			}
		}
	}
	if prompt == "" {
		return opts, errors.New("no task given\n\n" + Usage)
	}
	opts.Prompt = prompt

	if opts.Model != "" && !strings.Contains(opts.Model, "/") {
		return opts, errors.New(`--model must look like "provider/model"`)
	}
	if opts.Timeout < 0 {
		return opts, errors.New("--timeout cannot be negative")
	}
	return opts, nil
}

// reorder moves flags in front of the task, so `cal run "do it" --allow x` works
// as well as `cal run --allow x "do it"`. Go's flag package stops at the first
// non-flag, which would turn the second form's flags into task text.
func reorder(args []string) []string {
	var flags, rest []string
	valueFlags := map[string]bool{"-C": true, "--C": true, "-allow": true, "--allow": true,
		"-timeout": true, "--timeout": true, "-model": true, "--model": true}
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			rest = append(rest, args[i+1:]...)
			break
		}
		if strings.HasPrefix(a, "-") && len(a) > 1 {
			flags = append(flags, a)
			if valueFlags[a] && !strings.Contains(a, "=") && i+1 < len(args) {
				i++
				flags = append(flags, args[i])
			}
			continue
		}
		rest = append(rest, a)
	}
	return append(flags, append([]string{"--"}, rest...)...)
}

// Result is what a finished run reports.
type Result struct {
	Exit    int
	Answer  string
	Blocked []string // actions that were refused
	Steps   int
	Err     string
}

// Run executes the task against an authenticated client.
func Run(ctx context.Context, c *api.Client, opts Options, out, errw io.Writer) Result {
	if opts.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, opts.Timeout)
		defer cancel()
	}
	say := func(format string, args ...any) {
		if !opts.Quiet && !opts.JSON {
			fmt.Fprintf(errw, format+"\n", args...)
		}
	}
	emit := func(v map[string]any) {
		if opts.JSON {
			line, _ := json.Marshal(v)
			fmt.Fprintln(out, string(line))
		}
	}

	dir := opts.Dir
	if dir == "" {
		dir, _ = os.Getwd()
	}
	if abs, err := filepath.Abs(dir); err == nil {
		dir = abs
	}

	provider, model := "", ""
	if opts.Model != "" {
		provider, model, _ = strings.Cut(opts.Model, "/")
	}
	chat, err := c.CreateChat(ctx, api.CreateChatRequest{Provider: provider, Model: model, WorkspacePath: dir})
	if err != nil {
		return finish(ctx, Result{Exit: ExitSetup, Err: "could not start a session: " + err.Error()}, opts, out, errw, emit)
	}

	scope := &api.TaskScope{MaxTier: 1}
	if len(opts.Allow) > 0 {
		scope = &api.TaskScope{MaxTier: 2, Commands: opts.Allow}
	}

	var (
		answer   strings.Builder
		res      Result
		seen     = map[string]string{} // call id → last status printed
		blocked  = map[string]bool{}
		streamed bool
	)
	sendErr := c.Send(ctx, chat.ID, api.SendRequest{
		Content: opts.Prompt, Provider: provider, Model: model, Unattended: true, Scope: scope,
	}, func(ev api.Event) {
		switch ev.Kind {
		case api.EventToken:
			answer.WriteString(ev.Text)
			if opts.JSON {
				emit(map[string]any{"type": "token", "text": ev.Text})
			} else {
				fmt.Fprint(out, ev.Text)
				streamed = true
			}
		case api.EventTrace:
			if ev.Trace == nil {
				return
			}
			for _, step := range ev.Trace.Steps {
				for _, part := range step.Parts {
					if part.Kind != api.PartTool {
						continue
					}
					key := part.CallID
					if key == "" {
						key = fmt.Sprintf("%d:%s:%s", step.Number, part.Name, part.Input)
					}
					state := part.Status
					if seen[key] == state {
						continue
					}
					seen[key] = state
					refused := strings.Contains(part.Error, "permission denied") || strings.Contains(part.Output, "permission denied")
					if refused && !blocked[key] {
						blocked[key] = true
						res.Blocked = append(res.Blocked, describeTool(part))
					}
					emit(map[string]any{"type": "tool", "name": part.Name, "input": part.Input, "status": part.Status, "refused": refused})
					if part.Status != "running" && part.Status != "" || refused {
						mark := "•"
						if refused {
							mark = "✗ refused:"
						}
						say("%s %s", mark, describeTool(part))
					}
				}
			}
		case api.EventDone:
			if ev.Done != nil {
				res.Steps = ev.Done.Steps
				if ev.Done.StepsExhausted {
					res.Err = "the agent ran out of steps before it finished"
				} else if ev.Done.Interrupted && res.Err == "" {
					res.Err = "the run was interrupted"
				}
			}
		case api.EventError:
			res.Err = ev.Text
		}
	})

	res.Answer = answer.String()
	if streamed && !strings.HasSuffix(res.Answer, "\n") {
		fmt.Fprintln(out)
	}

	switch {
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		res.Exit, res.Err = ExitTimeout, fmt.Sprintf("timed out after %s", opts.Timeout)
	case errors.Is(ctx.Err(), context.Canceled):
		res.Exit, res.Err = ExitFailed, "interrupted"
	case sendErr != nil && res.Err == "":
		res.Exit, res.Err = ExitSetup, sendErr.Error()
	case strings.Contains(res.Err, "no model is available"):
		res.Exit = ExitSetup
		res.Err = "no model is set up. Open Calabrass once (the desktop app, or `cal`, then /connect) and add a provider key."
	case res.Err != "":
		res.Exit = ExitFailed
	case len(res.Blocked) > 0:
		res.Exit = ExitBlocked
	default:
		res.Exit = ExitOK
	}
	return finish(ctx, res, opts, out, errw, emit)
}

func finish(_ context.Context, res Result, opts Options, out, errw io.Writer, emit func(map[string]any)) Result {
	if len(res.Blocked) > 0 {
		if !opts.Quiet && !opts.JSON {
			fmt.Fprintf(errw, "\n%d action(s) were refused because they were not pre-approved:\n", len(res.Blocked))
			for _, b := range res.Blocked {
				fmt.Fprintf(errw, "  - %s\n", b)
			}
			fmt.Fprintln(errw, "Re-run with --allow for the commands you trust, for example: --allow \"npm test\"")
		}
	}
	if res.Err != "" && !opts.JSON {
		fmt.Fprintln(errw, "calabrass:", res.Err)
	}
	emit(map[string]any{
		"type": "result", "ok": res.Exit == ExitOK, "exit": res.Exit, "answer": res.Answer,
		"blocked": res.Blocked, "steps": res.Steps, "error": res.Err,
	})
	return res
}

func describeTool(p api.TracePart) string {
	input := strings.TrimSpace(p.Input)
	if input != "" {
		var args map[string]any
		if json.Unmarshal([]byte(input), &args) == nil {
			for _, key := range []string{"command", "path", "pattern", "query"} {
				if v, ok := args[key].(string); ok && v != "" {
					input = v
					break
				}
			}
		}
	}
	if len(input) > 100 {
		input = input[:100] + "…"
	}
	if input == "" {
		return p.Name
	}
	return p.Name + ": " + input
}
