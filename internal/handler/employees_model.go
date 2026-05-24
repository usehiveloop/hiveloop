package handler

import (
	"context"
	"fmt"
	"strings"

	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/registry"
)

func (h *EmployeeHandler) employeeModelRegistry() *registry.Registry {
	if h != nil && h.registry != nil {
		return h.registry
	}
	return registry.Global()
}

func pickActiveSystemCredentialForModel(ctx context.Context, db *gorm.DB, reg *registry.Registry, modelID string) (*model.Credential, error) {
	modelID = strings.TrimSpace(modelID)
	if modelID == "" {
		return nil, fmt.Errorf("model is required")
	}
	if reg == nil {
		reg = registry.Global()
	}

	if err := reg.ValidateCanonicalModel(modelID); err != nil {
		return nil, err
	}

	var creds []model.Credential
	if err := db.WithContext(ctx).
		Where("is_system = ? AND revoked_at IS NULL", true).
		Order("created_at ASC").
		Find(&creds).Error; err != nil {
		return nil, fmt.Errorf("list active system credentials: %w", err)
	}

	for i := range creds {
		if _, ok := reg.ResolveModel(creds[i].ProviderID, modelID); ok {
			return &creds[i], nil
		}
	}
	return nil, fmt.Errorf("model %q is not backed by an active system credential", modelID)
}
