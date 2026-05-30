package tasks

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
)

const employeeSandboxUpgradeTimeout = 90 * time.Minute
const employeeSandboxRetireDelay = 24 * time.Hour

type EmployeeSandboxUpgradePayload struct {
	UpgradeID  uuid.UUID `json:"upgrade_id"`
	EmployeeID uuid.UUID `json:"employee_id"`
}

type EmployeeSandboxRetirePayload struct {
	UpgradeID  uuid.UUID `json:"upgrade_id"`
	EmployeeID uuid.UUID `json:"employee_id"`
	SandboxID  uuid.UUID `json:"sandbox_id"`
}

func NewEmployeeSandboxUpgradeTask(upgradeID, agentID uuid.UUID) (*asynq.Task, []asynq.Option, error) {
	payload, err := json.Marshal(EmployeeSandboxUpgradePayload{
		UpgradeID:  upgradeID,
		EmployeeID: agentID,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("marshal employee sandbox upgrade payload: %w", err)
	}
	opts := []asynq.Option{
		asynq.Queue(QueueBulk),
		asynq.MaxRetry(0),
		asynq.Timeout(employeeSandboxUpgradeTimeout),
		asynq.TaskID(EmployeeSandboxUpgradeTaskID(agentID)),
	}
	return asynq.NewTask(TypeEmployeeSandboxUpgrade, payload), opts, nil
}

func EmployeeSandboxUpgradeTaskID(agentID uuid.UUID) string {
	return "employee-sandbox-upgrade:" + agentID.String()
}

func NewEmployeeSandboxRetireTask(payload EmployeeSandboxRetirePayload) (*asynq.Task, []asynq.Option, error) {
	if payload.UpgradeID == uuid.Nil || payload.EmployeeID == uuid.Nil || payload.SandboxID == uuid.Nil {
		return nil, nil, fmt.Errorf("employee sandbox retire payload missing ids")
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal employee sandbox retire payload: %w", err)
	}
	opts := []asynq.Option{
		asynq.Queue(QueueDefault),
		asynq.MaxRetry(3),
		asynq.Timeout(2 * time.Minute),
		asynq.ProcessIn(employeeSandboxRetireDelay),
		asynq.TaskID("employee-sandbox-retire:" + payload.SandboxID.String()),
	}
	return asynq.NewTask(TypeEmployeeSandboxRetire, body), opts, nil
}
