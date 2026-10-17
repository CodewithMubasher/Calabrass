package httpapi

import (
	"context"

	goai "github.com/zendev-sh/goai"

	"github.com/CodewithMubasher/Calabrass/backend/internal/agent"
)

// turnSetup is what a turn takes from the harness: the prompt to think with, and
// the tools it may call.
type turnSetup struct {
	system string
	tools  []goai.Tool
}

// setupTurn builds both together, because the prompt has to describe the tools
// the model is about to be given. A model told only about the project, and then
// handed the tools that reach the desktop, answers "open the settings app" by
// reading the project.
func (s *Server) setupTurn(ctx context.Context, workspace *agent.Workspace) turnSetup {
	tools := agent.Tools(workspace)
	var described []agent.Connector

	if s.connectors != nil {
		briefs, offered := s.connectors.Turn(ctx)
		var skipped []string
		tools, skipped = mergeTools(tools, offered)
		for _, name := range skipped {
			s.log.Warn("a connector tool was skipped: the name is already taken", "tool", name)
		}

		// The prompt describes the tools that are really there, so one that lost
		// its name to a built-in is dropped from the description as well.
		for _, brief := range briefs {
			described = append(described, agent.Connector{
				Name:         brief.Name,
				Instructions: brief.Instructions,
				Tools:        without(brief.Tools, skipped),
			})
		}
	}

	// With no project and nothing acting on the machine, there is no harness to
	// explain: the turn is a plain completion, and the application's own prompt
	// is the whole of it.
	if workspace == nil && len(described) == 0 {
		return turnSetup{system: s.system, tools: tools}
	}
	return turnSetup{system: agent.SystemPrompt(workspace, s.system, described), tools: tools}
}

// mergeTools adds what the connectors offer to a turn's tools, leaving out the
// ones that would take a name already in use, and reports the names it left out.
//
// The model picks a tool by its name, so a name that means two things is the
// wrong thing half the time: a connector must not be able to quietly answer for
// the built-in it shadows.
func mergeTools(tools, offered []goai.Tool) ([]goai.Tool, []string) {
	taken := make(map[string]bool, len(tools)+len(offered))
	for _, tool := range tools {
		taken[tool.Name] = true
	}

	var skipped []string
	for _, tool := range offered {
		if taken[tool.Name] {
			skipped = append(skipped, tool.Name)
			continue
		}
		taken[tool.Name] = true
		tools = append(tools, tool)
	}
	return tools, skipped
}

// without is names with the unwanted ones taken out.
func without(names, unwanted []string) []string {
	if len(unwanted) == 0 {
		return names
	}

	gone := make(map[string]bool, len(unwanted))
	for _, name := range unwanted {
		gone[name] = true
	}

	kept := make([]string, 0, len(names))
	for _, name := range names {
		if !gone[name] {
			kept = append(kept, name)
		}
	}
	return kept
}
