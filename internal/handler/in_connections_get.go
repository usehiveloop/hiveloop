package handler

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/middleware"
	"github.com/usehiveloop/hiveloop/internal/model"
)

func (h *InConnectionHandler) Get(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	connID := chi.URLParam(r, "id")
	if connID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "connection id required"})
		return
	}

	var conn model.InConnection
	if err := h.db.Preload("InIntegration").
		Where("id = ? AND org_id = ? AND revoked_at IS NULL", connID, org.ID).
		First(&conn).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "connection not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get connection"})
		return
	}

	resp := h.toInConnectionResponse(conn)

	nk := inNangoKey(conn.InIntegration.UniqueKey)
	nangoResp, err := h.nango.GetConnection(r.Context(), conn.NangoConnectionID, nk)
	if err != nil {
		slog.Warn("nango: get connection failed, returning without provider_config",
			"error", err, "connection_id", connID, "nango_connection_id", conn.NangoConnectionID)
	} else if nangoResp != nil {
		pc := buildConnectionProviderConfig(nangoResp)
		if len(pc) > 0 {
			resp.ProviderConfig = pc
		}
	}

	writeJSON(w, http.StatusOK, resp)
}
