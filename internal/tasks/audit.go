package tasks

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hibiken/asynq"
	"gorm.io/gorm"
)

// AuditHandler handles TypeAuditWrite tasks by inserting audit entries.
type AuditHandler struct {
	db *gorm.DB
}

// NewAuditHandler creates a new handler for audit log writes.
func NewAuditHandler(db *gorm.DB) *AuditHandler {
	return &AuditHandler{db: db}
}

// Handle inserts the audit entry into the database.
func (h *AuditHandler) Handle(ctx context.Context, t *asynq.Task) error {
	var p AuditWritePayload
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return fmt.Errorf("unmarshal payload: %w", err)
	}

	if err := h.db.WithContext(ctx).Create(&p.Entry).Error; err != nil {
		return fmt.Errorf("create audit entry: %w", err)
	}

	return nil
}
