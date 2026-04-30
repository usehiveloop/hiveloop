package credentials

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"

	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/model"
	"github.com/usehiveloop/hiveloop/internal/registry"
	subagents "github.com/usehiveloop/hiveloop/internal/sub-agents"
)

var ErrNoSystemCredential = errors.New("credentials: no system credential configured")

type Picker interface {
	Pick(ctx context.Context, providerGroup string) (*model.Credential, error)
	PickByModel(ctx context.Context, modelID string) (*model.Credential, error)
}

type PostgresPicker struct {
	db  *gorm.DB
	reg *registry.Registry
}

func NewPicker(db *gorm.DB) *PostgresPicker {
	return &PostgresPicker{db: db, reg: registry.Global()}
}

func NewPickerWithRegistry(db *gorm.DB, reg *registry.Registry) *PostgresPicker {
	return &PostgresPicker{db: db, reg: reg}
}

func (p *PostgresPicker) Pick(ctx context.Context, providerGroup string) (*model.Credential, error) {
	if providerGroup == "" {
		return nil, fmt.Errorf("credentials: Pick requires a non-empty provider group")
	}

	var all []model.Credential
	if err := p.db.WithContext(ctx).
		Where("is_system = ? AND revoked_at IS NULL", true).
		Find(&all).Error; err != nil {
		return nil, fmt.Errorf("list system credentials: %w", err)
	}

	matching := all[:0]
	for _, c := range all {
		if subagents.MapProviderToGroup(c.ProviderID, "") == providerGroup {
			matching = append(matching, c)
		}
	}
	return pickRandom(matching, fmt.Sprintf("group=%q", providerGroup))
}

func (p *PostgresPicker) PickByModel(ctx context.Context, modelID string) (*model.Credential, error) {
	if modelID == "" {
		return nil, fmt.Errorf("credentials: PickByModel requires a non-empty model id")
	}

	var all []model.Credential
	if err := p.db.WithContext(ctx).
		Where("is_system = ? AND revoked_at IS NULL", true).
		Find(&all).Error; err != nil {
		return nil, fmt.Errorf("list system credentials: %w", err)
	}

	matching := all[:0]
	for _, c := range all {
		provider, ok := p.reg.GetProvider(c.ProviderID)
		if !ok {
			continue
		}
		if _, exists := provider.Models[modelID]; exists {
			matching = append(matching, c)
		}
	}
	return pickRandom(matching, fmt.Sprintf("model=%q", modelID))
}

func pickRandom(matching []model.Credential, what string) (*model.Credential, error) {
	if len(matching) == 0 {
		return nil, fmt.Errorf("%w: %s", ErrNoSystemCredential, what)
	}
	idx := 0
	if len(matching) > 1 {
		idx = rand.IntN(len(matching)) //nolint:gosec // load balancing, not security
	}
	chosen := matching[idx]
	return &chosen, nil
}
