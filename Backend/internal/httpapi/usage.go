package httpapi

import (
	"context"
	"net/http"
	"time"

	"github.com/CodewithMubasher/Calabrass/backend/internal/ai"
	"github.com/CodewithMubasher/Calabrass/backend/internal/store"
)

// usageDayCount is how many calendar days the weekly chart covers: today and
// the six before it.
const usageDayCount = 7

type usageDayPayload struct {
	Date string `json:"date"`
	store.UsageTotals
}

type usageModelPayload struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	RPMLimit int    `json:"rpmLimit"`
	RPDLimit int    `json:"rpdLimit"`
	TPMLimit int    `json:"tpmLimit"`
	Status   string `json:"status"`
}

type usageProviderPayload struct {
	store.UsageTotals
	Minute   store.UsageTotals   `json:"minute"`
	ID       string              `json:"id"`
	Name     string              `json:"name"`
	Enabled  bool                `json:"enabled"`
	RPDLimit int                 `json:"rpdLimit"`
	TPMLimit int                 `json:"tpmLimit"`
	Models   []usageModelPayload `json:"models"`
	Daily    []usageDayPayload   `json:"daily"`
}

// handleUsage returns the real figures the Usage page shows: how much was used
// today, how much of the free-tier allowance is left, and the last seven days
// of activity. It is a plain snapshot; the page asks for a new one on a timer.
func (s *Server) handleUsage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	now := time.Now()
	startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	sinceDay := startOfDay.UnixMilli()
	sinceWeek := startOfDay.AddDate(0, 0, -(usageDayCount - 1)).UnixMilli()
	sinceMinute := now.Add(-time.Minute).UnixMilli()

	today, err := s.store.UsageTotalsSince(ctx, sinceDay)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	minute, err := s.store.UsageTotalsSince(ctx, sinceMinute)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	byProvider, err := s.store.UsageByProviderSince(ctx, sinceDay)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	minuteByProvider, err := s.store.UsageByProviderSince(ctx, sinceMinute)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	daily, err := s.store.UsageDailySince(ctx, sinceWeek)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	dailyByProvider, err := s.store.UsageDailyByProviderSince(ctx, sinceWeek)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	providers := []usageProviderPayload{}
	totalRPD, totalTPM := 0, 0
	if s.registry != nil {
		for _, provider := range s.registry.Providers(ctx) {
			rpd, tpm := s.quotas.Limits(provider.ID)
			if provider.Enabled {
				totalRPD += rpd
				totalTPM += tpm
			}

			models := []usageModelPayload{}
			if block, ok := s.quotas.Provider(provider.ID); ok {
				for _, model := range block.Models {
					models = append(models, usageModelPayload{
						ID:       model.ID,
						Name:     model.Name,
						RPMLimit: model.RPMLimit,
						RPDLimit: model.RPDLimit,
						TPMLimit: model.TPMLimit,
						Status:   model.Status,
					})
				}
			}

			providers = append(providers, usageProviderPayload{
				UsageTotals: byProvider[provider.ID],
				Minute:      minuteByProvider[provider.ID],
				ID:          provider.ID,
				Name:        provider.Name,
				Enabled:     provider.Enabled,
				RPDLimit:    rpd,
				TPMLimit:    tpm,
				Models:      models,
				Daily:       daySeries(startOfDay, dailyByProvider[provider.ID]),
			})
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"today":       today,
		"minute":      minute,
		"limits":      map[string]int{"rpd": totalRPD, "tpm": totalTPM},
		"providers":   providers,
		"daily":       daySeries(startOfDay, daily),
		"generatedAt": now.UnixMilli(),
	})
}

// recordUsage writes one completed model call down for the Usage page. It runs
// detached from the request so a client that disconnected mid-answer still
// leaves its tokens behind.
func (s *Server) recordUsage(ctx context.Context, providerID, modelID string, usage ai.Usage) {
	if s.store == nil {
		return
	}
	event := store.UsageEvent{
		ProviderID:   providerID,
		ModelID:      modelID,
		InputTokens:  usage.InputTokens,
		OutputTokens: usage.OutputTokens,
		CreatedAt:    time.Now().UnixMilli(),
	}
	if err := s.store.RecordUsage(context.WithoutCancel(ctx), event); err != nil {
		s.log.Warn("could not record usage",
			"provider", providerID, "model", modelID, "error", err)
	}
}

// daySeries fills the last usageDayCount local days, oldest first, so the chart
// always has one entry per day rather than only the days that were used.
func daySeries(today time.Time, totals map[string]store.UsageTotals) []usageDayPayload {
	out := make([]usageDayPayload, 0, usageDayCount)
	for offset := usageDayCount - 1; offset >= 0; offset-- {
		day := today.AddDate(0, 0, -offset)
		key := day.Format("2006-01-02")
		out = append(out, usageDayPayload{Date: key, UsageTotals: totals[key]})
	}
	return out
}
