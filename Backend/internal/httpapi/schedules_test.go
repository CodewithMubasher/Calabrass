package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/CodewithMubasher/Calabrass/backend/internal/store"
)

// signUpAndGetToken creates a test user and returns their auth token.
func signUpAndGetToken(t *testing.T, server *httptest.Server) string {
	t.Helper()

	signUp := map[string]string{
		"name":     "Test User",
		"email":    "test@example.com",
		"password": "a long enough password",
	}

	var body bytes.Buffer
	if err := json.NewEncoder(&body).Encode(signUp); err != nil {
		t.Fatalf("encode sign up: %v", err)
	}

	req, err := http.NewRequest(http.MethodPost, server.URL+"/api/auth/signup", &body)
	if err != nil {
		t.Fatalf("build signup request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("signup request: %v", err)
	}
	defer func() { _ = res.Body.Close() }()

	if res.StatusCode != http.StatusCreated {
		t.Fatalf("signup status = %d, want 201", res.StatusCode)
	}

	var payload map[string]any
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatalf("decode signup response: %v", err)
	}

	token, ok := payload["token"].(string)
	if !ok || token == "" {
		t.Fatal("signup returned no token")
	}
	return token
}

// A delay is the plainest reading of "in ten minutes", and it does not depend on
// the clock the test runs on.
func TestResolveRunAtFromADelay(t *testing.T) {
	now := time.Date(2026, 10, 17, 21, 0, 0, 0, time.Local)

	at, err := resolveRunAt(now, "", 10)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if want := now.Add(10 * time.Minute); !at.Equal(want) {
		t.Errorf("got %s, want %s", at, want)
	}
}

// A clock time is read on this machine's clock, and one that has already been
// and gone today is tomorrow.
func TestResolveRunAtFromAClockTime(t *testing.T) {
	now := time.Date(2026, 10, 17, 21, 0, 0, 0, time.Local)

	later, err := resolveRunAt(now, "23:30", 0)
	if err != nil {
		t.Fatalf("resolve a time later today: %v", err)
	}
	if later.Hour() != 23 || later.Minute() != 30 || later.Day() != 17 {
		t.Errorf("got %s, want 23:30 on the 17th", later)
	}

	tomorrow, err := resolveRunAt(now, "08:00", 0)
	if err != nil {
		t.Fatalf("resolve a time already past: %v", err)
	}
	if tomorrow.Hour() != 8 || tomorrow.Day() != 18 {
		t.Errorf("got %s, want 08:00 on the 18th", tomorrow)
	}
}

func TestResolveRunAtRejectsNonsense(t *testing.T) {
	now := time.Now()

	if _, err := resolveRunAt(now, "", 0); err == nil {
		t.Error("a task with no time was accepted")
	}
	if _, err := resolveRunAt(now, "tomorrow maybe", 0); err == nil {
		t.Error("an unreadable time was accepted")
	}
}

