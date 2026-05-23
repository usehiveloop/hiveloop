package tasks

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
)

// EmployeeCleanupPayload is the payload for TypeEmployeeCleanup tasks.
type EmployeeCleanupPayload struct {
	EmployeeID         uuid.UUID `json:"employee_id"`
	SandboxExternalIDs []string  `json:"sandbox_external_ids,omitempty"`
}

// NewEmployeeCleanupTask creates a task that cleans up provider sandboxes left behind by an employee hard delete.
func NewEmployeeCleanupTask(employeeID uuid.UUID, sandboxExternalIDs ...string) (*asynq.Task, error) {
	payload, err := json.Marshal(EmployeeCleanupPayload{
		EmployeeID:         employeeID,
		SandboxExternalIDs: sandboxExternalIDs,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal employee cleanup payload: %w", err)
	}
	return asynq.NewTask(
		TypeEmployeeCleanup,
		payload,
		asynq.Queue(QueueDefault),
		asynq.MaxRetry(3),
		asynq.Timeout(2*time.Minute),
	), nil
}

// SandboxTemplateBuildPayload is the payload for TypeSandboxTemplateBuild tasks.
type SandboxTemplateBuildPayload struct {
	TemplateID uuid.UUID `json:"template_id"`
}

// NewSandboxTemplateBuildTask creates a task that builds a sandbox template snapshot.
func NewSandboxTemplateBuildTask(templateID uuid.UUID) (*asynq.Task, error) {
	payload, err := json.Marshal(SandboxTemplateBuildPayload{TemplateID: templateID})
	if err != nil {
		return nil, fmt.Errorf("marshal sandbox template build payload: %w", err)
	}
	return asynq.NewTask(
		TypeSandboxTemplateBuild,
		payload,
		asynq.Queue(QueueDefault),
		asynq.MaxRetry(2),
		asynq.Timeout(30*time.Minute),
	), nil
}

// SandboxTemplateRetryBuildPayload is the payload for retry tasks.
type SandboxTemplateRetryBuildPayload struct {
	TemplateID    uuid.UUID `json:"template_id"`
	BuildCommands []string  `json:"build_commands,omitempty"`
}

// NewSandboxTemplateRetryBuildTask creates a task that retries building a sandbox template.
func NewSandboxTemplateRetryBuildTask(templateID uuid.UUID, buildCommands []string) (*asynq.Task, error) {
	payload := SandboxTemplateRetryBuildPayload{
		TemplateID:    templateID,
		BuildCommands: buildCommands,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal sandbox template retry payload: %w", err)
	}
	return asynq.NewTask(
		TypeSandboxTemplateRetryBuild,
		data,
		asynq.Queue(QueueDefault),
		asynq.MaxRetry(2),
		asynq.Timeout(30*time.Minute),
	), nil
}

// SkillHydratePayload is the payload for TypeSkillHydrate tasks.
type SkillHydratePayload struct {
	SkillID uuid.UUID `json:"skill_id"`
}

// NewSkillHydrateTask creates a task that pulls a git-sourced skill at its
// tracked ref and updates the skill's current bundle.
func NewSkillHydrateTask(skillID uuid.UUID) (*asynq.Task, error) {
	payload, err := json.Marshal(SkillHydratePayload{SkillID: skillID})
	if err != nil {
		return nil, fmt.Errorf("marshal skill hydrate payload: %w", err)
	}
	return asynq.NewTask(
		TypeSkillHydrate,
		payload,
		asynq.Queue(QueueDefault),
		asynq.MaxRetry(3),
		asynq.Timeout(2*time.Minute),
	), nil
}
