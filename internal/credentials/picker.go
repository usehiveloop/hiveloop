package credentials

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"

	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/providergroups"
	"github.com/usehivy/hivy/internal/registry"
)

var ErrNoSystemCredential = errors.New("credentials: no system credential configured")

type Picker interface {
	Pick(ctx context.Context, providerGroup string) (*model.Credential, error)
	PickByModel(ctx context.Context, canonicalModelID string) (*model.Credential, error)
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
		Where("org_id IS NULL AND revoked_at IS NULL").
		Find(&all).Error; err != nil {
		return nil, fmt.Errorf("list system credentials: %w", err)
	}

	matching := all[:0]
	for _, c := range all {
		if providergroups.MapProviderToGroup(c.ProviderID, "") == providerGroup {
			matching = append(matching, c)
		}
	}
	return pickRandom(matching, fmt.Sprintf("group=%q", providerGroup))
}

func (p *PostgresPicker) PickByModel(ctx context.Context, canonicalModelID string) (*model.Credential, error) {
	if canonicalModelID == "" {
		return nil, fmt.Errorf("credentials: PickByModel requires a non-empty model id")
	}
	reg := p.reg
	if reg == nil {
		reg = registry.Global()
	}

	var all []model.Credential
	if err := p.db.WithContext(ctx).
		Where("org_id IS NULL AND revoked_at IS NULL").
		Order("created_at ASC").
		Find(&all).Error; err != nil {
		return nil, fmt.Errorf("list system credentials: %w", err)
	}

	for _, providerID := range reg.ProviderPreferenceForModel(canonicalModelID) {
		for _, c := range all {
			if c.ProviderID == providerID {
				chosen := c
				return &chosen, nil
			}
		}
	}
	return nil, fmt.Errorf("%w: model=%q", ErrNoSystemCredential, canonicalModelID)
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
