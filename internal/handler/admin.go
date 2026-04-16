package handler

import (
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/lib/pq"
	"gorm.io/gorm"

	"github.com/ziraloop/ziraloop/internal/auth"
	"github.com/ziraloop/ziraloop/internal/enqueue"
	"github.com/ziraloop/ziraloop/internal/mcp/catalog"
	"github.com/ziraloop/ziraloop/internal/middleware"
	"github.com/ziraloop/ziraloop/internal/model"
	"github.com/ziraloop/ziraloop/internal/nango"
	"github.com/ziraloop/ziraloop/internal/sandbox"
	"github.com/ziraloop/ziraloop/internal/skills"
	"github.com/ziraloop/ziraloop/internal/tasks"
)

// setAuditDiff computes a changes map from old and new values for the given
// updates, and writes it into the shared audit bucket on the request context.
// Only fields that actually changed are included. Values are recorded as
// {"field": {"old": ..., "new": ...}}.
func setAuditDiff(r *http.Request, old map[string]any, updates map[string]any) {
	changes := middleware.AdminAuditChanges{}
	for field, newVal := range updates {
		oldVal, exists := old[field]
		if !exists || fmt.Sprintf("%v", oldVal) != fmt.Sprintf("%v", newVal) {
			changes[field] = map[string]any{"old": oldVal, "new": newVal}
		}
	}
	if len(changes) > 0 {
		middleware.SetAdminAuditChanges(r, changes)
	}
}

// AdminHandler provides platform-wide administration endpoints.
// All methods bypass org-scoping and operate across the entire platform.
type AdminHandler struct {
	db           *gorm.DB
	orchestrator *sandbox.Orchestrator
	nango        *nango.Client
	catalog      *catalog.Catalog
	enqueuer     enqueue.TaskEnqueuer

	// Token-issuing dependencies for impersonation.
	privateKey *rsa.PrivateKey
	signingKey []byte
	issuer     string
	audience   string
	accessTTL  time.Duration
	refreshTTL time.Duration
}

// NewAdminHandler creates a new admin handler.
func NewAdminHandler(
	db *gorm.DB,
	orchestrator *sandbox.Orchestrator,
	nangoClient *nango.Client,
	cat *catalog.Catalog,
	privateKey *rsa.PrivateKey,
	signingKey []byte,
	issuer, audience string,
	accessTTL, refreshTTL time.Duration,
	enqueuer enqueue.TaskEnqueuer,
) *AdminHandler {
	return &AdminHandler{
		db:           db,
		orchestrator: orchestrator,
		nango:        nangoClient,
		catalog:      cat,
		enqueuer:     enqueuer,
		privateKey:   privateKey,
		signingKey:   signingKey,
		issuer:       issuer,
		audience:     audience,
		accessTTL:    accessTTL,
		refreshTTL:   refreshTTL,
	}
}

// ---------------------------------------------------------------------------
// Response types
// ---------------------------------------------------------------------------

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
	ID          string  `json:"id"`
	OrgID       string  `json:"org_id"`
	Name        string  `json:"name"`
	Model       string  `json:"model"`
	SandboxType string  `json:"sandbox_type"`
	Status      string  `json:"status"`
	SandboxID   *string `json:"sandbox_id,omitempty"`
	CreatedAt   string  `json:"created_at"`
}

func toAdminAgentResponse(a model.Agent) adminAgentResponse {
	resp := adminAgentResponse{
		ID:          a.ID.String(),
		Name:        a.Name,
		Model:       a.Model,
		SandboxType: a.SandboxType,
		Status:      a.Status,
		CreatedAt:   a.CreatedAt.Format(time.RFC3339),
	}
	if a.OrgID != nil {
		resp.OrgID = a.OrgID.String()
	}
	if a.SandboxID != nil {
		id := a.SandboxID.String()
		resp.SandboxID = &id
	}
	return resp
}

type adminSandboxResponse struct {
	ID               string  `json:"id"`
	OrgID            *string `json:"org_id,omitempty"`
	SandboxType      string  `json:"sandbox_type"`
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
		SandboxType:      s.SandboxType,
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
	TotalUsers              int64   `json:"total_users"`
	TotalOrgs               int64   `json:"total_orgs"`
	TotalAgents             int64   `json:"total_agents"`
	TotalSandboxesRunning   int64   `json:"total_sandboxes_running"`
	TotalSandboxesStopped   int64   `json:"total_sandboxes_stopped"`
	TotalSandboxesError     int64   `json:"total_sandboxes_error"`
	TotalGenerations        int64   `json:"total_generations"`
	TotalConversationsActive int64  `json:"total_conversations_active"`
	TotalCredentials        int64   `json:"total_credentials"`
	TotalCost               float64 `json:"total_cost"`
}

type adminGenerationStatsResponse struct {
	TotalGenerations int64                      `json:"total_generations"`
	TotalCost        float64                    `json:"total_cost"`
	TotalInput       int64                      `json:"total_input_tokens"`
	TotalOutput      int64                      `json:"total_output_tokens"`
	ByProvider       []adminProviderStatEntry   `json:"by_provider"`
	ByModel          []adminModelStatEntry      `json:"by_model"`
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

// ---------------------------------------------------------------------------
// Platform Stats
// ---------------------------------------------------------------------------

// Stats handles GET /admin/v1/stats.
// @Summary Platform stats
// @Description Returns platform-wide aggregate statistics.
// @Tags admin
// @Produce json
// @Success 200 {object} adminStatsResponse
// @Security BearerAuth
// @Router /admin/v1/stats [get]
func (h *AdminHandler) Stats(w http.ResponseWriter, r *http.Request) {
	var stats adminStatsResponse

	h.db.Model(&model.User{}).Count(&stats.TotalUsers)
	h.db.Model(&model.Org{}).Count(&stats.TotalOrgs)
	h.db.Model(&model.Agent{}).Where("status = ?", "active").Count(&stats.TotalAgents)
	h.db.Model(&model.Sandbox{}).Where("status = ?", "running").Count(&stats.TotalSandboxesRunning)
	h.db.Model(&model.Sandbox{}).Where("status = ?", "stopped").Count(&stats.TotalSandboxesStopped)
	h.db.Model(&model.Sandbox{}).Where("status = ?", "error").Count(&stats.TotalSandboxesError)
	h.db.Model(&model.Generation{}).Count(&stats.TotalGenerations)
	h.db.Model(&model.AgentConversation{}).Where("status = ?", "active").Count(&stats.TotalConversationsActive)
	h.db.Model(&model.Credential{}).Where("revoked_at IS NULL").Count(&stats.TotalCredentials)

	var costResult struct{ Total float64 }
	h.db.Model(&model.Generation{}).Select("COALESCE(SUM(cost), 0) as total").Scan(&costResult)
	stats.TotalCost = costResult.Total

	writeJSON(w, http.StatusOK, stats)
}

// ---------------------------------------------------------------------------
// Users
// ---------------------------------------------------------------------------

// ListUsers handles GET /admin/v1/users.
// @Summary List all users
// @Description Returns all users across the platform with optional filters.
// @Tags admin
// @Produce json
// @Param search query string false "Search by email or name"
// @Param banned query string false "Filter by banned status (true/false)"
// @Param confirmed query string false "Filter by email confirmed status (true/false)"
// @Param limit query int false "Page size"
// @Param cursor query string false "Pagination cursor"
// @Success 200 {object} paginatedResponse[adminUserResponse]
// @Security BearerAuth
// @Router /admin/v1/users [get]
func (h *AdminHandler) ListUsers(w http.ResponseWriter, r *http.Request) {
	limit, cursor, err := parsePagination(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	q := h.db.Model(&model.User{})

	if search := r.URL.Query().Get("search"); search != "" {
		q = q.Where("email ILIKE ? OR name ILIKE ?", "%"+search+"%", "%"+search+"%")
	}
	if r.URL.Query().Get("banned") == "true" {
		q = q.Where("banned_at IS NOT NULL")
	} else if r.URL.Query().Get("banned") == "false" {
		q = q.Where("banned_at IS NULL")
	}
	if r.URL.Query().Get("confirmed") == "true" {
		q = q.Where("email_confirmed_at IS NOT NULL")
	} else if r.URL.Query().Get("confirmed") == "false" {
		q = q.Where("email_confirmed_at IS NULL")
	}

	q = applyPagination(q, cursor, limit)

	var users []model.User
	if err := q.Find(&users).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list users"})
		return
	}

	hasMore := len(users) > limit
	if hasMore {
		users = users[:limit]
	}

	resp := make([]adminUserResponse, len(users))
	for i, u := range users {
		resp[i] = toAdminUserResponse(u)
	}

	result := paginatedResponse[adminUserResponse]{Data: resp, HasMore: hasMore}
	if hasMore {
		last := users[len(users)-1]
		c := encodeCursor(last.CreatedAt, last.ID)
		result.NextCursor = &c
	}
	writeJSON(w, http.StatusOK, result)
}

// GetUser handles GET /admin/v1/users/{id}.
// @Summary Get user details
// @Description Returns user details including org memberships.
// @Tags admin
// @Produce json
// @Param id path string true "User ID"
// @Success 200 {object} adminUserResponse
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /admin/v1/users/{id} [get]
func (h *AdminHandler) GetUser(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var user model.User
	if err := h.db.Where("id = ?", id).First(&user).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "user not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get user"})
		return
	}

	// Enrich with memberships
	var memberships []model.OrgMembership
	h.db.Preload("Org").Where("user_id = ?", user.ID).Find(&memberships)

	type membershipEntry struct {
		OrgID   string `json:"org_id"`
		OrgName string `json:"org_name"`
		Role    string `json:"role"`
	}
	orgs := make([]membershipEntry, len(memberships))
	for i, m := range memberships {
		orgs[i] = membershipEntry{
			OrgID:   m.OrgID.String(),
			OrgName: m.Org.Name,
			Role:    m.Role,
		}
	}

	type detailResponse struct {
		adminUserResponse
		Orgs []membershipEntry `json:"orgs"`
	}

	writeJSON(w, http.StatusOK, detailResponse{
		adminUserResponse: toAdminUserResponse(user),
		Orgs:              orgs,
	})
}

// BanUser handles POST /admin/v1/users/{id}/ban.
// @Summary Ban a user
// @Description Bans a user account and revokes all refresh tokens.
// @Tags admin
// @Accept json
// @Produce json
// @Param id path string true "User ID"
// @Success 200 {object} adminUserResponse
// @Failure 404 {object} errorResponse
// @Failure 409 {object} errorResponse
// @Security BearerAuth
// @Router /admin/v1/users/{id}/ban [post]
func (h *AdminHandler) BanUser(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var req struct {
		Reason string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		// reason is optional
		req.Reason = ""
	}

	var user model.User
	if err := h.db.Where("id = ?", id).First(&user).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "user not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get user"})
		return
	}

	if user.BannedAt != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "user is already banned"})
		return
	}

	now := time.Now()
	if err := h.db.Model(&user).Updates(map[string]any{
		"banned_at": now,
		"ban_reason": req.Reason,
	}).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to ban user"})
		return
	}

	// Revoke all refresh tokens
	h.db.Model(&model.RefreshToken{}).Where("user_id = ? AND revoked_at IS NULL", user.ID).
		Update("revoked_at", now)

	slog.Info("admin: user banned", "user_id", id, "reason", req.Reason)

	user.BannedAt = &now
	user.BanReason = req.Reason
	writeJSON(w, http.StatusOK, toAdminUserResponse(user))
}

// UnbanUser handles POST /admin/v1/users/{id}/unban.
// @Summary Unban a user
// @Description Removes the ban from a user account.
// @Tags admin
// @Produce json
// @Param id path string true "User ID"
// @Success 200 {object} adminUserResponse
// @Failure 404 {object} errorResponse
// @Failure 409 {object} errorResponse
// @Security BearerAuth
// @Router /admin/v1/users/{id}/unban [post]
func (h *AdminHandler) UnbanUser(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var user model.User
	if err := h.db.Where("id = ?", id).First(&user).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "user not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get user"})
		return
	}

	if user.BannedAt == nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "user is not banned"})
		return
	}

	if err := h.db.Model(&user).Updates(map[string]any{
		"banned_at": nil,
		"ban_reason": "",
	}).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to unban user"})
		return
	}

	slog.Info("admin: user unbanned", "user_id", id)

	user.BannedAt = nil
	user.BanReason = ""
	writeJSON(w, http.StatusOK, toAdminUserResponse(user))
}

