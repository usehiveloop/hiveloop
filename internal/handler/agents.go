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

// agentTriggerInput defines an employee trigger.
// TriggerType may be "webhook" (default) or "http".
type agentTriggerInput struct {
	ID           string              `json:"id,omitempty"`
	TriggerType  string              `json:"trigger_type,omitempty"` // "webhook" (default), "http"
	ConnectionID string              `json:"connection_id,omitempty"`
	TriggerKeys  []string            `json:"trigger_keys,omitempty"`
	Conditions   *model.TriggerMatch `json:"conditions,omitempty"`
	Instructions string              `json:"instructions,omitempty"`
	// SecretKey is the optional plaintext shared secret for HTTP triggers.
	// When provided, the server bcrypt-hashes it before storing. Never returned
	// in any response — see agentTriggerResponse.SecretSet for visibility.
	SecretKey string `json:"secret_key,omitempty"`
}

type agentTriggerResponse struct {
	ID           string   `json:"id"`
	TriggerType  string   `json:"trigger_type"`
	ConnectionID string   `json:"connection_id,omitempty"`
	Provider     string   `json:"provider,omitempty"`
	TriggerKeys  []string `json:"trigger_keys,omitempty"`
	Enabled      bool     `json:"enabled"`
	Conditions   any      `json:"conditions,omitempty"`
	Instructions string   `json:"instructions,omitempty"`
	// SecretSet indicates whether an HTTP trigger has a shared secret configured.
	// True when the trigger requires auth on incoming requests. The secret value
	// is never returned.
	SecretSet bool `json:"secret_set"`
}

type agentSkillSummary struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Description *string `json:"description,omitempty"`
	SourceType  string  `json:"source_type"`
	Locked      bool    `json:"locked,omitempty"`
	Required    bool    `json:"required,omitempty"`
}

type agentResponse struct {
	ID                        string                   `json:"id"`
	Name                      string                   `json:"name"`
	Description               *string                  `json:"description,omitempty"`
	AvatarURL                 *string                  `json:"avatar_url,omitempty"`
	Category                  *string                  `json:"category,omitempty"`
	CredentialID              string                   `json:"credential_id"`
	ProviderID                string                   `json:"provider_id"`
	SandboxTemplateID         *string                  `json:"sandbox_template_id,omitempty"`
	SystemPrompt              string                   `json:"system_prompt"`
	IdentityPrompt            string                   `json:"identity_prompt,omitempty"`
	PromptOperatingPrinciples string                   `json:"prompt_operating_principles,omitempty"`
	ProviderPrompts           model.ProviderPromptsMap `json:"provider_prompts"`
	Instructions              *string                  `json:"instructions,omitempty"`
	Model                     string                   `json:"model"`
	Tools                     model.JSON               `json:"tools"`
	McpServers                model.JSON               `json:"mcp_servers"`
	Skills                    model.JSON               `json:"skills"`
	Integrations              model.JSON               `json:"integrations"`
	AgentConfig               model.JSON               `json:"agent_config"`
	Permissions               model.JSON               `json:"permissions"`
	Resources                 model.JSON               `json:"resources"`
	SharedMemory              bool                     `json:"shared_memory"`
	SandboxTools              []string                 `json:"sandbox_tools"`
	Harness                   string                   `json:"harness"`
	Status                    string                   `json:"status"`
	IsEmployee                bool                     `json:"is_employee"`
	LastMemoryRefreshedAt     *string                  `json:"last_memory_refreshed_at,omitempty"`
	MemoryRefreshStatus       string                   `json:"memory_refresh_status,omitempty"`
	MemoryRefreshError        string                   `json:"memory_refresh_error,omitempty"`
	SubagentIDs               []string                 `json:"subagent_ids,omitempty"`
	Triggers                  []agentTriggerResponse   `json:"triggers"`
	AttachedSkills            []agentSkillSummary      `json:"attached_skills"`
	CreatedAt                 string                   `json:"created_at"`
	UpdatedAt                 string                   `json:"updated_at"`
}

func toAgentResponse(a model.Agent) agentResponse {
	description := hivyEmployeeDescription
	avatarURL := hivyEmployeeAvatarURL
	resp := agentResponse{
		ID:                        a.ID.String(),
		Name:                      hivyEmployeeName,
		Description:               &description,
		AvatarURL:                 &avatarURL,
		Category:                  nil,
		SystemPrompt:              "",
		IdentityPrompt:            "",
		PromptOperatingPrinciples: "",
		ProviderPrompts:           a.ProviderPrompts,
		Instructions:              a.Instructions,
		Model:                     a.Model,
		Tools:                     a.Tools,
		McpServers:                a.McpServers,
		Skills:                    a.Skills,
		Integrations:              model.JSON{},
		AgentConfig:               a.AgentConfig,
		Permissions:               a.Permissions,
		Resources:                 a.Resources,
		SharedMemory:              a.SharedMemory,
		SandboxTools:              []string(a.SandboxTools),
		Harness:                   a.Harness,
		Status:                    a.Status,
		IsEmployee:                true,
		MemoryRefreshStatus:       a.MemoryRefreshStatus,
		MemoryRefreshError:        a.MemoryRefreshError,
		CreatedAt:                 a.CreatedAt.Format(time.RFC3339),
		UpdatedAt:                 a.UpdatedAt.Format(time.RFC3339),
	}
	if a.LastMemoryRefreshedAt != nil {
		s := a.LastMemoryRefreshedAt.Format(time.RFC3339)
		resp.LastMemoryRefreshedAt = &s
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
