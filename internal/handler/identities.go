package handler

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/llmvault/llmvault/internal/crypto"
	"github.com/llmvault/llmvault/internal/middleware"
	"github.com/llmvault/llmvault/internal/model"
)

// IdentityHandler manages identity CRUD operations.
type IdentityHandler struct {
	db     *gorm.DB
	encKey *crypto.SymmetricKey // for encrypting env vars (nil if not configured)
}

// NewIdentityHandler creates a new identity handler.
func NewIdentityHandler(db *gorm.DB, encKey *crypto.SymmetricKey) *IdentityHandler {
	return &IdentityHandler{db: db, encKey: encKey}
}

type createIdentityRequest struct {
	ExternalID   string                    `json:"external_id"`
	Meta         model.JSON                `json:"meta,omitempty"`
	MemoryConfig model.JSON                `json:"memory_config,omitempty"`
	RateLimits   []identityRateLimitParams `json:"ratelimits,omitempty"`
}

type updateIdentityRequest struct {
	ExternalID   *string                   `json:"external_id,omitempty"`
	Meta         model.JSON                `json:"meta,omitempty"`
	MemoryConfig model.JSON                `json:"memory_config,omitempty"`
	RateLimits   []identityRateLimitParams `json:"ratelimits,omitempty"`
}

type identityRateLimitParams struct {
	Name     string `json:"name"`
	Limit    int64  `json:"limit"`
	Duration int64  `json:"duration"` // milliseconds
}

type identityResponse struct {
	ID           string                    `json:"id"`
	ExternalID   string                    `json:"external_id"`
	Meta         model.JSON                `json:"meta,omitempty"`
	MemoryConfig model.JSON                `json:"memory_config,omitempty"`
	RateLimits   []identityRateLimitParams `json:"ratelimits,omitempty"`
	RequestCount int64                     `json:"request_count"`
	LastUsedAt   *string                   `json:"last_used_at,omitempty"`
	CreatedAt    string                    `json:"created_at"`
	UpdatedAt    string                    `json:"updated_at"`
}

func toIdentityResponse(ident model.Identity) identityResponse {
	resp := identityResponse{
		ID:           ident.ID.String(),
		ExternalID:   ident.ExternalID,
		Meta:         ident.Meta,
		MemoryConfig: ident.MemoryConfig,
		CreatedAt:  ident.CreatedAt.Format(time.RFC3339),
		UpdatedAt:  ident.UpdatedAt.Format(time.RFC3339),
	}
	for _, rl := range ident.RateLimits {
		resp.RateLimits = append(resp.RateLimits, identityRateLimitParams{
			Name:     rl.Name,
			Limit:    rl.Limit,
			Duration: rl.Duration,
		})
	}
	return resp
}

// Create handles POST /v1/identities.
// @Summary Create an identity
// @Description Creates a new identity for the current organization.
// @Tags identities
// @Accept json
// @Produce json
// @Param body body createIdentityRequest true "Identity details"
// @Success 201 {object} identityResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 409 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/identities [post]
func (h *IdentityHandler) Create(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	var req createIdentityRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	if req.ExternalID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "external_id is required"})
		return
	}

	for _, rl := range req.RateLimits {
		if rl.Name == "" || rl.Limit <= 0 || rl.Duration <= 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "each ratelimit must have name, limit > 0, and duration > 0"})
			return
		}
	}

	ident := model.Identity{
		ID:           uuid.New(),
		OrgID:        org.ID,
		ExternalID:   req.ExternalID,
		Meta:         req.Meta,
		MemoryConfig: req.MemoryConfig,
	}

	for _, rl := range req.RateLimits {
		ident.RateLimits = append(ident.RateLimits, model.IdentityRateLimit{
			ID:         uuid.New(),
			IdentityID: ident.ID,
			Name:       rl.Name,
			Limit:      rl.Limit,
			Duration:   rl.Duration,
		})
	}

	if err := h.db.Create(&ident).Error; err != nil {
		if isDuplicateKeyError(err) {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "identity with this external_id already exists"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create identity"})
		return
	}

	writeJSON(w, http.StatusCreated, toIdentityResponse(ident))
}

// Get handles GET /v1/identities/{id}.
// @Summary Get an identity
// @Description Returns a single identity by ID.
// @Tags identities
// @Produce json
// @Param id path string true "Identity ID"
// @Success 200 {object} identityResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/identities/{id} [get]
func (h *IdentityHandler) Get(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	identID := chi.URLParam(r, "id")
	if identID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "identity id required"})
		return
	}

	var ident model.Identity
	if err := h.db.Preload("RateLimits").Where("id = ? AND org_id = ?", identID, org.ID).First(&ident).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "identity not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get identity"})
		return
	}

	writeJSON(w, http.StatusOK, toIdentityResponse(ident))
}

