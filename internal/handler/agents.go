package handler

import (
	"time"

	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/crypto"
	"github.com/usehiveloop/hiveloop/internal/enqueue"
	"github.com/usehiveloop/hiveloop/internal/mcp/catalog"
	"github.com/usehiveloop/hiveloop/internal/model"
	"github.com/usehiveloop/hiveloop/internal/registry"
)

type AgentHandler struct {
	db             *gorm.DB
	registry       *registry.Registry
	encKey         *crypto.SymmetricKey
	enqueuer       enqueue.TaskEnqueuer
	actionsCatalog *catalog.Catalog
}

func NewAgentHandler(db *gorm.DB, reg *registry.Registry, encKey *crypto.SymmetricKey, enqueuer ...enqueue.TaskEnqueuer) *AgentHandler {
	h := &AgentHandler{db: db, registry: reg, encKey: encKey}
	if len(enqueuer) > 0 {
		h.enqueuer = enqueuer[0]
	}
	return h
}

func (h *AgentHandler) SetCatalog(c *catalog.Catalog) {
	h.actionsCatalog = c
}

// agentTriggerInput defines a trigger to create alongside the agent.
// Each entry creates a RouterTrigger + RoutingRule that automatically invokes
// this agent when the trigger fires.
//
// TriggerType determines the kind of trigger:
//   - "webhook" (default): fires on matching webhook events. Requires connection_id and trigger_keys.
//   - "http": fires on HTTP requests to /incoming/triggers/{id}. No connection required.
//   - "cron": fires on a cron schedule. Requires cron_schedule.
type agentTriggerInput struct {
	TriggerType  string              `json:"trigger_type,omitempty"` // "webhook" (default), "http", "cron"
	ConnectionID string              `json:"connection_id,omitempty"`
	TriggerKeys  []string            `json:"trigger_keys,omitempty"`
	Conditions   *model.TriggerMatch `json:"conditions,omitempty"`
	CronSchedule string              `json:"cron_schedule,omitempty"`
	Instructions string              `json:"instructions,omitempty"`
}

type agentTriggerResponse struct {
	ID           string   `json:"id"`
	TriggerType  string   `json:"trigger_type"`
	ConnectionID string   `json:"connection_id,omitempty"`
	Provider     string   `json:"provider,omitempty"`
	TriggerKeys  []string `json:"trigger_keys,omitempty"`
	Enabled      bool     `json:"enabled"`
	Conditions   any      `json:"conditions,omitempty"`
	CronSchedule string   `json:"cron_schedule,omitempty"`
	Instructions string   `json:"instructions,omitempty"`
}

type createAgentRequest struct {
	Name              string              `json:"name"`
	Description       *string             `json:"description,omitempty"`
	AvatarURL         *string             `json:"avatar_url,omitempty"`
	CredentialID      string              `json:"credential_id"`
	SandboxTemplateID *string             `json:"sandbox_template_id,omitempty"`
	SystemPrompt      string              `json:"system_prompt"`
	ProviderPrompts   model.ProviderPromptsMap `json:"provider_prompts,omitempty"`
	Instructions      *string             `json:"instructions,omitempty"`
	Model             string              `json:"model"`
	Tools             model.JSON          `json:"tools,omitempty"`
	McpServers        model.JSON          `json:"mcp_servers,omitempty"`
	Skills            model.JSON          `json:"skills,omitempty"`
	Integrations      model.JSON          `json:"integrations,omitempty"`
	AgentConfig       model.JSON          `json:"agent_config,omitempty"`
	Permissions       model.JSON          `json:"permissions,omitempty"`
	Resources         model.JSON          `json:"resources,omitempty"`
	Team              string              `json:"team,omitempty"`
	SharedMemory      bool                `json:"shared_memory,omitempty"`
	SandboxTools      []string            `json:"sandbox_tools,omitempty"`
	SkillIDs          []string            `json:"skill_ids,omitempty"`
	SubagentIDs       []string            `json:"subagent_ids,omitempty"`
	Triggers          []agentTriggerInput `json:"triggers,omitempty"`
}