// ConfirmUserEmail handles POST /admin/v1/users/{id}/confirm-email.
// @Summary Force-confirm user email
// @Description Administratively confirms a user's email address.
// @Tags admin
// @Produce json
// @Param id path string true "User ID"
// @Success 200 {object} adminUserResponse
// @Failure 404 {object} errorResponse
// @Failure 409 {object} errorResponse
// @Security BearerAuth
// @Router /admin/v1/users/{id}/confirm-email [post]
func (h *AdminHandler) ConfirmUserEmail(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var user model.User
	if err := h.db.Where("id = ?", id).First(&user).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "user not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get user"})
		return
	}

	if user.EmailConfirmedAt != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "email already confirmed"})
		return
	}

	now := time.Now()
	if err := h.db.Model(&user).Update("email_confirmed_at", now).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to confirm email"})
		return
	}

	slog.Info("admin: user email confirmed", "user_id", id)

	user.EmailConfirmedAt = &now
	writeJSON(w, http.StatusOK, toAdminUserResponse(user))
}

// DeleteUser handles DELETE /admin/v1/users/{id}.
// @Summary Delete a user
// @Description Permanently deletes a user and all associated data.
// @Tags admin
// @Produce json
// @Param id path string true "User ID"
// @Success 200 {object} map[string]string
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /admin/v1/users/{id} [delete]
func (h *AdminHandler) DeleteUser(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	uid, err := uuid.Parse(id)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid user ID"})
		return
	}

	var user model.User
	if err := h.db.Where("id = ?", uid).First(&user).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "user not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get user"})
		return
	}

	// Delete in transaction: memberships, refresh tokens, oauth accounts, then user
	if err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("user_id = ?", uid).Delete(&model.OrgMembership{}).Error; err != nil {
			return err
		}
		if err := tx.Where("user_id = ?", uid).Delete(&model.RefreshToken{}).Error; err != nil {
			return err
		}
		if err := tx.Where("user_id = ?", uid).Delete(&model.OAuthAccount{}).Error; err != nil {
			return err
		}
		if err := tx.Where("user_id = ?", uid).Delete(&model.EmailVerification{}).Error; err != nil {
			return err
		}
		if err := tx.Where("user_id = ?", uid).Delete(&model.PasswordReset{}).Error; err != nil {
			return err
		}
		if err := tx.Delete(&user).Error; err != nil {
			return err
		}
		return nil
	}); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to delete user"})
		return
	}

	slog.Info("admin: user deleted", "user_id", id, "email", user.Email)
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// ---------------------------------------------------------------------------
// Organizations
// ---------------------------------------------------------------------------

// ListOrgs handles GET /admin/v1/orgs.
// @Summary List all organizations
// @Description Returns all organizations with optional filters.
// @Tags admin
// @Produce json
// @Param search query string false "Search by name"
// @Param active query string false "Filter by active status (true/false)"
// @Param limit query int false "Page size"
// @Param cursor query string false "Pagination cursor"
// @Success 200 {object} paginatedResponse[adminOrgResponse]
// @Security BearerAuth
// @Router /admin/v1/orgs [get]
func (h *AdminHandler) ListOrgs(w http.ResponseWriter, r *http.Request) {
	limit, cursor, err := parsePagination(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	q := h.db.Model(&model.Org{})

	if search := r.URL.Query().Get("search"); search != "" {
		q = q.Where("name ILIKE ?", "%"+search+"%")
	}
	if r.URL.Query().Get("active") == "true" {
		q = q.Where("active = true")
	} else if r.URL.Query().Get("active") == "false" {
		q = q.Where("active = false")
	}

	q = applyPagination(q, cursor, limit)

	var orgs []model.Org
	if err := q.Find(&orgs).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list orgs"})
		return
	}

	hasMore := len(orgs) > limit
	if hasMore {
		orgs = orgs[:limit]
	}

	resp := make([]adminOrgResponse, len(orgs))
	for i, o := range orgs {
		resp[i] = toAdminOrgResponse(o)
	}

	result := paginatedResponse[adminOrgResponse]{Data: resp, HasMore: hasMore}
	if hasMore {
		last := orgs[len(orgs)-1]
		c := encodeCursor(last.CreatedAt, last.ID)
		result.NextCursor = &c
	}
	writeJSON(w, http.StatusOK, result)
}

// GetOrg handles GET /admin/v1/orgs/{id}.
// @Summary Get organization details
// @Description Returns org details with member, credential, agent, and sandbox counts.
// @Tags admin
// @Produce json
// @Param id path string true "Org ID"
// @Success 200 {object} adminOrgDetailResponse
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /admin/v1/orgs/{id} [get]
func (h *AdminHandler) GetOrg(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var org model.Org
	if err := h.db.Where("id = ?", id).First(&org).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "org not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get org"})
		return
	}

	detail := adminOrgDetailResponse{adminOrgResponse: toAdminOrgResponse(org)}
	h.db.Model(&model.OrgMembership{}).Where("org_id = ?", org.ID).Count(&detail.MemberCount)
	h.db.Model(&model.Credential{}).Where("org_id = ? AND revoked_at IS NULL", org.ID).Count(&detail.CredentialCount)
	h.db.Model(&model.Agent{}).Where("org_id = ? AND status = ?", org.ID, "active").Count(&detail.AgentCount)
	h.db.Model(&model.Sandbox{}).Where("org_id = ?", org.ID).Count(&detail.SandboxCount)

	writeJSON(w, http.StatusOK, detail)
}

