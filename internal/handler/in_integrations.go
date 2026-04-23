package handler

import (
	"encoding/json"
	"time"

	"github.com/usehiveloop/hiveloop/internal/mcp/catalog"
	"github.com/usehiveloop/hiveloop/internal/model"
	"github.com/usehiveloop/hiveloop/internal/nango"
	"gorm.io/gorm"
)

type InIntegrationHandler struct {
	db      *gorm.DB
	nango   *nango.Client
	catalog *catalog.Catalog
}

func NewInIntegrationHandler(db *gorm.DB, nangoClient *nango.Client, cat *catalog.Catalog) *InIntegrationHandler {
	return &InIntegrationHandler{db: db, nango: nangoClient, catalog: cat}
}

type createInIntegrationRequest struct {
	Provider    string             `json:"provider"`
	DisplayName string             `json:"display_name"`
	Credentials *nango.Credentials `json:"credentials,omitempty"`
	Meta        model.JSON         `json:"meta,omitempty"`
}

type updateInIntegrationRequest struct {
	DisplayName *string            `json:"display_name,omitempty"`
	Credentials *nango.Credentials `json:"credentials,omitempty"`
	Meta        model.JSON         `json:"meta,omitempty"`
}

type inIntegrationResponse struct {
	ID          string             `json:"id"`
	UniqueKey   string             `json:"unique_key"`
	Provider    string             `json:"provider"`
	DisplayName string             `json:"display_name"`
	Meta        model.JSON         `json:"meta,omitempty"`
	NangoConfig *model.NangoConfig `json:"nango_config,omitempty"`
	CreatedAt   string             `json:"created_at"`
	UpdatedAt   string             `json:"updated_at"`
}

type inIntegrationAvailableResponse struct {
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

func toInIntegrationResponse(integ model.InIntegration) inIntegrationResponse {
	return inIntegrationResponse{
		ID:          integ.ID.String(),
		UniqueKey:   integ.UniqueKey,
		Provider:    integ.Provider,
		DisplayName: integ.DisplayName,
		Meta:        integ.Meta,
		NangoConfig: parseNangoConfig(integ.NangoConfig),
		CreatedAt:   integ.CreatedAt.Format(time.RFC3339),
		UpdatedAt:   integ.UpdatedAt.Format(time.RFC3339),
	}
}

func toInIntegrationAvailableResponse(integ model.InIntegration) inIntegrationAvailableResponse {
	cfg := parseNangoConfig(integ.NangoConfig)
	if cfg != nil {
		cfg.WebhookSecret = ""
		cfg.WebhookURL = ""
		cfg.WebhookRoutingScript = ""
		cfg.CredentialsSchema = nil
		cfg.WebhookUserDefinedSecret = false
	}
	return inIntegrationAvailableResponse{
		ID:          integ.ID.String(),
		Provider:    integ.Provider,
		DisplayName: integ.DisplayName,
		Meta:        integ.Meta,
		NangoConfig: cfg,
		CreatedAt:   integ.CreatedAt.Format(time.RFC3339),
	}
}

func inNangoKey(uniqueKey string) string {
	return "in_" + uniqueKey
}

// nangoConnectionBelongsToUser inspects a Nango GetConnection response and
// returns true if its end_user.id matches the expected authenticated user.
// When creating a connect session we supply `end_user.id = user.ID.String()`,
// and Nango persists that identifier against the resulting connection. A
// connection whose end_user does not match the caller must be treated as
// foreign and rejected.
//
// Nango's response shape for GET /connection/{id} nests data under either
// "data" or is returned at the top level depending on version; we accept both.
func nangoConnectionBelongsToUser(resp map[string]any, expectedUserID string) bool {
	if resp == nil || expectedUserID == "" {
		return false
	}
	candidates := []map[string]any{resp}
	if inner, ok := resp["data"].(map[string]any); ok {
		candidates = append(candidates, inner)
	}
	if inner, ok := resp["connection"].(map[string]any); ok {
		candidates = append(candidates, inner)
	}
	for _, c := range candidates {
		if eu, ok := c["end_user"].(map[string]any); ok {
			if id, _ := eu["id"].(string); id != "" && id == expectedUserID {
				return true
			}
		}
		// Some Nango responses surface end_user_id as a top-level string.
		if id, ok := c["end_user_id"].(string); ok && id != "" && id == expectedUserID {
			return true
		}
	}
	return false
}
