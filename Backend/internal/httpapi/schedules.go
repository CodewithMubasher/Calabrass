package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	goai "github.com/zendev-sh/goai"

	"github.com/CodewithMubasher/Calabrass/backend/internal/store"
)

// schedulePoll is how often an open Schedules page re-reads the tasks, and how
// long after a change it can take to appear. The list is small and read from a
// local database, so this is cheap; the point is that a task the model booked a
// moment ago, or one that has just run, turns up without a refresh.
const schedulePoll = 2 * time.Second

// --- endpoints --------------------------------------------------------------

func (s *Server) handleListSchedules(w http.ResponseWriter, r *http.Request) {
	s.writeSchedules(w, r)
}

func (s *Server) handleCreateSchedule(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r.Context())
	var req struct {
		Prompt string `json:"prompt"`
		RunAt  int64  `json:"runAt"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	prompt := strings.TrimSpace(req.Prompt)
	if prompt == "" {
		writeError(w, http.StatusBadRequest, errors.New("a prompt is required"))
		return
	}
	if req.RunAt <= 0 {
		writeError(w, http.StatusBadRequest, errors.New("a time is required"))
		return
	}

	if _, err := s.store.CreateScheduledTask(r.Context(), store.ScheduledTask{
		UserID:  user.ID,
		Prompt:  prompt,
		RunAt:   req.RunAt,
		Enabled: true,
	}); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	s.writeSchedules(w, r)
}

func (s *Server) handleUpdateSchedule(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r.Context())
	var req struct {
		Prompt  *string `json:"prompt"`
		RunAt   *int64  `json:"runAt"`
		Enabled *bool   `json:"enabled"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	ctx := r.Context()
	task, err := s.store.GetScheduledTask(ctx, r.PathValue("taskID"), user.ID)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, err)
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	task.UserID = user.ID

	if req.Prompt != nil {
		prompt := strings.TrimSpace(*req.Prompt)
		if prompt == "" {
			writeError(w, http.StatusBadRequest, errors.New("a prompt is required"))
			return
		}
		task.Prompt = prompt
	}
	if req.RunAt != nil && *req.RunAt != task.RunAt {
		// A new time is a new task: it is armed again, and the run that
		// already happened no longer describes it.
		task.RunAt = *req.RunAt
		task.LastRunAt, task.LastStatus, task.LastError = 0, "", ""
	}
	if req.Enabled != nil {
		task.Enabled = *req.Enabled
	}

	if _, err := s.store.UpdateScheduledTask(ctx, task); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	s.writeSchedules(w, r)
}

func (s *Server) handleDeleteSchedule(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r.Context())
	err := s.store.DeleteScheduledTask(r.Context(), r.PathValue("taskID"), user.ID)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, err)
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	s.writeSchedules(w, r)
}

// handleScheduleStream pushes the whole list whenever it changes, and once when
// the connection opens. It is what lets the page show a task running — or being
// booked by the model — while the user is looking at it.
func (s *Server) handleScheduleStream(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user := currentUser(ctx)

	stream, err := newSSEWriter(w)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	ticker := time.NewTicker(schedulePoll)
	defer ticker.Stop()

	sent := ""
	for {
		tasks, err := s.store.ListScheduledTasks(ctx, user.ID)
		if err != nil {
			_ = stream.event("error", errorPayload{Message: err.Error()})
			return
		}
		payload, err := json.Marshal(map[string]any{"tasks": tasks})
		if err != nil {
			return
		}
		// Only on a change: the page is redrawn from a snapshot, not streamed
		// row by row.
		if string(payload) != sent {
			sent = string(payload)
			if err := stream.event("schedules", json.RawMessage(payload)); err != nil {
				return
			}
		}

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// writeSchedules answers with the whole list, which is what the page keeps in
// its store. A mutation answers the same way as a read, so the store has one
// shape to hold.
func (s *Server) writeSchedules(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r.Context())
	tasks, err := s.store.ListScheduledTasks(r.Context(), user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"tasks": tasks})
}

// --- the tool ---------------------------------------------------------------

