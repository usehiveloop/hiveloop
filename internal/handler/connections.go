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

	"github.com/llmvault/llmvault/internal/mcp/catalog"
	"github.com/llmvault/llmvault/internal/middleware"
	"github.com/llmvault/llmvault/internal/model"
	"github.com/llmvault/llmvault/internal/nango"
)

type ConnectionHandler struct {
	db      *gorm.DB
	nango   *nango.Client
	catalog *catalog.Catalog
}

func NewConnectionHandler(db *gorm.DB, nangoClient *nango.Client, cat *catalog.Catalog) *ConnectionHandler {
	return &ConnectionHandler{db: db, nango: nangoClient, catalog: cat}
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
	ProviderConfig    model.JSON `json:"provider_config,omitempty"`
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

// buildConnectionProviderConfig extracts relevant fields from
// the Nango connection response, including credentials for token access.
// Sensitive internal fields (e.g. jwtToken) are stripped from connection_config.
func buildConnectionProviderConfig(nangoResp map[string]any) model.JSON {
	config := model.JSON{}
	for _, key := range []string{"connection_config", "metadata", "credentials", "provider"} {
		if v, exists := nangoResp[key]; exists && v != nil {
			config[key] = v
		}
	}
	// Strip sensitive internal fields from connection_config.
	if cc, ok := config["connection_config"].(map[string]any); ok {
		delete(cc, "jwtToken")
	}
	return config
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
	if err := h.db.Preload("Integration").
		Where("id = ? AND org_id = ? AND revoked_at IS NULL", connID, org.ID).
		First(&conn).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "connection not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get connection"})
		return
	}

	resp := toIntegConnResponse(conn)

	nangoKey := fmt.Sprintf("%s_%s", org.ID.String(), conn.Integration.UniqueKey)
	nangoResp, err := h.nango.GetConnection(r.Context(), conn.NangoConnectionID, nangoKey)
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

// Proxy handles /v1/connections/{id}/proxy/* — forwards any HTTP method/path/body
// through Nango's proxy to the upstream provider API.
func (h *ConnectionHandler) Proxy(w http.ResponseWriter, r *http.Request) {
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
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "connection not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get connection"})
		return
	}

	// Extract wildcard path from chi router
	proxyPath := chi.URLParam(r, "*")
	if proxyPath == "" || proxyPath[0] != '/' {
		proxyPath = "/" + proxyPath
	}

	nangoKey := fmt.Sprintf("%s_%s", org.ID.String(), conn.Integration.UniqueKey)
	nangoResp, err := h.nango.RawProxyRequest(
		r.Context(),
		r.Method,
		nangoKey,
		conn.NangoConnectionID,
		proxyPath,
		r.URL.RawQuery,
		r.Body,
		r.Header.Get("Content-Type"),
	)
	if err != nil {
		slog.Error("nango: proxy request failed",
			"error", err, "connection_id", connID, "path", proxyPath)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "upstream request failed"})
		return
	}

	if ct := nangoResp.Header.Get("Content-Type"); ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	w.WriteHeader(nangoResp.StatusCode)
	w.Write(nangoResp.Body)
}

// availableScopeAction describes a single action available on a connection.
type availableScopeAction struct {
	Key          string `json:"key"`
	DisplayName  string `json:"display_name"`
	Description  string `json:"description"`
	ResourceType string `json:"resource_type,omitempty"`
}

// availableScopeResource describes resources configured for a connection.
type availableScopeResource struct {
	DisplayName string                       `json:"display_name"`
	Selected    []availableScopeResourceItem `json:"selected"`
}

type availableScopeResourceItem struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// availableScopeConnection describes a connection with its available actions.
type availableScopeConnection struct {
	ConnectionID  string                            `json:"connection_id"`
	IntegrationID string                            `json:"integration_id"`
	Provider      string                            `json:"provider"`
	DisplayName   string                            `json:"display_name"`
	Actions       []availableScopeAction            `json:"actions"`
	Resources     map[string]availableScopeResource `json:"resources,omitempty"`
}

// AvailableScopes handles GET /v1/connections/available-scopes.
// Returns all active connections for the org, enriched with available actions from the catalog.
//
// @Summary List available scopes
// @Description Returns active connections enriched with their available actions from the catalog.
// @Tags connections
// @Produce json
// @Success 200 {array} availableScopeConnection
// @Failure 401 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/connections/available-scopes [get]
func (h *ConnectionHandler) AvailableScopes(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	// Load all active connections with their integrations
	var connections []model.Connection
	if err := h.db.Preload("Integration").
		Where("connections.org_id = ? AND connections.revoked_at IS NULL", org.ID).
		Joins("JOIN integrations ON integrations.id = connections.integration_id AND integrations.deleted_at IS NULL").
		Find(&connections).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list connections"})
		return
	}

	result := make([]availableScopeConnection, 0, len(connections))

	for _, conn := range connections {
		provider := conn.Integration.Provider

		// Look up provider in catalog
		providerActions, ok := h.catalog.GetProvider(provider)
		if !ok || len(providerActions.Actions) == 0 {
			continue // skip providers with no catalog actions
		}

		asc := availableScopeConnection{
			ConnectionID:  conn.ID.String(),
			IntegrationID: conn.IntegrationID.String(),
			Provider:      provider,
			DisplayName:   conn.Integration.DisplayName,
		}

		// Build actions list
		for key, action := range providerActions.Actions {
			if action.Execution == nil {
				continue // skip actions without execution config
			}
			asc.Actions = append(asc.Actions, availableScopeAction{
				Key:          key,
				DisplayName:  action.DisplayName,
				Description:  action.Description,
				ResourceType: action.ResourceType,
			})
		}

		if len(asc.Actions) == 0 {
			continue
		}

		// Build resources from connection meta
		if len(providerActions.Resources) > 0 {
			asc.Resources = make(map[string]availableScopeResource)
			for resourceType, resDef := range providerActions.Resources {
				res := availableScopeResource{
					DisplayName: resDef.DisplayName,
				}

				// Extract selected resources from connection meta
				if conn.Meta != nil {
					if resources, ok := conn.Meta["resources"]; ok {
						if resMap, ok := resources.(map[string]any); ok {
							if items, ok := resMap[resourceType]; ok {
								if itemList, ok := items.([]any); ok {
									for _, item := range itemList {
										if itemMap, ok := item.(map[string]any); ok {
											id, _ := itemMap["id"].(string)
											name, _ := itemMap["name"].(string)
											if id != "" {
												res.Selected = append(res.Selected, availableScopeResourceItem{
													ID:   id,
													Name: name,
												})
											}
										}
									}
								}
							}
						}
					}
				}

				asc.Resources[resourceType] = res
			}
		}

		result = append(result, asc)
	}

	writeJSON(w, http.StatusOK, result)
}