// UpdateOrg handles PUT /admin/v1/orgs/{id}.
// @Summary Update organization
// @Description Updates org settings (rate_limit, active, allowed_origins).
// @Tags admin
// @Accept json
// @Produce json
// @Param id path string true "Org ID"
// @Success 200 {object} adminOrgResponse
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /admin/v1/orgs/{id} [put]
func (h *AdminHandler) UpdateOrg(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var org model.Org
	if err := h.db.Where("id = ?", id).First(&org).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "org not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get org"})
		return
	}

	var req struct {
		RateLimit      *int      `json:"rate_limit,omitempty"`
		Active         *bool     `json:"active,omitempty"`
		AllowedOrigins *[]string `json:"allowed_origins,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	updates := map[string]any{}
	if req.RateLimit != nil {
		updates["rate_limit"] = *req.RateLimit
	}
	if req.Active != nil {
		updates["active"] = *req.Active
	}
	if req.AllowedOrigins != nil {
		updates["allowed_origins"] = *req.AllowedOrigins
	}

	if len(updates) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no fields to update"})
		return
	}

	oldOrg := map[string]any{"rate_limit": org.RateLimit, "active": org.Active, "allowed_origins": org.AllowedOrigins}
	setAuditDiff(r, oldOrg, updates)

	if err := h.db.Model(&org).Updates(updates).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update org"})
		return
	}

	h.db.Where("id = ?", id).First(&org)
	slog.Info("admin: org updated", "org_id", id)
	writeJSON(w, http.StatusOK, toAdminOrgResponse(org))
}

// DeactivateOrg handles POST /admin/v1/orgs/{id}/deactivate.
// @Summary Deactivate organization
// @Description Deactivates an organization, blocking all API access.
// @Tags admin
// @Produce json
// @Param id path string true "Org ID"
// @Success 200 {object} map[string]string
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /admin/v1/orgs/{id}/deactivate [post]
func (h *AdminHandler) DeactivateOrg(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	result := h.db.Model(&model.Org{}).Where("id = ? AND active = true", id).Update("active", false)
	if result.Error != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to deactivate org"})
		return
	}
	if result.RowsAffected == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "org not found or already inactive"})
		return
	}

	slog.Info("admin: org deactivated", "org_id", id)
	writeJSON(w, http.StatusOK, map[string]string{"status": "deactivated"})
}

// ActivateOrg handles POST /admin/v1/orgs/{id}/activate.
// @Summary Activate organization
// @Description Reactivates a previously deactivated organization.
// @Tags admin
// @Produce json
// @Param id path string true "Org ID"
// @Success 200 {object} map[string]string
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /admin/v1/orgs/{id}/activate [post]
func (h *AdminHandler) ActivateOrg(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	result := h.db.Model(&model.Org{}).Where("id = ? AND active = false", id).Update("active", true)
	if result.Error != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to activate org"})
		return
	}
	if result.RowsAffected == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "org not found or already active"})
		return
	}

	slog.Info("admin: org activated", "org_id", id)
	writeJSON(w, http.StatusOK, map[string]string{"status": "activated"})
}

// ListOrgMembers handles GET /admin/v1/orgs/{id}/members.
// @Summary List organization members
// @Description Returns all members of an organization.
// @Tags admin
// @Produce json
// @Param id path string true "Org ID"
// @Success 200 {object} map[string]any
// @Security BearerAuth
// @Router /admin/v1/orgs/{id}/members [get]
func (h *AdminHandler) ListOrgMembers(w http.ResponseWriter, r *http.Request) {
	orgID := chi.URLParam(r, "id")

	var memberships []model.OrgMembership
	if err := h.db.Preload("User").Where("org_id = ?", orgID).Find(&memberships).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list members"})
		return
	}

	resp := make([]adminMembershipResponse, len(memberships))
	for i, m := range memberships {
		resp[i] = adminMembershipResponse{
			ID:        m.ID.String(),
			UserID:    m.UserID.String(),
			OrgID:     m.OrgID.String(),
			Role:      m.Role,
			UserEmail: m.User.Email,
			UserName:  m.User.Name,
			CreatedAt: m.CreatedAt.Format(time.RFC3339),
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": resp})
}

// DeleteOrg handles DELETE /admin/v1/orgs/{id}.
// @Summary Delete organization
// @Description Permanently deletes an organization and cascades to related data.
// @Tags admin
// @Produce json
// @Param id path string true "Org ID"
// @Success 200 {object} map[string]string
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /admin/v1/orgs/{id} [delete]
func (h *AdminHandler) DeleteOrg(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	uid, err := uuid.Parse(id)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid org ID"})
		return
	}

	var org model.Org
	if err := h.db.Where("id = ?", uid).First(&org).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "org not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get org"})
		return
	}

	// Cascade delete: let DB foreign keys handle most cascades
	if err := h.db.Delete(&org).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to delete org"})
		return
	}

	slog.Info("admin: org deleted", "org_id", id, "name", org.Name)
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// ---------------------------------------------------------------------------
// Credentials
// ---------------------------------------------------------------------------

// ListCredentials handles GET /admin/v1/credentials.
// @Summary List all credentials
// @Description Returns credentials across all organizations.
// @Tags admin
// @Produce json
// @Param org_id query string false "Filter by org ID"
// @Param provider_id query string false "Filter by provider ID"
// @Param revoked query string false "Filter by revoked status (true/false)"
// @Param limit query int false "Page size"
// @Param cursor query string false "Pagination cursor"
// @Success 200 {object} paginatedResponse[adminCredentialResponse]
// @Security BearerAuth
// @Router /admin/v1/credentials [get]
func (h *AdminHandler) ListCredentials(w http.ResponseWriter, r *http.Request) {
	limit, cursor, err := parsePagination(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	q := h.db.Model(&model.Credential{})
	if orgID := r.URL.Query().Get("org_id"); orgID != "" {
		q = q.Where("org_id = ?", orgID)
	}
	if provider := r.URL.Query().Get("provider_id"); provider != "" {
		q = q.Where("provider_id = ?", provider)
	}
	if r.URL.Query().Get("revoked") == "true" {
		q = q.Where("revoked_at IS NOT NULL")
	} else if r.URL.Query().Get("revoked") == "false" {
		q = q.Where("revoked_at IS NULL")
	}

	q = applyPagination(q, cursor, limit)

	var creds []model.Credential
	if err := q.Find(&creds).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list credentials"})
		return
	}

	hasMore := len(creds) > limit
	if hasMore {
		creds = creds[:limit]
	}

	resp := make([]adminCredentialResponse, len(creds))
	for i, c := range creds {
		resp[i] = toAdminCredentialResponse(c)
	}

	result := paginatedResponse[adminCredentialResponse]{Data: resp, HasMore: hasMore}
	if hasMore {
		last := creds[len(creds)-1]
		c := encodeCursor(last.CreatedAt, last.ID)
		result.NextCursor = &c
	}
	writeJSON(w, http.StatusOK, result)
}

// GetCredential handles GET /admin/v1/credentials/{id}.
// @Summary Get credential details
// @Description Returns credential metadata (no decrypted key).
// @Tags admin
// @Produce json
// @Param id path string true "Credential ID"
// @Success 200 {object} adminCredentialResponse
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /admin/v1/credentials/{id} [get]
func (h *AdminHandler) GetCredential(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var cred model.Credential
	if err := h.db.Where("id = ?", id).First(&cred).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "credential not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get credential"})
		return
	}

	writeJSON(w, http.StatusOK, toAdminCredentialResponse(cred))
}

// RevokeCredential handles POST /admin/v1/credentials/{id}/revoke.
// @Summary Revoke a credential
// @Description Force-revokes a credential.
// @Tags admin
// @Produce json
// @Param id path string true "Credential ID"
// @Success 200 {object} map[string]string
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /admin/v1/credentials/{id}/revoke [post]
func (h *AdminHandler) RevokeCredential(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	now := time.Now()

	result := h.db.Model(&model.Credential{}).Where("id = ? AND revoked_at IS NULL", id).Update("revoked_at", now)
	if result.Error != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to revoke credential"})
		return
	}
	if result.RowsAffected == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "credential not found or already revoked"})
		return
	}

	slog.Info("admin: credential revoked", "credential_id", id)
	writeJSON(w, http.StatusOK, map[string]string{"status": "revoked"})
}

// ---------------------------------------------------------------------------
// API Keys
// ---------------------------------------------------------------------------

// ListAPIKeys handles GET /admin/v1/api-keys.
// @Summary List all API keys
// @Description Returns API keys across all organizations.
// @Tags admin
// @Produce json
// @Param org_id query string false "Filter by org ID"
// @Param revoked query string false "Filter by revoked status (true/false)"
// @Param limit query int false "Page size"
// @Param cursor query string false "Pagination cursor"
// @Success 200 {object} paginatedResponse[adminAPIKeyResponse]
// @Security BearerAuth
// @Router /admin/v1/api-keys [get]
func (h *AdminHandler) ListAPIKeys(w http.ResponseWriter, r *http.Request) {
	limit, cursor, err := parsePagination(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	q := h.db.Model(&model.APIKey{})
	if orgID := r.URL.Query().Get("org_id"); orgID != "" {
		q = q.Where("org_id = ?", orgID)
	}
	if r.URL.Query().Get("revoked") == "true" {
		q = q.Where("revoked_at IS NOT NULL")
	} else if r.URL.Query().Get("revoked") == "false" {
		q = q.Where("revoked_at IS NULL")
	}

	q = applyPagination(q, cursor, limit)

	var keys []model.APIKey
	if err := q.Find(&keys).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list api keys"})
		return
	}

	hasMore := len(keys) > limit
	if hasMore {
		keys = keys[:limit]
	}

	resp := make([]adminAPIKeyResponse, len(keys))
	for i, k := range keys {
		resp[i] = toAdminAPIKeyResponse(k)
	}

	result := paginatedResponse[adminAPIKeyResponse]{Data: resp, HasMore: hasMore}
	if hasMore {
		last := keys[len(keys)-1]
		c := encodeCursor(last.CreatedAt, last.ID)
		result.NextCursor = &c
	}
	writeJSON(w, http.StatusOK, result)
}

// RevokeAPIKey handles POST /admin/v1/api-keys/{id}/revoke.
// @Summary Revoke an API key
// @Description Force-revokes an API key.
// @Tags admin
// @Produce json
// @Param id path string true "API Key ID"
// @Success 200 {object} map[string]string
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /admin/v1/api-keys/{id}/revoke [post]
func (h *AdminHandler) RevokeAPIKey(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	now := time.Now()

	result := h.db.Model(&model.APIKey{}).Where("id = ? AND revoked_at IS NULL", id).Update("revoked_at", now)
	if result.Error != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to revoke api key"})
		return
	}
	if result.RowsAffected == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "api key not found or already revoked"})
		return
	}

	slog.Info("admin: api key revoked", "api_key_id", id)
	writeJSON(w, http.StatusOK, map[string]string{"status": "revoked"})
}

// ---------------------------------------------------------------------------
// Tokens
// ---------------------------------------------------------------------------

// ListTokens handles GET /admin/v1/tokens.
// @Summary List all proxy tokens
// @Description Returns proxy tokens across all organizations.
// @Tags admin
// @Produce json
// @Param org_id query string false "Filter by org ID"
// @Param revoked query string false "Filter by revoked status (true/false)"
// @Param limit query int false "Page size"
// @Param cursor query string false "Pagination cursor"
// @Success 200 {object} paginatedResponse[adminTokenResponse]
// @Security BearerAuth
// @Router /admin/v1/tokens [get]
func (h *AdminHandler) ListTokens(w http.ResponseWriter, r *http.Request) {
	limit, cursor, err := parsePagination(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	q := h.db.Model(&model.Token{})
	if orgID := r.URL.Query().Get("org_id"); orgID != "" {
		q = q.Where("org_id = ?", orgID)
	}
	if r.URL.Query().Get("revoked") == "true" {
		q = q.Where("revoked_at IS NOT NULL")
	} else if r.URL.Query().Get("revoked") == "false" {
		q = q.Where("revoked_at IS NULL")
	}

	q = applyPagination(q, cursor, limit)

	var tokens []model.Token
	if err := q.Find(&tokens).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list tokens"})
		return
	}

	hasMore := len(tokens) > limit
	if hasMore {
		tokens = tokens[:limit]
	}

	resp := make([]adminTokenResponse, len(tokens))
	for i, t := range tokens {
		resp[i] = toAdminTokenResponse(t)
	}

	result := paginatedResponse[adminTokenResponse]{Data: resp, HasMore: hasMore}
	if hasMore {
		last := tokens[len(tokens)-1]
		c := encodeCursor(last.CreatedAt, last.ID)
		result.NextCursor = &c
	}
	writeJSON(w, http.StatusOK, result)
}

// RevokeToken handles POST /admin/v1/tokens/{id}/revoke.
// @Summary Revoke a proxy token
// @Description Force-revokes a proxy token.
// @Tags admin
// @Produce json
// @Param id path string true "Token ID"
// @Success 200 {object} map[string]string
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /admin/v1/tokens/{id}/revoke [post]
func (h *AdminHandler) RevokeToken(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	now := time.Now()

	result := h.db.Model(&model.Token{}).Where("id = ? AND revoked_at IS NULL", id).Update("revoked_at", now)
	if result.Error != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to revoke token"})
		return
	}
	if result.RowsAffected == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "token not found or already revoked"})
		return
	}

	slog.Info("admin: token revoked", "token_id", id)
	writeJSON(w, http.StatusOK, map[string]string{"status": "revoked"})
}

// ---------------------------------------------------------------------------
// Agents
// ---------------------------------------------------------------------------

// ListAgents handles GET /admin/v1/agents.
// @Summary List all agents
// @Description Returns agents across all organizations.
// @Tags admin
// @Produce json
// @Param org_id query string false "Filter by org ID"
// @Param status query string false "Filter by status (active, archived)"
// @Param sandbox_type query string false "Filter by sandbox type (shared, dedicated)"
// @Param limit query int false "Page size"
// @Param cursor query string false "Pagination cursor"
// @Success 200 {object} paginatedResponse[adminAgentResponse]
// @Security BearerAuth
// @Router /admin/v1/agents [get]
func (h *AdminHandler) ListAgents(w http.ResponseWriter, r *http.Request) {
	limit, cursor, err := parsePagination(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	q := h.db.Model(&model.Agent{})
	if orgID := r.URL.Query().Get("org_id"); orgID != "" {
		q = q.Where("org_id = ?", orgID)
	}
	if status := r.URL.Query().Get("status"); status != "" {
		q = q.Where("status = ?", status)
	}
	if stype := r.URL.Query().Get("sandbox_type"); stype != "" {
		q = q.Where("sandbox_type = ?", stype)
	}

	q = applyPagination(q, cursor, limit)

	var agents []model.Agent
	if err := q.Find(&agents).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list agents"})
		return
	}

	hasMore := len(agents) > limit
	if hasMore {
		agents = agents[:limit]
	}

	resp := make([]adminAgentResponse, len(agents))
	for i, a := range agents {
		resp[i] = toAdminAgentResponse(a)
	}

	result := paginatedResponse[adminAgentResponse]{Data: resp, HasMore: hasMore}
	if hasMore {
		last := agents[len(agents)-1]
		c := encodeCursor(last.CreatedAt, last.ID)
		result.NextCursor = &c
	}
	writeJSON(w, http.StatusOK, result)
}

// GetAgent handles GET /admin/v1/agents/{id}.
// @Summary Get agent details
// @Description Returns agent details.
// @Tags admin
// @Produce json
// @Param id path string true "Agent ID"
// @Success 200 {object} adminAgentResponse
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /admin/v1/agents/{id} [get]
func (h *AdminHandler) GetAgent(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var agent model.Agent
	if err := h.db.Where("id = ?", id).First(&agent).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "agent not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get agent"})
		return
	}

	writeJSON(w, http.StatusOK, toAdminAgentResponse(agent))
}

// ArchiveAgent handles POST /admin/v1/agents/{id}/archive.
// @Summary Archive an agent
// @Description Force-archives an active agent.
// @Tags admin
// @Produce json
// @Param id path string true "Agent ID"
// @Success 200 {object} map[string]string
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /admin/v1/agents/{id}/archive [post]
func (h *AdminHandler) ArchiveAgent(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	result := h.db.Model(&model.Agent{}).Where("id = ? AND status = ?", id, "active").Update("status", "archived")
	if result.Error != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to archive agent"})
		return
	}
	if result.RowsAffected == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "agent not found or already archived"})
		return
	}

	slog.Info("admin: agent archived", "agent_id", id)
	writeJSON(w, http.StatusOK, map[string]string{"status": "archived"})
}

// DeleteAgent handles DELETE /admin/v1/agents/{id}.
// @Summary Delete an agent
// @Description Permanently deletes an agent.
// @Tags admin
// @Produce json
// @Param id path string true "Agent ID"
// @Success 200 {object} map[string]string
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /admin/v1/agents/{id} [delete]
func (h *AdminHandler) DeleteAgent(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var agent model.Agent
	if err := h.db.Where("id = ?", id).First(&agent).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "agent not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get agent"})
		return
	}

	if err := h.db.Delete(&agent).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to delete agent"})
		return
	}

	slog.Info("admin: agent deleted", "agent_id", id)
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// ---------------------------------------------------------------------------
// Sandboxes
// ---------------------------------------------------------------------------

// ListSandboxes handles GET /admin/v1/sandboxes.
// @Summary List all sandboxes
// @Description Returns sandboxes across all organizations with resource metrics.
// @Tags admin
// @Produce json
// @Param org_id query string false "Filter by org ID"
// @Param status query string false "Filter by status (running, stopped, error)"
// @Param sandbox_type query string false "Filter by type (shared, dedicated)"
// @Param limit query int false "Page size"
// @Param cursor query string false "Pagination cursor"
// @Success 200 {object} paginatedResponse[adminSandboxResponse]
// @Security BearerAuth
// @Router /admin/v1/sandboxes [get]
func (h *AdminHandler) ListSandboxes(w http.ResponseWriter, r *http.Request) {
	limit, cursor, err := parsePagination(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	q := h.db.Model(&model.Sandbox{})
	if orgID := r.URL.Query().Get("org_id"); orgID != "" {
		q = q.Where("org_id = ?", orgID)
	}
	if status := r.URL.Query().Get("status"); status != "" {
		q = q.Where("status = ?", status)
	}
	if stype := r.URL.Query().Get("sandbox_type"); stype != "" {
		q = q.Where("sandbox_type = ?", stype)
	}

	q = applyPagination(q, cursor, limit)

	var sandboxes []model.Sandbox
	if err := q.Find(&sandboxes).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list sandboxes"})
		return
	}

	hasMore := len(sandboxes) > limit
	if hasMore {
		sandboxes = sandboxes[:limit]
	}

	resp := make([]adminSandboxResponse, len(sandboxes))
	for i, s := range sandboxes {
		resp[i] = toAdminSandboxResponse(s)
	}

	result := paginatedResponse[adminSandboxResponse]{Data: resp, HasMore: hasMore}
	if hasMore {
		last := sandboxes[len(sandboxes)-1]
		c := encodeCursor(last.CreatedAt, last.ID)
		result.NextCursor = &c
	}
	writeJSON(w, http.StatusOK, result)
}

// GetSandbox handles GET /admin/v1/sandboxes/{id}.
// @Summary Get sandbox details
// @Description Returns sandbox details with resource metrics.
// @Tags admin
// @Produce json
// @Param id path string true "Sandbox ID"
// @Success 200 {object} adminSandboxResponse
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /admin/v1/sandboxes/{id} [get]
func (h *AdminHandler) GetSandbox(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var sb model.Sandbox
	if err := h.db.Where("id = ?", id).First(&sb).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "sandbox not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get sandbox"})
		return
	}

	writeJSON(w, http.StatusOK, toAdminSandboxResponse(sb))
}

// StopSandbox handles POST /admin/v1/sandboxes/{id}/stop.
// @Summary Stop a sandbox
// @Description Force-stops a running sandbox.
// @Tags admin
// @Produce json
// @Param id path string true "Sandbox ID"
// @Success 200 {object} map[string]string
// @Failure 404 {object} errorResponse
// @Failure 503 {object} errorResponse
// @Security BearerAuth
// @Router /admin/v1/sandboxes/{id}/stop [post]
func (h *AdminHandler) StopSandbox(w http.ResponseWriter, r *http.Request) {
	if h.orchestrator == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "sandbox orchestrator not configured"})
		return
	}

	id := chi.URLParam(r, "id")
	var sb model.Sandbox
	if err := h.db.Where("id = ?", id).First(&sb).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "sandbox not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get sandbox"})
		return
	}

	if err := h.orchestrator.StopSandbox(r.Context(), &sb); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to stop sandbox"})
		return
	}

	slog.Info("admin: sandbox stopped", "sandbox_id", id)
	writeJSON(w, http.StatusOK, map[string]string{"status": "stopped"})
}

// DeleteSandbox handles DELETE /admin/v1/sandboxes/{id}.
// @Summary Delete a sandbox
// @Description Force-deletes a sandbox from the provider and DB.
// @Tags admin
// @Produce json
// @Param id path string true "Sandbox ID"
// @Success 200 {object} map[string]string
// @Failure 404 {object} errorResponse
// @Failure 503 {object} errorResponse
// @Security BearerAuth
// @Router /admin/v1/sandboxes/{id} [delete]
func (h *AdminHandler) DeleteSandbox(w http.ResponseWriter, r *http.Request) {
	if h.orchestrator == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "sandbox orchestrator not configured"})
		return
	}

	id := chi.URLParam(r, "id")
	var sb model.Sandbox
	if err := h.db.Where("id = ?", id).First(&sb).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "sandbox not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get sandbox"})
		return
	}

	if err := h.orchestrator.DeleteSandbox(r.Context(), &sb); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to delete sandbox"})
		return
	}

	slog.Info("admin: sandbox deleted", "sandbox_id", id)
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// CleanupSandboxes handles POST /admin/v1/sandboxes/cleanup.
// @Summary Bulk cleanup sandboxes
// @Description Deletes all errored and stale stopped sandboxes (stopped > 24h).
// @Tags admin
// @Produce json
// @Success 200 {object} map[string]any
// @Failure 503 {object} errorResponse
// @Security BearerAuth
// @Router /admin/v1/sandboxes/cleanup [post]
func (h *AdminHandler) CleanupSandboxes(w http.ResponseWriter, r *http.Request) {
	if h.orchestrator == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "sandbox orchestrator not configured"})
		return
	}

	// Find all sandboxes in error state or stopped for > 24h
	var sandboxes []model.Sandbox
	cutoff := time.Now().Add(-24 * time.Hour)
	h.db.Where("status = 'error' OR (status = 'stopped' AND updated_at < ?)", cutoff).Find(&sandboxes)

	deleted := 0
	for _, sb := range sandboxes {
		if err := h.orchestrator.DeleteSandbox(r.Context(), &sb); err != nil {
			slog.Warn("admin: cleanup failed for sandbox", "sandbox_id", sb.ID, "error", err)
			continue
		}
		deleted++
	}

	slog.Info("admin: sandbox cleanup", "found", len(sandboxes), "deleted", deleted)
	writeJSON(w, http.StatusOK, map[string]any{
		"found":   len(sandboxes),
		"deleted": deleted,
	})
}

// ---------------------------------------------------------------------------
// Sandbox Templates
// ---------------------------------------------------------------------------

// ListSandboxTemplates handles GET /admin/v1/sandbox-templates.
// @Summary List all sandbox templates
// @Description Returns sandbox templates across all organizations. Use scope=public to list only public templates.
// @Tags admin
// @Produce json
// @Param org_id query string false "Filter by org ID"
// @Param scope query string false "Filter by scope (public = org_id IS NULL)"
// @Param build_status query string false "Filter by build status (pending, building, ready, failed)"
// @Param limit query int false "Page size"
// @Param cursor query string false "Pagination cursor"
// @Success 200 {object} paginatedResponse[adminSandboxTemplateResponse]
// @Security BearerAuth
// @Router /admin/v1/sandbox-templates [get]
func (h *AdminHandler) ListSandboxTemplates(w http.ResponseWriter, r *http.Request) {
	limit, cursor, err := parsePagination(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	q := h.db.Model(&model.SandboxTemplate{})
	if scope := r.URL.Query().Get("scope"); scope == "public" {
		q = q.Where("org_id IS NULL")
	} else if orgID := r.URL.Query().Get("org_id"); orgID != "" {
		q = q.Where("org_id = ?", orgID)
	}
	if status := r.URL.Query().Get("build_status"); status != "" {
		q = q.Where("build_status = ?", status)
	}

	q = applyPagination(q, cursor, limit)

	var templates []model.SandboxTemplate
	if err := q.Find(&templates).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list sandbox templates"})
		return
	}

	hasMore := len(templates) > limit
	if hasMore {
		templates = templates[:limit]
	}

	resp := make([]adminSandboxTemplateResponse, len(templates))
	for i, t := range templates {
		resp[i] = toAdminSandboxTemplateResponse(t)
	}

	result := paginatedResponse[adminSandboxTemplateResponse]{Data: resp, HasMore: hasMore}
	if hasMore {
		last := templates[len(templates)-1]
		c := encodeCursor(last.CreatedAt, last.ID)
		result.NextCursor = &c
	}
	writeJSON(w, http.StatusOK, result)
}

// DeleteSandboxTemplate handles DELETE /admin/v1/sandbox-templates/{id}.
// @Summary Delete a sandbox template
// @Description Permanently deletes a sandbox template.
// @Tags admin
// @Produce json
// @Param id path string true "Template ID"
// @Success 200 {object} map[string]string
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /admin/v1/sandbox-templates/{id} [delete]
func (h *AdminHandler) DeleteSandboxTemplate(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var tmpl model.SandboxTemplate
	if err := h.db.Where("id = ?", id).First(&tmpl).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "sandbox template not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get sandbox template"})
		return
	}

	if err := h.db.Delete(&tmpl).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to delete sandbox template"})
		return
	}

	slog.Info("admin: sandbox template deleted", "template_id", id)
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// ---------------------------------------------------------------------------
// Conversations
// ---------------------------------------------------------------------------

// ListConversations handles GET /admin/v1/conversations.
// @Summary List all conversations
// @Description Returns agent conversations across all organizations.
// @Tags admin
// @Produce json
// @Param org_id query string false "Filter by org ID"
// @Param agent_id query string false "Filter by agent ID"
// @Param status query string false "Filter by status (active, ended, error)"
// @Param limit query int false "Page size"
// @Param cursor query string false "Pagination cursor"
// @Success 200 {object} paginatedResponse[adminConversationResponse]
// @Security BearerAuth
// @Router /admin/v1/conversations [get]
func (h *AdminHandler) ListConversations(w http.ResponseWriter, r *http.Request) {
	limit, cursor, err := parsePagination(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	q := h.db.Model(&model.AgentConversation{})
	if orgID := r.URL.Query().Get("org_id"); orgID != "" {
		q = q.Where("org_id = ?", orgID)
	}
	if agentID := r.URL.Query().Get("agent_id"); agentID != "" {
		q = q.Where("agent_id = ?", agentID)
	}
	if status := r.URL.Query().Get("status"); status != "" {
		q = q.Where("status = ?", status)
	}

	q = applyPagination(q, cursor, limit)

	var conversations []model.AgentConversation
	if err := q.Find(&conversations).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list conversations"})
		return
	}

	hasMore := len(conversations) > limit
	if hasMore {
		conversations = conversations[:limit]
	}

	resp := make([]adminConversationResponse, len(conversations))
	for i, c := range conversations {
		resp[i] = toAdminConversationResponse(c)
	}

	result := paginatedResponse[adminConversationResponse]{Data: resp, HasMore: hasMore}
	if hasMore {
		last := conversations[len(conversations)-1]
		c := encodeCursor(last.CreatedAt, last.ID)
		result.NextCursor = &c
	}
	writeJSON(w, http.StatusOK, result)
}

// GetConversation handles GET /admin/v1/conversations/{id}.
// @Summary Get conversation details
// @Description Returns conversation details with event count.
// @Tags admin
// @Produce json
// @Param id path string true "Conversation ID"
// @Success 200 {object} adminConversationResponse
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /admin/v1/conversations/{id} [get]
func (h *AdminHandler) GetConversation(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var conv model.AgentConversation
	if err := h.db.Where("id = ?", id).First(&conv).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "conversation not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get conversation"})
		return
	}

	// Also load events
	var events []model.ConversationEvent
	h.db.Where("conversation_id = ?", conv.ID).Order("created_at ASC").Find(&events)

	type detailResponse struct {
		adminConversationResponse
		EventCount int `json:"event_count"`
	}

	writeJSON(w, http.StatusOK, detailResponse{
		adminConversationResponse: toAdminConversationResponse(conv),
		EventCount:                len(events),
	})
}

// EndConversation handles DELETE /admin/v1/conversations/{id}.
// @Summary End a conversation
// @Description Force-ends an active conversation.
// @Tags admin
// @Produce json
// @Param id path string true "Conversation ID"
// @Success 200 {object} map[string]string
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /admin/v1/conversations/{id} [delete]
func (h *AdminHandler) EndConversation(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	now := time.Now()

	result := h.db.Model(&model.AgentConversation{}).
		Where("id = ? AND status = ?", id, "active").
		Updates(map[string]any{"status": "ended", "ended_at": now})
	if result.Error != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to end conversation"})
		return
	}
	if result.RowsAffected == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "conversation not found or not active"})
		return
	}

	slog.Info("admin: conversation ended", "conversation_id", id)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ended"})
}


// ---------------------------------------------------------------------------
// Generations
// ---------------------------------------------------------------------------

// ListGenerations handles GET /admin/v1/generations.
// @Summary List all generations
// @Description Returns LLM generations across all organizations.
// @Tags admin
// @Produce json
// @Param org_id query string false "Filter by org ID"
// @Param provider_id query string false "Filter by provider ID"
// @Param model query string false "Filter by model name"
// @Param limit query int false "Page size"
// @Success 200 {object} map[string]any
// @Security BearerAuth
// @Router /admin/v1/generations [get]
func (h *AdminHandler) ListGenerations(w http.ResponseWriter, r *http.Request) {
	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := parseInt(l); err == nil && n > 0 {
			if n > 100 {
				n = 100
			}
			limit = n
		}
	}

	q := h.db.Model(&model.Generation{})
	if orgID := r.URL.Query().Get("org_id"); orgID != "" {
		q = q.Where("org_id = ?", orgID)
	}
	if provider := r.URL.Query().Get("provider_id"); provider != "" {
		q = q.Where("provider_id = ?", provider)
	}
	if m := r.URL.Query().Get("model"); m != "" {
		q = q.Where("model = ?", m)
	}

	q = q.Order("created_at DESC").Limit(limit + 1)

	var gens []model.Generation
	if err := q.Find(&gens).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list generations"})
		return
	}

	hasMore := len(gens) > limit
	if hasMore {
		gens = gens[:limit]
	}

	resp := make([]adminGenerationResponse, len(gens))
	for i, g := range gens {
		resp[i] = toAdminGenerationResponse(g)
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": resp, "has_more": hasMore})
}

// GenerationStats handles GET /admin/v1/generations/stats.
// @Summary Generation statistics
// @Description Returns aggregate generation statistics by provider and model.
// @Tags admin
// @Produce json
// @Param org_id query string false "Filter by org ID"
// @Success 200 {object} adminGenerationStatsResponse
// @Security BearerAuth
// @Router /admin/v1/generations/stats [get]
func (h *AdminHandler) GenerationStats(w http.ResponseWriter, r *http.Request) {
	var stats adminGenerationStatsResponse

	q := h.db.Model(&model.Generation{})
	if orgID := r.URL.Query().Get("org_id"); orgID != "" {
		q = q.Where("org_id = ?", orgID)
	}

	var totals struct {
		Count  int64
		Cost   float64
		Input  int64
		Output int64
	}
	q.Select("COUNT(*) as count, COALESCE(SUM(cost), 0) as cost, COALESCE(SUM(input_tokens), 0) as input, COALESCE(SUM(output_tokens), 0) as output").Scan(&totals)

	stats.TotalGenerations = totals.Count
	stats.TotalCost = totals.Cost
	stats.TotalInput = totals.Input
	stats.TotalOutput = totals.Output

	// By provider
	q2 := h.db.Model(&model.Generation{})
	if orgID := r.URL.Query().Get("org_id"); orgID != "" {
		q2 = q2.Where("org_id = ?", orgID)
	}
	q2.Select("provider_id, COUNT(*) as count, COALESCE(SUM(cost), 0) as cost, COALESCE(SUM(input_tokens), 0) as input_tokens, COALESCE(SUM(output_tokens), 0) as output_tokens").
		Group("provider_id").Order("count DESC").Limit(20).Scan(&stats.ByProvider)

	// By model
	q3 := h.db.Model(&model.Generation{})
	if orgID := r.URL.Query().Get("org_id"); orgID != "" {
		q3 = q3.Where("org_id = ?", orgID)
	}
	q3.Select("model, COUNT(*) as count, COALESCE(SUM(cost), 0) as cost, COALESCE(SUM(input_tokens), 0) as input_tokens, COALESCE(SUM(output_tokens), 0) as output_tokens").
		Group("model").Order("count DESC").Limit(20).Scan(&stats.ByModel)

	if stats.ByProvider == nil {
		stats.ByProvider = []adminProviderStatEntry{}
	}
	if stats.ByModel == nil {
		stats.ByModel = []adminModelStatEntry{}
	}

	writeJSON(w, http.StatusOK, stats)
}

// ---------------------------------------------------------------------------

// ListInIntegrations handles GET /admin/v1/in-integrations.
// @Summary List platform integrations
// @Description Returns all app-owned (platform) integrations.
// @Tags admin
// @Produce json
// @Success 200 {object} map[string]any
// @Security BearerAuth
// @Router /admin/v1/in-integrations [get]
func (h *AdminHandler) ListInIntegrations(w http.ResponseWriter, r *http.Request) {
	var integrations []model.InIntegration
	if err := h.db.Order("created_at DESC").Find(&integrations).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list in-integrations"})
		return
	}

	resp := make([]adminInIntegrationResponse, len(integrations))
	for i, integ := range integrations {
		resp[i] = toAdminInIntegrationResponse(integ)
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": resp})
}

// ListInConnections handles GET /admin/v1/in-connections.
// @Summary List user connections to platform integrations
// @Description Returns all user connections to app-owned integrations.
// @Tags admin
// @Produce json
// @Param user_id query string false "Filter by user ID"
// @Param revoked query string false "Filter by revoked status (true/false)"
// @Param limit query int false "Page size"
// @Param cursor query string false "Pagination cursor"
// @Success 200 {object} map[string]any
// @Security BearerAuth
// @Router /admin/v1/in-connections [get]
func (h *AdminHandler) ListInConnections(w http.ResponseWriter, r *http.Request) {
	limit, cursor, err := parsePagination(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	q := h.db.Model(&model.InConnection{})
	if userID := r.URL.Query().Get("user_id"); userID != "" {
		q = q.Where("user_id = ?", userID)
	}
	if r.URL.Query().Get("revoked") == "true" {
		q = q.Where("revoked_at IS NOT NULL")
	} else if r.URL.Query().Get("revoked") == "false" {
		q = q.Where("revoked_at IS NULL")
	}

	q = applyPagination(q, cursor, limit)

	var connections []model.InConnection
	if err := q.Find(&connections).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list in-connections"})
		return
	}

	hasMore := len(connections) > limit
	if hasMore {
		connections = connections[:limit]
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": connections, "has_more": hasMore})
}

// ---------------------------------------------------------------------------
// Custom Domains
// ---------------------------------------------------------------------------

// ListCustomDomains handles GET /admin/v1/custom-domains.
// @Summary List all custom domains
// @Description Returns custom domains across all organizations.
// @Tags admin
// @Produce json
// @Param org_id query string false "Filter by org ID"
// @Param verified query string false "Filter by verified status (true/false)"
// @Param limit query int false "Page size"
// @Param cursor query string false "Pagination cursor"
// @Success 200 {object} paginatedResponse[adminCustomDomainResponse]
// @Security BearerAuth
// @Router /admin/v1/custom-domains [get]
func (h *AdminHandler) ListCustomDomains(w http.ResponseWriter, r *http.Request) {
	limit, cursor, err := parsePagination(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	q := h.db.Model(&model.CustomDomain{})
	if orgID := r.URL.Query().Get("org_id"); orgID != "" {
		q = q.Where("org_id = ?", orgID)
	}
	if r.URL.Query().Get("verified") == "true" {
		q = q.Where("verified = true")
	} else if r.URL.Query().Get("verified") == "false" {
		q = q.Where("verified = false")
	}

	q = applyPagination(q, cursor, limit)

	var domains []model.CustomDomain
	if err := q.Find(&domains).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list custom domains"})
		return
	}

	hasMore := len(domains) > limit
	if hasMore {
		domains = domains[:limit]
	}

	resp := make([]adminCustomDomainResponse, len(domains))
	for i, d := range domains {
		resp[i] = toAdminCustomDomainResponse(d)
	}

	result := paginatedResponse[adminCustomDomainResponse]{Data: resp, HasMore: hasMore}
	if hasMore {
		last := domains[len(domains)-1]
		c := encodeCursor(last.CreatedAt, last.ID)
		result.NextCursor = &c
	}
	writeJSON(w, http.StatusOK, result)
}

// DeleteCustomDomain handles DELETE /admin/v1/custom-domains/{id}.
// @Summary Delete a custom domain
// @Description Force-deletes a custom domain.
// @Tags admin
// @Produce json
// @Param id path string true "Domain ID"
// @Success 200 {object} map[string]string
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /admin/v1/custom-domains/{id} [delete]
func (h *AdminHandler) DeleteCustomDomain(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	result := h.db.Where("id = ?", id).Delete(&model.CustomDomain{})
	if result.Error != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to delete custom domain"})
		return
	}
	if result.RowsAffected == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "custom domain not found"})
		return
	}

	slog.Info("admin: custom domain deleted", "domain_id", id)
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// ---------------------------------------------------------------------------
// Audit
// ---------------------------------------------------------------------------

// ListAudit handles GET /admin/v1/audit.
// @Summary List all audit entries
// @Description Returns audit entries across all organizations.
// @Tags admin
// @Produce json
// @Param org_id query string false "Filter by org ID"
// @Param action query string false "Filter by action"
// @Param limit query int false "Page size"
// @Success 200 {object} map[string]any
// @Security BearerAuth
// @Router /admin/v1/audit [get]
func (h *AdminHandler) ListAudit(w http.ResponseWriter, r *http.Request) {
	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := parseInt(l); err == nil && n > 0 {
			if n > 100 {
				n = 100
			}
			limit = n
		}
	}

	q := h.db.Model(&model.AuditEntry{})
	if orgID := r.URL.Query().Get("org_id"); orgID != "" {
		q = q.Where("org_id = ?", orgID)
	}
	if action := r.URL.Query().Get("action"); action != "" {
		q = q.Where("action = ?", action)
	}

	q = q.Order("created_at DESC").Limit(limit + 1)

	var entries []model.AuditEntry
	if err := q.Find(&entries).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list audit entries"})
		return
	}

	hasMore := len(entries) > limit
	if hasMore {
		entries = entries[:limit]
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": entries, "has_more": hasMore})
}

// ---------------------------------------------------------------------------
// Usage
// ---------------------------------------------------------------------------

// ListUsage handles GET /admin/v1/usage.
// @Summary Aggregate usage by org
// @Description Returns aggregate request counts grouped by organization.
// @Tags admin
// @Produce json
// @Success 200 {object} map[string]any
// @Security BearerAuth
// @Router /admin/v1/usage [get]
func (h *AdminHandler) ListUsage(w http.ResponseWriter, r *http.Request) {
	var results []struct {
		OrgID        uuid.UUID `json:"org_id"`
		RequestCount int64     `json:"request_count"`
	}

	q := h.db.Model(&model.Usage{}).
		Select("org_id, SUM(request_count) as request_count").
		Group("org_id").Order("request_count DESC").Limit(100)

	if err := q.Scan(&results).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list usage"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": results})
}

// ---------------------------------------------------------------------------
// Workspace Storage
// ---------------------------------------------------------------------------

// ListWorkspaceStorage handles GET /admin/v1/workspace-storage.
// @Summary List all workspace storage
// @Description Returns all provisioned workspace databases.
// @Tags admin
// @Produce json
// @Success 200 {object} map[string]any
// @Security BearerAuth
// @Router /admin/v1/workspace-storage [get]
func (h *AdminHandler) ListWorkspaceStorage(w http.ResponseWriter, r *http.Request) {
	var storages []model.WorkspaceStorage
	if err := h.db.Order("created_at DESC").Find(&storages).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list workspace storage"})
		return
	}

	resp := make([]adminWorkspaceStorageResponse, len(storages))
	for i, ws := range storages {
		resp[i] = toAdminWorkspaceStorageResponse(ws)
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": resp})
}

// DeleteWorkspaceStorage handles DELETE /admin/v1/workspace-storage/{id}.
// @Summary Delete workspace storage
// @Description Deletes a workspace storage record.
// @Tags admin
// @Produce json
// @Param id path string true "Storage ID"
// @Success 200 {object} map[string]string
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /admin/v1/workspace-storage/{id} [delete]
func (h *AdminHandler) DeleteWorkspaceStorage(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	result := h.db.Where("id = ?", id).Delete(&model.WorkspaceStorage{})
	if result.Error != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to delete workspace storage"})
		return
	}
	if result.RowsAffected == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "workspace storage not found"})
		return
	}

	slog.Info("admin: workspace storage deleted", "storage_id", id)
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// ===========================================================================
// IN-INTEGRATION CRUD (platform-owned OAuth integrations)
// ===========================================================================

// ListInIntegrationProviders handles GET /admin/v1/in-integration-providers.
// @Summary List available integration providers
// @Description Returns providers supported for platform integrations (filtered by action catalog).
// @Tags admin
// @Produce json
// @Success 200 {array} map[string]any
// @Security BearerAuth
// @Router /admin/v1/in-integration-providers [get]
func (h *AdminHandler) ListInIntegrationProviders(w http.ResponseWriter, r *http.Request) {
	supported := h.catalog.ListProviders()
	supportedSet := make(map[string]struct{}, len(supported))
	for _, name := range supported {
		supportedSet[name] = struct{}{}
	}

	providers := h.nango.GetProviders()

	type providerInfo struct {
		Name                     string `json:"name"`
		DisplayName              string `json:"display_name"`
		AuthMode                 string `json:"auth_mode"`
		WebhookUserDefinedSecret bool   `json:"webhook_user_defined_secret,omitempty"`
	}

	resp := make([]providerInfo, 0, len(supported))
	for _, p := range providers {
		if _, ok := supportedSet[p.Name]; !ok {
			continue
		}
		resp = append(resp, providerInfo{
			Name:                     p.Name,
			DisplayName:              p.DisplayName,
			AuthMode:                 p.AuthMode,
			WebhookUserDefinedSecret: p.WebhookUserDefinedSecret,
		})
	}

	writeJSON(w, http.StatusOK, resp)
}

type adminCreateInIntegrationRequest struct {
	Provider    string             `json:"provider"`
	DisplayName string             `json:"display_name"`
	Credentials *nango.Credentials `json:"credentials,omitempty"`
	Meta        model.JSON         `json:"meta,omitempty"`
}

type adminInIntegrationResponse struct {
	ID          string     `json:"id"`
	UniqueKey   string     `json:"unique_key"`
	Provider    string     `json:"provider"`
	DisplayName string     `json:"display_name"`
	Meta        model.JSON `json:"meta,omitempty"`
	NangoConfig model.JSON `json:"nango_config,omitempty"`
	CreatedAt   string     `json:"created_at"`
	UpdatedAt   string     `json:"updated_at"`
}

func toAdminInIntegrationResponse(i model.InIntegration) adminInIntegrationResponse {
	return adminInIntegrationResponse{
		ID:          i.ID.String(),
		UniqueKey:   i.UniqueKey,
		Provider:    i.Provider,
		DisplayName: i.DisplayName,
		Meta:        i.Meta,
		NangoConfig: i.NangoConfig,
		CreatedAt:   i.CreatedAt.Format(time.RFC3339),
		UpdatedAt:   i.UpdatedAt.Format(time.RFC3339),
	}
}

// CreateInIntegration handles POST /admin/v1/in-integrations.
// @Summary Create a platform integration
// @Description Creates a new app-owned integration with OAuth credentials via Nango.
// @Tags admin
// @Accept json
// @Produce json
// @Param body body adminCreateInIntegrationRequest true "Integration details"
// @Success 201 {object} adminInIntegrationResponse
// @Failure 400 {object} errorResponse
// @Failure 409 {object} errorResponse
// @Security BearerAuth
// @Router /admin/v1/in-integrations [post]
func (h *AdminHandler) CreateInIntegration(w http.ResponseWriter, r *http.Request) {
	var req adminCreateInIntegrationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	if req.Provider == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "provider is required"})
		return
	}
	if req.DisplayName == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "display_name is required"})
		return
	}

	// Validate provider exists in Nango
	provider, ok := h.nango.GetProvider(req.Provider)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("unsupported provider %q", req.Provider)})
		return
	}

	// Validate provider has action definitions
	if _, ok := h.catalog.GetProvider(req.Provider); !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("provider %q has no action definitions", req.Provider)})
		return
	}

	// Validate credentials against auth mode
	if err := validateCredentials(provider, req.Credentials); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	// Generate unique key
	uniqueKey := fmt.Sprintf("%s-%s", req.Provider, uuid.New().String()[:8])
	nangoKey := "in_" + uniqueKey

	// Create in Nango
	nangoReq := nango.CreateIntegrationRequest{
		UniqueKey:   nangoKey,
		Provider:    req.Provider,
		Credentials: req.Credentials,
	}
	if err := h.nango.CreateIntegration(r.Context(), nangoReq); err != nil {
		slog.Error("admin: failed to create integration in Nango", "error", err, "provider", req.Provider)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create integration in provider"})
		return
	}

	// Fetch integration details + template from Nango to build config
	integResp, err := h.nango.GetIntegration(r.Context(), nangoKey)
	if err != nil {
		slog.Warn("admin: created in Nango but failed to fetch details", "error", err)
	}
	template, _ := h.nango.GetProviderTemplate(req.Provider)
	nangoConfig := buildNangoConfig(integResp, template, h.nango.CallbackURL())

	// Store locally
	integ := model.InIntegration{
		UniqueKey:   uniqueKey,
		Provider:    req.Provider,
		DisplayName: req.DisplayName,
		Meta:        req.Meta,
		NangoConfig: nangoConfig,
	}
	if err := h.db.Create(&integ).Error; err != nil {
		// Rollback Nango on DB failure
		_ = h.nango.DeleteIntegration(r.Context(), nangoKey)
		if isDuplicateKeyError(err) {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "integration already exists for this provider"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to store integration"})
		return
	}

	slog.Info("admin: in-integration created", "id", integ.ID, "provider", req.Provider)
	writeJSON(w, http.StatusCreated, toAdminInIntegrationResponse(integ))
}

// GetInIntegration handles GET /admin/v1/in-integrations/{id}.
// @Summary Get a platform integration
// @Description Returns a single platform integration by ID.
// @Tags admin
// @Produce json
// @Param id path string true "Integration ID"
// @Success 200 {object} adminInIntegrationResponse
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /admin/v1/in-integrations/{id} [get]
func (h *AdminHandler) GetInIntegration(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var integ model.InIntegration
	if err := h.db.Where("id = ? AND deleted_at IS NULL", id).First(&integ).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "integration not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get integration"})
		return
	}

	writeJSON(w, http.StatusOK, toAdminInIntegrationResponse(integ))
}

type adminUpdateInIntegrationRequest struct {
	DisplayName *string            `json:"display_name,omitempty"`
	Credentials *nango.Credentials `json:"credentials,omitempty"`
	Meta        model.JSON         `json:"meta,omitempty"`
}

// UpdateInIntegration handles PUT /admin/v1/in-integrations/{id}.
// @Summary Update a platform integration
// @Description Updates display name, credentials, or metadata for a platform integration.
// @Tags admin
// @Accept json
// @Produce json
// @Param id path string true "Integration ID"
// @Param body body adminUpdateInIntegrationRequest true "Fields to update"
// @Success 200 {object} adminInIntegrationResponse
// @Failure 400 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /admin/v1/in-integrations/{id} [put]
func (h *AdminHandler) UpdateInIntegration(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var integ model.InIntegration
	if err := h.db.Where("id = ? AND deleted_at IS NULL", id).First(&integ).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "integration not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get integration"})
		return
	}

	var req adminUpdateInIntegrationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	updates := map[string]any{}

	if req.DisplayName != nil {
		name := strings.TrimSpace(*req.DisplayName)
		if name == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "display_name cannot be empty"})
			return
		}
		updates["display_name"] = name
	}
	if req.Meta != nil {
		updates["meta"] = req.Meta
	}

	// If credentials are being updated, validate and push to Nango
	if req.Credentials != nil {
		provider, ok := h.nango.GetProvider(integ.Provider)
		if !ok {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "provider no longer available"})
			return
		}
		if err := validateCredentials(provider, req.Credentials); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}

		nangoKey := "in_" + integ.UniqueKey
		nangoReq := nango.UpdateIntegrationRequest{Credentials: req.Credentials}
		if err := h.nango.UpdateIntegration(r.Context(), nangoKey, nangoReq); err != nil {
			slog.Error("admin: failed to update integration in Nango", "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update credentials in provider"})
			return
		}

		// Refresh NangoConfig
		integResp, _ := h.nango.GetIntegration(r.Context(), nangoKey)
		template, _ := h.nango.GetProviderTemplate(integ.Provider)
		updates["nango_config"] = buildNangoConfig(integResp, template, h.nango.CallbackURL())
	}

	if len(updates) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no fields to update"})
		return
	}

	if err := h.db.Model(&integ).Updates(updates).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update integration"})
		return
	}

	h.db.Where("id = ?", id).First(&integ)
	slog.Info("admin: in-integration updated", "id", id)
	writeJSON(w, http.StatusOK, toAdminInIntegrationResponse(integ))
}

// DeleteInIntegration handles DELETE /admin/v1/in-integrations/{id}.
// @Summary Delete a platform integration
// @Description Soft-deletes a platform integration and removes it from Nango.
// @Tags admin
// @Produce json
// @Param id path string true "Integration ID"
// @Success 200 {object} map[string]string
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /admin/v1/in-integrations/{id} [delete]
func (h *AdminHandler) DeleteInIntegration(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var integ model.InIntegration
	if err := h.db.Where("id = ? AND deleted_at IS NULL", id).First(&integ).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "integration not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get integration"})
		return
	}

	// Remove from Nango (best-effort)
	nangoKey := "in_" + integ.UniqueKey
	if err := h.nango.DeleteIntegration(r.Context(), nangoKey); err != nil {
		slog.Warn("admin: failed to delete integration from Nango", "error", err, "key", nangoKey)
	}

	// Soft-delete locally
	now := time.Now()
	if err := h.db.Model(&integ).Update("deleted_at", now).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to delete integration"})
		return
	}

	slog.Info("admin: in-integration deleted", "id", id, "provider", integ.Provider)
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// ---------------------------------------------------------------------------
// Admin Audit Log
// ---------------------------------------------------------------------------

// ListAdminAudit handles GET /admin/v1/admin-audit.
// @Summary List admin audit log
// @Description Returns admin operation audit entries with filters.
// @Tags admin
// @Produce json
// @Param resource query string false "Filter by resource (users, orgs, agents, etc.)"
// @Param action query string false "Filter by action (update_user, ban_user, delete_org, etc.)"
// @Param admin_id query string false "Filter by admin user ID"
// @Param limit query int false "Page size"
// @Success 200 {object} map[string]any
// @Security BearerAuth
// @Router /admin/v1/admin-audit [get]
func (h *AdminHandler) ListAdminAudit(w http.ResponseWriter, r *http.Request) {
	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := parseInt(l); err == nil && n > 0 {
			if n > 100 {
				n = 100
			}
			limit = n
		}
	}

	q := h.db.Model(&model.AdminAuditEntry{})
	if resource := r.URL.Query().Get("resource"); resource != "" {
		q = q.Where("resource = ?", resource)
	}
	if action := r.URL.Query().Get("action"); action != "" {
		q = q.Where("action = ?", action)
	}
	if adminID := r.URL.Query().Get("admin_id"); adminID != "" {
		q = q.Where("admin_id = ?", adminID)
	}

	q = q.Order("created_at DESC").Limit(limit + 1)

	var entries []model.AdminAuditEntry
	if err := q.Find(&entries).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list admin audit entries"})
		return
	}

	hasMore := len(entries) > limit
	if hasMore {
		entries = entries[:limit]
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": entries, "has_more": hasMore})
}

// ===========================================================================
// UPDATE ENDPOINTS — full admin edit capabilities with validation
// ===========================================================================

// ---------------------------------------------------------------------------
// Update User
// ---------------------------------------------------------------------------

type adminUpdateUserRequest struct {
	Name  *string `json:"name,omitempty"`
	Email *string `json:"email,omitempty"`
}

// UpdateUser handles PUT /admin/v1/users/{id}.
// @Summary Update a user
// @Description Updates user name and/or email with validation.
// @Tags admin
// @Accept json
// @Produce json
// @Param id path string true "User ID"
// @Param body body adminUpdateUserRequest true "Fields to update"
// @Success 200 {object} adminUserResponse
// @Failure 400 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 409 {object} errorResponse
// @Security BearerAuth
// @Router /admin/v1/users/{id} [put]
func (h *AdminHandler) UpdateUser(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var user model.User
	if err := h.db.Where("id = ?", id).First(&user).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "user not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get user"})
		return
	}

	var req struct {
		Name  *string `json:"name,omitempty"`
		Email *string `json:"email,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	updates := map[string]any{}

	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name cannot be empty"})
			return
		}
		updates["name"] = name
	}

	if req.Email != nil {
		email := strings.TrimSpace(strings.ToLower(*req.Email))
		if email == "" || !strings.Contains(email, "@") || !strings.Contains(email, ".") {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid email format"})
			return
		}
		// Check uniqueness
		var existing model.User
		if err := h.db.Where("email = ? AND id != ?", email, user.ID).First(&existing).Error; err == nil {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "email already in use by another user"})
			return
		}
		updates["email"] = email
	}

	if len(updates) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no fields to update"})
		return
	}

	// Compute diff for audit (only what actually changed)
	old := map[string]any{"name": user.Name, "email": user.Email}
	setAuditDiff(r, old, updates)

	if err := h.db.Model(&user).Updates(updates).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update user"})
		return
	}

	h.db.Where("id = ?", id).First(&user)
	slog.Info("admin: user updated", "user_id", id)
	writeJSON(w, http.StatusOK, toAdminUserResponse(user))
}

