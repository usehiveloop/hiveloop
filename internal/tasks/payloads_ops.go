package tasks

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
)

// BillingUsageEventPayload is the payload for TypeBillingUsageEvent tasks.
type BillingUsageEventPayload struct {
	OrgID       uuid.UUID `json:"org_id"`
	AgentID     uuid.UUID `json:"agent_id"`
	SandboxType string    `json:"sandbox_type"`
	RunID       uuid.UUID `json:"run_id"`
}

// NewBillingUsageEventTask creates a task that sends a usage event to Polar.
func NewBillingUsageEventTask(orgID, agentID, runID uuid.UUID, sandboxType string) (*asynq.Task, error) {
	payload, err := json.Marshal(BillingUsageEventPayload{
		OrgID:       orgID,
		AgentID:     agentID,
		SandboxType: sandboxType,
		RunID:       runID,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal billing usage event payload: %w", err)
	}
	return asynq.NewTask(
		TypeBillingUsageEvent,
		payload,
		asynq.Queue(QueueDefault),
		asynq.MaxRetry(5),
		asynq.Timeout(15*time.Second),
	), nil
}

// AgentCleanupPayload is the payload for TypeAgentCleanup tasks.
type AgentCleanupPayload struct {
	AgentID uuid.UUID `json:"agent_id"`
}

// NewAgentCleanupTask creates a task that cleans up an agent's sandboxes and then hard-deletes it.
func NewAgentCleanupTask(agentID uuid.UUID) (*asynq.Task, error) {
	payload, err := json.Marshal(AgentCleanupPayload{AgentID: agentID})
	if err != nil {
		return nil, fmt.Errorf("marshal agent cleanup payload: %w", err)
	}
	return asynq.NewTask(
		TypeAgentCleanup,
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
// tracked ref and writes a new SkillVersion.
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
