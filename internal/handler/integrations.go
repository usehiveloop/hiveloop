package handler

import (
	"encoding/json"
	"time"

	"github.com/usehivy/hivy/internal/mcp/catalog"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/nango"
	"gorm.io/gorm"
)

type IntegrationHandler struct {
	db      *gorm.DB
	nango   *nango.Client
	catalog *catalog.Catalog
}

func NewIntegrationHandler(db *gorm.DB, nangoClient *nango.Client, cat *catalog.Catalog) *IntegrationHandler {
	return &IntegrationHandler{db: db, nango: nangoClient, catalog: cat}
}

type createIntegrationRequest struct {
	Provider    string             `json:"provider"`
	DisplayName string             `json:"display_name"`
	Credentials *nango.Credentials `json:"credentials,omitempty"`
	Meta        model.JSON         `json:"meta,omitempty"`
}

type updateIntegrationRequest struct {
	DisplayName *string            `json:"display_name,omitempty"`
	Credentials *nango.Credentials `json:"credentials,omitempty"`
	Meta        model.JSON         `json:"meta,omitempty"`
}

type integrationResponse struct {
	ID          string             `json:"id"`
	UniqueKey   string             `json:"unique_key"`
	Provider    string             `json:"provider"`
	DisplayName string             `json:"display_name"`
	OrgID       *string            `json:"org_id,omitempty"`
	EmployeeID  *string            `json:"employee_id,omitempty"`
	CustomApp   bool               `json:"custom_app"`
	Meta        model.JSON         `json:"meta,omitempty"`
	NangoConfig *model.NangoConfig `json:"nango_config,omitempty"`
	CreatedAt   string             `json:"created_at"`
	UpdatedAt   string             `json:"updated_at"`
}

type integrationAvailableResponse struct {
	ID          string             `json:"id"`
	Provider    string             `json:"provider"`
	DisplayName string             `json:"display_name"`
	Meta        model.JSON         `json:"meta,omitempty"`
	NangoConfig *model.NangoConfig `json:"nango_config,omitempty"`
	CreatedAt   string             `json:"created_at"`
}

func parseNangoConfig(raw model.JSON) *model.NangoConfig {
	if len(raw) == 0 {
		return nil
	}
	b, err := json.Marshal(raw)
	if err != nil {
		return nil
	}
	var cfg model.NangoConfig
	if err := json.Unmarshal(b, &cfg); err != nil {
		return nil
	}
	return &cfg
}

func toIntegrationResponse(integ model.Integration) integrationResponse {
	var orgID *string
	if integ.OrgID != nil {
		s := integ.OrgID.String()
		orgID = &s
	}
	var agentID *string
	if integ.EmployeeID != nil {
		s := integ.EmployeeID.String()
		agentID = &s
	}
	return integrationResponse{
		ID:          integ.ID.String(),
		UniqueKey:   integ.UniqueKey,
		Provider:    integ.Provider,
		DisplayName: integ.DisplayName,
		OrgID:       orgID,
		EmployeeID:  agentID,
		CustomApp:   integ.CustomApp,
		Meta:        integ.Meta,
		NangoConfig: parseNangoConfig(integ.NangoConfig),
		CreatedAt:   integ.CreatedAt.Format(time.RFC3339),
		UpdatedAt:   integ.UpdatedAt.Format(time.RFC3339),
	}
}

func toIntegrationAvailableResponse(integ model.Integration) integrationAvailableResponse {
	cfg := parseNangoConfig(integ.NangoConfig)
	if cfg != nil {
		cfg.WebhookSecret = ""
		cfg.WebhookURL = ""
		cfg.WebhookRoutingScript = ""
		cfg.CredentialsSchema = nil
		cfg.WebhookUserDefinedSecret = false
	}
	return integrationAvailableResponse{
		ID:          integ.ID.String(),
		Provider:    integ.Provider,
		DisplayName: integ.DisplayName,
		Meta:        integ.Meta,
		NangoConfig: cfg,
		CreatedAt:   integ.CreatedAt.Format(time.RFC3339),
	}
}

func nangoProviderConfigKey(uniqueKey string) string {
	return uniqueKey
}

// nangoProviderName maps our internal provider key to the Nango catalog
// provider name.
func nangoProviderName(provider string) string {
	switch provider {
	case "github-app-code-reviews":
		return "github-app"
	}
	return provider
}