// ---------------------------------------------------------------------------
// Update Org (enhanced — adds name editing)
// ---------------------------------------------------------------------------

type adminUpdateOrgRequest struct {
	Name           *string   `json:"name,omitempty"`
	RateLimit      *int      `json:"rate_limit,omitempty"`
	Active         *bool     `json:"active,omitempty"`
	AllowedOrigins *[]string `json:"allowed_origins,omitempty"`
}

// UpdateOrgFull handles PUT /admin/v1/orgs/{id}.
// @Summary Update organization
// @Description Updates org name, rate_limit, active status, and allowed_origins.
// @Tags admin
// @Accept json
// @Produce json
// @Param id path string true "Org ID"
// @Param body body adminUpdateOrgRequest true "Fields to update"
// @Success 200 {object} adminOrgResponse
// @Failure 400 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 409 {object} errorResponse
// @Security BearerAuth
// @Router /admin/v1/orgs/{id} [put]
func (h *AdminHandler) UpdateOrgFull(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var org model.Org
	if err := h.db.Where("id = ?", id).First(&org).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "org not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get org"})
		return
	}

	var req struct {
		Name           *string   `json:"name,omitempty"`
		RateLimit      *int      `json:"rate_limit,omitempty"`
		Active         *bool     `json:"active,omitempty"`
		AllowedOrigins *[]string `json:"allowed_origins,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	updates := map[string]any{}

	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name cannot be empty"})
			return
		}
		// Check uniqueness
		var existing model.Org
		if err := h.db.Where("name = ? AND id != ?", name, org.ID).First(&existing).Error; err == nil {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "org name already in use"})
			return
		}
		updates["name"] = name
	}
	if req.RateLimit != nil {
		if *req.RateLimit < 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "rate_limit cannot be negative"})
			return
		}
		updates["rate_limit"] = *req.RateLimit
	}
	if req.Active != nil {
		updates["active"] = *req.Active
	}
	if req.AllowedOrigins != nil {
		updates["allowed_origins"] = *req.AllowedOrigins
	}

	if len(updates) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no fields to update"})
		return
	}

	old := map[string]any{"name": org.Name, "rate_limit": org.RateLimit, "active": org.Active, "allowed_origins": org.AllowedOrigins}
	setAuditDiff(r, old, updates)

	if err := h.db.Model(&org).Updates(updates).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update org"})
		return
	}

	h.db.Where("id = ?", id).First(&org)
	slog.Info("admin: org updated", "org_id", id)
	writeJSON(w, http.StatusOK, toAdminOrgResponse(org))
}

// ---------------------------------------------------------------------------
// Update Credential
// ---------------------------------------------------------------------------

type adminUpdateCredentialRequest struct {
	Label      *string `json:"label,omitempty"`
}

// UpdateCredential handles PUT /admin/v1/credentials/{id}.
// @Summary Update a credential
// @Description Updates credential label.
// @Tags admin
// @Accept json
// @Produce json
// @Param id path string true "Credential ID"
// @Param body body adminUpdateCredentialRequest true "Fields to update"
// @Success 200 {object} adminCredentialResponse
// @Failure 400 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /admin/v1/credentials/{id} [put]
func (h *AdminHandler) UpdateCredential(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var cred model.Credential
	if err := h.db.Where("id = ?", id).First(&cred).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "credential not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get credential"})
		return
	}

	if cred.RevokedAt != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "cannot update a revoked credential"})
		return
	}

	var req struct {
		Label      *string `json:"label,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	updates := map[string]any{}

	if req.Label != nil {
		label := strings.TrimSpace(*req.Label)
		if label == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "label cannot be empty"})
			return
		}
		updates["label"] = label
	}


	if len(updates) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no fields to update"})
		return
	}

	old := map[string]any{"label": cred.Label}
	setAuditDiff(r, old, updates)

	if err := h.db.Model(&cred).Updates(updates).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update credential"})
		return
	}

	h.db.Where("id = ?", id).First(&cred)
	slog.Info("admin: credential updated", "credential_id", id)
	writeJSON(w, http.StatusOK, toAdminCredentialResponse(cred))
}

