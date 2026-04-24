package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/usehiveloop/hiveloop/internal/middleware"
	"github.com/usehiveloop/hiveloop/internal/model"
)

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
	// is_system = false prevents a compromised org from revoking a platform
	// credential even if the credential_id was guessed or leaked.
	result := h.db.Model(&model.Credential{}).
		Where("id = ? AND org_id = ? AND is_system = ? AND revoked_at IS NULL", credID, org.ID, false).
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

	_ = h.cacheManager.InvalidateCredential(r.Context(), credID)

	slog.Info("credential revoked", "org_id", org.ID, "credential_id", credID)
	writeJSON(w, http.StatusOK, map[string]string{"status": "revoked"})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
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
