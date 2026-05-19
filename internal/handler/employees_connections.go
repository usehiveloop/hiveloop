package handler

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/usehiveloop/hiveloop/internal/middleware"
	"github.com/usehiveloop/hiveloop/internal/model"
)

var employeeConnectionSkillNames = map[string][]string{
	"bugsink":                 {"bugsink"},
	"github":                  {"git-github"},
	"github-app":              {"git-github"},
	"github-app-code-reviews": {"git-github"},
	"linear":                  {"linear"},
	"notion":                  {"notion"},
}

type employeeConnectionResponse struct {
	ID                string     `json:"id"`
	OrgID             string     `json:"org_id"`
	InIntegrationID   string     `json:"in_integration_id"`
	Provider          string     `json:"provider"`
	DisplayName       string     `json:"display_name"`
	NangoConnectionID string     `json:"nango_connection_id"`
	Meta              model.JSON `json:"meta,omitempty"`
	CreatedAt         string     `json:"created_at"`
	UpdatedAt         string     `json:"updated_at"`
}

// ListAvailableConnections handles GET /v1/employees/{id}/connections/available.
// @Summary List assignable employee connections
// @Description Returns non-revoked org connections available to the managed employee.
// @Tags employees
// @Produce json
// @Param id path string true "Employee agent ID"
// @Success 200 {object} paginatedResponse[employeeConnectionResponse]
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/employees/{id}/connections/available [get]
func (h *EmployeeHandler) ListAvailableConnections(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	org, ok := middleware.OrgFromContext(ctx)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	agentID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid employee id"})
		return
	}

	var count int64
	if err := h.db.WithContext(ctx).Model(&model.Agent{}).
		Where("id = ? AND org_id = ? AND is_employee = true AND is_system = false", agentID, org.ID).
		Count(&count).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load employee"})
		return
	}
	if count == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "employee not found"})
		return
	}

	var connections []model.InConnection
	if err := h.db.WithContext(ctx).
		Preload("InIntegration").
		Joins("JOIN in_integrations ON in_integrations.id = in_connections.in_integration_id AND in_integrations.deleted_at IS NULL").
		Where("in_connections.org_id = ? AND in_connections.revoked_at IS NULL", org.ID).
		Order("in_connections.created_at DESC, in_connections.id DESC").
		Find(&connections).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list employee connections"})
		return
	}

	resp := make([]employeeConnectionResponse, 0, len(connections))
	for _, conn := range connections {
		resp = append(resp, toEmployeeConnectionResponse(conn))
	}

	writeJSON(w, http.StatusOK, paginatedResponse[employeeConnectionResponse]{
		Data:    resp,
		HasMore: false,
	})
}

func employeeConnectionProvider(conn model.InConnection) string {
	if conn.InIntegration.Provider != "" {
		return conn.InIntegration.Provider
	}
	return conn.InIntegration.UniqueKey
}

func toEmployeeConnectionResponse(conn model.InConnection) employeeConnectionResponse {
	return employeeConnectionResponse{
		ID:                conn.ID.String(),
		OrgID:             conn.OrgID.String(),
		InIntegrationID:   conn.InIntegrationID.String(),
		Provider:          employeeConnectionProvider(conn),
		DisplayName:       conn.InIntegration.DisplayName,
		NangoConnectionID: conn.NangoConnectionID,
		Meta:              conn.Meta,
		CreatedAt:         conn.CreatedAt.Format(time.RFC3339),
		UpdatedAt:         conn.UpdatedAt.Format(time.RFC3339),
	}
}
