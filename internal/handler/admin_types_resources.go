package handler

import (
	"time"

	"github.com/usehiveloop/hiveloop/internal/model"
)

type adminCredentialResponse struct {
	ID         string  `json:"id"`
	OrgID      string  `json:"org_id"`
	Label      string  `json:"label"`
	ProviderID string  `json:"provider_id"`
	RevokedAt  *string `json:"revoked_at,omitempty"`
	CreatedAt  string  `json:"created_at"`
}

func toAdminCredentialResponse(c model.Credential) adminCredentialResponse {
	resp := adminCredentialResponse{
		ID:         c.ID.String(),
		OrgID:      c.OrgID.String(),
		Label:      c.Label,
		ProviderID: c.ProviderID,
		CreatedAt:  c.CreatedAt.Format(time.RFC3339),
	}
	if c.RevokedAt != nil {
		t := c.RevokedAt.Format(time.RFC3339)
		resp.RevokedAt = &t
	}
	return resp
}

type adminAPIKeyResponse struct {
	ID        string   `json:"id"`
	OrgID     string   `json:"org_id"`
	Name      string   `json:"name"`
	KeyPrefix string   `json:"key_prefix"`
	Scopes    []string `json:"scopes"`
	ExpiresAt *string  `json:"expires_at,omitempty"`
	RevokedAt *string  `json:"revoked_at,omitempty"`
	CreatedAt string   `json:"created_at"`
}

func toAdminAPIKeyResponse(k model.APIKey) adminAPIKeyResponse {
	resp := adminAPIKeyResponse{
		ID:        k.ID.String(),
		OrgID:     k.OrgID.String(),
		Name:      k.Name,
		KeyPrefix: k.KeyPrefix,
		Scopes:    k.Scopes,
		CreatedAt: k.CreatedAt.Format(time.RFC3339),
	}
	if k.ExpiresAt != nil {
		t := k.ExpiresAt.Format(time.RFC3339)
		resp.ExpiresAt = &t
	}
	if k.RevokedAt != nil {
		t := k.RevokedAt.Format(time.RFC3339)
		resp.RevokedAt = &t
	}
	return resp
}

type adminTokenResponse struct {
	ID           string  `json:"id"`
	OrgID        string  `json:"org_id"`
	CredentialID string  `json:"credential_id"`
	JTI          string  `json:"jti"`
	ExpiresAt    string  `json:"expires_at"`
	RevokedAt    *string `json:"revoked_at,omitempty"`
	CreatedAt    string  `json:"created_at"`
}

func toAdminTokenResponse(t model.Token) adminTokenResponse {
	resp := adminTokenResponse{
		ID:           t.ID.String(),
		OrgID:        t.OrgID.String(),
		CredentialID: t.CredentialID.String(),
		JTI:          t.JTI,
		ExpiresAt:    t.ExpiresAt.Format(time.RFC3339),
		CreatedAt:    t.CreatedAt.Format(time.RFC3339),
	}
	if t.RevokedAt != nil {
		ts := t.RevokedAt.Format(time.RFC3339)
		resp.RevokedAt = &ts
	}
	return resp
}

type adminAgentResponse struct {
	ID        string `json:"id"`
	OrgID     string `json:"org_id"`
	Name      string `json:"name"`
	Model     string `json:"model"`
	Status    string `json:"status"`
	CreatedAt string `json:"created_at"`
}

func toAdminAgentResponse(a model.Agent) adminAgentResponse {
	resp := adminAgentResponse{
		ID:        a.ID.String(),
		Name:      a.Name,
		Model:     a.Model,
		Status:    a.Status,
		CreatedAt: a.CreatedAt.Format(time.RFC3339),
	}
	if a.OrgID != nil {
		resp.OrgID = a.OrgID.String()
	}
	return resp
}

type adminSandboxResponse struct {
	ID               string  `json:"id"`
	OrgID            *string `json:"org_id,omitempty"`
	Status           string  `json:"status"`
	ExternalID       string  `json:"external_id"`
	AgentID          *string `json:"agent_id,omitempty"`
	ErrorMessage     *string `json:"error_message,omitempty"`
	MemoryLimitBytes int64   `json:"memory_limit_bytes"`
	MemoryUsedBytes  int64   `json:"memory_used_bytes"`
	CPUUsageUsec     int64   `json:"cpu_usage_usec"`
	LastActiveAt     *string `json:"last_active_at,omitempty"`
	CreatedAt        string  `json:"created_at"`
}

func toAdminSandboxResponse(s model.Sandbox) adminSandboxResponse {
	resp := adminSandboxResponse{
		ID:               s.ID.String(),
		Status:           s.Status,
		ExternalID:       s.ExternalID,
		ErrorMessage:     s.ErrorMessage,
		MemoryLimitBytes: s.MemoryLimitBytes,
		MemoryUsedBytes:  s.MemoryUsedBytes,
		CPUUsageUsec:     s.CPUUsageUsec,
		CreatedAt:        s.CreatedAt.Format(time.RFC3339),
	}
	if s.OrgID != nil {
		id := s.OrgID.String()
		resp.OrgID = &id
	}
	if s.AgentID != nil {
		id := s.AgentID.String()
		resp.AgentID = &id
	}
	if s.LastActiveAt != nil {
		t := s.LastActiveAt.Format(time.RFC3339)
		resp.LastActiveAt = &t
	}
	return resp
}