// The tool is what the model reaches for when it is asked to do something later,
// so booking one has to leave a row behind that the runner will find.
func TestScheduleToolBooksATask(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(filepath.Join(t.TempDir(), "calabrass.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	s := &Server{store: st}

	answer, err := s.scheduleFromTool(ctx, "chat-1", "", scheduleTaskArgs{
		Prompt:       "Shut the laptop down",
		AfterMinutes: 10,
	})
	if err != nil {
		t.Fatalf("schedule: %v", err)
	}
	if answer == "" {
		t.Fatal("the model was told nothing to pass on")
	}

	due, err := st.DueScheduledTasks(ctx, time.Now().Add(11*time.Minute).UnixMilli(), 10)
	if err != nil {
		t.Fatalf("read due tasks: %v", err)
	}
	if len(due) != 1 {
		t.Fatalf("got %d due tasks, want 1", len(due))
	}
	if due[0].Prompt != "Shut the laptop down" {
		t.Errorf("got prompt %q", due[0].Prompt)
	}
	if due[0].ChatID != "chat-1" {
		t.Errorf("got chat %q, want the conversation it was booked from", due[0].ChatID)
	}
}

// A user who says "every morning" gets a repeating row, and a repeat the
// version does not understand is refused rather than stored.
func TestScheduleToolBooksARepeatingTask(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(filepath.Join(t.TempDir(), "calabrass.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	s := &Server{store: st}

	if _, err := s.scheduleFromTool(ctx, "chat-1", "", scheduleTaskArgs{
		Prompt:       "Water the plants",
		AfterMinutes: 60,
		Repeat:       "daily",
	}); err != nil {
		t.Fatalf("schedule: %v", err)
	}

	tasks, err := st.ListScheduledTasks(ctx)
	if err != nil {
		t.Fatalf("list tasks: %v", err)
	}
	if len(tasks) != 1 || tasks[0].Repeat != string(store.RepeatDaily) {
		t.Fatalf("got %+v, want one daily task", tasks)
	}

	if _, err := s.scheduleFromTool(ctx, "chat-1", "", scheduleTaskArgs{
		Prompt:       "Never",
		AfterMinutes: 60,
		Repeat:       "hourly",
	}); err == nil {
		t.Fatal("an unknown repeat was accepted")
	}
}

// A time that has already been and gone cannot be scheduled, and saying so is
// better than booking a task that fires the moment it is created.
func TestScheduleToolRefusesThePast(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(filepath.Join(t.TempDir(), "calabrass.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	s := &Server{store: st}

	if _, err := s.scheduleFromTool(ctx, "", "", scheduleTaskArgs{
		Prompt: "Too late",
		RunAt:  time.Now().Add(-time.Hour).Format("2006-01-02T15:04"),
	}); err == nil {
		t.Fatal("a time in the past was accepted")
	}
}

// Turning a task off takes it out of the runner's reach; turning it back on
// brings it back, and giving it a new time arms it again.
func TestTaskLifecycle(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(filepath.Join(t.TempDir(), "calabrass.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	due := time.Now().Add(time.Minute).UnixMilli()
	task, err := st.CreateScheduledTask(ctx, store.ScheduledTask{
		Prompt:  "Say good morning",
		RunAt:   due,
		Enabled: true,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	if tasks, _ := st.DueScheduledTasks(ctx, time.Now().Add(2*time.Minute).UnixMilli(), 10); len(tasks) != 1 {
		t.Fatalf("an armed task is not due: got %d", len(tasks))
	}

	task.Enabled = false
	if _, err := st.UpdateScheduledTask(ctx, task); err != nil {
		t.Fatalf("pause: %v", err)
	}
	if tasks, _ := st.DueScheduledTasks(ctx, time.Now().Add(2*time.Minute).UnixMilli(), 10); len(tasks) != 0 {
		t.Fatalf("a paused task is still due: got %d", len(tasks))
	}

	// Running it once is what stops it coming due again.
	if err := st.FinishScheduledTask(ctx, task.ID, time.Now().UnixMilli(), "ok", "", 0); err != nil {
		t.Fatalf("mark: %v", err)
	}
	task, err = st.GetScheduledTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !task.HasRun() || task.LastStatus != "ok" {
		t.Errorf("the run was not recorded: %+v", task)
	}
	if tasks, _ := st.DueScheduledTasks(ctx, time.Now().Add(time.Hour).UnixMilli(), 10); len(tasks) != 0 {
		t.Fatalf("a task ran and came due again: got %d", len(tasks))
	}

	// A new time is a new task: it is armed again, and the old run no longer
	// counts.
	task.RunAt = time.Now().Add(time.Hour).UnixMilli()
	task.Enabled = true
	task.LastRunAt, task.LastStatus, task.LastError = 0, "", ""
	if _, err := st.UpdateScheduledTask(ctx, task); err != nil {
		t.Fatalf("re-arm: %v", err)
	}
	if tasks, _ := st.DueScheduledTasks(ctx, time.Now().Add(2*time.Hour).UnixMilli(), 10); len(tasks) != 1 {
		t.Fatalf("a re-armed task is not due: got %d", len(tasks))
	}
}

// A repeating task is a different shape of timer: it keeps having turns, so a
// run advances it to its next occurrence instead of retiring it.
func TestRepeatingTaskReArmsAfterARun(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(filepath.Join(t.TempDir(), "calabrass.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	due := time.Now().Add(-time.Minute).UnixMilli()
	task, err := st.CreateScheduledTask(ctx, store.ScheduledTask{
		Prompt:  "Water the plants",
		RunAt:   due,
		Enabled: true,
		Repeat:  string(store.RepeatDaily),
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if tasks, _ := st.DueScheduledTasks(ctx, time.Now().UnixMilli(), 10); len(tasks) != 1 {
		t.Fatalf("a due repeating task is not due: got %d", len(tasks))
	}

	s := &Server{store: st, log: slog.Default()}
	s.recordRun(ctx, task, "ok", "")

	task, err = st.GetScheduledTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if task.HasRun() {
		t.Error("a repeating task was retired after one run")
	}
	if task.LastStatus != "ok" || task.LastRunAt == 0 {
		t.Errorf("the run was not recorded: %+v", task)
	}
	if task.RunAt <= time.Now().UnixMilli() {
		t.Errorf("the next occurrence is still in the past: %d", task.RunAt)
	}
	if tasks, _ := st.DueScheduledTasks(ctx, time.Now().UnixMilli(), 10); len(tasks) != 0 {
		t.Errorf("a repeating task came due twice in one occurrence: got %d", len(tasks))
	}
	// It is due again when the next occurrence arrives.
	if tasks, _ := st.DueScheduledTasks(ctx, task.RunAt, 10); len(tasks) != 1 {
		t.Errorf("the next occurrence is not due when it arrives: got %d", len(tasks))
	}
}

// nextOccurrence steps from the scheduled time, not from now: a laptop woken
// late must still run at its chosen clock time, and missed turns are skipped
// rather than replayed in a burst.
func TestNextOccurrence(t *testing.T) {
	base := time.Date(2026, time.March, 2, 9, 30, 0, 0, time.UTC)

	tests := []struct {
		name   string
		from   time.Time
		repeat store.Repeat
		now    time.Time
		want   time.Time
	}{
		{"once", base, store.RepeatOnce, base, time.Time{}},
		{"unknown", base, store.Repeat("hourly"), base, time.Time{}},
		{"daily", base, store.RepeatDaily, base, base.AddDate(0, 0, 1)},
		{"weekly", base, store.RepeatWeekly, base, base.AddDate(0, 0, 7)},
		{"monthly", base, store.RepeatMonthly, base, base.AddDate(0, 1, 0)},
		{
			// Woken on the fourth day after base, three turns were missed: only
			// the one still ahead is scheduled.
			"daily skips missed turns instead of replaying them",
			base, store.RepeatDaily, base.AddDate(0, 0, 3).Add(2 * time.Hour),
			base.AddDate(0, 0, 4),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := nextOccurrence(test.from, test.repeat, test.now)
			if !got.Equal(test.want) {
				t.Errorf("got %v, want %v", got, test.want)
			}
		})
	}
}

// An unknown repeat is refused at the edge rather than stored and ignored.
func TestScheduleRejectsUnknownRepeat(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "calabrass.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	server := httptest.NewServer(NewServer(Deps{Store: st, Logger: slog.Default()}))
	t.Cleanup(server.Close)
	token := signUpAndGetToken(t, server)

	body := `{"prompt":"Never","runAt":` + strconv.FormatInt(time.Now().Add(time.Hour).UnixMilli(), 10) + `,"repeat":"hourly"}`
	req, err := http.NewRequest(http.MethodPost, server.URL+"/api/schedules", strings.NewReader(body))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	res, err := server.Client().Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("an unknown repeat answered %s, want 400", res.Status)
	}
}

// The endpoints the page drives, end to end through the router: booking a task,
// reading it back, pausing it, giving it a new time, and removing it. Nothing
// here reaches a model.
func TestScheduleEndpoints(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "calabrass.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	// Building the server is itself worth testing: a route that collides with
	// another one panics here rather than at startup.
	server := httptest.NewServer(NewServer(Deps{Store: st, Logger: slog.Default()}))
	t.Cleanup(server.Close)

	// Sign up a test user and get a token for authenticated requests
	token := signUpAndGetToken(t, server)

	due := time.Now().Add(time.Minute).UnixMilli()
	tasks := callSchedules(t, server, token, http.MethodPost, "/api/schedules",
		`{"prompt":"Shut the laptop down","runAt":`+strconv.FormatInt(due, 10)+`,"repeat":"daily"}`)
	if len(tasks) != 1 {
		t.Fatalf("booking a task answered with %d of them", len(tasks))
	}
	task := tasks[0]
	if task.Prompt != "Shut the laptop down" || task.RunAt != due || !task.Enabled {
		t.Fatalf("the task came back as %+v", task)
	}
	if task.Repeat != string(store.RepeatDaily) {
		t.Fatalf("the repeat came back as %q", task.Repeat)
	}

	tasks = callSchedules(t, server, token, http.MethodGet, "/api/schedules", "")
	if len(tasks) != 1 {
		t.Fatalf("the list has %d tasks", len(tasks))
	}

	tasks = callSchedules(t, server, token, http.MethodPatch, "/api/schedules/"+task.ID, `{"enabled":false}`)
	if len(tasks) != 1 || tasks[0].Enabled {
		t.Fatalf("pausing it answered with %+v", tasks)
	}

	// A new time arms it again, and the run that already happened stops
	// describing it.
	tasks = callSchedules(t, server, token, http.MethodPatch, "/api/schedules/"+task.ID,
		`{"runAt":`+strconv.FormatInt(due+60_000, 10)+`}`)
	if len(tasks) != 1 || tasks[0].RunAt != due+60_000 {
		t.Fatalf("re-timing it answered with %+v", tasks)
	}

	tasks = callSchedules(t, server, token, http.MethodDelete, "/api/schedules/"+task.ID, "")
	if len(tasks) != 0 {
		t.Fatalf("after removing it the list has %d tasks", len(tasks))
	}
}

// The point of all of this: a chat has to be able to book a task, because that
// is what the user is asking for when they say "shut the laptop down in ten
// minutes". The turn's tool list is where that either happens or does not.
func TestAChatTurnIsOfferedTheScheduleTool(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "calabrass.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	s := &Server{store: st, log: slog.Default()}
	setup := s.setupTurn(context.Background(), nil, "chat-1")

	for _, tool := range setup.tools {
		if tool.Name == "schedule_task" {
			return
		}
	}
	t.Fatal("a chat turn was not offered the schedule tool")
}

// callSchedules makes one request and returns the list it answers with.
func callSchedules(t *testing.T, server *httptest.Server, token, method, path, body string) []store.ScheduledTask {
	t.Helper()

	req, err := http.NewRequest(method, server.URL+path, strings.NewReader(body))
	if err != nil {
		t.Fatalf("build %s %s: %v", method, path, err)
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	res, err := server.Client().Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer func() { _ = res.Body.Close() }()

	if res.StatusCode != http.StatusOK {
		t.Fatalf("%s %s answered %s", method, path, res.Status)
	}

	var payload struct {
		Tasks []store.ScheduledTask `json:"tasks"`
	}
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatalf("read the answer to %s %s: %v", method, path, err)
	}
	return payload.Tasks
}
