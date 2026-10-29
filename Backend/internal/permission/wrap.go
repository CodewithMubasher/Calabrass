package permission

import (
	"context"
	"encoding/json"

	goai "github.com/zendev-sh/goai"
)

// Gate puts the session's rules in front of every tool. Tier 0 and 1 actions
// pass straight through; anything above waits for an answer or is refused.
//
// The session is captured here, when the turn is built, rather than looked up
// from the context when the tool runs: a tool may be called with a context
// that never saw the request.
func Gate(tools []goai.Tool, s *Session) []goai.Tool {
	out := make([]goai.Tool, len(tools))
	for i, tool := range tools {
		out[i] = gateOne(tool, s)
	}
	return out
}

func gateOne(tool goai.Tool, s *Session) goai.Tool {
	execute := tool.Execute
	if execute == nil {
		return tool
	}
	name := tool.Name
	tool.Execute = func(ctx context.Context, input json.RawMessage) (string, error) {
		act := Classify(name, input, s.workspace())
		if err := s.Authorize(ctx, act); err != nil {
			return "", err
		}
		return execute(ctx, input)
	}
	return tool
}

func (s *Session) workspace() string {
	if s == nil {
		return ""
	}
	return s.Workspace
}
