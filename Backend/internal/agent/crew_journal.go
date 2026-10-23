package agent

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// The crew journal is the durable record of a long task. Each chat gets one
// append-only JSON-lines file beside its todo and report folders:
//
//	.crew/chats/<chat>/journal.jsonl
//
// A todo is appended before the worker starts and a report is appended when it
// finishes. If the process is interrupted in between, the todo is still there,
// so the next turn can see that a delegation was started and never reported.
// The conversation history is not the only place the task state lives.
const (
	// CrewJournalFileName is the append-only work log for one chat.
	CrewJournalFileName = "journal.jsonl"
	// Journal kinds.
	JournalTodo   = "todo"
	JournalReport = "report"
	JournalGuard  = "guard"
)

// CrewJournalEntry is one line of the journal. Todo and report entries share a
// shape so a reader can render either without a second schema: a report fills
// in Status and the result fields, a todo leaves them empty.
type CrewJournalEntry struct {
	Time         string   `json:"time"`
	Kind         string   `json:"kind"`
	WorkerID     string   `json:"workerId"`
	WorkerName   string   `json:"workerName,omitempty"`
	Task         string   `json:"task,omitempty"`
	Context      string   `json:"context,omitempty"`
	Acceptance   []string `json:"acceptance,omitempty"`
	Status       string   `json:"status,omitempty"`
	Summary      string   `json:"summary,omitempty"`
	FilesChanged []string `json:"filesChanged,omitempty"`
	Checks       []string `json:"checks,omitempty"`
	Unresolved   []string `json:"unresolved,omitempty"`
	Error        string   `json:"error,omitempty"`
	Reason       string   `json:"reason,omitempty"`
}

// CrewJournalPath is one chat's journal.
func CrewJournalPath(workspace, chatID string) string {
	return filepath.Join(CrewRoot(workspace, chatID), CrewJournalFileName)
}

// AppendCrewJournal adds one entry to the journal. The write is a single
// append, so a reader never sees a half line; a later run that cannot append
// loses state but does not corrupt the file.
func AppendCrewJournal(path string, entry CrewJournalEntry) error {
	if strings.TrimSpace(entry.Time) == "" {
		entry.Time = time.Now().Format(time.RFC3339)
	}
	line, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	if _, err := file.Write(append(line, '\n')); err != nil {
		return err
	}
	return nil
}

// ReadCrewJournal returns the last max entries, oldest first. A malformed line
// is skipped rather than failing the whole read: the journal is an aid, and a
// truncated line from a killed process must not hide the rest of the work.
func ReadCrewJournal(path string, max int) []CrewJournalEntry {
	file, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer func() { _ = file.Close() }()

	entries := make([]CrewJournalEntry, 0, 32)
	scanner := bufio.NewScanner(file)
	// A long task and a long summary can exceed the default 64 KB line limit.
	scanner.Buffer(make([]byte, 0, 64<<10), 1<<20)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var entry CrewJournalEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		entries = append(entries, entry)
	}
	if max > 0 && len(entries) > max {
		entries = entries[len(entries)-max:]
	}
	return entries
}

// CrewDelegationKey identifies a piece of work for duplicate detection: the
// worker plus the task, with case and whitespace normalised. Context is not part
// of the key, because a completed task is complete regardless of how it was
// explained; a failed one is matched with context separately.
func CrewDelegationKey(workerID, task string) string {
	return strings.ToLower(strings.TrimSpace(workerID)) + "\x00" + normalizeTaskText(task)
}

func normalizeTaskText(text string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(strings.ToLower(text))), " ")
}

// CrewDuplicates looks through a chat's journal for work that should not be
// blindly repeated. A completed identical task is returned as completed; an
// identical failed task with identical context is returned as failed. A retry
// with different context is allowed, because that is new information.
func CrewDuplicates(entries []CrewJournalEntry, workerID, task, context string) (completed, failed *CrewJournalEntry) {
	key := CrewDelegationKey(workerID, task)
	contextKey := normalizeTaskText(context)
	for i := range entries {
		entry := &entries[i]
		if entry.Kind != JournalReport {
			continue
		}
		if CrewDelegationKey(entry.WorkerID, entry.Task) != key {
			continue
		}
		switch NormalizeReportStatus(entry.Status) {
		case ReportCompleted:
			completed = entry
		case ReportFailed, ReportBlocked:
			if normalizeTaskText(entry.Context) == contextKey {
				failed = entry
			}
		}
	}
	return completed, failed
}

// CrewTaskState renders the persisted state for a prompt: what was completed,
// what failed or is blocked, what was started but never reported, and what the
// workers said remained unresolved. It is bounded in both entries and bytes so
// a long task does not crowd out the actual instructions.
func CrewTaskState(workspace, chatID string) string {
	return renderCrewTaskState(ReadCrewJournal(CrewJournalPath(workspace, chatID), 200), 40, 4<<10, "", "")
}

// CrewTaskStateFor is the state a worker sees: the same record, with the
// worker's own current task left out (it is already in front of it), and a
// smaller budget because the worker's own instructions matter more.
func CrewTaskStateFor(workspace, chatID, workerID, task string) string {
	return renderCrewTaskState(ReadCrewJournal(CrewJournalPath(workspace, chatID), 200), 24, 3<<10, workerID, task)
}

// crewStateItem is one line rendered into a prompt's task-state section.
type crewStateItem struct {
	time   string
	worker string
	task   string
	status string
	detail string
}