type updateAgentRequest struct {
	Name              *string              `json:"name,omitempty"`
	Description       *string              `json:"description,omitempty"`
	AvatarURL         *string              `json:"avatar_url,omitempty"`
	CredentialID      *string              `json:"credential_id,omitempty"`
	SandboxTemplateID *string              `json:"sandbox_template_id,omitempty"`
	SystemPrompt      *string              `json:"system_prompt,omitempty"`
	ProviderPrompts   model.ProviderPromptsMap  `json:"provider_prompts,omitempty"`
	Instructions      *string              `json:"instructions,omitempty"`
	Model             *string              `json:"model,omitempty"`
	Tools             model.JSON           `json:"tools,omitempty"`
	McpServers        model.JSON           `json:"mcp_servers,omitempty"`
	Skills            model.JSON           `json:"skills,omitempty"`
	Integrations      model.JSON           `json:"integrations,omitempty"`
	AgentConfig       model.JSON           `json:"agent_config,omitempty"`
	Permissions       model.JSON           `json:"permissions,omitempty"`
	Resources         model.JSON           `json:"resources,omitempty"`
	Team              *string              `json:"team,omitempty"`
	SharedMemory      *bool                `json:"shared_memory,omitempty"`
	SandboxTools      []string             `json:"sandbox_tools,omitempty"`
	SkillIDs          *[]string            `json:"skill_ids,omitempty"`
	Triggers          *[]agentTriggerInput `json:"triggers,omitempty"`
}

type agentSubagentSummary struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Description *string `json:"description,omitempty"`
	Model       string  `json:"model"`
}

type setupRequest struct {
	SetupCommands []string          `json:"setup_commands"`
	EnvVars       map[string]string `json:"env_vars"`
}

type setupResponse struct {
	SetupCommands []string `json:"setup_commands"`
	EnvVarKeys    []string `json:"env_var_keys"`
}

type agentSkillSummary struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Description *string `json:"description,omitempty"`
	SourceType  string  `json:"source_type"`
}

type agentResponse struct {
	ID                string                 `json:"id"`
	Name              string                 `json:"name"`
	Description       *string                `json:"description,omitempty"`
	AvatarURL         *string                `json:"avatar_url,omitempty"`
	CredentialID      string                 `json:"credential_id"`
	ProviderID        string                 `json:"provider_id"`
	SandboxTemplateID *string                `json:"sandbox_template_id,omitempty"`
	SystemPrompt      string                 `json:"system_prompt"`
	ProviderPrompts   model.ProviderPromptsMap    `json:"provider_prompts"`
	Instructions      *string                `json:"instructions,omitempty"`
	Model             string                 `json:"model"`
	Tools             model.JSON             `json:"tools"`
	McpServers        model.JSON             `json:"mcp_servers"`
	Skills            model.JSON             `json:"skills"`
	Integrations      model.JSON             `json:"integrations"`
	AgentConfig       model.JSON             `json:"agent_config"`
	Permissions       model.JSON             `json:"permissions"`
	Resources         model.JSON             `json:"resources"`
	Team              string                 `json:"team"`
	SharedMemory      bool                   `json:"shared_memory"`
	SandboxTools      []string               `json:"sandbox_tools"`
	Status            string                 `json:"status"`
	Triggers          []agentTriggerResponse `json:"triggers"`
	AttachedSubagents []agentSubagentSummary `json:"subagents"`
	AttachedSkills    []agentSkillSummary    `json:"attached_skills"`
	CreatedAt         string                 `json:"created_at"`
	UpdatedAt         string                 `json:"updated_at"`
}

func toAgentResponse(a model.Agent) agentResponse {
	resp := agentResponse{
		ID:           a.ID.String(),
		Name:         a.Name,
		Description:  a.Description,
		AvatarURL:    a.AvatarURL,
		SystemPrompt:    a.SystemPrompt,
		ProviderPrompts: a.ProviderPrompts,
		Instructions:    a.Instructions,
		Model:           a.Model,
		Tools:        a.Tools,
		McpServers:   a.McpServers,
		Skills:       a.Skills,
		Integrations: a.Integrations,
		AgentConfig:  a.AgentConfig,
		Permissions:  a.Permissions,
		Resources:    a.Resources,
		Team:         a.Team,
		SharedMemory: a.SharedMemory,
		SandboxTools: ensureStringSlice(a.SandboxTools),
		Status:       a.Status,
		CreatedAt:    a.CreatedAt.Format(time.RFC3339),
		UpdatedAt:    a.UpdatedAt.Format(time.RFC3339),
	}
	if a.CredentialID != nil {
		resp.CredentialID = a.CredentialID.String()
	}
	if a.SandboxTemplateID != nil {
		s := a.SandboxTemplateID.String()
		resp.SandboxTemplateID = &s
	}
	if a.Credential != nil && a.Credential.ProviderID != "" {
		resp.ProviderID = a.Credential.ProviderID
	}
	return resp
}
