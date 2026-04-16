package handler

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/ziraloop/ziraloop/internal/cache"
	"github.com/ziraloop/ziraloop/internal/counter"
	"github.com/ziraloop/ziraloop/internal/crypto"
	"github.com/ziraloop/ziraloop/internal/middleware"
	"github.com/ziraloop/ziraloop/internal/model"
	"github.com/ziraloop/ziraloop/internal/proxy"
	"github.com/ziraloop/ziraloop/internal/registry"
)

type CredentialHandler struct {
	db           *gorm.DB
	kms          *crypto.KeyWrapper
	cacheManager *cache.Manager
	counter      *counter.Counter
}

func NewCredentialHandler(db *gorm.DB, kms *crypto.KeyWrapper, cm *cache.Manager, ctr *counter.Counter) *CredentialHandler {
	return &CredentialHandler{db: db, kms: kms, cacheManager: cm, counter: ctr}
}

// providerAuthSchemes maps provider IDs to their default auth schemes.
// Providers not listed default to "bearer".
var providerAuthSchemes = map[string]string{
	"anthropic":      "x-api-key",
	"google":         "query_param",
	"azure":          "api-key",
	"amazon-bedrock": "bearer",
}

type createCredentialRequest struct {
	Label          string     `json:"label"`
	ProviderID     string     `json:"provider_id"`
	BaseURL        string     `json:"base_url"`
	AuthScheme     string     `json:"auth_scheme"`
	APIKey         string     `json:"api_key"`
	ExternalID     *string    `json:"external_id,omitempty"`
	Remaining      *int64     `json:"remaining,omitempty"`
	RefillAmount   *int64     `json:"refill_amount,omitempty"`
	RefillInterval *string    `json:"refill_interval,omitempty"`
	Meta           model.JSON `json:"meta,omitempty"`
}

type credentialResponse struct {
	ID             string     `json:"id"`
	Label          string     `json:"label"`
	BaseURL        string     `json:"base_url"`
	AuthScheme     string     `json:"auth_scheme"`
	ProviderID     string     `json:"provider_id,omitempty"`
	Remaining      *int64     `json:"remaining,omitempty"`
	RefillAmount   *int64     `json:"refill_amount,omitempty"`
	RefillInterval *string    `json:"refill_interval,omitempty"`
	Meta           model.JSON `json:"meta,omitempty"`
	RequestCount   int64      `json:"request_count"`
	LastUsedAt     *string    `json:"last_used_at,omitempty"`
	CreatedAt      string     `json:"created_at"`
	RevokedAt      *string    `json:"revoked_at,omitempty"`
}

