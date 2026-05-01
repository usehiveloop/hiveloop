package handler

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/usehiveloop/hiveloop/internal/logging"
	"github.com/usehiveloop/hiveloop/internal/middleware"
	"github.com/usehiveloop/hiveloop/internal/model"
)

// @Summary Revoke a proxy token
// @Description Revokes a proxy token by its JTI and propagates through cache tiers.
// @Tags tokens
// @Produce json
// @Param jti path string true "Token JTI"
// @Success 200 {object} map[string]string
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/tokens/{jti} [delete]
func (h *TokenHandler) Revoke(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	jti := chi.URLParam(r, "jti")
	if jti == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "jti required"})
		return
	}

	now := time.Now()
	result := h.db.Model(&model.Token{}).
		Where("jti = ? AND org_id = ? AND revoked_at IS NULL", jti, org.ID).
		Update("revoked_at", &now)

	if result.Error != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "failed to revoke token", "error", result.Error, "org_id", org.ID, "jti", jti)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to revoke token"})
		return
	}
	if result.RowsAffected == 0 {
		logging.FromContext(r.Context()).WarnContext(r.Context(), "token not found or already revoked", "org_id", org.ID, "jti", jti)
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "token not found or already revoked"})
		return
	}

	_ = h.cacheManager.InvalidateToken(r.Context(), jti, 24*time.Hour)

	if h.serverCache != nil {
		h.serverCache.Evict(jti)
	}

	logging.FromContext(r.Context()).InfoContext(r.Context(), "token revoked", "org_id", org.ID, "jti", jti)
	writeJSON(w, http.StatusOK, map[string]string{"status": "revoked"})
}
