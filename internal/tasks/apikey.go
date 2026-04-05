package tasks

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/hibiken/asynq"
	"gorm.io/gorm"

	"github.com/ziraloop/ziraloop/internal/model"
)

// APIKeyHandler handles TypeAPIKeyUpdate tasks by updating last_used_at.
type APIKeyHandler struct {
	db *gorm.DB
}

// NewAPIKeyHandler creates a new handler for API key last-used updates.
func NewAPIKeyHandler(db *gorm.DB) *APIKeyHandler {
	return &APIKeyHandler{db: db}
}

// Handle updates the API key's last_used_at timestamp.
func (h *APIKeyHandler) Handle(ctx context.Context, t *asynq.Task) error {
	var p APIKeyUpdatePayload
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return fmt.Errorf("unmarshal payload: %w", err)
	}

	if err := h.db.WithContext(ctx).
		Model(&model.APIKey{}).
		Where("id = ?", p.KeyID).
		Update("last_used_at", time.Now()).Error; err != nil {
		return fmt.Errorf("update api key last_used_at: %w", err)
	}

	return nil
}