// Create handles POST /v1/credentials.
// @Summary Create a credential
// @Description Stores an encrypted LLM API credential for the current organization.
// @Tags credentials
// @Accept json
// @Produce json
// @Param body body createCredentialRequest true "Credential details"
// @Success 201 {object} credentialResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/credentials [post]
func (h *CredentialHandler) Create(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	var req createCredentialRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	if req.APIKey == "" || req.ProviderID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "provider_id and api_key are required"})
		return
	}

	provider, ok := registry.Global().GetProvider(req.ProviderID)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("unknown provider_id %q", req.ProviderID)})
		return
	}

	// Auto-fill base_url from registry if not provided
	if req.BaseURL == "" {
		if provider.API == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("provider %q does not have a base URL configured; please provide base_url", req.ProviderID)})
			return
		}
		req.BaseURL = provider.API
	}

	// Auto-fill auth_scheme from provider mapping if not provided
	if req.AuthScheme == "" {
		if scheme, ok := providerAuthSchemes[req.ProviderID]; ok {
			req.AuthScheme = scheme
		} else {
			req.AuthScheme = "bearer" // default
		}
	}

	validSchemes := map[string]bool{"bearer": true, "x-api-key": true, "api-key": true, "query_param": true}
	if !validSchemes[req.AuthScheme] {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid auth_scheme"})
		return
	}

	if err := proxy.ValidateBaseURL(req.BaseURL); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("invalid base_url: %v", err)})
		return
	}

	dek, err := crypto.GenerateDEK()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "encryption failed"})
		return
	}

	encryptedKey, err := crypto.EncryptCredential([]byte(req.APIKey), dek)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "encryption failed"})
		return
	}

	wrappedDEK, err := h.kms.Wrap(r.Context(), dek)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "key wrapping failed"})
		return
	}

	for i := range dek {
		dek[i] = 0
	}

	if req.RefillInterval != nil {
		if _, err := time.ParseDuration(*req.RefillInterval); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid refill_interval: must be a valid Go duration (e.g. 1h, 24h)"})
			return
		}
	}


	cred := model.Credential{
		ID:             uuid.New(),
		OrgID:          org.ID,
		Label:          req.Label,
		BaseURL:        req.BaseURL,
		AuthScheme:     req.AuthScheme,
		ProviderID:     req.ProviderID,
		EncryptedKey:   encryptedKey,
		WrappedDEK:     wrappedDEK,
		Remaining:      req.Remaining,
		RefillAmount:   req.RefillAmount,
		RefillInterval: req.RefillInterval,
		Meta:           req.Meta,
	}

	if err := h.db.Create(&cred).Error; err != nil {
		slog.Error("failed to store credential", "error", err, "org_id", org.ID)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to store credential"})
		return
	}

	slog.Info("credential created", "org_id", org.ID, "credential_id", cred.ID, "provider_id", req.ProviderID, "label", req.Label)

	// Seed Redis counter if a cap is configured
	if cred.Remaining != nil && h.counter != nil {
		_ = h.counter.SeedCredential(r.Context(), cred.ID.String(), *cred.Remaining)
	}

	credResp := credentialResponse{
		ID:             cred.ID.String(),
		Label:          cred.Label,
		BaseURL:        cred.BaseURL,
		AuthScheme:     cred.AuthScheme,
		ProviderID:     cred.ProviderID,
		Remaining:      cred.Remaining,
		RefillAmount:   cred.RefillAmount,
		RefillInterval: cred.RefillInterval,
		Meta:           cred.Meta,
		CreatedAt:      cred.CreatedAt.Format(time.RFC3339),
	}
	writeJSON(w, http.StatusCreated, credResp)
}

