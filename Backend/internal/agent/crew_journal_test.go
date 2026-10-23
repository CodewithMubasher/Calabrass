package agent

import (
	"os"
	"strings"
	"testing"
)

func TestCrewJournalRecordsTodoAndReport(t *testing.T) {
	workspace := t.TempDir()
	path := CrewJournalPath(workspace, "chat-1")

	todo := CrewJournalEntry{
		Kind:     JournalTodo,
		WorkerID: "builtin-frontend-developer",
		Task:     "Build the settings page",
	}
	if err := AppendCrewJournal(path, todo); err != nil {
		t.Fatalf("append todo: %v", err)
	}

	state := CrewTaskState(workspace, "chat-1")
	if !strings.Contains(state, "Pending") || !strings.Contains(state, "Build the settings page") {
		t.Fatalf("state does not show the pending delegation:\n%s", state)
	}

	report := CrewJournalEntry{
		Kind:         JournalReport,
		WorkerID:     "builtin-frontend-developer",
		Task:         "Build the settings page",
		Status:       ReportCompleted,
		Summary:      "Added the page and wired the route.",
		FilesChanged: []string{"src/routes/settings.tsx"},
	}
	if err := AppendCrewJournal(path, report); err != nil {
		t.Fatalf("append report: %v", err)
	}

	entries := ReadCrewJournal(path, 10)
	if len(entries) != 2 {
		t.Fatalf("read %d entries, want 2", len(entries))
	}

	state = CrewTaskState(workspace, "chat-1")
	if !strings.Contains(state, "Completed") {
		t.Fatalf("state does not show completed work:\n%s", state)
	}
	if strings.Contains(state, "Pending") {
		t.Fatalf("a reported todo is still pending:\n%s", state)
	}
	if !strings.Contains(state, "settings.tsx") {
		t.Fatalf("state does not carry the files changed:\n%s", state)
	}
}

func TestCrewJournalSurvivesAMalformedLine(t *testing.T) {
	workspace := t.TempDir()
	path := CrewJournalPath(workspace, "chat-1")
	if err := AppendCrewJournal(path, CrewJournalEntry{Kind: JournalTodo, WorkerID: "w", Task: "one"}); err != nil {
		t.Fatal(err)
	}
	// A process killed mid-write leaves a partial line. It must not hide the
	// entries that are intact.
	if err := appendRaw(path, "{\"kind\":\"report\""); err != nil {
		t.Fatal(err)
	}
	if err := AppendCrewJournal(path, CrewJournalEntry{Kind: JournalTodo, WorkerID: "w", Task: "two"}); err != nil {
		t.Fatal(err)
	}

	entries := ReadCrewJournal(path, 10)
	if len(entries) != 2 {
		t.Fatalf("read %d entries, want the two intact lines", len(entries))
	}
}

func appendRaw(path, line string) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	_, err = file.WriteString(line + "\n")
	return err
}

func TestCrewDuplicatesFindsCompletedAndFailedWork(t *testing.T) {
	entries := []CrewJournalEntry{
		{Kind: JournalReport, WorkerID: "w", Task: "Build  the page", Status: ReportCompleted, Summary: "done"},
		{Kind: JournalReport, WorkerID: "w", Task: "Fix the header", Context: "desktop", Status: ReportFailed, Error: "missing token"},
	}

	completed, failed := CrewDuplicates(entries, "w", "build the page", "")
	if completed == nil || failed != nil {
		t.Fatalf("completed = %v, failed = %v; want the completed match", completed, failed)
	}

	completed, failed = CrewDuplicates(entries, "w", "Fix the header", "desktop")
	if completed != nil || failed == nil {
		t.Fatalf("completed = %v, failed = %v; want the failed match", completed, failed)
	}

	// New context is new evidence, so the failed retry is allowed.
	completed, failed = CrewDuplicates(entries, "w", "Fix the header", "mobile")
	if completed != nil || failed != nil {
		t.Fatalf("a retry with different context should be allowed, got completed=%v failed=%v", completed, failed)
	}
}

func TestCrewTaskStateLeavesOutTheWorkersOwnTask(t *testing.T) {
	workspace := t.TempDir()
	path := CrewJournalPath(workspace, "chat-1")
	if err := AppendCrewJournal(path, CrewJournalEntry{Kind: JournalTodo, WorkerID: "w", Task: "the current task"}); err != nil {
		t.Fatal(err)
	}
	if err := AppendCrewJournal(path, CrewJournalEntry{Kind: JournalTodo, WorkerID: "worker-2", Task: "the other task"}); err != nil {
		t.Fatal(err)
	}

	state := CrewTaskStateFor(workspace, "chat-1", "w", "the current task")
	if strings.Contains(state, "the current task") {
		t.Fatalf("the worker's own task was repeated back to it:\n%s", state)
	}
	if !strings.Contains(state, "the other task") {
		t.Fatalf("other pending work is missing:\n%s", state)
	}
}
