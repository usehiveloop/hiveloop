package tasks

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/hibiken/asynq"

	"github.com/usehiveloop/hiveloop/internal/model"
)

// AdminAuditWritePayload is the payload for TypeAdminAuditWrite tasks.
type AdminAuditWritePayload struct {
	Entry model.AdminAuditEntry `json:"entry"`
}

// NewAdminAuditWriteTask creates a task that writes an admin audit log entry.
func NewAdminAuditWriteTask(entry model.AdminAuditEntry) (*asynq.Task, error) {
	payload, err := json.Marshal(AdminAuditWritePayload{Entry: entry})
	if err != nil {
		return nil, fmt.Errorf("marshal admin audit payload: %w", err)
	}
	return asynq.NewTask(
		TypeAdminAuditWrite,
		payload,
		asynq.Queue(QueueBulk),
		asynq.MaxRetry(3),
		asynq.Timeout(10*time.Second),
	), nil
}
