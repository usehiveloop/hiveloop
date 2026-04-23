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

// AuditWritePayload is the payload for TypeAuditWrite tasks.
type AuditWritePayload struct {
	Entry model.AuditEntry `json:"entry"`
}

// NewAuditWriteTask creates a task that writes an audit log entry.
func NewAuditWriteTask(entry model.AuditEntry) (*asynq.Task, error) {
	payload, err := json.Marshal(AuditWritePayload{Entry: entry})
	if err != nil {
		return nil, fmt.Errorf("marshal audit payload: %w", err)
	}
	return asynq.NewTask(
		TypeAuditWrite,
		payload,
		asynq.Queue(QueueBulk),
		asynq.MaxRetry(3),
		asynq.Timeout(10*time.Second),
	), nil
}

// GenerationWritePayload is the payload for TypeGenerationWrite tasks.
type GenerationWritePayload struct {
	Entry model.Generation `json:"entry"`
}

// NewGenerationWriteTask creates a task that writes a generation record.
func NewGenerationWriteTask(entry model.Generation) (*asynq.Task, error) {
	payload, err := json.Marshal(GenerationWritePayload{Entry: entry})
	if err != nil {
		return nil, fmt.Errorf("marshal generation payload: %w", err)
	}
	return asynq.NewTask(
		TypeGenerationWrite,
		payload,
		asynq.Queue(QueueBulk),
		asynq.MaxRetry(3),
		asynq.Timeout(10*time.Second),
	), nil
}