// ---------------------------------------------------------------------------
// Update Agent
// ---------------------------------------------------------------------------

type adminUpdateAgentRequest struct {
	Name              *string    `json:"name,omitempty"`
	Description       *string    `json:"description,omitempty"`
	CredentialID      *string    `json:"credential_id,omitempty"`
	SandboxType       *string    `json:"sandbox_type,omitempty"`
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

// UpdateAgent handles PUT /admin/v1/agents/{id}.
// @Summary Update an agent
// @Description Updates agent configuration with full validation.
// @Tags admin
// @Accept json
// @Produce json
// @Param id path string true "Agent ID"
// @Param body body adminUpdateAgentRequest true "Fields to update"
// @Success 200 {object} adminAgentResponse
// @Failure 400 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 409 {object} errorResponse
// @Security BearerAuth
// @Router /admin/v1/agents/{id} [put]
func (h *AdminHandler) UpdateAgent(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var agent model.Agent
	if err := h.db.Where("id = ?", id).First(&agent).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "agent not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get agent"})
		return
	}

	var req struct {
		Name              *string    `json:"name,omitempty"`
		Description       *string    `json:"description,omitempty"`
		CredentialID      *string    `json:"credential_id,omitempty"`
		SandboxType       *string    `json:"sandbox_type,omitempty"`
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
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	updates := map[string]any{}

	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name cannot be empty"})
			return
		}
		updates["name"] = name
	}
	if req.Description != nil {
		updates["description"] = *req.Description
	}
	if req.SystemPrompt != nil {
		updates["system_prompt"] = *req.SystemPrompt
	}
	if req.Model != nil {
		updates["model"] = *req.Model
	}
	if req.Status != nil {
		if *req.Status != "active" && *req.Status != "archived" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "status must be 'active' or 'archived'"})
			return
		}
		updates["status"] = *req.Status
	}

	if req.SandboxType != nil {
		if *req.SandboxType != "dedicated" && *req.SandboxType != "shared" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "sandbox_type must be 'dedicated' or 'shared'"})
			return
		}
		updates["sandbox_type"] = *req.SandboxType
	}

	// Validate credential if changing
	if req.CredentialID != nil {
		credID, err := uuid.Parse(*req.CredentialID)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid credential_id format"})
			return
		}
		var cred model.Credential
		if err := h.db.Where("id = ? AND org_id = ? AND revoked_at IS NULL", credID, agent.OrgID).First(&cred).Error; err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "credential not found, not in same org, or revoked"})
			return
		}
		updates["credential_id"] = credID
	}

	// Validate sandbox template if changing
	if req.SandboxTemplateID != nil {
		if *req.SandboxTemplateID == "" {
			updates["sandbox_template_id"] = nil
		} else {
			tmplID, err := uuid.Parse(*req.SandboxTemplateID)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid sandbox_template_id format"})
				return
			}
			var tmpl model.SandboxTemplate
			if err := h.db.Where("id = ? AND org_id = ?", tmplID, agent.OrgID).First(&tmpl).Error; err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "sandbox template not found in the same org"})
				return
			}
			updates["sandbox_template_id"] = tmplID
		}
	}

	if req.Tools != nil {
		updates["tools"] = req.Tools
	}
	if req.McpServers != nil {
		updates["mcp_servers"] = req.McpServers
	}
	if req.Skills != nil {
		updates["skills"] = req.Skills
	}
	if req.Integrations != nil {
		updates["integrations"] = req.Integrations
	}
	if req.AgentConfig != nil {
		if errMsg := validateJSONSchema(req.AgentConfig); errMsg != "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": errMsg})
			return
		}
		updates["agent_config"] = req.AgentConfig
	}
	if req.Permissions != nil {
		updates["permissions"] = req.Permissions
	}
	if req.Team != nil {
		updates["team"] = *req.Team
	}
	if req.SharedMemory != nil {
		updates["shared_memory"] = *req.SharedMemory
	}

	if len(updates) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no fields to update"})
		return
	}

	old := map[string]any{
		"name": agent.Name, "description": agent.Description, "model": agent.Model,
		"system_prompt": agent.SystemPrompt, "sandbox_type": agent.SandboxType, "status": agent.Status,
		"credential_id": agent.CredentialID.String(), "team": agent.Team, "shared_memory": agent.SharedMemory,
	}
	setAuditDiff(r, old, updates)

	if err := h.db.Model(&agent).Updates(updates).Error; err != nil {
		if isDuplicateKeyError(err) {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "agent with that name already exists in this workspace"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update agent"})
		return
	}

	h.db.Where("id = ?", id).First(&agent)
	slog.Info("admin: agent updated", "agent_id", id)
	writeJSON(w, http.StatusOK, toAdminAgentResponse(agent))
}

