package tasks

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
)

// AgentCleanupPayload is the payload for TypeAgentCleanup tasks.
type AgentCleanupPayload struct {
	AgentID            uuid.UUID `json:"agent_id"`
	SandboxExternalIDs []string  `json:"sandbox_external_ids,omitempty"`
}

// NewAgentCleanupTask creates a task that cleans up provider sandboxes left behind by an agent hard delete.
func NewAgentCleanupTask(agentID uuid.UUID, sandboxExternalIDs ...string) (*asynq.Task, error) {
	payload, err := json.Marshal(AgentCleanupPayload{
		AgentID:            agentID,
		SandboxExternalIDs: sandboxExternalIDs,
	})
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

// NangoConnectionDeleteTarget identifies one Nango connection to remove.
type NangoConnectionDeleteTarget struct {
	ConnectionID      string    `json:"connection_id"`
	ProviderConfigKey string    `json:"provider_config_key"`
	ProfileID         uuid.UUID `json:"profile_id,omitempty"`
	Provider          string    `json:"provider,omitempty"`
}

// NangoIntegrationDeleteTarget identifies one Nango integration to remove.
type NangoIntegrationDeleteTarget struct {
	ProviderConfigKey string    `json:"provider_config_key"`
	IntegrationID     uuid.UUID `json:"integration_id,omitempty"`
	Provider          string    `json:"provider,omitempty"`
}

// AgentProfileNangoCleanupPayload is the payload for TypeAgentProfileNangoCleanup tasks.
type AgentProfileNangoCleanupPayload struct {
	AgentID      uuid.UUID                      `json:"agent_id"`
	Connections  []NangoConnectionDeleteTarget  `json:"connections"`
	Integrations []NangoIntegrationDeleteTarget `json:"integrations,omitempty"`
}

// NewAgentProfileNangoCleanupTask creates a task that permanently deletes Nango
// connections captured from agent profiles before the agent row is hard-deleted.
func NewAgentProfileNangoCleanupTask(agentID uuid.UUID, connections []NangoConnectionDeleteTarget, integrations ...NangoIntegrationDeleteTarget) (*asynq.Task, error) {
	payload, err := json.Marshal(AgentProfileNangoCleanupPayload{
		AgentID:      agentID,
		Connections:  connections,
		Integrations: integrations,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal agent profile nango cleanup payload: %w", err)
	}
	return asynq.NewTask(
		TypeAgentProfileNangoCleanup,
		payload,
		asynq.Queue(QueueDefault),
		asynq.MaxRetry(5),
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
