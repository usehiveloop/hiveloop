package handler

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/mcp/catalog"
	"github.com/usehiveloop/hiveloop/internal/middleware"
	"github.com/usehiveloop/hiveloop/internal/model"
)

var employeeConnectionSkillNames = map[string][]string{
	"bugsink":                 {"bugsink"},
	"github":                  {"git-github"},
	"github-app":              {"git-github"},
	"github-app-code-reviews": {"git-github"},
	"linear":                  {"linear"},
}

type employeeConnectionResponse struct {
	ID                string                             `json:"id"`
	OrgID             string                             `json:"org_id"`
	InIntegrationID   string                             `json:"in_integration_id"`
	Provider          string                             `json:"provider"`
	DisplayName       string                             `json:"display_name"`
	NangoConnectionID string                             `json:"nango_connection_id"`
	Meta              model.JSON                         `json:"meta,omitempty"`
	EmployeeProfile   *catalog.EmployeeProfileCapability `json:"employee_profile,omitempty"`
	CreatedAt         string                             `json:"created_at"`
	UpdatedAt         string                             `json:"updated_at"`
}

// ListAvailableConnections handles GET /v1/employees/{id}/connections/available.
// @Summary List assignable employee connections
// @Description Returns non-revoked org connections that may be attached to an employee as tool capabilities. Connections used as employee profiles, such as Slack and GitHub identity profiles, are excluded.
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
		if employeeProfileConnectionProvider(employeeConnectionProvider(conn)) {
			continue
		}
		resp = append(resp, toEmployeeConnectionResponse(conn))
	}

	writeJSON(w, http.StatusOK, paginatedResponse[employeeConnectionResponse]{
		Data:    resp,
		HasMore: false,
	})
}

func employeeIntegrationsFromConnectionIDs(ids []uuid.UUID) model.JSON {
	out := model.JSON{}
	for _, id := range ids {
		out[id.String()] = map[string]any{"actions": []string{}}
	}
	return out
}

func employeeConnectionIDsFromIntegrations(integrations model.JSON) []uuid.UUID {
	if len(integrations) == 0 {
		return nil
	}
	ids := make([]uuid.UUID, 0, len(integrations))
	for rawID := range integrations {
		id, err := uuid.Parse(rawID)
		if err == nil {
			ids = append(ids, id)
		}
	}
	sort.Slice(ids, func(i, j int) bool {
		return ids[i].String() < ids[j].String()
	})
	return ids
}

func validateEmployeeConnectionIDs(ctx context.Context, db *gorm.DB, orgID uuid.UUID, rawIDs []string) ([]uuid.UUID, []model.InConnection, error) {
	if len(rawIDs) == 0 {
		return nil, nil, nil
	}
	seen := map[uuid.UUID]bool{}
	ids := make([]uuid.UUID, 0, len(rawIDs))
	for _, rawID := range rawIDs {
		id, err := uuid.Parse(rawID)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid connection_id %q", rawID)
		}
		if seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return nil, nil, nil
	}
	var connections []model.InConnection
	if err := db.WithContext(ctx).
		Preload("InIntegration").
		Where("id IN ? AND org_id = ? AND revoked_at IS NULL", ids, orgID).
		Find(&connections).Error; err != nil {
		return nil, nil, fmt.Errorf("load employee connections: %w", err)
	}
	if len(connections) != len(ids) {
		return nil, nil, fmt.Errorf("connection not found or revoked")
	}
	for _, conn := range connections {
		if employeeProfileConnectionProvider(employeeConnectionProvider(conn)) {
			display := conn.InIntegration.DisplayName
			if display == "" {
				display = employeeConnectionProvider(conn)
			}
			return nil, nil, fmt.Errorf("%s connections must be connected as employee profiles", display)
		}
	}
	return ids, connections, nil
}

func employeeConnectionProvider(conn model.InConnection) string {
	if conn.InIntegration.Provider != "" {
		return conn.InIntegration.Provider
	}
	return conn.InIntegration.UniqueKey
}

func employeeProfileConnectionProvider(provider string) bool {
	return integrationEmployeeProfileCapability(provider) != nil
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
		EmployeeProfile:   integrationEmployeeProfileCapability(employeeConnectionProvider(conn)),
		CreatedAt:         conn.CreatedAt.Format(time.RFC3339),
		UpdatedAt:         conn.UpdatedAt.Format(time.RFC3339),
	}
}