// ---------------------------------------------------------------------------
// ---------------------------------------------------------------------------


// ---------------------------------------------------------------------------
// Update Sandbox Template
// ---------------------------------------------------------------------------

type adminCreateSandboxTemplateRequest struct {
	Name string   `json:"name"`
	Slug string   `json:"slug"` // Daytona snapshot name (built via make build-templates)
	Tags []string `json:"tags"` // user-facing tags, e.g. ["python","ml"]
	Size string   `json:"size"` // small, medium, large, xlarge
}

// CreateSandboxTemplate handles POST /admin/v1/sandbox-templates.
// @Summary Register a public sandbox template
// @Description Registers a pre-built Daytona snapshot as a public (platform-wide) sandbox template.
// @Tags admin
// @Accept json
// @Produce json
// @Param body body adminCreateSandboxTemplateRequest true "Template details"
// @Success 201 {object} adminSandboxTemplateResponse
// @Failure 400 {object} errorResponse
// @Security BearerAuth
// @Router /admin/v1/sandbox-templates [post]
func (h *AdminHandler) CreateSandboxTemplate(w http.ResponseWriter, r *http.Request) {
	var req adminCreateSandboxTemplateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name is required"})
		return
	}

	slug := strings.TrimSpace(req.Slug)
	if slug == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "slug is required (Daytona snapshot name)"})
		return
	}

	size := req.Size
	if size == "" {
		size = "medium"
	}
	if !model.ValidTemplateSize(size) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid size (valid: small, medium, large, xlarge)"})
		return
	}

	tags := model.JSON{}
	if len(req.Tags) > 0 {
		tagSlice := make([]any, len(req.Tags))
		for idx, tag := range req.Tags {
			tagSlice[idx] = tag
		}
		tagsJSON, _ := json.Marshal(tagSlice)
		_ = json.Unmarshal(tagsJSON, &tags)
	}

	tmpl := model.SandboxTemplate{
		OrgID:       nil, // public template
		Name:        name,
		Slug:        slug,
		Tags:        tags,
		Size:        size,
		ExternalID:  &slug, // slug IS the Daytona snapshot name
		BuildStatus: "ready",
		Config:      model.JSON{},
	}

	if err := h.db.Create(&tmpl).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create sandbox template"})
		return
	}

	slog.Info("admin: public sandbox template registered", "template_id", tmpl.ID, "name", name, "slug", slug, "size", size)
	writeJSON(w, http.StatusCreated, toAdminSandboxTemplateResponse(tmpl))
}

