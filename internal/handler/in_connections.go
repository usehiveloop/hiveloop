package handler

import (
	"time"

	"github.com/usehiveloop/hiveloop/internal/mcp/catalog"
	"github.com/usehiveloop/hiveloop/internal/model"
	"github.com/usehiveloop/hiveloop/internal/nango"
	"github.com/usehiveloop/hiveloop/internal/resources"
	"gorm.io/gorm"
)

type InConnectionHandler struct {
	db        *gorm.DB
	nango     *nango.Client
	catalog   *catalog.Catalog
	discovery *resources.Discovery
}

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
