package handler

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/logging"
	"github.com/usehiveloop/hiveloop/internal/model"
)

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
		Label *string `json:"label,omitempty"`
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
	logging.FromContext(r.Context()).InfoContext(r.Context(), "admin: credential updated", "credential_id", id)
	writeJSON(w, http.StatusOK, toAdminCredentialResponse(cred))
}
