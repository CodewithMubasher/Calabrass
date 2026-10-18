package store

import (
	"context"
	"fmt"
	"time"
)

// UsageEvent is one model call. Tokens are what the provider reported; a call
// that failed before the model answered is recorded with zero tokens only when
// it still reached the provider.
type UsageEvent struct {
	ProviderID   string
	ModelID      string
	InputTokens  int
	OutputTokens int
	CreatedAt    int64
}

// UsageTotals is a count of requests and the tokens they reported.
type UsageTotals struct {
	Requests     int64 `json:"requests"`
	InputTokens  int64 `json:"inputTokens"`
	OutputTokens int64 `json:"outputTokens"`
	TotalTokens  int64 `json:"totalTokens"`
}

// RecordUsage appends one model call.
func (s *Store) RecordUsage(ctx context.Context, event UsageEvent) error {
	if event.CreatedAt == 0 {
		event.CreatedAt = time.Now().UnixMilli()
	}
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO usage_events (provider_id, model_id, input_tokens, output_tokens, created_at)
		VALUES (?, ?, ?, ?, ?)`,
		event.ProviderID, event.ModelID, event.InputTokens, event.OutputTokens, event.CreatedAt,
	); err != nil {
		return fmt.Errorf("store: record usage: %w", err)
	}
	return nil
}

// UsageTotalsSince sums every call at or after since (Unix milliseconds).
func (s *Store) UsageTotalsSince(ctx context.Context, since int64) (UsageTotals, error) {
	return queryUsageTotals(s.db.QueryRowContext(ctx, `
		SELECT COUNT(*),
		       COALESCE(SUM(input_tokens), 0),
		       COALESCE(SUM(output_tokens), 0)
		FROM usage_events
		WHERE created_at >= ?`, since))
}

// UsageByProviderSince groups the calls since a time by provider.
func (s *Store) UsageByProviderSince(ctx context.Context, since int64) (map[string]UsageTotals, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT provider_id, COUNT(*),
		       COALESCE(SUM(input_tokens), 0),
		       COALESCE(SUM(output_tokens), 0)
		FROM usage_events
		WHERE created_at >= ?
		GROUP BY provider_id`, since)
	if err != nil {
		return nil, fmt.Errorf("store: usage by provider: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := map[string]UsageTotals{}
	for rows.Next() {
		var (
			providerID string
			totals     UsageTotals
		)
		if err := rows.Scan(&providerID, &totals.Requests, &totals.InputTokens, &totals.OutputTokens); err != nil {
			return nil, fmt.Errorf("store: read usage by provider: %w", err)
		}
		totals.TotalTokens = totals.InputTokens + totals.OutputTokens
		out[providerID] = totals
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: read usage by provider: %w", err)
	}
	return out, nil
}

// UsageDailySince groups the calls since a time by the local calendar day,
// keyed as YYYY-MM-DD.
func (s *Store) UsageDailySince(ctx context.Context, since int64) (map[string]UsageTotals, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT date(created_at / 1000, 'unixepoch', 'localtime') AS day,
		       COUNT(*),
		       COALESCE(SUM(input_tokens), 0),
		       COALESCE(SUM(output_tokens), 0)
		FROM usage_events
		WHERE created_at >= ?
		GROUP BY day`, since)
	if err != nil {
		return nil, fmt.Errorf("store: daily usage: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := map[string]UsageTotals{}
	for rows.Next() {
		var (
			day    string
			totals UsageTotals
		)
		if err := rows.Scan(&day, &totals.Requests, &totals.InputTokens, &totals.OutputTokens); err != nil {
			return nil, fmt.Errorf("store: read daily usage: %w", err)
		}
		totals.TotalTokens = totals.InputTokens + totals.OutputTokens
		out[day] = totals
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: read daily usage: %w", err)
	}
	return out, nil
}

// UsageDailyByProviderSince is UsageDailySince split one level further.
func (s *Store) UsageDailyByProviderSince(ctx context.Context, since int64) (map[string]map[string]UsageTotals, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT provider_id,
		       date(created_at / 1000, 'unixepoch', 'localtime') AS day,
		       COUNT(*),
		       COALESCE(SUM(input_tokens), 0),
		       COALESCE(SUM(output_tokens), 0)
		FROM usage_events
		WHERE created_at >= ?
		GROUP BY provider_id, day`, since)
	if err != nil {
		return nil, fmt.Errorf("store: daily usage by provider: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := map[string]map[string]UsageTotals{}
	for rows.Next() {
		var (
			providerID string
			day        string
			totals     UsageTotals
		)
		if err := rows.Scan(&providerID, &day, &totals.Requests, &totals.InputTokens, &totals.OutputTokens); err != nil {
			return nil, fmt.Errorf("store: read daily usage by provider: %w", err)
		}
		totals.TotalTokens = totals.InputTokens + totals.OutputTokens
		if out[providerID] == nil {
			out[providerID] = map[string]UsageTotals{}
		}
		out[providerID][day] = totals
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: read daily usage by provider: %w", err)
	}
	return out, nil
}

type usageScanner interface {
	Scan(dest ...any) error
}

func queryUsageTotals(row usageScanner) (UsageTotals, error) {
	var totals UsageTotals
	if err := row.Scan(&totals.Requests, &totals.InputTokens, &totals.OutputTokens); err != nil {
		return UsageTotals{}, fmt.Errorf("store: read usage totals: %w", err)
	}
	totals.TotalTokens = totals.InputTokens + totals.OutputTokens
	return totals, nil
}
