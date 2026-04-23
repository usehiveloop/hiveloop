package handler

import (
	"time"

	"github.com/usehiveloop/hiveloop/internal/model"
)

type adminUserResponse struct {
	ID               string  `json:"id"`
	Email            string  `json:"email"`
	Name             string  `json:"name"`
	EmailConfirmedAt *string `json:"email_confirmed_at,omitempty"`
	BannedAt         *string `json:"banned_at,omitempty"`
	BanReason        string  `json:"ban_reason,omitempty"`
	CreatedAt        string  `json:"created_at"`
	UpdatedAt        string  `json:"updated_at"`
}

func toAdminUserResponse(u model.User) adminUserResponse {
	resp := adminUserResponse{
		ID:        u.ID.String(),
		Email:     u.Email,
		Name:      u.Name,
		BanReason: u.BanReason,
		CreatedAt: u.CreatedAt.Format(time.RFC3339),
		UpdatedAt: u.UpdatedAt.Format(time.RFC3339),
	}
	if u.EmailConfirmedAt != nil {
		t := u.EmailConfirmedAt.Format(time.RFC3339)
		resp.EmailConfirmedAt = &t
	}
	if u.BannedAt != nil {
		t := u.BannedAt.Format(time.RFC3339)
		resp.BannedAt = &t
	}
	return resp
}

type adminOrgResponse struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	RateLimit      int      `json:"rate_limit"`
	Active         bool     `json:"active"`
	AllowedOrigins []string `json:"allowed_origins"`
	CreatedAt      string   `json:"created_at"`
	UpdatedAt      string   `json:"updated_at"`
}

func toAdminOrgResponse(o model.Org) adminOrgResponse {
	origins := make([]string, len(o.AllowedOrigins))
	copy(origins, o.AllowedOrigins)
	return adminOrgResponse{
		ID:             o.ID.String(),
		Name:           o.Name,
		RateLimit:      o.RateLimit,
		Active:         o.Active,
		AllowedOrigins: origins,
		CreatedAt:      o.CreatedAt.Format(time.RFC3339),
		UpdatedAt:      o.UpdatedAt.Format(time.RFC3339),
	}
}

type adminOrgDetailResponse struct {
	adminOrgResponse
	MemberCount     int64 `json:"member_count"`
	CredentialCount int64 `json:"credential_count"`
	AgentCount      int64 `json:"agent_count"`
	SandboxCount    int64 `json:"sandbox_count"`
}

type adminMembershipResponse struct {
	ID        string `json:"id"`
	UserID    string `json:"user_id"`
	OrgID     string `json:"org_id"`
	Role      string `json:"role"`
	UserEmail string `json:"user_email"`
	UserName  string `json:"user_name"`
	CreatedAt string `json:"created_at"`
}

type adminStatsResponse struct {
	TotalUsers               int64   `json:"total_users"`
	TotalOrgs                int64   `json:"total_orgs"`
	TotalAgents              int64   `json:"total_agents"`
	TotalSandboxesRunning    int64   `json:"total_sandboxes_running"`
	TotalSandboxesStopped    int64   `json:"total_sandboxes_stopped"`
	TotalSandboxesError      int64   `json:"total_sandboxes_error"`
	TotalGenerations         int64   `json:"total_generations"`
	TotalConversationsActive int64   `json:"total_conversations_active"`
	TotalCredentials         int64   `json:"total_credentials"`
	TotalCost                float64 `json:"total_cost"`
}

type adminGenerationStatsResponse struct {
	TotalGenerations int64                    `json:"total_generations"`
	TotalCost        float64                  `json:"total_cost"`
	TotalInput       int64                    `json:"total_input_tokens"`
	TotalOutput      int64                    `json:"total_output_tokens"`
	ByProvider       []adminProviderStatEntry `json:"by_provider"`
	ByModel          []adminModelStatEntry    `json:"by_model"`
}

type adminProviderStatEntry struct {
	ProviderID   string  `json:"provider_id"`
	Count        int64   `json:"count"`
	Cost         float64 `json:"cost"`
	InputTokens  int64   `json:"input_tokens"`
	OutputTokens int64   `json:"output_tokens"`
}

type adminModelStatEntry struct {
	Model        string  `json:"model"`
	Count        int64   `json:"count"`
	Cost         float64 `json:"cost"`
	InputTokens  int64   `json:"input_tokens"`
	OutputTokens int64   `json:"output_tokens"`
}
