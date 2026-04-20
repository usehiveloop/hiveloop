package tasks

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/hibiken/asynq"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/model"
	"github.com/usehiveloop/hiveloop/internal/skills"
)

// SkillHydrateHandler pulls a git-sourced skill and creates a SkillVersion.
type SkillHydrateHandler struct {
	db      *gorm.DB
	fetcher *skills.GitFetcher
}

// NewSkillHydrateHandler constructs a handler. The fetcher is shared across
// invocations so its underlying *http.Client can reuse connections.
func NewSkillHydrateHandler(db *gorm.DB, fetcher *skills.GitFetcher) *SkillHydrateHandler {
	return &SkillHydrateHandler{db: db, fetcher: fetcher}
}

// Handle runs one hydration job. On failure, the error message is persisted
// to the latest SkillVersion row (if any) for UI surfacing, and the task is
// returned so asynq will retry according to the task's MaxRetry setting.
func (h *SkillHydrateHandler) Handle(ctx context.Context, t *asynq.Task) error {
	var payload SkillHydratePayload
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		return fmt.Errorf("unmarshal payload: %w", err)
	}

	_, err := skills.HydrateFromGit(ctx, h.db, h.fetcher, payload.SkillID)
	if err != nil {
		msg := err.Error()
		now := time.Now()
		h.db.Model(&model.SkillVersion{}).
			Where("skill_id = ?", payload.SkillID).
			Order("created_at DESC").
			Limit(1).
			Updates(map[string]any{
				"hydration_error": msg,
				"hydrated_at":     now,
			})
		slog.Error("skill hydrate failed", "skill_id", payload.SkillID, "error", err)
		return fmt.Errorf("hydrate skill %s: %w", payload.SkillID, err)
	}

	slog.Info("skill hydrated", "skill_id", payload.SkillID)
	return nil
}
