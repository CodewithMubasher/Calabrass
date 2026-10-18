package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// ScheduledTask is one instruction to run at a time: what to do, the
// conversation it speaks in, and how its last run went.
//
// It is a timer rather than a cron: it runs once, and the row remembers that it
// has. Editing the time arms it again (see UpdateScheduledTask).
type ScheduledTask struct {
	ID string `json:"id"`
	// UserID is the account whose conversation the task speaks in. It is not
	// sent to the client: the session already says who is asking.
	UserID string `json:"-"`
	Prompt string `json:"prompt"`
	// ChatID is the conversation the task speaks in. Empty means it has not run
	// yet, and will be given one of its own when it does.
	ChatID string `json:"chatId"`
	// RunAt is when it is due, in Unix milliseconds.
	RunAt   int64 `json:"runAt"`
	Enabled bool  `json:"enabled"`
	// LastRunAt is when it last ran, zero while it has not. LastStatus is "ok",
	// "error" or "missed" and LastError is what went wrong.
	LastRunAt  int64  `json:"lastRunAt"`
	LastStatus string `json:"lastStatus"`
	LastError  string `json:"lastError"`
	CreatedAt  int64  `json:"createdAt"`
	UpdatedAt  int64  `json:"updatedAt"`
}

// HasRun reports whether this task has already had its turn.
func (t ScheduledTask) HasRun() bool { return t.LastRunAt > 0 }

const scheduledTaskColumns = `id, user_id, prompt, chat_id, run_at, enabled,
	last_run_at, last_status, last_error, created_at, updated_at`

func scanScheduledTask(row interface{ Scan(...any) error }) (ScheduledTask, error) {
	var (
		task    ScheduledTask
		enabled int
	)
	if err := row.Scan(
		&task.ID, &task.UserID, &task.Prompt, &task.ChatID, &task.RunAt, &enabled,
		&task.LastRunAt, &task.LastStatus, &task.LastError,
		&task.CreatedAt, &task.UpdatedAt,
	); err != nil {
		return ScheduledTask{}, err
	}
	task.Enabled = enabled != 0
	return task, nil
}

// ListScheduledTasks returns every task for an account, the next one due first
// and the ones that have already run at the end.
func (s *Store) ListScheduledTasks(ctx context.Context, userID ...string) ([]ScheduledTask, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+scheduledTaskColumns+`
		FROM scheduled_tasks
		WHERE user_id = ?
		ORDER BY last_run_at > 0 ASC, run_at ASC, rowid ASC`, ownerOf(userID))
	if err != nil {
		return nil, fmt.Errorf("store: list scheduled tasks: %w", err)
	}
	defer func() { _ = rows.Close() }()

	tasks := []ScheduledTask{}
	for rows.Next() {
		task, err := scanScheduledTask(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan scheduled task: %w", err)
		}
		tasks = append(tasks, task)
	}
	return tasks, rows.Err()
}

// GetScheduledTask returns one task by id for an account.
func (s *Store) GetScheduledTask(ctx context.Context, id string, userID ...string) (ScheduledTask, error) {
	task, err := scanScheduledTask(s.db.QueryRowContext(ctx,
		`SELECT `+scheduledTaskColumns+` FROM scheduled_tasks WHERE id = ? AND user_id = ?`, id, ownerOf(userID)))
	if errors.Is(err, sql.ErrNoRows) {
		return ScheduledTask{}, ErrNotFound
	}
	if err != nil {
		return ScheduledTask{}, fmt.Errorf("store: get scheduled task: %w", err)
	}
	return task, nil
}