// GetSandboxTemplate handles GET /admin/v1/sandbox-templates/{id}.
// @Summary Get a sandbox template
// @Description Returns a single sandbox template by ID.
// @Tags admin
// @Produce json
// @Param id path string true "Template ID"
// @Success 200 {object} adminSandboxTemplateResponse
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /admin/v1/sandbox-templates/{id} [get]
func (h *AdminHandler) GetSandboxTemplate(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var tmpl model.SandboxTemplate
	if err := h.db.Where("id = ?", id).First(&tmpl).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "sandbox template not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get sandbox template"})
		return
	}

	writeJSON(w, http.StatusOK, toAdminSandboxTemplateResponse(tmpl))
}

type adminUpdateSandboxTemplateRequest struct {
	Name *string  `json:"name,omitempty"`
	Slug *string  `json:"slug,omitempty"` // Daytona snapshot name
	Tags []string `json:"tags,omitempty"` // user-facing tags
	Size *string  `json:"size,omitempty"`
}

// UpdateSandboxTemplate handles PUT /admin/v1/sandbox-templates/{id}.
// @Summary Update a sandbox template
// @Description Updates sandbox template name, slug, tags, and size.
// @Tags admin
// @Accept json
// @Produce json
// @Param id path string true "Template ID"
// @Param body body adminUpdateSandboxTemplateRequest true "Fields to update"
// @Success 200 {object} adminSandboxTemplateResponse
// @Failure 400 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /admin/v1/sandbox-templates/{id} [put]
func (h *AdminHandler) UpdateSandboxTemplate(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var tmpl model.SandboxTemplate
	if err := h.db.Where("id = ?", id).First(&tmpl).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "sandbox template not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get sandbox template"})
		return
	}

	var req adminUpdateSandboxTemplateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	updates := map[string]any{}

	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name cannot be empty"})
			return
		}
		updates["name"] = name
	}
	if req.Size != nil {
		if !model.ValidTemplateSize(*req.Size) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid size (valid: small, medium, large, xlarge)"})
			return
		}
		updates["size"] = *req.Size
	}
	if req.Slug != nil {
		slug := strings.TrimSpace(*req.Slug)
		if slug == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "slug cannot be empty"})
			return
		}
		updates["slug"] = slug
		updates["external_id"] = slug // slug IS the Daytona snapshot name
		updates["build_status"] = "ready"
	}
	if req.Tags != nil {
		tagsJSON, _ := json.Marshal(req.Tags)
		var tagsModel model.JSON
		_ = json.Unmarshal(tagsJSON, &tagsModel)
		updates["tags"] = tagsModel
	}

	if len(updates) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no fields to update"})
		return
	}

	old := map[string]any{"name": tmpl.Name}
	setAuditDiff(r, old, updates)

	if err := h.db.Model(&tmpl).Updates(updates).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update sandbox template"})
		return
	}

	h.db.Where("id = ?", id).First(&tmpl)
	slog.Info("admin: sandbox template updated", "template_id", id)
	writeJSON(w, http.StatusOK, toAdminSandboxTemplateResponse(tmpl))
}


// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func parseInt(s string) (int, error) {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("invalid integer: %s", s)
		}
		n = n*10 + int(c-'0')
	}
	return n, nil
}

// ---------------------------------------------------------------------------
// Impersonation
// ---------------------------------------------------------------------------

