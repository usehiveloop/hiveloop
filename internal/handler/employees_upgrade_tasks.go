package handler

import (
	"errors"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"

	"github.com/usehiveloop/hiveloop/internal/tasks"
)

func (h *EmployeeHandler) deleteStaleEmployeeSandboxUpgradeTask(agentID uuid.UUID) error {
	if h.taskCleaner == nil {
		return nil
	}
	err := h.taskCleaner.DeleteTask(tasks.QueueBulk, tasks.EmployeeSandboxUpgradeTaskID(agentID))
	if errors.Is(err, asynq.ErrTaskNotFound) || errors.Is(err, asynq.ErrQueueNotFound) {
		return nil
	}
	return err
}
