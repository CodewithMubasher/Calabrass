package httpapi

import (
	"context"
	"errors"
	"strings"
	"time"

	goai "github.com/zendev-sh/goai"

	"github.com/CodewithMubasher/Calabrass/backend/internal/agent"
	"github.com/CodewithMubasher/Calabrass/backend/internal/connectors"
	"github.com/CodewithMubasher/Calabrass/backend/internal/crew"
)

// turnSetup is what a turn takes from the harness: the prompt to think with, and
// the tools it may call.
type turnSetup struct {
	system   string
	tools    []goai.Tool
	origins  map[string]string
	runState *agent.RunState // budget / loop guard state
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
	var described []agent.Connector
	var origins map[string]string
	var agentSystemPrompt string
	var agentConnectorID string
	var agentMode string

	// If this chat is associated with an agent, load its configuration. A
	// built-in with no saved override has no row, so the shipped definition
	// stands in for it.
	if agentID != "" {
		agentRec, _, err := s.resolveAgent(ctx, userID, agentID)
		if err == nil {
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

	// The Orchestrator is not a coding agent, so it is not handed the coding
	// tools. Its whole job is to ask what it does not know and to hand work to
	// a worker; giving it a shell and a text editor would invite it to do the
	// work itself, which is exactly what its prompt tells it not to do.
	//
	// A crew chat that names no agent is still the Orchestrator's: crew mode is
	// how the crew is reached, and an empty agent there used to fall through to
	// the ordinary coding harness and do the task itself.
	orchestrator := crew.IsOrchestrator(agentID) ||
		(agentID == "" && mode == agent.ModeCrew)
	if orchestrator {
		prompt := agentSystemPrompt
		if strings.TrimSpace(prompt) == "" {
			if builtin, ok := crew.Find(crew.BuiltinOrchestratorID); ok {
				prompt = builtin.Persona()
			}
		}
		return s.orchestratorTurn(ctx, userID, workspace, chatID, prompt)
	}

	// Both ordinary modes have the same tools: chat is for ordinary work, crew
	// is for long work, and neither of them is held back from the project.
	tools := agent.Tools(workspace)

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
	tools = wrapBudget(tools, runState)

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

// orchestratorTurn builds the Orchestrator's turn: two tools, and a prompt that
// names the workers it may hand work to. It deliberately skips the coding
// harness rules, because "you can read, write and edit files" is the opposite of
// what the Orchestrator is for.
func (s *Server) orchestratorTurn(ctx context.Context, userID string, workspace *agent.Workspace, chatID, prompt string) turnSetup {
	tools := []goai.Tool{s.askQuestionsTool()}

	workspaceRoot := ""
	if workspace != nil {
		workspaceRoot = workspace.Root()
	}
	tools = append(tools, agent.DelegateTaskTool(func(toolCtx context.Context, args agent.DelegateTaskArgs) (string, error) {
		if workspaceRoot == "" {
			return "", errors.New("this chat has no workspace, so there is nowhere to run a worker")
		}
		return s.delegateTask(toolCtx, userID, chatID, workspaceRoot, args)
	}))

	workers, err := s.workerSummaries(ctx, userID)
	if err != nil {
		s.log.Warn("could not list workers for the Orchestrator", "error", err)
	}

	rendered := agent.RenderTemplate(prompt, agent.PromptVars{
		Workspace: workspaceRoot,
		Date:      time.Now().Format("Monday, 2 January 2006, 15:04"),
		Tools:     toolSummaries(tools),
		Workers:   workers,
	})

	budget := agent.DefaultBudget(agent.ModeCrew)
	runState := agent.NewRunState(budget)
	return turnSetup{
		system:   rendered,
		tools:    wrapBudget(tools, runState),
		runState: runState,
	}
}

// workerSummaries lists the agents the Orchestrator may hand work to: every
// saved agent, and every shipped worker that was never edited, minus the
// Orchestrator itself.
func (s *Server) workerSummaries(ctx context.Context, userID string) ([]agent.WorkerSummary, error) {
	rows, err := s.agentRowsFor(ctx, userID)
	if err != nil {
		return nil, err
	}

	workers := make([]agent.WorkerSummary, 0, len(rows))
	for _, row := range rows {
		if crew.IsOrchestrator(row.ID) {
			continue
		}
		workers = append(workers, agent.WorkerSummary{
			ID:          row.ID,
			Name:        row.Name,
			Description: row.Description,
		})
	}
	return workers, nil
}

// toolSummaries is the list a prompt template renders under a heading.
func toolSummaries(tools []goai.Tool) []agent.ToolSummary {
	summaries := make([]agent.ToolSummary, 0, len(tools))
	for _, tool := range tools {
		summaries = append(summaries, agent.ToolSummary{
			Name:        tool.Name,
			Description: tool.Description,
		})
	}
	return summaries
}

// wrapBudget puts the run's loop guard around every tool exactly once.
func wrapBudget(tools []goai.Tool, runState *agent.RunState) []goai.Tool {
	wrapped := make([]goai.Tool, len(tools))
	for i, tool := range tools {
		wrapped[i] = agent.BudgetMiddleware(tool, runState)
	}
	return wrapped
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