// List handles GET /v1/identities.
// @Summary List identities
// @Description Returns identities for the current organization with cursor-based pagination and usage stats.
// @Tags identities
// @Produce json
// @Param external_id query string false "Filter by external ID"
// @Param meta query string false "Filter by JSONB metadata (e.g. {\"key\":\"value\"})"
// @Param limit query int false "Page size (default 50, max 100)"
// @Param cursor query string false "Pagination cursor from previous response"
// @Success 200 {object} paginatedResponse[identityResponse]
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/identities [get]
func (h *IdentityHandler) List(w http.ResponseWriter, r *http.Request) {
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

	q := h.db.Preload("RateLimits").Where("org_id = ?", org.ID)

	if extID := r.URL.Query().Get("external_id"); extID != "" {
		q = q.Where("external_id = ?", extID)
	}
	if metaFilter := r.URL.Query().Get("meta"); metaFilter != "" {
		q = q.Where("meta @> ?::jsonb", metaFilter)
	}

	q = applyPagination(q, cursor, limit)

	var identities []model.Identity
	if err := q.Find(&identities).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list identities"})
		return
	}

	hasMore := len(identities) > limit
	if hasMore {
		identities = identities[:limit]
	}

	identIDs := make([]uuid.UUID, len(identities))
	for i, ident := range identities {
		identIDs[i] = ident.ID
	}

	type identStats struct {
		IdentityID   uuid.UUID  `gorm:"column:identity_id"`
		RequestCount int64      `gorm:"column:request_count"`
		LastUsedAt   *time.Time `gorm:"column:last_used_at"`
	}
	statsMap := make(map[string]identStats)
	if len(identIDs) > 0 {
		var stats []identStats
		h.db.Raw(`SELECT identity_id, COUNT(*) AS request_count, MAX(created_at) AS last_used_at
			FROM audit_log
			WHERE org_id = ? AND action = 'proxy.request' AND identity_id IN (?)
			GROUP BY identity_id`, org.ID, identIDs).Scan(&stats)
		for _, s := range stats {
			statsMap[s.IdentityID.String()] = s
		}
	}

	resp := make([]identityResponse, len(identities))
	for i, ident := range identities {
		resp[i] = toIdentityResponse(ident)
		if st, ok := statsMap[ident.ID.String()]; ok {
			resp[i].RequestCount = st.RequestCount
			if st.LastUsedAt != nil {
				s := st.LastUsedAt.Format(time.RFC3339)
				resp[i].LastUsedAt = &s
			}
		}
	}

	result := paginatedResponse[identityResponse]{
		Data:    resp,
		HasMore: hasMore,
	}
	if hasMore {
		last := identities[len(identities)-1]
		c := encodeCursor(last.CreatedAt, last.ID)
		result.NextCursor = &c
	}

	writeJSON(w, http.StatusOK, result)
}

// Update handles PUT /v1/identities/{id}.
// @Summary Update an identity
// @Description Updates an existing identity's external_id, metadata, or rate limits.
// @Tags identities
// @Accept json
// @Produce json
// @Param id path string true "Identity ID"
// @Param body body updateIdentityRequest true "Fields to update"
// @Success 200 {object} identityResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 409 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/identities/{id} [put]
func (h *IdentityHandler) Update(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	identID := chi.URLParam(r, "id")
	if identID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "identity id required"})
		return
	}

	var ident model.Identity
	if err := h.db.Preload("RateLimits").Where("id = ? AND org_id = ?", identID, org.ID).First(&ident).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "identity not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to find identity"})
		return
	}

	var req updateIdentityRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	for _, rl := range req.RateLimits {
		if rl.Name == "" || rl.Limit <= 0 || rl.Duration <= 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "each ratelimit must have name, limit > 0, and duration > 0"})
			return
		}
	}

	err := h.db.Transaction(func(tx *gorm.DB) error {
		updates := map[string]any{}
		if req.ExternalID != nil {
			updates["external_id"] = *req.ExternalID
		}
		if req.Meta != nil {
			updates["meta"] = req.Meta
		}
		if req.MemoryConfig != nil {
			updates["memory_config"] = req.MemoryConfig
		}
		if len(updates) > 0 {
			if err := tx.Model(&ident).Updates(updates).Error; err != nil {
				return err
			}
		}

		if req.RateLimits != nil {
			if err := tx.Where("identity_id = ?", ident.ID).Delete(&model.IdentityRateLimit{}).Error; err != nil {
				return err
			}
			for _, rl := range req.RateLimits {
				newRL := model.IdentityRateLimit{
					ID:         uuid.New(),
					IdentityID: ident.ID,
					Name:       rl.Name,
					Limit:      rl.Limit,
					Duration:   rl.Duration,
				}
				if err := tx.Create(&newRL).Error; err != nil {
					return err
				}
			}
		}

		return nil
	})

	if err != nil {
		if isDuplicateKeyError(err) {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "external_id already in use"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update identity"})
		return
	}

	h.db.Preload("RateLimits").Where("id = ?", ident.ID).First(&ident)

	writeJSON(w, http.StatusOK, toIdentityResponse(ident))
}

// Delete handles DELETE /v1/identities/{id}.
// @Summary Delete an identity
// @Description Permanently deletes an identity by ID.
// @Tags identities
// @Produce json
// @Param id path string true "Identity ID"
// @Success 200 {object} map[string]string
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/identities/{id} [delete]
func (h *IdentityHandler) Delete(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	identID := chi.URLParam(r, "id")
	if identID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "identity id required"})
		return
	}

	result := h.db.Where("id = ? AND org_id = ?", identID, org.ID).Delete(&model.Identity{})
	if result.Error != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to delete identity"})
		return
	}
	if result.RowsAffected == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "identity not found"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

type setupRequest struct {
	SetupCommands []string          `json:"setup_commands"`
	EnvVars       map[string]string `json:"env_vars"`
}

type setupResponse struct {
	SetupCommands []string `json:"setup_commands"`
	EnvVarKeys    []string `json:"env_var_keys"`
}


func isDuplicateKeyError(err error) bool {
	return err != nil && (contains(err.Error(), "duplicate key") || contains(err.Error(), "UNIQUE constraint"))
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstr(s, substr))
}

func containsSubstr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
