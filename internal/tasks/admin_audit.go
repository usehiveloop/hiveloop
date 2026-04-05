package tasks

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hibiken/asynq"
	"gorm.io/gorm"
)

// AdminAuditHandler handles TypeAdminAuditWrite tasks by inserting admin audit entries.
type AdminAuditHandler struct {
	db *gorm.DB
}

// NewAdminAuditHandler creates a new handler for admin audit log writes.
func NewAdminAuditHandler(db *gorm.DB) *AdminAuditHandler {
	return &AdminAuditHandler{db: db}
}

// Handle inserts the admin audit entry into the database.
func (h *AdminAuditHandler) Handle(ctx context.Context, t *asynq.Task) error {
	var p AdminAuditWritePayload
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return fmt.Errorf("unmarshal payload: %w", err)
	}

	if err := h.db.WithContext(ctx).Create(&p.Entry).Error; err != nil {
		return fmt.Errorf("create admin audit entry: %w", err)
	}

	return nil
}
