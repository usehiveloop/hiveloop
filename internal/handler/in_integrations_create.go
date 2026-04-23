package handler

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/google/uuid"

	"github.com/usehiveloop/hiveloop/internal/middleware"
	"github.com/usehiveloop/hiveloop/internal/model"
	"github.com/usehiveloop/hiveloop/internal/nango"
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

	provider, found := h.nango.GetProvider(req.Provider)
	if !found {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("unknown provider %q", req.Provider)})
		return
	}

	if _, ok := h.catalog.GetProvider(req.Provider); !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("provider %q is not supported — no action definitions available", req.Provider)})
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
		Provider:    req.Provider,
		Credentials: req.Credentials,
	}
	slog.Info("creating in-integration in nango", "provider", req.Provider, "nango_key", nk)
	if err := h.nango.CreateIntegration(r.Context(), nangoReq); err != nil {
		slog.Error("nango in-integration creation failed", "error", err, "provider", req.Provider)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "failed to create integration in Nango: " + err.Error()})
		return
	}

	var nangoConfig model.JSON
	integResp, err := h.nango.GetIntegration(r.Context(), nk)
	if err != nil {
		slog.Warn("failed to fetch nango in-integration details for config", "error", err, "nango_key", nk)
	} else {
		template, _ := h.nango.GetProviderTemplate(req.Provider)
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
		slog.Error("failed to store in-integration", "error", err, "provider", req.Provider)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create integration"})
		return
	}

	slog.Info("in-integration created", "integration_id", integ.ID, "provider", req.Provider)
	writeJSON(w, http.StatusCreated, toInIntegrationResponse(integ))
}
