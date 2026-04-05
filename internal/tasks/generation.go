package tasks

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hibiken/asynq"
	"gorm.io/gorm"
)

// GenerationHandler handles TypeGenerationWrite tasks by inserting generation records.
type GenerationHandler struct {
	db *gorm.DB
}

// NewGenerationHandler creates a new handler for generation record writes.
func NewGenerationHandler(db *gorm.DB) *GenerationHandler {
	return &GenerationHandler{db: db}
}

// Handle inserts the generation record into the database.
func (h *GenerationHandler) Handle(ctx context.Context, t *asynq.Task) error {
	var p GenerationWritePayload
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return fmt.Errorf("unmarshal payload: %w", err)
	}

	if err := h.db.WithContext(ctx).Create(&p.Entry).Error; err != nil {
		return fmt.Errorf("create generation record: %w", err)
	}

	return nil
}
