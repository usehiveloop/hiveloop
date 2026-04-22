package embedder

import (
	"fmt"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	ragmodel "github.com/usehiveloop/hiveloop/internal/rag/model"
)

// SeedRegistry upserts every entry from Registry() into the
// rag_embedding_models table. Safe to call on every boot — idempotent
// by design via ON CONFLICT (id) DO UPDATE.
//
// The Go-side registry is the source of truth: editing a RegistryEntry
// and redeploying will overwrite the DB row's mutable columns
// (ModelName, Dimension, pricing, prefixes, IsActive, DatasetName) on
// the next seed. CreatedAt is preserved on update; UpdatedAt is bumped
// by the database default / gorm on write.
//
// Rows that exist in the DB but are not in Registry() are left alone,
// not deleted: historical FK references from RAGIndexAttempt to
// retired models must still resolve. Flip IsActive=false in Registry()
// to retire a model from the admin UI.
func SeedRegistry(db *gorm.DB) error {
	return seedFromEntries(db, Registry())
}

// seedFromEntries is the injectable core used by tests that need to
// seed with a modified entry list (e.g. to verify that a price change
// in Registry() propagates to the DB on the next seed). Production
// code always goes through SeedRegistry.
func seedFromEntries(db *gorm.DB, entries []RegistryEntry) error {
	if len(entries) == 0 {
		return nil
	}

	rows := make([]ragmodel.RAGEmbeddingModel, 0, len(entries))
	for _, e := range entries {
		rows = append(rows, ragmodel.RAGEmbeddingModel{
			ID:                    e.ID,
			Provider:              e.Provider,
			ModelName:             e.ModelName,
			Dimension:             e.Dimension,
			MaxInputTokens:        e.MaxInputTokens,
			DatasetName:           e.DatasetName,
			QueryPrefix:           e.QueryPrefix,
			PassagePrefix:         e.PassagePrefix,
			PricingPer1MTokensUSD: e.PricingPer1MTokensUSD,
			IsActive:              e.IsActive,
		})
	}

	// ON CONFLICT (id) DO UPDATE every mutable column. created_at stays
	// put because it is not listed in DoUpdates; updated_at is refreshed
	// by gorm's Updates/Create hook on write.
	err := db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"provider",
			"model_name",
			"dimension",
			"max_input_tokens",
			"dataset_name",
			"query_prefix",
			"passage_prefix",
			"pricing_per_1m_tokens_usd",
			"is_active",
			"updated_at",
		}),
	}).Create(&rows).Error
	if err != nil {
		return fmt.Errorf("seed rag_embedding_models: %w", err)
	}
	return nil
}
