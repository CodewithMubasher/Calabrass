package agent

import (
	"context"

	"github.com/zendev-sh/goai"
)

// DelegateArgs is the wire shape of delegate: the one tool the voice agent has.
type DelegateArgs struct {
	Task    string `json:"task" jsonschema:"description=The work to hand over, written the way the user asked for it"`
	Context string `json:"context,omitempty" jsonschema:"description=Anything already known that the worker should not have to rediscover"`
}

// Delegate runs one piece of work with the full chat agent: every tool, the big
// model, the whole project prompt.
type Delegate func(ctx context.Context, args DelegateArgs) (string, error)

// DelegateTool is the single tool voice mode offers. Keeping the voice agent down
// to this one tool is what keeps its prompt tiny and its answers quick — it
// decides, and the coding agent does.
func DelegateTool(run Delegate) goai.Tool {
	return goai.NewTool("delegate",
		"Hand a task to the coding agent. It has every tool and the full project context, and it will do the work and report back what it did and what changed. Call this for anything that needs files, commands, research, or something opened or played on the user's own computer.",
		func(ctx context.Context, args DelegateArgs) (string, error) {
			return run(ctx, args)
		})
}

// VoiceSystemPrompt is deliberately tiny. The voice agent answers briefly and
// hands real work to delegate, so it needs almost no instructions — which is
// what keeps a spoken answer fast.
const VoiceSystemPrompt = "You are Cal, a voice assistant. Answer in one or two short sentences, " +
	"the way you would say them out loud. If the user asks you to do work — read or change files, run " +
	"anything, look something up in the project, or open or play something on their computer — call the " +
	"delegate tool with the task instead of doing it yourself, then say briefly what was done. " +
	"Never list files or commands."
