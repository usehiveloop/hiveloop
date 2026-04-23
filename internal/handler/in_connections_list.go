package handler

import (
	"net/http"

	"github.com/usehiveloop/hiveloop/internal/middleware"
	"github.com/usehiveloop/hiveloop/internal/model"
)

// @Summary List user's in-connections
// @Description Returns the authenticated user's non-revoked platform integration connections.
// @Tags in-connections
// @Produce json
// @Param provider query string false "Filter by provider"
// @Param limit query int false "Page size"
// @Param cursor query string false "Pagination cursor"
// @Success 200 {object} paginatedResponse[inConnectionResponse]
// @Security BearerAuth
// @Router /v1/in/connections [get]
func (h *InConnectionHandler) List(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	limit, cursor, err := parsePagination(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	q := h.db.Preload("InIntegration").
		Where("in_connections.org_id = ? AND in_connections.revoked_at IS NULL", org.ID).
		Joins("JOIN in_integrations ON in_integrations.id = in_connections.in_integration_id AND in_integrations.deleted_at IS NULL")

	if provider := r.URL.Query().Get("provider"); provider != "" {
		q = q.Where("in_integrations.provider = ?", provider)
	}

	q = applyPagination(q, cursor, limit)

	var connections []model.InConnection
	if err := q.Find(&connections).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list connections"})
		return
	}

	hasMore := len(connections) > limit
	if hasMore {
		connections = connections[:limit]
	}

	resp := make([]inConnectionResponse, len(connections))
	for i, conn := range connections {
		resp[i] = h.toInConnectionResponse(conn)
	}

	result := paginatedResponse[inConnectionResponse]{
		Data:    resp,
		HasMore: hasMore,
	}
	if hasMore {
		last := connections[len(connections)-1]
		c := encodeCursor(last.CreatedAt, last.ID)
		result.NextCursor = &c
	}

	writeJSON(w, http.StatusOK, result)
}
