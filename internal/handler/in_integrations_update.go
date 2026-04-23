package handler

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/model"
	"github.com/usehiveloop/hiveloop/internal/nango"
)

func (h *InIntegrationHandler) Update(w http.ResponseWriter, r *http.Request) {
	integID := chi.URLParam(r, "id")
	if integID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "integration id required"})
		return
	}

	var integ model.InIntegration
	if err := h.db.Where("id = ? AND deleted_at IS NULL", integID).First(&integ).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "integration not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to find integration"})
		return
	}

	var req updateInIntegrationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	if req.Credentials != nil {
		provider, found := h.nango.GetProvider(integ.Provider)
		if !found {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("unknown provider %q", integ.Provider)})
			return
		}
		if err := validateCredentials(provider, req.Credentials); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}

		nk := inNangoKey(integ.UniqueKey)
		nangoReq := nango.UpdateIntegrationRequest{
			Credentials: req.Credentials,
		}
		slog.Info("updating nango in-integration credentials", "integration_id", integ.ID, "nango_key", nk)
		if err := h.nango.UpdateIntegration(r.Context(), nk, nangoReq); err != nil {
			slog.Error("nango in-integration update failed", "error", err, "integration_id", integ.ID)
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "failed to update integration in Nango: " + err.Error()})
			return
		}

		integResp, fetchErr := h.nango.GetIntegration(r.Context(), nk)
		if fetchErr != nil {
			slog.Warn("failed to fetch nango in-integration details for config rebuild", "error", fetchErr, "nango_key", nk)
		} else {
			template, _ := h.nango.GetProviderTemplate(integ.Provider)
			integ.NangoConfig = buildNangoConfig(integResp, template, h.nango.CallbackURL())
		}
	}

	updates := map[string]any{}
	if req.DisplayName != nil {
		updates["display_name"] = *req.DisplayName
	}
	if req.Meta != nil {
		updates["meta"] = req.Meta
	}
	if integ.NangoConfig != nil {
		updates["nango_config"] = integ.NangoConfig
	}
	if len(updates) > 0 {
		if err := h.db.Model(&integ).Updates(updates).Error; err != nil {
			slog.Error("failed to update in-integration", "error", err, "integration_id", integ.ID)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update integration"})
			return
		}
	}

	h.db.Where("id = ?", integ.ID).First(&integ)

	slog.Info("in-integration updated", "integration_id", integ.ID, "credentials_updated", req.Credentials != nil)
	writeJSON(w, http.StatusOK, toInIntegrationResponse(integ))
}
