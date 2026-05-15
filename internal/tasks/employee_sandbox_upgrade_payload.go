package tasks

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
)

const employeeSandboxUpgradeTimeout = 90 * time.Minute

type EmployeeSandboxUpgradePayload struct {
	UpgradeID uuid.UUID `json:"upgrade_id"`
	AgentID   uuid.UUID `json:"agent_id"`
	SmokeTest bool      `json:"smoke_test,omitempty"`
}

func NewEmployeeSandboxUpgradeTask(upgradeID, agentID uuid.UUID, smokeTest bool) (*asynq.Task, []asynq.Option, error) {
	payload, err := json.Marshal(EmployeeSandboxUpgradePayload{
		UpgradeID: upgradeID,
		AgentID:   agentID,
		SmokeTest: smokeTest,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("marshal employee sandbox upgrade payload: %w", err)
	}
	opts := []asynq.Option{
		asynq.Queue(QueueBulk),
		asynq.MaxRetry(0),
		asynq.Timeout(employeeSandboxUpgradeTimeout),
		asynq.TaskID("employee-sandbox-upgrade:" + agentID.String()),
	}
	return asynq.NewTask(TypeEmployeeSandboxUpgrade, payload), opts, nil
}
