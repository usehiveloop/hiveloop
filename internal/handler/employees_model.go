package handler

import (
	"context"
	"fmt"
	"strings"

	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/model"
	"github.com/usehiveloop/hiveloop/internal/registry"
)

func (h *EmployeeHandler) employeeModelRegistry() *registry.Registry {
	if h != nil && h.agents != nil && h.agents.registry != nil {
		return h.agents.registry
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

	known := false
	selectable := false
	for _, provider := range reg.AllProviders() {
		mdl, ok := provider.Models[modelID]
		if !ok {
			continue
		}
		known = true
		if !mdl.Hidden {
			selectable = true
		}
	}
	if !known {
		return nil, fmt.Errorf("model %q is not in the catalog", modelID)
	}
	if !selectable {
		return nil, fmt.Errorf("model %q is not selectable", modelID)
	}

	var creds []model.Credential
	if err := db.WithContext(ctx).
		Where("is_system = ? AND revoked_at IS NULL", true).
		Order("created_at ASC").
		Find(&creds).Error; err != nil {
		return nil, fmt.Errorf("list active system credentials: %w", err)
	}

	for i := range creds {
		provider, ok := reg.GetProvider(creds[i].ProviderID)
		if !ok {
			continue
		}
		mdl, ok := provider.Models[modelID]
		if ok && !mdl.Hidden {
			return &creds[i], nil
		}
	}
	return nil, fmt.Errorf("model %q is not backed by an active system credential", modelID)
}