// Impersonate issues tokens for the target user, allowing a platform admin
// to view the application as that user.
//
// @Summary Impersonate a user
// @Description Issues access and refresh tokens for the target user. Requires platform admin privileges.
// @Tags admin
// @Accept json
// @Produce json
// @Param id path string true "User ID"
// @Success 200 {object} authResponse
// @Failure 400 {object} errorResponse
// @Failure 403 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /admin/v1/users/{id}/impersonate [post]
func (h *AdminHandler) Impersonate(w http.ResponseWriter, r *http.Request) {
	targetID := chi.URLParam(r, "id")
	if _, err := uuid.Parse(targetID); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid user id"})
		return
	}

	var targetUser model.User
	if err := h.db.Where("id = ?", targetID).First(&targetUser).Error; err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "user not found"})
		return
	}

	if targetUser.BannedAt != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "cannot impersonate a banned user"})
		return
	}

	var memberships []model.OrgMembership
	h.db.Preload("Org").Where("user_id = ?", targetUser.ID).Find(&memberships)

	if len(memberships) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "user has no organization memberships"})
		return
	}

	orgID := memberships[0].OrgID.String()
	role := memberships[0].Role

	accessToken, err := auth.IssueAccessToken(h.privateKey, h.issuer, h.audience, targetUser.ID.String(), orgID, role, h.accessTTL)
	if err != nil {
		slog.Error("impersonate: failed to issue access token", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	refreshToken, err := auth.IssueRefreshToken(h.signingKey, targetUser.ID.String(), h.refreshTTL)
	if err != nil {
		slog.Error("impersonate: failed to issue refresh token", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	storedRefresh := model.RefreshToken{
		UserID:    targetUser.ID,
		TokenHash: hashToken(refreshToken),
		ExpiresAt: time.Now().Add(h.refreshTTL),
	}
	if err := h.db.Create(&storedRefresh).Error; err != nil {
		slog.Error("impersonate: failed to store refresh token", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	// Record audit trail for the impersonation event.
	admin, _ := middleware.UserFromContext(r.Context())
	adminEmail := ""
	if admin != nil {
		adminEmail = admin.Email
	}
	middleware.SetAdminAuditChanges(r, middleware.AdminAuditChanges{
		"action":        map[string]any{"old": nil, "new": "impersonate"},
		"target_user_id":    map[string]any{"old": nil, "new": targetUser.ID.String()},
		"target_email":      map[string]any{"old": nil, "new": targetUser.Email},
		"impersonator_email": map[string]any{"old": nil, "new": adminEmail},
	})

	orgs := make([]orgMemberDTO, 0, len(memberships))
	for _, membership := range memberships {
		orgs = append(orgs, orgMemberDTO{
			ID:   membership.OrgID.String(),
			Name: membership.Org.Name,
			Role: membership.Role,
		})
	}

	slog.Info("admin impersonating user", "admin_email", adminEmail, "target_user_id", targetUser.ID, "target_email", targetUser.Email)

	writeJSON(w, http.StatusOK, authResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresIn:    int(h.accessTTL.Seconds()),
		User: userResponse{
			ID:             targetUser.ID.String(),
			Email:          targetUser.Email,
			Name:           targetUser.Name,
			EmailConfirmed: targetUser.EmailConfirmedAt != nil,
		},
		Orgs: orgs,
	})
}

// ---------------------------------------------------------------------------
// Skills (global / org-scoped)
// ---------------------------------------------------------------------------

type adminSkillResponse struct {
	ID           string   `json:"id"`
	OrgID        *string  `json:"org_id"`
	PublisherID  *string  `json:"publisher_id"`
	Slug         string   `json:"slug"`
	Name         string   `json:"name"`
	Description  *string  `json:"description"`
	SourceType   string   `json:"source_type"`
	RepoURL      *string  `json:"repo_url"`
	RepoSubpath  *string  `json:"repo_subpath"`
	RepoRef      string   `json:"repo_ref"`
	Tags         []string `json:"tags"`
	InstallCount int      `json:"install_count"`
	Featured     bool     `json:"featured"`
	Status       string   `json:"status"`
	CreatedAt    string   `json:"created_at"`
	UpdatedAt    string   `json:"updated_at"`
}

func toAdminSkillResponse(skill model.Skill) adminSkillResponse {
	resp := adminSkillResponse{
		ID:           skill.ID.String(),
		Slug:         skill.Slug,
		Name:         skill.Name,
		Description:  skill.Description,
		SourceType:   skill.SourceType,
		RepoURL:      skill.RepoURL,
		RepoSubpath:  skill.RepoSubpath,
		RepoRef:      skill.RepoRef,
		Tags:         skill.Tags,
		InstallCount: skill.InstallCount,
		Featured:     skill.Featured,
		Status:       skill.Status,
		CreatedAt:    skill.CreatedAt.Format(time.RFC3339),
		UpdatedAt:    skill.UpdatedAt.Format(time.RFC3339),
	}
	if skill.OrgID != nil {
		orgIDStr := skill.OrgID.String()
		resp.OrgID = &orgIDStr
	}
	if skill.PublisherID != nil {
		pubIDStr := skill.PublisherID.String()
		resp.PublisherID = &pubIDStr
	}
	if resp.Tags == nil {
		resp.Tags = []string{}
	}
	return resp
}

// ListSkills handles GET /admin/v1/skills.
// @Summary List skills
// @Description Lists all skills with optional filters.
// @Tags admin
// @Produce json
// @Param status query string false "Filter by status"
// @Param scope query string false "Filter scope (global)"
// @Param source_type query string false "Filter by source type"
// @Param q query string false "Search by name"
// @Success 200 {object} paginatedResponse[adminSkillResponse]
// @Security BearerAuth
// @Router /admin/v1/skills [get]
func (h *AdminHandler) ListSkills(w http.ResponseWriter, r *http.Request) {
	limit, cursor, err := parsePagination(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	q := h.db.Model(&model.Skill{})
	if orgID := r.URL.Query().Get("org_id"); orgID != "" {
		q = q.Where("org_id = ?", orgID)
	}
	if scope := r.URL.Query().Get("scope"); scope == "global" {
		q = q.Where("org_id IS NULL")
	}
	if status := r.URL.Query().Get("status"); status != "" {
		q = q.Where("status = ?", status)
	}
	if sourceType := r.URL.Query().Get("source_type"); sourceType != "" {
		q = q.Where("source_type = ?", sourceType)
	}
	if search := r.URL.Query().Get("q"); search != "" {
		q = q.Where("name ILIKE ? OR slug ILIKE ?", "%"+search+"%", "%"+search+"%")
	}

	q = applyPagination(q, cursor, limit)

	var skills []model.Skill
	if err := q.Find(&skills).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list skills"})
		return
	}

	hasMore := len(skills) > limit
	if hasMore {
		skills = skills[:limit]
	}

	resp := make([]adminSkillResponse, len(skills))
	for index, skill := range skills {
		resp[index] = toAdminSkillResponse(skill)
	}

	result := paginatedResponse[adminSkillResponse]{Data: resp, HasMore: hasMore}
	if hasMore {
		last := skills[len(skills)-1]
		cursorStr := encodeCursor(last.CreatedAt, last.ID)
		result.NextCursor = &cursorStr
	}
	writeJSON(w, http.StatusOK, result)
}

// GetSkill handles GET /admin/v1/skills/{id}.
// @Summary Get skill details
// @Description Returns a skill by ID.
// @Tags admin
// @Produce json
// @Param id path string true "Skill ID"
// @Success 200 {object} adminSkillResponse
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /admin/v1/skills/{id} [get]
func (h *AdminHandler) GetSkill(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var skill model.Skill
	if err := h.db.Where("id = ?", id).First(&skill).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "skill not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get skill"})
		return
	}

	writeJSON(w, http.StatusOK, toAdminSkillResponse(skill))
}

type adminCreateSkillRequest struct {
	Name        string   `json:"name"`
	Description *string  `json:"description,omitempty"`
	SourceType  string   `json:"source_type"` // "inline" or "git"
	Tags        []string `json:"tags,omitempty"`
	Status      string   `json:"status,omitempty"` // defaults to "published" for global skills
	Featured    bool     `json:"featured,omitempty"`

	// Inline source
	Bundle *skills.Bundle `json:"bundle,omitempty"`

	// Git source
	RepoURL     *string `json:"repo_url,omitempty"`
	RepoSubpath *string `json:"repo_subpath,omitempty"`
	RepoRef     *string `json:"repo_ref,omitempty"`
}

// CreateSkill handles POST /admin/v1/skills.
// @Summary Create a global skill
// @Description Creates a global skill (org_id = nil) visible to all users.
// @Tags admin
// @Accept json
// @Produce json
// @Param body body adminCreateSkillRequest true "Skill to create"
// @Success 201 {object} adminSkillResponse
// @Failure 400 {object} errorResponse
// @Failure 409 {object} errorResponse
// @Security BearerAuth
// @Router /admin/v1/skills [post]
func (h *AdminHandler) CreateSkill(w http.ResponseWriter, r *http.Request) {
	var req adminCreateSkillRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	if req.Name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name is required"})
		return
	}
	if req.SourceType == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "source_type is required (inline or git)"})
		return
	}
	if req.SourceType != model.SkillSourceInline && req.SourceType != model.SkillSourceGit {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "source_type must be 'inline' or 'git'"})
		return
	}
	if req.SourceType == model.SkillSourceGit && (req.RepoURL == nil || *req.RepoURL == "") {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "repo_url is required for git skills"})
		return
	}

	status := req.Status
	if status == "" {
		status = model.SkillStatusPublished
	}

	slug := model.GenerateSlug(req.Name)
	repoRef := "main"
	if req.RepoRef != nil && *req.RepoRef != "" {
		repoRef = *req.RepoRef
	}

	skill := model.Skill{
		OrgID:       nil, // global skill
		Slug:        slug,
		Name:        req.Name,
		Description: req.Description,
		SourceType:  req.SourceType,
		RepoURL:     req.RepoURL,
		RepoSubpath: req.RepoSubpath,
		RepoRef:     repoRef,
		Tags:        req.Tags,
		Featured:    req.Featured,
		Status:      status,
	}

	if err := h.db.Create(&skill).Error; err != nil {
		if isDuplicateKeyError(err) {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "a skill with this name already exists"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create skill"})
		return
	}

	if req.SourceType == model.SkillSourceInline && req.Bundle != nil {
		if req.Bundle.ID == "" {
			req.Bundle.ID = skill.Slug
		}
		if req.Bundle.Title == "" {
			req.Bundle.Title = skill.Name
		}
		if req.Bundle.Description == "" && skill.Description != nil {
			req.Bundle.Description = *skill.Description
		}
		if _, err := skills.HydrateInline(r.Context(), h.db, skill.ID, req.Bundle, "v1"); err != nil {
			slog.Error("admin: failed to hydrate inline skill", "skill_id", skill.ID, "error", err)
		}
		_ = h.db.First(&skill, "id = ?", skill.ID).Error
	} else if req.SourceType == model.SkillSourceGit {
		if h.enqueuer != nil {
			task, err := tasks.NewSkillHydrateTask(skill.ID)
			if err == nil {
				_, _ = h.enqueuer.Enqueue(task)
			}
		}
	}

	writeJSON(w, http.StatusCreated, toAdminSkillResponse(skill))
}

type adminUpdateSkillRequest struct {
	Name        *string   `json:"name,omitempty"`
	Description *string   `json:"description,omitempty"`
	Status      *string   `json:"status,omitempty"`
	Featured    *bool     `json:"featured,omitempty"`
	Tags        *[]string `json:"tags,omitempty"`
	RepoRef     *string   `json:"repo_ref,omitempty"`
}

// UpdateSkill handles PUT /admin/v1/skills/{id}.
// @Summary Update a skill
// @Description Updates skill properties.
// @Tags admin
// @Accept json
// @Produce json
// @Param id path string true "Skill ID"
// @Param body body adminUpdateSkillRequest true "Fields to update"
// @Success 200 {object} adminSkillResponse
// @Failure 404 {object} errorResponse
// @Failure 409 {object} errorResponse
// @Security BearerAuth
// @Router /admin/v1/skills/{id} [put]
func (h *AdminHandler) UpdateSkill(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var skill model.Skill
	if err := h.db.Where("id = ?", id).First(&skill).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "skill not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get skill"})
		return
	}

	var req adminUpdateSkillRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	updates := map[string]any{}
	if req.Name != nil {
		updates["name"] = *req.Name
		updates["slug"] = model.GenerateSlug(*req.Name)
	}
	if req.Description != nil {
		updates["description"] = *req.Description
	}
	if req.Status != nil {
		updates["status"] = *req.Status
	}
	if req.Featured != nil {
		updates["featured"] = *req.Featured
	}
	if req.Tags != nil {
		updates["tags"] = pq.StringArray(*req.Tags)
	}
	if req.RepoRef != nil {
		updates["repo_ref"] = *req.RepoRef
	}

	if len(updates) > 0 {
		if err := h.db.Model(&skill).Updates(updates).Error; err != nil {
			if isDuplicateKeyError(err) {
				writeJSON(w, http.StatusConflict, map[string]string{"error": "a skill with this name already exists"})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update skill"})
			return
		}
	}

	h.db.Where("id = ?", id).First(&skill)
	writeJSON(w, http.StatusOK, toAdminSkillResponse(skill))
}

// DeleteSkill handles DELETE /admin/v1/skills/{id}.
// @Summary Delete a skill
// @Description Permanently deletes a skill and all its versions.
// @Tags admin
// @Produce json
// @Param id path string true "Skill ID"
// @Success 200 {object} map[string]string
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /admin/v1/skills/{id} [delete]
func (h *AdminHandler) DeleteSkill(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var skill model.Skill
	if err := h.db.Where("id = ?", id).First(&skill).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "skill not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get skill"})
		return
	}

	if err := h.db.Delete(&skill).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to delete skill"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}
