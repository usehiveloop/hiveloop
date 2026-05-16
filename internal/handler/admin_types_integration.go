package handler

import (
	"time"

	"github.com/usehiveloop/hiveloop/internal/mcp/catalog"
	"github.com/usehiveloop/hiveloop/internal/model"
	"github.com/usehiveloop/hiveloop/internal/nango"
)

type adminInIntegrationResponse struct {
	ID              string                             `json:"id"`
	UniqueKey       string                             `json:"unique_key"`
	Provider        string                             `json:"provider"`
	DisplayName     string                             `json:"display_name"`
	Meta            model.JSON                         `json:"meta,omitempty"`
	NangoConfig     model.JSON                         `json:"nango_config,omitempty"`
	EmployeeProfile *catalog.EmployeeProfileCapability `json:"employee_profile,omitempty"`
	CreatedAt       string                             `json:"created_at"`
	UpdatedAt       string                             `json:"updated_at"`
}

func toAdminInIntegrationResponse(i model.InIntegration) adminInIntegrationResponse {
	return adminInIntegrationResponse{
		ID:              i.ID.String(),
		UniqueKey:       i.UniqueKey,
		Provider:        i.Provider,
		DisplayName:     i.DisplayName,
		Meta:            i.Meta,
		NangoConfig:     i.NangoConfig,
		EmployeeProfile: integrationEmployeeProfileCapability(i.Provider),
		CreatedAt:       i.CreatedAt.Format(time.RFC3339),
		UpdatedAt:       i.UpdatedAt.Format(time.RFC3339),
	}
}

type adminCreateInIntegrationRequest struct {
	Provider    string             `json:"provider"`
	DisplayName string             `json:"display_name"`
	Credentials *nango.Credentials `json:"credentials,omitempty"`
	Meta        model.JSON         `json:"meta,omitempty"`
}

type adminUpdateInIntegrationRequest struct {
	DisplayName *string            `json:"display_name,omitempty"`
	Credentials *nango.Credentials `json:"credentials,omitempty"`
	Meta        model.JSON         `json:"meta,omitempty"`
}
