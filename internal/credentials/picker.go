package credentials

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"

	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/model"
	subagents "github.com/usehiveloop/hiveloop/internal/sub-agents"
)

// ErrNoSystemCredential is returned when no system credential is configured
// for the requested provider group.
var ErrNoSystemCredential = errors.New("credentials: no system credential configured")

// Picker selects a system credential for a given provider group at runtime.
// Implementations must be safe for concurrent use.
type Picker interface {
	// Pick returns a system credential matching the given provider group
	// ("anthropic", "openai", "kimi", ...). When multiple credentials are
	// configured for the same group, the picker chooses one; callers must
	// not assume a specific credential is returned across calls.
	Pick(ctx context.Context, providerGroup string) (*model.Credential, error)
}

// PostgresPicker is the production Picker backed by the credentials table.
// Selection strategy: uniform-random across non-revoked system credentials
// whose provider_id maps to the requested group.
type PostgresPicker struct {
	db *gorm.DB
}

// NewPicker returns a Postgres-backed picker.
func NewPicker(db *gorm.DB) *PostgresPicker {
	return &PostgresPicker{db: db}
}

// Pick implements Picker.
//
// We load all system credentials and filter by provider group in Go rather
// than in SQL because the provider_id -> provider_group mapping lives in
// sub-agents.MapProviderToGroup and would duplicate logic if reimplemented
// as a SQL case-expression. The number of system credentials is expected
// to stay small (< 20), so the extra rows don't matter.
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
	if len(matching) == 0 {
		return nil, fmt.Errorf("%w: group=%q", ErrNoSystemCredential, providerGroup)
	}

	idx := 0
	if len(matching) > 1 {
		// Load-balancing pick, not a security boundary — every candidate is
		// an equivalent valid credential. math/rand/v2 is appropriate.
		idx = rand.IntN(len(matching)) //nolint:gosec // load balancing, not security
	}
	chosen := matching[idx]
	return &chosen, nil
}
