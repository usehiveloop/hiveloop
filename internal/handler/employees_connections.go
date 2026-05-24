package handler

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
)

type employeeConnectionResponse struct {
	ID                string     `json:"id"`
	OrgID             string     `json:"org_id"`
	IntegrationID     string     `json:"integration_id"`
	Provider          string     `json:"provider"`
	DisplayName       string     `json:"display_name"`
	NangoConnectionID string     `json:"nango_connection_id"`
	Meta              model.JSON `json:"meta,omitempty"`
	CreatedAt         string     `json:"created_at"`
	UpdatedAt         string     `json:"updated_at"`
}

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
	if err := h.db.WithContext(ctx).Model(&model.Employee{}).
		Where("id = ? AND org_id = ? AND status <> ?", agentID, org.ID, "archived").
		Count(&count).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load employee"})
		return
	}
	if count == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "employee not found"})
		return
	}

	var connections []model.Connection
	if err := h.db.WithContext(ctx).
		Preload("Integration").
		Joins("JOIN integrations ON integrations.id = connections.integration_id AND integrations.deleted_at IS NULL").
		Where("connections.org_id = ? AND connections.revoked_at IS NULL", org.ID).
		Order("connections.created_at DESC, connections.id DESC").
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

func employeeConnectionProvider(conn model.Connection) string {
	if conn.Integration.Provider != "" {
		return conn.Integration.Provider
	}
	return conn.Integration.UniqueKey
}

func toEmployeeConnectionResponse(conn model.Connection) employeeConnectionResponse {
	return employeeConnectionResponse{
		ID:                conn.ID.String(),
		OrgID:             conn.OrgID.String(),
		IntegrationID:     conn.IntegrationID.String(),
		Provider:          employeeConnectionProvider(conn),
		DisplayName:       conn.Integration.DisplayName,
		NangoConnectionID: conn.NangoConnectionID,
		Meta:              conn.Meta,
		CreatedAt:         conn.CreatedAt.Format(time.RFC3339),
		UpdatedAt:         conn.UpdatedAt.Format(time.RFC3339),
	}
}
