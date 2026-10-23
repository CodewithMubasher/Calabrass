// Package crew holds the agents that ship with Calabrass: the orchestrator and
// the prompt it starts life with.
//
// A built-in is not a row in the database. It is a definition here, and a row
// carrying the same id is one account's saved version of it — an override.
// That is what makes a built-in impossible to delete: removing the row only
// puts the shipped definition back. It is also what lets a built-in be edited
// and saved, and edited again, without ever becoming a second agent.
//
// The identity of a built-in is the same for every account, so the row that
// holds an account's version of it carries the account with it. The agents
// table keys on (user_id, id) for exactly this reason.
package crew

import (
	"embed"
	"strings"

	"github.com/CodewithMubasher/Calabrass/backend/internal/store"
)

//go:embed prompts/orchestrator.md
//go:embed prompts/frontend-developer.md
//go:embed prompts/project-analyzer.md
var promptFS embed.FS

// BuiltinIDPrefix marks the ids of the agents the app ships with. Every
// built-in's id starts with it, so a custom agent can never be mistaken for
// one and a built-in can never be mistaken for a custom agent.
const BuiltinIDPrefix = "builtin-"

// The agents the app ships with. The Orchestrator plans; the workers do.
const (
	BuiltinOrchestratorID      = BuiltinIDPrefix + "orchestrator"
	BuiltinFrontendDeveloperID = BuiltinIDPrefix + "frontend-developer"
	BuiltinProjectAnalyzerID   = BuiltinIDPrefix + "project-analyzer"
)

// The two roles a built-in can have. A built-in is an orchestrator when its id
// is the Orchestrator's; every other built-in is a worker the crew can hand
// work to.
const (
	RoleOrchestrator = "orchestrator"
	RoleWorker       = "worker"
)

// IsBuiltinID reports whether an id names an agent the app ships with.
func IsBuiltinID(id string) bool {
	return strings.HasPrefix(strings.TrimSpace(id), BuiltinIDPrefix)
}

// IsOrchestrator reports whether an id is the Orchestrator's. That id is what
// decides which tools and which prompt a crew run is given.
func IsOrchestrator(id string) bool {
	return strings.TrimSpace(id) == BuiltinOrchestratorID
}

// Builtin is one agent as the app ships it.
type Builtin struct {
	// ID never changes: a saved row carries this id, so the account's
	// version of the agent is that same agent.
	ID string
	// Name is what the card says.
	Name string
	// Description is what the agent is for, in one line. The orchestrator
	// reads it to decide whether to hand work to this agent.
	Description string
	// Mode is the operating mode the agent runs in.
	Mode string
	// Role is "orchestrator" or "worker".
	Role string
	// PromptName is the embedded document the agent's prompt starts as.
	PromptName string
	// Skills are the skill folders a worker must read before it works.
	Skills []string
}

// Builtins returns the agents the app ships with, in the order they are
// shown: the Orchestrator first, then the workers.
func Builtins() []Builtin {
	return []Builtin{
		{
			ID:          BuiltinOrchestratorID,
			Name:        "Orchestrator",
			Description: "Understands what the user needs, plans the work, hands it to the right worker, and reports back.",
			Mode:        store.ModeCrew,
			Role:        RoleOrchestrator,
			PromptName:  "orchestrator.md",
		},
		{
			ID:          BuiltinFrontendDeveloperID,
			Name:        "Frontend Developer",
			Description: "Builds and changes user interfaces: layouts, components, styling, responsive behaviour, and accessibility, following the frontend-design skill.",
			Mode:        store.ModeCrew,
			Role:        RoleWorker,
			PromptName:  "frontend-developer.md",
			Skills:      []string{"frontend-design"},
		},
		{
			ID:          BuiltinProjectAnalyzerID,
			Name:        "Project Analyzer",
			Description: "Reads and understands large codebases: structure, conventions, dependencies, and call chains. Uses parallel reads, the symbol index, and targeted searches to answer questions accurately without reading everything.",
			Mode:        store.ModeCrew,
			Role:        RoleWorker,
			PromptName:  "project-analyzer.md",
			Skills:      []string{"project-analysis"},
		},
	}
}

// Find returns the built-in whose id matches, and whether there was one.
func Find(id string) (Builtin, bool) {
	for _, b := range Builtins() {
		if b.ID == strings.TrimSpace(id) {
			return b, true
		}
	}
	return Builtin{}, false
}

// Persona is the prompt a built-in starts life with: its own words about how
// it works. This is what the dialog shows for a built-in that has never been
// edited, and what a reset puts back.
func (b Builtin) Persona() string {
	data, err := promptFS.ReadFile("prompts/" + b.PromptName)
	if err != nil {
		// Every name in Builtins() is compiled in, so this is unreachable.
		return ""
	}
	return strings.TrimSpace(string(data))
}

// Default is the built-in as a stored record: what the account sees before it
// has saved anything. The model is deliberately empty — a built-in ships
// without a model, and the account picks one when it edits the agent.
func (b Builtin) Default() store.AgentRecord {
	return store.AgentRecord{
		ID:           b.ID,
		Name:         b.Name,
		Prompt:       b.Persona(),
		SystemPrompt: "",
		Description:  b.Description,
		Mode:         b.Mode,
		ConnectorID:  "",
		Provider:     "",
		Model:        "",
		Skills:       append([]string(nil), b.Skills...),
	}
}
