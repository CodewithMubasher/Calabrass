package agent

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"time"

	"github.com/zendev-sh/goai"
)

// The crew's file contract. The Orchestrator writes a todo, the worker writes a
// report, and both are ordinary JSON files in the workspace so a person can
// open them, diff them, or keep them after the run. The backend owns the shape:
// the tool arguments are validated here, and the files are written here, so a
// model cannot invent a different contract and call it done.
const (
	// CrewDirName is the folder inside a workspace that holds crew work.
	CrewDirName = ".crew"
	// OrchestratorSubdir holds the todos the Orchestrator writes.
	OrchestratorSubdir = "orchestrator"
	// WorkersSubdir holds one folder per worker, each with its report.
	WorkersSubdir = "workers"
	// TodoSuffix is what a todo file is named after the worker id.
	TodoSuffix = ".todo.json"
	// ReportFileName is the one report a worker writes.
	ReportFileName = "report.json"

	// ReportCompleted is the only status that lets the Orchestrator carry on.
	ReportCompleted = "completed"
	// ReportFailed says the worker could not do the work.
	ReportFailed = "failed"
	// ReportBlocked says the worker needs something it does not have.
	ReportBlocked = "blocked"
)

// DelegateTaskArgs is the wire shape of delegate_task.
type DelegateTaskArgs struct {
	WorkerID string `json:"workerId" jsonschema:"description=The id of the worker to delegate to, exactly as listed in the Available workers section"`
	Task     string `json:"task" jsonschema:"description=What the worker must do, written as a clear instruction"`
	Context  string `json:"context,omitempty" jsonschema:"description=Anything already known that the worker should not have to rediscover"`
}

