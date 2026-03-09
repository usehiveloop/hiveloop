package handler

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/useportal/llmvault/internal/middleware"
	"github.com/useportal/llmvault/internal/model"
)

// IdentityHandler manages identity CRUD operations.
type IdentityHandler struct {
	db *gorm.DB
}

// NewIdentityHandler creates a new identity handler.
func NewIdentityHandler(db *gorm.DB) *IdentityHandler {
	return &IdentityHandler{db: db}
}

type createIdentityRequest struct {
	ExternalID string                    `json:"external_id"`
	Meta       model.JSON                `json:"meta,omitempty"`
	RateLimits []identityRateLimitParams `json:"ratelimits,omitempty"`
}

type updateIdentityRequest struct {
	ExternalID *string                   `json:"external_id,omitempty"`
	Meta       model.JSON                `json:"meta,omitempty"`
	RateLimits []identityRateLimitParams `json:"ratelimits,omitempty"`
}

type identityRateLimitParams struct {
	Name     string `json:"name"`
	Limit    int64  `json:"limit"`
	Duration int64  `json:"duration"` // milliseconds
}

type identityResponse struct {
	ID         string                    `json:"id"`
	ExternalID string                    `json:"external_id"`
	Meta       model.JSON                `json:"meta,omitempty"`
	RateLimits []identityRateLimitParams `json:"ratelimits,omitempty"`
	CreatedAt  string                    `json:"created_at"`
	UpdatedAt  string                    `json:"updated_at"`
}

func toIdentityResponse(ident model.Identity) identityResponse {
	resp := identityResponse{
		ID:         ident.ID.String(),
		ExternalID: ident.ExternalID,
		Meta:       ident.Meta,
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
		ID:         uuid.New(),
		OrgID:      org.ID,
		ExternalID: req.ExternalID,
		Meta:       req.Meta,
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
// Supports query params: ?external_id=, ?meta={"key":"value"}
func (h *IdentityHandler) List(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	q := h.db.Preload("RateLimits").Where("org_id = ?", org.ID)

	if extID := r.URL.Query().Get("external_id"); extID != "" {
		q = q.Where("external_id = ?", extID)
	}
	if metaFilter := r.URL.Query().Get("meta"); metaFilter != "" {
		q = q.Where("meta @> ?::jsonb", metaFilter)
	}

	var identities []model.Identity
	if err := q.Order("created_at DESC").Find(&identities).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list identities"})
		return
	}

	resp := make([]identityResponse, len(identities))
	for i, ident := range identities {
		resp[i] = toIdentityResponse(ident)
	}

	writeJSON(w, http.StatusOK, resp)
}

// Update handles PUT /v1/identities/{id}.
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
		if len(updates) > 0 {
			if err := tx.Model(&ident).Updates(updates).Error; err != nil {
				return err
			}
		}

		// Replace rate limits if provided
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

	// Reload with updated data
	h.db.Preload("RateLimits").Where("id = ?", ident.ID).First(&ident)

	writeJSON(w, http.StatusOK, toIdentityResponse(ident))
}

// Delete handles DELETE /v1/identities/{id}.
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
