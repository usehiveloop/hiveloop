package handler

import (
	"time"

	"github.com/usehiveloop/hiveloop/internal/model"
)

// employeeTriggerInput defines an employee trigger.
// TriggerType may be "webhook" (default) or "http".
type employeeTriggerInput struct {
	ID           string              `json:"id,omitempty"`
	TriggerType  string              `json:"trigger_type,omitempty"` // "webhook" (default), "http"
	ConnectionID string              `json:"connection_id,omitempty"`
	TriggerKeys  []string            `json:"trigger_keys,omitempty"`
	Conditions   *model.TriggerMatch `json:"conditions,omitempty"`
	Instructions string              `json:"instructions,omitempty"`
	// SecretKey is the optional plaintext shared secret for HTTP triggers.
	// When provided, the server bcrypt-hashes it before storing. Never returned
	// in any response — see employeeTriggerResponse.SecretSet for visibility.
	SecretKey string `json:"secret_key,omitempty"`
}

type employeeTriggerResponse struct {
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

type employeeSkillSummary struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Description *string `json:"description,omitempty"`
	SourceType  string  `json:"source_type"`
	Locked      bool    `json:"locked,omitempty"`
	Required    bool    `json:"required,omitempty"`
}

type employeeResponse struct {
	ID                    string                    `json:"id"`
	Name                  string                    `json:"name"`
	Description           *string                   `json:"description,omitempty"`
	AvatarURL             *string                   `json:"avatar_url,omitempty"`
	SandboxTemplateID     *string                   `json:"sandbox_template_id,omitempty"`
	Model                 string                    `json:"model"`
	Status                string                    `json:"status"`
	LastMemoryRefreshedAt *string                   `json:"last_memory_refreshed_at,omitempty"`
	MemoryRefreshStatus   string                    `json:"memory_refresh_status,omitempty"`
	MemoryRefreshError    string                    `json:"memory_refresh_error,omitempty"`
	SpecialistIDs         []string                  `json:"specialist_ids,omitempty"`
	Triggers              []employeeTriggerResponse `json:"triggers"`
	AttachedSkills        []employeeSkillSummary    `json:"attached_skills"`
	CreatedAt             string                    `json:"created_at"`
	UpdatedAt             string                    `json:"updated_at"`
}

func toEmployeeResponse(a model.Agent) employeeResponse {
	description := hivyEmployeeDescription
	avatarURL := hivyEmployeeAvatarURL
	resp := employeeResponse{
		ID:                  a.ID.String(),
		Name:                hivyEmployeeName,
		Description:         &description,
		AvatarURL:           &avatarURL,
		Model:               a.Model,
		Status:              a.Status,
		MemoryRefreshStatus: a.MemoryRefreshStatus,
		MemoryRefreshError:  a.MemoryRefreshError,
		CreatedAt:           a.CreatedAt.Format(time.RFC3339),
		UpdatedAt:           a.UpdatedAt.Format(time.RFC3339),
	}
	if a.LastMemoryRefreshedAt != nil {
		s := a.LastMemoryRefreshedAt.Format(time.RFC3339)
		resp.LastMemoryRefreshedAt = &s
	}
	if a.SandboxTemplateID != nil {
		s := a.SandboxTemplateID.String()
		resp.SandboxTemplateID = &s
	}
	return resp
}
