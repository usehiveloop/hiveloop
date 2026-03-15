package handler

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/llmvault/llmvault/internal/middleware"
	"github.com/llmvault/llmvault/internal/model"
	"github.com/llmvault/llmvault/internal/nango"
)

type ConnectionHandler struct {
	db    *gorm.DB
	nango *nango.Client
}

func NewConnectionHandler(db *gorm.DB, nangoClient *nango.Client) *ConnectionHandler {
	return &ConnectionHandler{db: db, nango: nangoClient}
}

type integConnCreateRequest struct {
	NangoConnectionID string     `json:"nango_connection_id"`
	IdentityID        *string    `json:"identity_id,omitempty"`
	Meta              model.JSON `json:"meta,omitempty"`
}

type integConnResponse struct {
	ID                string     `json:"id"`
	IntegrationID     string     `json:"integration_id"`
	NangoConnectionID string     `json:"nango_connection_id"`
	IdentityID        *string    `json:"identity_id,omitempty"`
	Meta              model.JSON `json:"meta,omitempty"`
	RevokedAt         *string    `json:"revoked_at,omitempty"`
	CreatedAt         string     `json:"created_at"`
	UpdatedAt         string     `json:"updated_at"`
}

func toIntegConnResponse(conn model.Connection) integConnResponse {
	resp := integConnResponse{
		ID:                conn.ID.String(),
		IntegrationID:     conn.IntegrationID.String(),
		NangoConnectionID: conn.NangoConnectionID,
		Meta:              conn.Meta,
		CreatedAt:         conn.CreatedAt.Format(time.RFC3339),
		UpdatedAt:         conn.UpdatedAt.Format(time.RFC3339),
	}
	if conn.IdentityID != nil {
		s := conn.IdentityID.String()
		resp.IdentityID = &s
	}
	if conn.RevokedAt != nil {
		s := conn.RevokedAt.Format(time.RFC3339)
		resp.RevokedAt = &s
	}
	return resp
}

// Create handles POST /v1/integrations/{id}/connections.
//
// @Summary Create a connection
// @Description Creates a new connection for an integration.
// @Tags connections
// @Accept json
// @Produce json
// @Param id path string true "Integration ID"
// @Param body body integConnCreateRequest true "Connection parameters"
// @Success 201 {object} integConnResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/integrations/{id}/connections [post]
func (h *ConnectionHandler) Create(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	integID := chi.URLParam(r, "id")
	if integID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "integration id required"})
		return
	}

	integUUID, err := uuid.Parse(integID)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid integration id"})
		return
	}

	var integ model.Integration
	if err := h.db.Where("id = ? AND org_id = ? AND deleted_at IS NULL", integUUID, org.ID).First(&integ).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "integration not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to find integration"})
		return
	}

	var req integConnCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	if req.NangoConnectionID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "nango_connection_id is required"})
		return
	}

	conn := model.Connection{
		ID:                uuid.New(),
		OrgID:             org.ID,
		IntegrationID:     integ.ID,
		NangoConnectionID: req.NangoConnectionID,
		Meta:              req.Meta,
	}

	if req.IdentityID != nil {
		identityUUID, err := uuid.Parse(*req.IdentityID)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid identity_id"})
			return
		}

		var ident model.Identity
		if err := h.db.Where("id = ? AND org_id = ?", identityUUID, org.ID).First(&ident).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "identity not found"})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to verify identity"})
			return
		}
		conn.IdentityID = &identityUUID
	}

	if err := h.db.Create(&conn).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create connection"})
		return
	}

	writeJSON(w, http.StatusCreated, toIntegConnResponse(conn))
}

// List handles GET /v1/integrations/{id}/connections.
//
// @Summary List connections
// @Description Returns connections for an integration with cursor pagination.
// @Tags connections
// @Produce json
// @Param id path string true "Integration ID"
// @Param limit query int false "Max items per page (1-100, default 50)"
// @Param cursor query string false "Pagination cursor from previous response"
// @Success 200 {object} paginatedResponse[integConnResponse]
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/integrations/{id}/connections [get]
func (h *ConnectionHandler) List(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	integID := chi.URLParam(r, "id")
	if integID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "integration id required"})
		return
	}

	integUUID, err := uuid.Parse(integID)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid integration id"})
		return
	}

	var integ model.Integration
	if err := h.db.Where("id = ? AND org_id = ? AND deleted_at IS NULL", integUUID, org.ID).First(&integ).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "integration not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to find integration"})
		return
	}

	limit, cursor, err := parsePagination(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	q := h.db.Where("org_id = ? AND integration_id = ? AND revoked_at IS NULL", org.ID, integ.ID)
	q = applyPagination(q, cursor, limit)

	var connections []model.Connection
	if err := q.Find(&connections).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list connections"})
		return
	}

	hasMore := len(connections) > limit
	if hasMore {
		connections = connections[:limit]
	}

	resp := make([]integConnResponse, len(connections))
	for i, conn := range connections {
		resp[i] = toIntegConnResponse(conn)
	}

	result := paginatedResponse[integConnResponse]{
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

// Get handles GET /v1/connections/{id}.
//
// @Summary Get a connection
// @Description Returns a single connection by ID.
// @Tags connections
// @Produce json
// @Param id path string true "Connection ID"
// @Success 200 {object} integConnResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/connections/{id} [get]
func (h *ConnectionHandler) Get(w http.ResponseWriter, r *http.Request) {
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

	var conn model.Connection
	if err := h.db.Where("id = ? AND org_id = ? AND revoked_at IS NULL", connID, org.ID).First(&conn).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "connection not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get connection"})
		return
	}

	writeJSON(w, http.StatusOK, toIntegConnResponse(conn))
}

// Revoke handles DELETE /v1/connections/{id}.
//
// @Summary Revoke a connection
// @Description Soft-deletes a connection by setting revoked_at.
// @Tags connections
// @Produce json
// @Param id path string true "Connection ID"
// @Success 200 {object} map[string]string
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/connections/{id} [delete]
func (h *ConnectionHandler) Revoke(w http.ResponseWriter, r *http.Request) {
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

	var conn model.Connection
	if err := h.db.Preload("Integration").
		Where("id = ? AND org_id = ? AND revoked_at IS NULL", connID, org.ID).
		First(&conn).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "connection not found or already revoked"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to revoke connection"})
		return
	}

	nangoProviderConfigKey := fmt.Sprintf("%s_%s", org.ID.String(), conn.Integration.UniqueKey)
	if err := h.nango.DeleteConnection(r.Context(), conn.NangoConnectionID, nangoProviderConfigKey); err != nil {
		slog.Error("nango: delete connection failed, proceeding with local revocation",
			"error", err, "connection_id", connID, "nango_connection_id", conn.NangoConnectionID)
	}

	now := time.Now()
	result := h.db.Model(&model.Connection{}).
		Where("id = ? AND revoked_at IS NULL", connID).
		Update("revoked_at", &now)

	if result.Error != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to revoke connection"})
		return
	}
	if result.RowsAffected == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "connection not found or already revoked"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "revoked"})
}