type adminConversationResponse struct {
	ID        string  `json:"id"`
	OrgID     string  `json:"org_id"`
	AgentID   string  `json:"agent_id"`
	SandboxID string  `json:"sandbox_id"`
	Status    string  `json:"status"`
	TokenID   *string `json:"token_id,omitempty"`
	CreatedAt string  `json:"created_at"`
	EndedAt   *string `json:"ended_at,omitempty"`
}

func toAdminConversationResponse(c model.AgentConversation) adminConversationResponse {
	resp := adminConversationResponse{
		ID:        c.ID.String(),
		OrgID:     c.OrgID.String(),
		AgentID:   c.AgentID.String(),
		SandboxID: c.SandboxID.String(),
		Status:    c.Status,
		CreatedAt: c.CreatedAt.Format(time.RFC3339),
	}
	if c.TokenID != nil {
		id := c.TokenID.String()
		resp.TokenID = &id
	}
	if c.EndedAt != nil {
		t := c.EndedAt.Format(time.RFC3339)
		resp.EndedAt = &t
	}
	return resp
}

type adminGenerationResponse struct {
	ID             string  `json:"id"`
	OrgID          string  `json:"org_id"`
	ProviderID     string  `json:"provider_id"`
	Model          string  `json:"model"`
	InputTokens    int     `json:"input_tokens"`
	OutputTokens   int     `json:"output_tokens"`
	Cost           float64 `json:"cost"`
	UpstreamStatus int     `json:"upstream_status"`
	ErrorType      string  `json:"error_type,omitempty"`
	CreatedAt      string  `json:"created_at"`
}

func toAdminGenerationResponse(g model.Generation) adminGenerationResponse {
	return adminGenerationResponse{
		ID:             g.ID,
		OrgID:          g.OrgID.String(),
		ProviderID:     g.ProviderID,
		Model:          g.Model,
		InputTokens:    g.InputTokens,
		OutputTokens:   g.OutputTokens,
		Cost:           g.Cost,
		UpstreamStatus: g.UpstreamStatus,
		ErrorType:      g.ErrorType,
		CreatedAt:      g.CreatedAt.Format(time.RFC3339),
	}
}

type adminCustomDomainResponse struct {
	ID         string  `json:"id"`
	OrgID      string  `json:"org_id"`
	Domain     string  `json:"domain"`
	Verified   bool    `json:"verified"`
	VerifiedAt *string `json:"verified_at,omitempty"`
	CreatedAt  string  `json:"created_at"`
}

func toAdminCustomDomainResponse(d model.CustomDomain) adminCustomDomainResponse {
	resp := adminCustomDomainResponse{
		ID:        d.ID.String(),
		OrgID:     d.OrgID.String(),
		Domain:    d.Domain,
		Verified:  d.Verified,
		CreatedAt: d.CreatedAt.Format(time.RFC3339),
	}
	if d.VerifiedAt != nil {
		t := d.VerifiedAt.Format(time.RFC3339)
		resp.VerifiedAt = &t
	}
	return resp
}

type adminSandboxTemplateResponse struct {
	ID             string     `json:"id"`
	OrgID          *string    `json:"org_id"`
	Name           string     `json:"name"`
	Slug           string     `json:"slug"`
	Tags           model.JSON `json:"tags"`
	Size           string     `json:"size"`
	BaseTemplateID *string    `json:"base_template_id,omitempty"`
	BuildStatus    string     `json:"build_status"`
	BuildError     *string    `json:"build_error,omitempty"`
	BuildLogs      string     `json:"build_logs,omitempty"`
	BuildCommands  string     `json:"build_commands,omitempty"`
	ExternalID     *string    `json:"external_id,omitempty"`
	BaseImageRef   *string    `json:"base_image_ref,omitempty"`
	CreatedAt      string     `json:"created_at"`
}

func toAdminSandboxTemplateResponse(t model.SandboxTemplate) adminSandboxTemplateResponse {
	resp := adminSandboxTemplateResponse{
		ID:            t.ID.String(),
		Name:          t.Name,
		Slug:          t.Slug,
		Tags:          t.Tags,
		Size:          t.Size,
		BuildStatus:   t.BuildStatus,
		BuildError:    t.BuildError,
		BuildLogs:     t.BuildLogs,
		BuildCommands: t.BuildCommands,
		ExternalID:    t.ExternalID,
		BaseImageRef:  t.BaseImageRef,
		CreatedAt:     t.CreatedAt.Format(time.RFC3339),
	}
	if t.OrgID != nil {
		orgIDStr := t.OrgID.String()
		resp.OrgID = &orgIDStr
	}
	if t.BaseTemplateID != nil {
		baseIDStr := t.BaseTemplateID.String()
		resp.BaseTemplateID = &baseIDStr
	}
	return resp
}

type adminWorkspaceStorageResponse struct {
	ID        string `json:"id"`
	OrgID     string `json:"org_id"`
	CreatedAt string `json:"created_at"`
}

func toAdminWorkspaceStorageResponse(ws model.WorkspaceStorage) adminWorkspaceStorageResponse {
	return adminWorkspaceStorageResponse{
		ID:        ws.ID.String(),
		OrgID:     ws.OrgID.String(),
		CreatedAt: ws.CreatedAt.Format(time.RFC3339),
	}
}
