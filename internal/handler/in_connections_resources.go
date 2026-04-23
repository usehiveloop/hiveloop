package handler

import (
	"fmt"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/middleware"
	"github.com/usehiveloop/hiveloop/internal/model"
	"github.com/usehiveloop/hiveloop/internal/resources"
)

// @Summary List available resources for a connection
// @Description Fetches available resources of a specific type from the provider API. For example, list all repositories for a GitHub connection.
// @Tags in-connections
// @Produce json
// @Param id path string true "In-Connection ID"
// @Param type path string true "Resource type (e.g., repository, project)"
// @Success 200 {object} resources.DiscoveryResult
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/in/connections/{id}/resources/{type} [get]
func (h *InConnectionHandler) ListResources(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	connID := chi.URLParam(r, "id")
	resourceType := chi.URLParam(r, "type")

	if connID == "" || resourceType == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "connection id and resource type are required"})
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

	provider := conn.InIntegration.Provider
	nangoProviderConfigKey := fmt.Sprintf("in_%s", conn.InIntegration.UniqueKey)

	result, err := h.discovery.Discover(r.Context(), provider, resourceType, nangoProviderConfigKey, conn.NangoConnectionID)
	if err != nil {
		slog.Error("resource discovery failed",
			"connection_id", connID,
			"provider", provider,
			"resource_type", resourceType,
			"error", err,
		)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to fetch resources"})
		return
	}

	writeJSON(w, http.StatusOK, result)
}
