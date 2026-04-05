package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/ziraloop/ziraloop/internal/mcp/catalog"
	"github.com/ziraloop/ziraloop/internal/middleware"
	"github.com/ziraloop/ziraloop/internal/model"
	"github.com/ziraloop/ziraloop/internal/nango"
)

// InConnectionHandler manages user-scoped connections to built-in integrations.
type InConnectionHandler struct {
	db      *gorm.DB
	nango   *nango.Client
	catalog *catalog.Catalog
}

// NewInConnectionHandler creates a new in-connection handler.
func NewInConnectionHandler(db *gorm.DB, nangoClient *nango.Client, cat *catalog.Catalog) *InConnectionHandler {
	return &InConnectionHandler{db: db, nango: nangoClient, catalog: cat}
}

type createInConnectionRequest struct {
	NangoConnectionID string     `json:"nango_connection_id"`
	Meta              model.JSON `json:"meta,omitempty"`
}

type inConnectionResponse struct {
	ID                string     `json:"id"`
	InIntegrationID   string     `json:"in_integration_id"`
	Provider          string     `json:"provider"`
	DisplayName       string     `json:"display_name"`
	NangoConnectionID string     `json:"nango_connection_id"`
	Meta              model.JSON `json:"meta,omitempty"`
	ProviderConfig    model.JSON `json:"provider_config,omitempty"`
	RevokedAt         *string    `json:"revoked_at,omitempty"`
	CreatedAt         string     `json:"created_at"`
	UpdatedAt         string     `json:"updated_at"`
}

func toInConnectionResponse(conn model.InConnection) inConnectionResponse {
	resp := inConnectionResponse{
		ID:                conn.ID.String(),
		InIntegrationID:   conn.InIntegrationID.String(),
		Provider:          conn.InIntegration.Provider,
		DisplayName:       conn.InIntegration.DisplayName,
		NangoConnectionID: conn.NangoConnectionID,
		Meta:              conn.Meta,
		CreatedAt:         conn.CreatedAt.Format(time.RFC3339),
		UpdatedAt:         conn.UpdatedAt.Format(time.RFC3339),
	}
	if conn.RevokedAt != nil {
		s := conn.RevokedAt.Format(time.RFC3339)
		resp.RevokedAt = &s
	}
	return resp
}

type inConnectSessionResponse struct {
	Token             string `json:"token"`
	ProviderConfigKey string `json:"provider_config_key"`
}

// CreateConnectSession handles POST /v1/in/integrations/{id}/connect-session.
// @Summary Create a connect session
// @Description Creates a Nango connect session for the authenticated user to initiate OAuth.
// @Tags in-connections
// @Produce json
// @Param id path string true "Integration ID"
// @Success 201 {object} inConnectSessionResponse
// @Failure 400 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /v1/in/integrations/{id}/connect-session [post]
func (h *InConnectionHandler) CreateConnectSession(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.UserFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing user context"})
		return
	}

	integID := chi.URLParam(r, "id")
	if integID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "integration id required"})
		return
	}

	var integ model.InIntegration
	if err := h.db.Where("id = ? AND deleted_at IS NULL", integID).First(&integ).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "integration not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to find integration"})
		return
	}

	nk := inNangoKey(integ.UniqueKey)
	nangoReq := nango.CreateConnectSessionRequest{
		EndUser: nango.ConnectSessionEndUser{
			ID: user.ID.String(),
		},
		AllowedIntegrations: []string{nk},
	}

	sess, err := h.nango.CreateConnectSession(r.Context(), nangoReq)
	if err != nil {
		slog.Error("nango connect session creation failed", "error", err, "integration_id", integ.ID, "user_id", user.ID)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "failed to create connect session: " + err.Error()})
		return
	}

	writeJSON(w, http.StatusCreated, inConnectSessionResponse{
		Token:             sess.Token,
		ProviderConfigKey: nk,
	})
}

// Create handles POST /v1/in/integrations/{id}/connections.
// @Summary Create an in-connection
// @Description Stores a connection after the OAuth flow completes via Nango.
// @Tags in-connections
// @Accept json
// @Produce json
// @Param id path string true "Integration ID"
// @Param body body createInConnectionRequest true "Connection details"
// @Success 201 {object} inConnectionResponse
// @Failure 400 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /v1/in/integrations/{id}/connections [post]
func (h *InConnectionHandler) Create(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.UserFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing user context"})
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

	var integ model.InIntegration
	if err := h.db.Where("id = ? AND deleted_at IS NULL", integUUID).First(&integ).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "integration not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to find integration"})
		return
	}

	var req createInConnectionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	if req.NangoConnectionID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "nango_connection_id is required"})
		return
	}

	conn := model.InConnection{
		ID:                uuid.New(),
		UserID:            user.ID,
		InIntegrationID:   integ.ID,
		NangoConnectionID: req.NangoConnectionID,
		Meta:              req.Meta,
	}

	if err := h.db.Create(&conn).Error; err != nil {
		slog.Error("failed to create in-connection", "error", err, "user_id", user.ID, "integration_id", integ.ID)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create connection"})
		return
	}

	conn.InIntegration = integ
	slog.Info("in-connection created", "connection_id", conn.ID, "user_id", user.ID, "provider", integ.Provider)
	writeJSON(w, http.StatusCreated, toInConnectionResponse(conn))
}

// List handles GET /v1/in/connections.
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
	user, ok := middleware.UserFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing user context"})
		return
	}

	limit, cursor, err := parsePagination(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	q := h.db.Preload("InIntegration").
		Where("in_connections.user_id = ? AND in_connections.revoked_at IS NULL", user.ID).
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
		resp[i] = toInConnectionResponse(conn)
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

// Get handles GET /v1/in/connections/{id}.
func (h *InConnectionHandler) Get(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.UserFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing user context"})
		return
	}

	connID := chi.URLParam(r, "id")
	if connID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "connection id required"})
		return
	}

	var conn model.InConnection
	if err := h.db.Preload("InIntegration").
		Where("id = ? AND user_id = ? AND revoked_at IS NULL", connID, user.ID).
		First(&conn).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "connection not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get connection"})
		return
	}

	resp := toInConnectionResponse(conn)

	// Live-fetch provider config from Nango (best-effort)
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

// Revoke handles DELETE /v1/in/connections/{id}.
func (h *InConnectionHandler) Revoke(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.UserFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing user context"})
		return
	}

	connID := chi.URLParam(r, "id")
	if connID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "connection id required"})
		return
	}

	var conn model.InConnection
	if err := h.db.Preload("InIntegration").
		Where("id = ? AND user_id = ? AND revoked_at IS NULL", connID, user.ID).
		First(&conn).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "connection not found or already revoked"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to revoke connection"})
		return
	}

	// Delete from Nango (best-effort — still revoke locally even if Nango fails)
	nk := inNangoKey(conn.InIntegration.UniqueKey)
	if err := h.nango.DeleteConnection(r.Context(), conn.NangoConnectionID, nk); err != nil {
		slog.Error("nango: delete connection failed, proceeding with local revocation",
			"error", err, "connection_id", connID, "nango_connection_id", conn.NangoConnectionID)
	}

	now := time.Now()
	result := h.db.Model(&model.InConnection{}).
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

	slog.Info("in-connection revoked", "connection_id", conn.ID, "user_id", user.ID, "provider", conn.InIntegration.Provider)
	writeJSON(w, http.StatusOK, map[string]string{"status": "revoked"})
}
