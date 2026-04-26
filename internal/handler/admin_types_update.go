package handler

import "github.com/usehiveloop/hiveloop/internal/model"

type adminUpdateUserRequest struct {
	Name  *string `json:"name,omitempty"`
	Email *string `json:"email,omitempty"`
}

type adminUpdateOrgRequest struct {
	Name           *string   `json:"name,omitempty"`
	RateLimit      *int      `json:"rate_limit,omitempty"`
	Active         *bool     `json:"active,omitempty"`
	AllowedOrigins *[]string `json:"allowed_origins,omitempty"`
}

type adminUpdateCredentialRequest struct {
	Label *string `json:"label,omitempty"`
}

type adminUpdateAgentRequest struct {
	Name              *string    `json:"name,omitempty"`
	Description       *string    `json:"description,omitempty"`
	CredentialID      *string    `json:"credential_id,omitempty"`
	SandboxTemplateID *string    `json:"sandbox_template_id,omitempty"`
	SystemPrompt      *string    `json:"system_prompt,omitempty"`
	Model             *string    `json:"model,omitempty"`
	Tools             model.JSON `json:"tools,omitempty"`
	McpServers        model.JSON `json:"mcp_servers,omitempty"`
	Skills            model.JSON `json:"skills,omitempty"`
	Integrations      model.JSON `json:"integrations,omitempty"`
	AgentConfig       model.JSON `json:"agent_config,omitempty"`
	Permissions       model.JSON `json:"permissions,omitempty"`
	Team              *string    `json:"team,omitempty"`
	SharedMemory      *bool      `json:"shared_memory,omitempty"`
	Status            *string    `json:"status,omitempty"`
}

type adminCreateSandboxTemplateRequest struct {
	Name string   `json:"name"`
	Slug string   `json:"slug"`
	Tags []string `json:"tags"`
	Size string   `json:"size"`
}

type adminUpdateSandboxTemplateRequest struct {
	Name *string  `json:"name,omitempty"`
	Slug *string  `json:"slug,omitempty"`
	Tags []string `json:"tags,omitempty"`
	Size *string  `json:"size,omitempty"`
}
