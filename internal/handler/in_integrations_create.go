package handler

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/nango"
)

func (h *InIntegrationHandler) Create(w http.ResponseWriter, r *http.Request) {
	if _, ok := middleware.UserFromContext(r.Context()); !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing user context"})
		return
	}

	var req createInIntegrationRequest
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

	provider, found := h.nango.GetProvider(nangoProviderName(req.Provider))
	if !found {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("unknown provider %q", req.Provider)})
		return
	}

	if _, ok := h.catalog.GetProvider(req.Provider); !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("provider %q is not supported — no action definitions available", req.Provider)})
		return
	}
	var existing int64
	if err := h.db.Model(&model.InIntegration{}).
		Where("provider = ? AND custom_app = false AND deleted_at IS NULL", req.Provider).
		Count(&existing).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to check integration"})
		return
	}
	if existing > 0 {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "integration already exists for this provider"})
		return
	}

	if err := validateCredentials(provider, req.Credentials); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	integID := uuid.New()
	uniqueKey := fmt.Sprintf("%s-%s", req.Provider, integID.String()[:8])

	nk := inNangoKey(uniqueKey)
	nangoReq := nango.CreateIntegrationRequest{
		UniqueKey:   nk,
		Provider:    nangoProviderName(req.Provider),
		Credentials: req.Credentials,
	}
	if err := h.nango.CreateIntegration(r.Context(), nangoReq); err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "nango in-integration creation failed", "error", err, "provider", req.Provider)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "failed to create integration in Nango: " + err.Error()})
		return
	}

	var nangoConfig model.JSON
	integResp, err := h.nango.GetIntegration(r.Context(), nk)
	if err != nil {
		logging.FromContext(r.Context()).WarnContext(r.Context(), "failed to fetch nango in-integration details for config", "error", err, "nango_key", nk)
	} else {
		template, _ := h.nango.GetProviderTemplate(nangoProviderName(req.Provider))
		nangoConfig = buildNangoConfig(integResp, template, h.nango.CallbackURL())
	}

	integ := model.InIntegration{
		ID:          integID,
		UniqueKey:   uniqueKey,
		Provider:    req.Provider,
		DisplayName: req.DisplayName,
		Meta:        req.Meta,
		NangoConfig: nangoConfig,
	}

	if err := h.db.Create(&integ).Error; err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "failed to store in-integration", "error", err, "provider", req.Provider)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create integration"})
		return
	}

	logging.FromContext(r.Context()).InfoContext(r.Context(), "in-integration created", "integration_id", integ.ID, "provider", req.Provider)
	writeJSON(w, http.StatusCreated, toInIntegrationResponse(integ))
}
