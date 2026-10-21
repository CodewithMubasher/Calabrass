package httpapi

import (
	"context"
	"strings"

	goai "github.com/zendev-sh/goai"

	"github.com/CodewithMubasher/Calabrass/backend/internal/agent"
	"github.com/CodewithMubasher/Calabrass/backend/internal/connectors"
)

// turnSetup is what a turn takes from the harness: the prompt to think with, and
// the tools it may call.
type turnSetup struct {
	system    string
	tools     []goai.Tool
	origins   map[string]string
	runState  *agent.RunState // budget / loop guard state
}

// setupTurn builds a turn for the shipped admin account. Handlers use
// setupTurnFor with the signed-in account.
func (s *Server) setupTurn(ctx context.Context, workspace *agent.Workspace, chatID string) turnSetup {
	return s.setupTurnFor(ctx, "", workspace, chatID, "")
}

// setupTurnFor builds both together, because the prompt has to describe the
// tools the model is about to be given. A model told only about the project, and
// then handed the tools that reach the desktop, answers "open the settings app"
// by reading the project.
//
// chatID is the conversation the turn belongs to, which is what a task booked
// during it is bound to: the answer then lands where it was asked for. An empty
// id means there is no conversation to bind to. userID scopes the connectors and
// the tasks the turn may see. agentID is the agent this chat is associated with.
func (s *Server) setupTurnFor(ctx context.Context, userID string, workspace *agent.Workspace, chatID, agentID string) turnSetup {
	var tools []goai.Tool
	var described []agent.Connector
	var origins map[string]string
	var agentSystemPrompt string
	var agentConnectorID string
	var agentMode string

	// If this chat is associated with an agent, load its configuration
	if agentID != "" {
		agentRec, err := s.store.GetAgent(ctx, agentID, userID)
		if err == nil {
			// Use agent's system prompt (or prompt as fallback) as the system prompt
			agentSystemPrompt = agentRec.SystemPrompt
			if agentSystemPrompt == "" {
				agentSystemPrompt = agentRec.Prompt
			}
			agentConnectorID = agentRec.ConnectorID
			agentMode = agent.NormalMode(agentRec.Mode)
		}
	}

	// The chat's own mode decides how the turn runs: the address it lives at
	// says which mode it is in, and that is what the turn obeys. The agent's
	// mode is the default a chat starts with, so it stands in only when the
	// chat itself does not say.
	mode := agent.ModeChat
	if chatID != "" {
		if chatDetail, err := s.store.GetChat(ctx, chatID, userID); err == nil {
			if normalised := agent.NormalMode(chatDetail.Mode); normalised != "" {
				mode = normalised
			}
		}
	}
	if mode == agent.ModeChat && agentMode != "" {
		mode = agentMode
	}

	// Both modes have the same tools: chat is for ordinary work, crew is for
	// long work, and neither of them is held back from the project.
	tools = agent.Tools(workspace)

	// Add connector tools
	if s.connectors != nil {
		briefs, offered := s.connectors.Turn(ctx, userID)
		var skipped []string

		// If agent has a specific connector, only use that one
		if agentConnectorID != "" {
			filteredBriefs := []connectors.Brief{}
			for _, brief := range briefs {
				if brief.ID == agentConnectorID {
					filteredBriefs = append(filteredBriefs, brief)
					break
				}
			}
			briefs = filteredBriefs
		}

		tools, skipped = mergeTools(tools, offered)
		for _, name := range skipped {
			s.log.Warn("a connector tool was skipped: the name is already taken", "tool", name)
		}

		origins = make(map[string]string)
		for _, brief := range briefs {
			kept := without(brief.Tools, skipped)
			for _, name := range kept {
				origins[name] = brief.Name
			}
			described = append(described, agent.Connector{
				Name:         brief.Name,
				Instructions: brief.Instructions,
				Tools:        kept,
			})
		}
	}

	// The model can ask the user for what it cannot work out for itself in
	// either mode: a question is not a change to the project, and a plan is
	// exactly the sort of work that runs into something only the user knows.
	tools = append(tools, s.askQuestionsTool())

	// The scheduler is the server's own tool rather than the workspace's: a task
	// is a row in the database and a turn of its own, not something done to the
	// project. It needs a store to write that row to, which is what keeps it out
	// of the tests that build a server for its workspace alone.
	if s.store != nil {
		tools = append(tools, s.scheduleTool(chatID, userID))
	}

	// Wrap ALL tools with budget middleware for loop guard (once, after merging)
	budget := agent.DefaultBudget(mode)
	runState := agent.NewRunState(budget)
	wrappedTools := make([]goai.Tool, len(tools))
	for i, t := range tools {
		wrappedTools[i] = agent.BudgetMiddleware(t, runState)
	}
	tools = wrappedTools

	// Determine the system prompt to use
	systemPrompt := s.system
	if agentSystemPrompt != "" {
		// Agent's system prompt REPLACES the global system prompt entirely
		systemPrompt = agentSystemPrompt
	}

	// The rule that says how to ask the user for something is in every turn
	// that carries the ask_questions tool, whether or not there is a project
	// to explain.
	if trimmed := strings.TrimSpace(systemPrompt); trimmed != "" {
		systemPrompt = trimmed + "\n\n" + agent.AskQuestionsSection()
	} else {
		systemPrompt = agent.AskQuestionsSection()
	}

	// With no project and nothing acting on the machine, there is no harness to
	// explain: the turn is a plain completion with the system prompt.
	if workspace == nil && len(described) == 0 {
		return turnSetup{system: systemPrompt, tools: tools, origins: origins, runState: runState}
	}
	// Scan project map fresh each run (depth 2)
	projectMap := agent.ScanProjectMap(ctx, workspace.Root()).ProjectMapString()
	return turnSetup{system: agent.SystemPrompt(workspace, systemPrompt, described, mode, projectMap), tools: tools, origins: origins, runState: runState}
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