// scheduleTaskArgs is the wire shape of schedule_task: what to do, and when —
// either a clock time or a delay from now.
//
// A comma in a jsonschema tag truncates the description the model is given, so
// these are written without one.
type scheduleTaskArgs struct {
	Prompt string `json:"prompt" jsonschema:"description=What to do when the time comes written as an instruction to yourself: Shut the laptop down"`
	RunAt  string `json:"run_at,omitempty" jsonschema:"description=When to run it as the local time of the machine you are on. Either a full time like 2026-10-17T21:35 or a clock time like 21:35 for the next time the clock reads it. Leave this out when you give after_minutes"`
	// AfterMinutes is a delay rather than a time, which is what "in ten
	// minutes" comes to without any arithmetic.
	AfterMinutes int `json:"after_minutes,omitempty" jsonschema:"description=How many minutes from now to run it. Leave this out when you give run_at"`
}

// scheduleTool lets a chat book work for later. It is added to a turn rather
// than to the file tools in the agent package: a task is a row in the database
// and a turn of its own, not something done to the project.
func (s *Server) scheduleTool(chatID, userID string) goai.Tool {
	return goai.NewTool("schedule_task",
		"Set a timer that runs one instruction later — in a few minutes, or at a clock time. "+
			"Use it whenever the user asks for something to happen after a delay or at a given time, "+
			"including anything about their own machine, which the instruction can still do when it runs. "+
			"The instruction runs in this conversation with every tool available to you. "+
			"Tell the user the time it is set for, in their own clock.",
		func(ctx context.Context, args scheduleTaskArgs) (string, error) {
			return s.scheduleFromTool(ctx, chatID, userID, args)
		})
}

// scheduleFromTool books one task and reports it back to the model, which is
// what it tells the user.
func (s *Server) scheduleFromTool(ctx context.Context, chatID, userID string, args scheduleTaskArgs) (string, error) {
	prompt := strings.TrimSpace(args.Prompt)
	if prompt == "" {
		return "", errors.New("no instruction was given to run later")
	}

	now := time.Now()
	runAt, err := resolveRunAt(now, args.RunAt, args.AfterMinutes)
	if err != nil {
		return "", err
	}
	if !runAt.After(now) {
		return "", fmt.Errorf("%s has already passed on this machine, which is %s now",
			runAt.Format("15:04"), now.Format("15:04"))
	}

	if _, err := s.store.CreateScheduledTask(ctx, store.ScheduledTask{
		UserID:  userID,
		Prompt:  prompt,
		ChatID:  chatID,
		RunAt:   runAt.UnixMilli(),
		Enabled: true,
	}); err != nil {
		return "", err
	}

	return fmt.Sprintf("Scheduled: %s. It runs %s — %s.",
		prompt,
		humanUntil(now, runAt),
		runAt.Format("Monday 2 January at 15:04"),
	), nil
}

// resolveRunAt turns what the model asked for into a moment on this machine's
// clock: a delay from now, or a time it named.
func resolveRunAt(now time.Time, at string, afterMinutes int) (time.Time, error) {
	at = strings.TrimSpace(at)
	if at == "" && afterMinutes <= 0 {
		return time.Time{}, errors.New("a time is needed: give run_at or after_minutes")
	}
	if afterMinutes > 0 {
		return now.Add(time.Duration(afterMinutes) * time.Minute), nil
	}

	for _, layout := range []string{
		"2006-01-02T15:04:05",
		"2006-01-02T15:04",
		"2006-01-02 15:04",
		"2006-01-02",
	} {
		if parsed, err := time.ParseInLocation(layout, at, time.Local); err == nil {
			return parsed, nil
		}
	}

	// A bare clock time is the next time the clock reads it, which is what "at
	// nine" means in the evening and "at nine tomorrow" means after nine.
	if parsed, err := time.ParseInLocation("15:04", at, time.Local); err == nil {
		next := time.Date(now.Year(), now.Month(), now.Day(),
			parsed.Hour(), parsed.Minute(), 0, 0, time.Local)
		if !next.After(now) {
			next = next.Add(24 * time.Hour)
		}
		return next, nil
	}

	return time.Time{}, fmt.Errorf("%q is not a time I can read; give it like 2026-10-17T21:35 or 21:35", at)
}

// humanUntil says how far off a moment is, the way a person would.
func humanUntil(from, to time.Time) string {
	d := to.Sub(from)
	switch {
	case d < time.Minute:
		return "in under a minute"
	case d < time.Hour:
		return fmt.Sprintf("in %d minutes", int(d.Minutes()+0.5))
	case d < 36*time.Hour:
		return fmt.Sprintf("in %d hours", int(d.Hours()+0.5))
	default:
		return fmt.Sprintf("in %d days", int(d.Hours()/24+0.5))
	}
}
