package handler

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/usehiveloop/hiveloop/internal/middleware"
	"github.com/usehiveloop/hiveloop/internal/model"
)

// TokenHandler manages sandbox proxy token operations.

// MCPServerCache is an interface for evicting cached MCP servers.

// NewTokenHandler creates a new token handler.
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
		slog.Error("failed to revoke token", "error", result.Error, "org_id", org.ID, "jti", jti)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to revoke token"})
		return
	}
	if result.RowsAffected == 0 {
		slog.Warn("token not found or already revoked", "org_id", org.ID, "jti", jti)
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "token not found or already revoked"})
		return
	}

	// Propagate revocation through cache tiers
	_ = h.cacheManager.InvalidateToken(r.Context(), jti, 24*time.Hour)

	// Evict cached MCP server for this token
	if h.serverCache != nil {
		h.serverCache.Evict(jti)
	}

	slog.Info("token revoked", "org_id", org.ID, "jti", jti)
	writeJSON(w, http.StatusOK, map[string]string{"status": "revoked"})
}
