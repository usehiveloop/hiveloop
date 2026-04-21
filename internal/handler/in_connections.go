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

	"github.com/usehiveloop/hiveloop/internal/mcp/catalog"
	"github.com/usehiveloop/hiveloop/internal/middleware"
	"github.com/usehiveloop/hiveloop/internal/model"
	"github.com/usehiveloop/hiveloop/internal/nango"
	"github.com/usehiveloop/hiveloop/internal/resources"
)

// InConnectionHandler manages user-scoped connections to built-in integrations.
type InConnectionHandler struct {
	db        *gorm.DB
	nango     *nango.Client
	catalog   *catalog.Catalog
	discovery *resources.Discovery
}

// NewInConnectionHandler creates a new in-connection handler.
func NewInConnectionHandler(db *gorm.DB, nangoClient *nango.Client, cat *catalog.Catalog) *InConnectionHandler {
	return &InConnectionHandler{
		db:        db,
		nango:     nangoClient,
		catalog:   cat,
		discovery: resources.NewDiscovery(cat, nangoClient),
	}
}

type createInConnectionRequest struct {
	NangoConnectionID string     `json:"nango_connection_id"`
	Meta              model.JSON `json:"meta,omitempty"`
}

type inConnectionResponse struct {
	ID                    string                                `json:"id"`
	OrgID                 string                                `json:"org_id"`
	InIntegrationID       string                                `json:"in_integration_id"`
	Provider              string                                `json:"provider"`
	DisplayName           string                                `json:"display_name"`
	NangoConnectionID     string                                `json:"nango_connection_id"`
	Meta                  model.JSON                            `json:"meta,omitempty"`
	ProviderConfig        model.JSON                            `json:"provider_config,omitempty"`
	ActionsCount          int                                   `json:"actions_count"`
	WebhookConfigured     bool                                  `json:"webhook_configured"`
	ConfigurableResources []catalog.ConfigurableResourceSummary `json:"configurable_resources"`
	RevokedAt             *string                               `json:"revoked_at,omitempty"`
	CreatedAt             string                                `json:"created_at"`
	UpdatedAt             string                                `json:"updated_at"`
}

func (h *InConnectionHandler) toInConnectionResponse(conn model.InConnection) inConnectionResponse {
	provider := conn.InIntegration.Provider
	configurableRes := h.catalog.GetConfigurableResources(provider)
	if configurableRes == nil {
		configurableRes = []catalog.ConfigurableResourceSummary{}
	}
	resp := inConnectionResponse{
		ID:                    conn.ID.String(),
		OrgID:                 conn.OrgID.String(),
		InIntegrationID:       conn.InIntegrationID.String(),
		Provider:              provider,
		DisplayName:           conn.InIntegration.DisplayName,
		NangoConnectionID:     conn.NangoConnectionID,
		Meta:                  conn.Meta,
		ActionsCount:          len(h.catalog.ListActions(provider)),
		WebhookConfigured:     derefBool(conn.WebhookConfigured, true),
		ConfigurableResources: configurableRes,
		CreatedAt:             conn.CreatedAt.Format(time.RFC3339),
		UpdatedAt:             conn.UpdatedAt.Format(time.RFC3339),
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

// CreateReconnectSession handles POST /v1/in/connections/{id}/reconnect-session.
// @Summary Create a reconnect session for an existing connection
// @Description Creates a Nango connect session scoped to an existing connection, allowing OAuth re-authorization without creating a duplicate.
// @Tags in-connections
// @Produce json
// @Param id path string true "Connection ID"
// @Success 201 {object} inConnectSessionResponse
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /v1/in/connections/{id}/reconnect-session [post]
func (h *InConnectionHandler) CreateReconnectSession(w http.ResponseWriter, r *http.Request) {
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
	if err := h.db.Preload("InIntegration").Where("id = ? AND revoked_at IS NULL", connID).First(&conn).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "connection not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to find connection"})
		return
	}

	nk := inNangoKey(conn.InIntegration.UniqueKey)

	sess, err := h.nango.CreateReconnectSession(r.Context(), nango.CreateReconnectSessionRequest{
		ConnectionID:  conn.NangoConnectionID,
		IntegrationID: nk,
	})
	if err != nil {
		slog.Error("nango reconnect session creation failed", "error", err, "connection_id", conn.ID, "user_id", user.ID)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "failed to create reconnect session: " + err.Error()})
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
		OrgID:             org.ID,
		UserID:            user.ID,
		InIntegrationID:   integ.ID,
		NangoConnectionID: req.NangoConnectionID,
		Meta:              req.Meta,
		WebhookConfigured: boolPtr(!providerRequiresWebhookConfig(integ.Provider)),
	}

	if err := h.db.Create(&conn).Error; err != nil {
		slog.Error("failed to create in-connection", "error", err, "org_id", org.ID, "user_id", user.ID, "integration_id", integ.ID)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create connection"})
		return
	}

	conn.InIntegration = integ
	slog.Info("in-connection created", "connection_id", conn.ID, "org_id", org.ID, "user_id", user.ID, "provider", integ.Provider)
	writeJSON(w, http.StatusCreated, h.toInConnectionResponse(conn))
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

// Get handles GET /v1/in/connections/{id}.
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

// MarkWebhookConfigured handles PATCH /v1/in/connections/{id}/webhook-configured.
// @Summary Mark webhook as configured
// @Description Sets the webhook_configured flag to true on a connection, indicating the user has manually configured the webhook URL in the provider's dashboard.
// @Tags in-connections
// @Produce json
// @Param id path string true "Connection ID"
// @Success 200 {object} inConnectionResponse
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /v1/in/connections/{id}/webhook-configured [patch]
func (h *InConnectionHandler) MarkWebhookConfigured(w http.ResponseWriter, r *http.Request) {
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

	result := h.db.Model(&model.InConnection{}).
		Where("id = ? AND org_id = ? AND revoked_at IS NULL", connID, org.ID).
		Update("webhook_configured", true)

	if result.Error != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update connection"})
		return
	}
	if result.RowsAffected == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "connection not found"})
		return
	}

	var conn model.InConnection
	if err := h.db.Preload("InIntegration").Where("id = ?", connID).First(&conn).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to reload connection"})
		return
	}

	writeJSON(w, http.StatusOK, h.toInConnectionResponse(conn))
}

// Revoke handles DELETE /v1/in/connections/{id}.
// @Summary Disconnect an in-connection
// @Description Revokes a user's platform integration connection and removes it from Nango.
// @Tags in-connections
// @Produce json
// @Param id path string true "Connection ID"
// @Success 200 {object} map[string]string
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /v1/in/connections/{id} [delete]
func (h *InConnectionHandler) Revoke(w http.ResponseWriter, r *http.Request) {
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

	slog.Info("in-connection revoked", "connection_id", conn.ID, "org_id", org.ID, "provider", conn.InIntegration.Provider)
	writeJSON(w, http.StatusOK, map[string]string{"status": "revoked"})
}

// ListResources handles GET /v1/in/connections/{id}/resources/{type}.
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