// UnmarshalJSON accepts the other names a model may reach for when it misses
// workerId. The schema offers workerId and nothing else, but a model that sends
// "worker" or the worker's name should still delegate rather than fail on a
// naming difference. The worker still has to resolve to a real one.
func (a *DelegateTaskArgs) UnmarshalJSON(data []byte) error {
	var raw struct {
		WorkerID    string `json:"workerId"`
		Worker      string `json:"worker"`
		WorkerName  string `json:"workerName"`
		AgentID     string `json:"agentId"`
		ID          string `json:"id"`
		Task        string `json:"task"`
		Instruction string `json:"instruction"`
		Context     string `json:"context"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	a.WorkerID = firstText(raw.WorkerID, raw.Worker, raw.WorkerName, raw.AgentID, raw.ID)
	a.Task = firstText(raw.Task, raw.Instruction)
	a.Context = strings.TrimSpace(raw.Context)
	return nil
}

// firstText returns the first value that has something in it.
func firstText(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

// DelegateTask runs a task with a worker agent: its prompt, its tools, its
// skills, in the same workspace, and returns the worker's report.
type DelegateTask func(ctx context.Context, args DelegateTaskArgs) (string, error)

// DelegateTaskTool is the tool the Orchestrator uses to hand work to a worker.
// The workers are named in the description as well as the prompt: the tool
// definition is read on every step, so the ids are in front of the model when
// it writes the call.
func DelegateTaskTool(workers []WorkerSummary, run DelegateTask) goai.Tool {
	description := "Hand a task to one worker agent."
	if choices := WorkerChoices(workers); choices != "" {
		description = "Hand a task to one worker agent, chosen by workerId, which must be one of: " + choices + "."
	}
	description += " It has its own prompt, tools, and skills, and runs to completion. It returns a report with a status, a summary, and the files it changed. This is the only way to get coding work done: do the planning, then delegate."

	return goai.NewTool("delegate_task", description,
		func(ctx context.Context, args DelegateTaskArgs) (string, error) {
			return run(ctx, args)
		})
}

// DelegateTaskTodo is what the Orchestrator hands a worker. The task and the
// context are the instruction; the paths say where the report belongs, and the
// workspace says which project the worker is in.
type DelegateTaskTodo struct {
	WorkerID   string `json:"workerId"`
	WorkerName string `json:"workerName"`
	Task       string `json:"task"`
	Context    string `json:"context,omitempty"`
	Workspace  string `json:"workspace"`
	CreatedAt  string `json:"createdAt"`
	TodoPath   string `json:"todoPath"`
	ReportPath string `json:"reportPath"`
}

// DelegateTaskReport is the strict interface a worker answers through. A run is
// only successful when Status is "completed"; anything else is surfaced to the
// Orchestrator as a failure with Error filled in.
type DelegateTaskReport struct {
	WorkerID     string   `json:"workerId"`
	WorkerName   string   `json:"workerName"`
	Status       string   `json:"status"`                 // completed | failed | blocked
	Summary      string   `json:"summary"`                // one line: what was done
	FilesChanged []string `json:"filesChanged,omitempty"` // paths relative to the workspace
	Checks       []string `json:"checks,omitempty"`       // what was verified: build, lint, tests
	Error        string   `json:"error,omitempty"`        // required when status is not completed
	StartedAt    string   `json:"startedAt,omitempty"`
	FinishedAt   string   `json:"finishedAt,omitempty"`
}

// ReportTaskArgs is the wire shape of report_task, the worker's only way to end
// a run.
type ReportTaskArgs struct {
	Status       string   `json:"status" jsonschema:"description=One of completed, failed, blocked"`
	Summary      string   `json:"summary" jsonschema:"description=One line saying what was done"`
	FilesChanged []string `json:"filesChanged,omitempty" jsonschema:"description=Paths of the files created or changed, relative to the workspace root"`
	Checks       []string `json:"checks,omitempty" jsonschema:"description=What you verified, for example build, lint, tests, manual check"`
	Error        string   `json:"error,omitempty" jsonschema:"description=What went wrong; required unless status is completed"`
}

// ReportTaskTool is the worker's closing tool. Writing the report through a
// tool rather than through prose is what makes the interface strict: the
// backend validates the status and writes the file, and a worker that never
// calls it has not finished.
func ReportTaskTool(write func(args ReportTaskArgs) (string, error)) goai.Tool {
	return goai.NewTool("report_task",
		"Finish the task by reporting the result. You MUST call this exactly once before you stop. status is completed, failed, or blocked. summary is one line. List every file you created or changed, and what you checked. If you are not completed, fill in error with the real reason.",
		func(ctx context.Context, args ReportTaskArgs) (string, error) {
			return write(args)
		})
}

// CrewDirs returns the .crew directories for a workspace: where the todos live
// and where the worker folders live.
func CrewDirs(workspace string) (orchestratorDir, workersDir string) {
	crew := filepath.Join(workspace, CrewDirName)
	return filepath.Join(crew, OrchestratorSubdir), filepath.Join(crew, WorkersSubdir)
}

// DelegateTaskTodoPath is where the todo for one worker is written.
func DelegateTaskTodoPath(orchestratorDir, workerID string) string {
	return filepath.Join(orchestratorDir, safeName(workerID)+TodoSuffix)
}

// DelegateTaskReportPath is where one worker's report is written.
func DelegateTaskReportPath(workersDir, workerID string) string {
	return filepath.Join(workersDir, safeName(workerID), ReportFileName)
}

// DelegateTaskTodoRel is the todo path as the workspace-relative form the UI
// and the worker use, always with forward slashes.
func DelegateTaskTodoRel(workerID string) string {
	return pathSlash(CrewDirName, OrchestratorSubdir, safeName(workerID)+TodoSuffix)
}

// DelegateTaskReportRel is the report path in workspace-relative form.
func DelegateTaskReportRel(workerID string) string {
	return pathSlash(CrewDirName, WorkersSubdir, safeName(workerID), ReportFileName)
}

// NormalizeReportStatus maps what a model wrote onto the three statuses the
// contract allows. An unknown word is a failure rather than a success: the
// Orchestrator must never be told a run worked when the worker said otherwise.
func NormalizeReportStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case ReportCompleted, "complete", "done", "success", "succeeded":
		return ReportCompleted
	case ReportBlocked, "blocker":
		return ReportBlocked
	case "", ReportFailed:
		return ReportFailed
	default:
		return ReportFailed
	}
}

// ValidateReport fills in defaults and refuses a report that cannot mean
// success. It is the backend's side of the strict interface.
func ValidateReport(report *DelegateTaskReport) error {
	if report == nil {
		return errors.New("no report")
	}
	report.Status = NormalizeReportStatus(report.Status)
	report.Summary = strings.TrimSpace(report.Summary)
	report.Error = strings.TrimSpace(report.Error)

	if report.Status == "" {
		report.Status = ReportFailed
	}
	if report.Summary == "" {
		if report.Status == ReportCompleted {
			report.Summary = "Completed."
		} else {
			report.Summary = "The worker did not finish."
		}
	}
	if report.Status == ReportCompleted {
		report.Error = ""
		return nil
	}
	if report.Error == "" {
		report.Error = "The worker reported " + report.Status + " without a reason."
	}
	return nil
}

// MarshalIndentJSON is how both crew files are written: indented and newline
// terminated, so they read well in an editor and diff cleanly.
func MarshalIndentJSON(value any) ([]byte, error) {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

// safeName keeps an id from reaching outside its folder. Worker ids are
// generated, but the value arrives from a model, so it is treated as untrusted.
func safeName(name string) string {
	name = strings.TrimSpace(name)
	name = strings.ReplaceAll(name, `\`, "-")
	name = strings.ReplaceAll(name, "/", "-")
	name = strings.ReplaceAll(name, "..", "-")
	if name == "" || name == "." {
		return "worker"
	}
	return name
}

// pathSlash joins with forward slashes, which is what the UI and the report
// use regardless of the platform the server runs on.
func pathSlash(parts ...string) string {
	clean := make([]string, 0, len(parts))
	for _, part := range parts {
		if part = strings.Trim(strings.TrimSpace(part), `/\`); part != "" {
			clean = append(clean, part)
		}
	}
	return strings.Join(clean, "/")
}

// NowRFC3339 is the timestamp both files carry.
func NowRFC3339() string {
	return time.Now().Format(time.RFC3339)
}