// List handles GET /v1/credentials.
// @Summary List credentials
// @Description Returns credentials for the current organization with cursor-based pagination and usage stats.
// @Tags credentials
// @Produce json
// @Param external_id query string false "Filter by identity external ID"
// @Param meta query string false "Filter by JSONB metadata (e.g. {\"key\":\"value\"})"
// @Param limit query int false "Page size (default 50, max 100)"
// @Param cursor query string false "Pagination cursor from previous response"
// @Success 200 {object} paginatedResponse[credentialResponse]
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/credentials [get]
func (h *CredentialHandler) List(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	limit, cursor, err := parsePagination(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	q := h.db.Where("credentials.org_id = ?", org.ID)

	if metaFilter := r.URL.Query().Get("meta"); metaFilter != "" {
		q = q.Where("credentials.meta @> ?::jsonb", metaFilter)
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

	// Collect credential IDs for usage stats query
	credIDs := make([]uuid.UUID, len(creds))
	for i, c := range creds {
		credIDs[i] = c.ID
	}

	// Fetch usage stats from audit_log
	type credStats struct {
		CredentialID uuid.UUID  `gorm:"column:credential_id"`
		RequestCount int64     `gorm:"column:request_count"`
		LastUsedAt   *time.Time `gorm:"column:last_used_at"`
	}
	statsMap := make(map[string]credStats)
	if len(credIDs) > 0 {
		var stats []credStats
		h.db.Raw(`SELECT credential_id, COUNT(*) AS request_count, MAX(created_at) AS last_used_at
			FROM audit_log
			WHERE org_id = ? AND action = 'proxy.request' AND credential_id IN (?)
			GROUP BY credential_id`, org.ID, credIDs).Scan(&stats)
		for _, s := range stats {
			statsMap[s.CredentialID.String()] = s
		}
	}

	resp := make([]credentialResponse, len(creds))
	for i, c := range creds {
		resp[i] = credentialResponse{
			ID:             c.ID.String(),
			Label:          c.Label,
			BaseURL:        c.BaseURL,
			AuthScheme:     c.AuthScheme,
			ProviderID:     c.ProviderID,
			Remaining:      c.Remaining,
			RefillAmount:   c.RefillAmount,
			RefillInterval: c.RefillInterval,
			Meta:           c.Meta,
			CreatedAt:      c.CreatedAt.Format(time.RFC3339),
		}
		if c.RevokedAt != nil {
			s := c.RevokedAt.Format(time.RFC3339)
			resp[i].RevokedAt = &s
		}
		if st, ok := statsMap[c.ID.String()]; ok {
			resp[i].RequestCount = st.RequestCount
			if st.LastUsedAt != nil {
				s := st.LastUsedAt.Format(time.RFC3339)
				resp[i].LastUsedAt = &s
			}
		}
	}

	result := paginatedResponse[credentialResponse]{
		Data:    resp,
		HasMore: hasMore,
	}
	if hasMore {
		last := creds[len(creds)-1]
		c := encodeCursor(last.CreatedAt, last.ID)
		result.NextCursor = &c
	}

	writeJSON(w, http.StatusOK, result)
}

// Get handles GET /v1/credentials/{id}.
// @Summary Get a credential
// @Description Returns a single credential by ID with usage stats.
// @Tags credentials
// @Produce json
// @Param id path string true "Credential ID"
// @Success 200 {object} credentialResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/credentials/{id} [get]
func (h *CredentialHandler) Get(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	credID := chi.URLParam(r, "id")
	if credID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "credential id required"})
		return
	}

	var cred model.Credential
	if err := h.db.Where("id = ? AND org_id = ?", credID, org.ID).First(&cred).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "credential not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get credential"})
		return
	}

	resp := credentialResponse{
		ID:             cred.ID.String(),
		Label:          cred.Label,
		BaseURL:        cred.BaseURL,
		AuthScheme:     cred.AuthScheme,
		ProviderID:     cred.ProviderID,
		Remaining:      cred.Remaining,
		RefillAmount:   cred.RefillAmount,
		RefillInterval: cred.RefillInterval,
		Meta:           cred.Meta,
		CreatedAt:      cred.CreatedAt.Format(time.RFC3339),
	}
	if cred.RevokedAt != nil {
		s := cred.RevokedAt.Format(time.RFC3339)
		resp.RevokedAt = &s
	}

	// Fetch usage stats from audit_log
	var stats struct {
		RequestCount int64      `gorm:"column:request_count"`
		LastUsedAt   *time.Time `gorm:"column:last_used_at"`
	}
	h.db.Raw(`SELECT COUNT(*) AS request_count, MAX(created_at) AS last_used_at
		FROM audit_log
		WHERE org_id = ? AND action = 'proxy.request' AND credential_id = ?`, org.ID, cred.ID).Scan(&stats)
	resp.RequestCount = stats.RequestCount
	if stats.LastUsedAt != nil {
		s := stats.LastUsedAt.Format(time.RFC3339)
		resp.LastUsedAt = &s
	}

	writeJSON(w, http.StatusOK, resp)
}

// Revoke handles DELETE /v1/credentials/{id}.
// @Summary Revoke a credential
// @Description Soft-deletes a credential by setting its revoked_at timestamp.
// @Tags credentials
// @Produce json
// @Param id path string true "Credential ID"
// @Success 200 {object} credentialResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/credentials/{id} [delete]
func (h *CredentialHandler) Revoke(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	credID := chi.URLParam(r, "id")
	if credID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "credential id required"})
		return
	}

	now := time.Now()
	result := h.db.Model(&model.Credential{}).
		Where("id = ? AND org_id = ? AND revoked_at IS NULL", credID, org.ID).
		Update("revoked_at", &now)

	if result.Error != nil {
		slog.Error("failed to revoke credential", "error", result.Error, "org_id", org.ID, "credential_id", credID)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to revoke"})
		return
	}
	if result.RowsAffected == 0 {
		slog.Warn("credential not found or already revoked", "org_id", org.ID, "credential_id", credID)
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "credential not found or already revoked"})
		return
	}

	// Invalidate all cache tiers
	_ = h.cacheManager.InvalidateCredential(r.Context(), credID)

	slog.Info("credential revoked", "org_id", org.ID, "credential_id", credID)
	writeJSON(w, http.StatusOK, map[string]string{"status": "revoked"})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	// Auto-log 5xx responses so server errors are always visible in production.
	if status >= 500 {
		if body, ok := v.(map[string]string); ok {
			slog.Error("server error response",
				"status", status,
				"error", body["error"],
			)
		} else {
			slog.Error("server error response",
				"status", status,
			)
		}
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