// CreateScheduledTask stores a new task, ready to run.
func (s *Store) CreateScheduledTask(ctx context.Context, task ScheduledTask) (ScheduledTask, error) {
	now := time.Now().UnixMilli()
	task.ID = NewID()
	task.CreatedAt, task.UpdatedAt = now, now

	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO scheduled_tasks (
			id, user_id, prompt, chat_id, run_at, enabled,
			last_run_at, last_status, last_error, created_at, updated_at
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		task.ID, task.UserID, task.Prompt, task.ChatID, task.RunAt, boolToInt(task.Enabled),
		task.LastRunAt, task.LastStatus, task.LastError, task.CreatedAt, task.UpdatedAt,
	); err != nil {
		return ScheduledTask{}, fmt.Errorf("store: create scheduled task: %w", err)
	}
	return task, nil
}

// UpdateScheduledTask writes a task's editable fields back: what it says, when
// it is due, whether it is armed, and who it has run as. The caller decides
// whether the run history still applies — a new time clears it, so the task is
// armed again.
func (s *Store) UpdateScheduledTask(ctx context.Context, task ScheduledTask) (ScheduledTask, error) {
	task.UpdatedAt = time.Now().UnixMilli()

	res, err := s.db.ExecContext(ctx, `
		UPDATE scheduled_tasks SET
			prompt = ?, chat_id = ?, run_at = ?, enabled = ?,
			last_run_at = ?, last_status = ?, last_error = ?, updated_at = ?
		WHERE id = ? AND user_id = ?`,
		task.Prompt, task.ChatID, task.RunAt, boolToInt(task.Enabled),
		task.LastRunAt, task.LastStatus, task.LastError, task.UpdatedAt, task.ID, task.UserID)
	if err != nil {
		return ScheduledTask{}, fmt.Errorf("store: update scheduled task: %w", err)
	}
	if affected, err := res.RowsAffected(); err == nil && affected == 0 {
		return ScheduledTask{}, ErrNotFound
	}
	return task, nil
}

// BindScheduledTaskChat remembers the conversation a task ran in, so the task
// keeps reporting into it rather than opening a new one each time.
func (s *Store) BindScheduledTaskChat(ctx context.Context, id, chatID string, userID ...string) error {
	if _, err := s.db.ExecContext(ctx,
		`UPDATE scheduled_tasks SET chat_id = ? WHERE id = ? AND user_id = ?`,
		chatID, id, ownerOf(userID)); err != nil {
		return fmt.Errorf("store: bind scheduled task chat: %w", err)
	}
	return nil
}

// DueScheduledTasks returns the tasks that are due at now and have not run yet,
// soonest first. A task that is turned off is not due.
func (s *Store) DueScheduledTasks(ctx context.Context, now int64, limit int) ([]ScheduledTask, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+scheduledTaskColumns+`
		FROM scheduled_tasks
		WHERE enabled = 1 AND last_run_at = 0 AND run_at <= ?
		ORDER BY run_at ASC, rowid ASC
		LIMIT ?`, now, limit)
	if err != nil {
		return nil, fmt.Errorf("store: due scheduled tasks: %w", err)
	}
	defer func() { _ = rows.Close() }()

	tasks := []ScheduledTask{}
	for rows.Next() {
		task, err := scanScheduledTask(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan scheduled task: %w", err)
		}
		tasks = append(tasks, task)
	}
	return tasks, rows.Err()
}

// MarkScheduledTask records one run: when it happened, and how it went. A task
// that has been marked does not come due again.
func (s *Store) MarkScheduledTask(ctx context.Context, id string, at int64, status, message string) error {
	if _, err := s.db.ExecContext(ctx, `
		UPDATE scheduled_tasks
		SET last_run_at = ?, last_status = ?, last_error = ?, updated_at = ?
		WHERE id = ?`,
		at, status, message, time.Now().UnixMilli(), id); err != nil {
		return fmt.Errorf("store: mark scheduled task: %w", err)
	}
	return nil
}

// DeleteScheduledTask removes a task owned by an account.
func (s *Store) DeleteScheduledTask(ctx context.Context, id string, userID ...string) error {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM scheduled_tasks WHERE id = ? AND user_id = ?`, id, ownerOf(userID))
	if err != nil {
		return fmt.Errorf("store: delete scheduled task: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: delete scheduled task: %w", err)
	}
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}
