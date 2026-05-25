package tasks

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
)

type EmployeeMemoryRetainPayload struct {
	EmployeeID        uuid.UUID `json:"employee_id"`
	SandboxID         uuid.UUID `json:"sandbox_id"`
	EmployeeSessionID uuid.UUID `json:"employee_session_id,omitempty"`
	SessionID         string    `json:"session_id"`
	Reason            string    `json:"reason,omitempty"`
	SourceEvent       string    `json:"source_event,omitempty"`
}

func NewEmployeeMemoryRetainTask(payload EmployeeMemoryRetainPayload) (*asynq.Task, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal employee memory retain payload: %w", err)
	}
	return asynq.NewTask(
		TypeEmployeeMemoryRetain,
		body,
		asynq.Queue(QueueDefault),
		asynq.MaxRetry(3),
		asynq.Timeout(4*time.Minute),
	), nil
}

type EmployeeMemoryRefreshPayload struct {
	EmployeeID uuid.UUID `json:"employee_id"`
	SandboxID  uuid.UUID `json:"sandbox_id,omitempty"`
	Reason     string    `json:"reason,omitempty"`
}

func NewEmployeeMemoryRefreshTask(payload EmployeeMemoryRefreshPayload) (*asynq.Task, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal employee memory refresh payload: %w", err)
	}
	return asynq.NewTask(
		TypeEmployeeMemoryRefresh,
		body,
		asynq.Queue(QueueDefault),
		asynq.MaxRetry(3),
		asynq.Timeout(2*time.Minute),
	), nil
}
