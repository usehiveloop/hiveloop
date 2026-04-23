package handler

import (
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/model"
	"github.com/usehiveloop/hiveloop/internal/nango"
)

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