func renderCrewTaskState(entries []CrewJournalEntry, maxEntries, maxBytes int, excludeWorker, excludeTask string) string {
	if len(entries) == 0 {
		return ""
	}

	var completed, failed []crewStateItem
	pendingByKey := make(map[string]crewStateItem)
	pendingOrder := make([]string, 0, 8)
	unresolved := make([]string, 0, 8)

	excludeKey := ""
	if excludeWorker != "" || excludeTask != "" {
		excludeKey = CrewDelegationKey(excludeWorker, excludeTask)
	}

	for _, entry := range entries {
		key := CrewDelegationKey(entry.WorkerID, entry.Task)
		if excludeKey != "" && key == excludeKey {
			continue
		}
		worker := strings.TrimSpace(entry.WorkerName)
		if worker == "" {
			worker = strings.TrimSpace(entry.WorkerID)
		}
		line := crewStateItem{
			time:   entry.Time,
			worker: worker,
			task:   truncateRunes(oneLine(entry.Task), 180),
			status: entry.Status,
		}

		switch entry.Kind {
		case JournalTodo:
			if _, seen := pendingByKey[key]; !seen {
				pendingOrder = append(pendingOrder, key)
			}
			pendingByKey[key] = line
		case JournalReport:
			delete(pendingByKey, key)
			line.detail = reportDetail(entry)
			switch NormalizeReportStatus(entry.Status) {
			case ReportCompleted:
				completed = append(completed, line)
			case ReportBlocked:
				line.status = ReportBlocked
				failed = append(failed, line)
				addUnresolved(&unresolved, entry, worker)
			default:
				line.status = ReportFailed
				failed = append(failed, line)
				addUnresolved(&unresolved, entry, worker)
			}
		}
	}

	pending := make([]crewStateItem, 0, len(pendingByKey))
	for _, key := range pendingOrder {
		if item, ok := pendingByKey[key]; ok {
			pending = append(pending, item)
		}
	}

	// Newest last in the journal; show newest first in the prompt so the most
	// relevant state survives the byte cap when the task is long.
	reverseItems(completed)
	reverseItems(failed)
	reverseItems(pending)
	if maxEntries > 0 {
		completed = headItems(completed, maxEntries)
		failed = headItems(failed, maxEntries)
		pending = headItems(pending, maxEntries)
	}

	var b strings.Builder
	b.WriteString("=== CREW TASK STATE (persisted) ===\n")
	b.WriteString("This is work already recorded for this conversation. It survives reloads and interruptions. Use it to avoid repeating completed work; verify the workspace before relying on a summary.\n")
	writeItems(&b, "Completed", completed)
	writeItems(&b, "Failed or blocked", failed)
	writeItems(&b, "Pending (delegated, no report yet)", pending)
	if len(unresolved) > 0 {
		b.WriteString("Unresolved reported by workers:\n")
		for _, text := range unresolved {
			b.WriteString("- " + text + "\n")
		}
	}

	text := b.String()
	if len(text) > maxBytes {
		text = truncateRunes(text, maxBytes) + "\n… [crew state truncated]\n"
	}
	return text
}

func addUnresolved(out *[]string, entry CrewJournalEntry, worker string) {
	for _, item := range entry.Unresolved {
		line := strings.TrimSpace(item)
		if line == "" {
			continue
		}
		*out = append(*out, worker+": "+truncateRunes(oneLine(line), 200))
	}
	if len(entry.Unresolved) == 0 && strings.TrimSpace(entry.Error) != "" {
		*out = append(*out, worker+": "+truncateRunes(oneLine(entry.Error), 200))
	}
}

func reportDetail(entry CrewJournalEntry) string {
	parts := make([]string, 0, 4)
	if summary := strings.TrimSpace(entry.Summary); summary != "" {
		parts = append(parts, truncateRunes(oneLine(summary), 220))
	}
	if errText := strings.TrimSpace(entry.Error); errText != "" {
		parts = append(parts, "error: "+truncateRunes(oneLine(errText), 220))
	}
	if len(entry.FilesChanged) > 0 {
		parts = append(parts, "files: "+truncateRunes(strings.Join(entry.FilesChanged, ", "), 220))
	}
	if len(entry.Checks) > 0 {
		parts = append(parts, "checks: "+truncateRunes(strings.Join(entry.Checks, ", "), 160))
	}
	return strings.Join(parts, "; ")
}

func writeItems(b *strings.Builder, heading string, items []crewStateItem) {
	if len(items) == 0 {
		return
	}
	b.WriteString(heading + ":\n")
	for _, it := range items {
		b.WriteString("- ")
		b.WriteString(it.worker)
		if it.task != "" {
			b.WriteString(": ")
			b.WriteString(it.task)
		}
		if it.detail != "" {
			b.WriteString(" — ")
			b.WriteString(it.detail)
		}
		b.WriteString("\n")
	}
}

func reverseItems(items []crewStateItem) {
	for i, j := 0, len(items)-1; i < j; i, j = i+1, j-1 {
		items[i], items[j] = items[j], items[i]
	}
}

func headItems(items []crewStateItem, max int) []crewStateItem {
	if len(items) > max {
		return items[:max]
	}
	return items
}

func oneLine(text string) string {
	return strings.Join(strings.Fields(strings.ReplaceAll(text, "\n", " ")), " ")
}

func truncateRunes(text string, limit int) string {
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	return string(runes[:limit]) + "…"
}
