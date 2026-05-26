package handler

import (
	"time"

	"github.com/usehivy/hivy/internal/enqueue"
	"github.com/usehivy/hivy/internal/mcp/catalog"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/nango"
	"github.com/usehivy/hivy/internal/resources"
	"gorm.io/gorm"
)

type ConnectionHandler struct {
	db        *gorm.DB
	nango     *nango.Client
	catalog   *catalog.Catalog
	discovery *resources.Discovery
	enq       enqueue.TaskEnqueuer
}

func NewConnectionHandler(db *gorm.DB, nangoClient *nango.Client, cat *catalog.Catalog, enq enqueue.TaskEnqueuer) *ConnectionHandler {
	return &ConnectionHandler{
		db:        db,
		nango:     nangoClient,
		catalog:   cat,
		discovery: resources.NewDiscovery(cat, nangoClient),
		enq:       enq,
	}
}

type createConnectionRequest struct {
	NangoConnectionID string     `json:"nango_connection_id"`
	Meta              model.JSON `json:"meta,omitempty"`
}

type connectionResponse struct {
	ID                    string                                `json:"id"`
	OrgID                 string                                `json:"org_id"`
	IntegrationID         string                                `json:"integration_id"`
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

func (h *ConnectionHandler) toConnectionResponse(conn model.Connection) connectionResponse {
	provider := conn.Integration.Provider
	configurableRes := h.catalog.GetConfigurableResources(provider)
	if configurableRes == nil {
		configurableRes = []catalog.ConfigurableResourceSummary{}
	}
	resp := connectionResponse{
		ID:                    conn.ID.String(),
		OrgID:                 conn.OrgID.String(),
		IntegrationID:         conn.IntegrationID.String(),
		Provider:              provider,
		DisplayName:           conn.Integration.DisplayName,
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

type connectSessionResponse struct {
	Token             string `json:"token"`
	ProviderConfigKey string `json:"provider_config_key"`
}
