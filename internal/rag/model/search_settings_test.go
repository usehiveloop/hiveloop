package model_test

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"

	ragmodel "github.com/usehiveloop/hiveloop/internal/rag/model"
	"github.com/usehiveloop/hiveloop/internal/rag/testhelpers"
)

// isFKViolation pins on Postgres error code 23503 so we don't
// false-positive on unique violations or NOT NULL errors.
func isFKViolation(err error) bool {
	var pg *pgconn.PgError
	if errors.As(err, &pg) {
		return pg.Code == "23503"
	}
	return false
}

// seedEmbeddingModelID ensures a row exists in rag_embedding_models for
// the given ID so search_settings inserts that reference it survive the
// FK constraint. Uses raw SQL so we don't take a compile-time dep on
// Tranche 1G's Go struct. Idempotent (ON CONFLICT DO NOTHING).
//
// Returns a cleanup func; tests that created rows referencing this seed
// must run before the cleanup, or restrict to rows they created.
func seedEmbeddingModelID(t *testing.T, db *gorm.DB, id string) {
	t.Helper()

	// Skip the seed entirely if rag_embedding_models doesn't exist
	// yet (shouldn't happen once 1F wires everything, but keeps this
	// helper useful if 1C's tests ever run in true isolation).
	var tableCount int64
	if err := db.Raw(`
		SELECT COUNT(*) FROM information_schema.tables
		WHERE table_schema = current_schema() AND table_name = 'rag_embedding_models'
	`).Scan(&tableCount).Error; err != nil {
		t.Fatalf("probe rag_embedding_models: %v", err)
	}
	if tableCount == 0 {
		t.Skip("blocked by tranche 1F/1G ordering — rag_embedding_models table is not migrated in this environment. Tests that reference embedding_model_id cannot run until 1F orders 1G before 1C in rag.AutoMigrate.")
	}

	// Raw insert so we don't depend on 1G's Go struct shape. The
	// columns here must stay in sync with 1G's migration; 1F should
	// lock both in. Required columns match 1G plan: id, provider,
	// model_name, dimension, max_input_tokens, dataset_name,
	// is_active, created_at, updated_at.
	err := db.Exec(`
		INSERT INTO rag_embedding_models
		  (id, provider, model_name, dimension, max_input_tokens,
		   dataset_name, pricing_per_1m_tokens_usd, is_active,
		   created_at, updated_at)
		VALUES (?, 'siliconflow', 'test/model', 2560, 8192,
		  'rag_chunks_test', 0, true, NOW(), NOW())
		ON CONFLICT (id) DO NOTHING
	`, id).Error
	if err != nil {
		t.Fatalf("seed rag_embedding_models row %q: %v", id, err)
	}
	t.Cleanup(func() {
		db.Exec(`DELETE FROM rag_embedding_models WHERE id = ?`, id)
	})
}

// TestRAGSearchSettings_OrgPK pins the one-settings-row-per-org
// invariant. Per plan §1C DEVIATION (1), the PK IS org_id — a second
// insert for the same org must fail with 23505.
func TestRAGSearchSettings_OrgPK(t *testing.T) {
	db := testhelpers.ConnectTestDB(t)
	migrate1C(t, db)

	const modelID = "test:pk-model-4b"
	seedEmbeddingModelID(t, db, modelID)

	org := testhelpers.NewTestOrg(t, db)

	t.Cleanup(func() {
		db.Where("org_id = ?", org.ID).Delete(&ragmodel.RAGSearchSettings{})
	})

	first := &ragmodel.RAGSearchSettings{
		OrgID:              org.ID,
		EmbeddingModelID:   modelID,
		EmbeddingDim:       2560,
		IndexName:          "rag_chunks__test_pk_model__2560",
		EmbeddingPrecision: ragmodel.EmbeddingPrecisionFloat,
	}
	if err := db.Create(first).Error; err != nil {
		t.Fatalf("first insert should succeed: %v", err)
	}

	second := &ragmodel.RAGSearchSettings{
		OrgID:              org.ID,
		EmbeddingModelID:   modelID,
		EmbeddingDim:       2560,
		IndexName:          "rag_chunks__test_pk_model__2560",
		EmbeddingPrecision: ragmodel.EmbeddingPrecisionFloat,
	}
	err := db.Create(second).Error
	if err == nil {
		t.Fatal("second insert for same org_id must violate PK constraint")
	}
	var pg *pgconn.PgError
	if !errors.As(err, &pg) || pg.Code != "23505" {
		t.Fatalf("expected 23505 unique_violation (PK), got: %v", err)
	}
}

// TestRAGSearchSettings_EmbeddingModelFK exercises the cross-tranche
// FK declared by AutoMigrate1C (only if 1G's table exists at
// migrate time). If 1G hasn't migrated yet, the FK isn't installed and
// this test skips with an ordering note for 1F.
func TestRAGSearchSettings_EmbeddingModelFK(t *testing.T) {
	db := testhelpers.ConnectTestDB(t)
	migrate1C(t, db)

	// Check whether the FK was installed. If not, 1G hasn't migrated.
	var fkCount int64
	if err := db.Raw(`
		SELECT COUNT(*) FROM information_schema.table_constraints
		WHERE table_name = 'rag_search_settings'
		  AND constraint_name = 'fk_rag_search_settings_embedding_model'
		  AND constraint_type = 'FOREIGN KEY'
	`).Scan(&fkCount).Error; err != nil {
		t.Fatalf("introspect FK: %v", err)
	}
	if fkCount == 0 {
		t.Skip("blocked by tranche 1F/1G ordering — rag_embedding_models table not migrated, so fk_rag_search_settings_embedding_model was not installed. 1F must run 1G's AutoMigrate before 1C's; once that happens the FK will exist and this test exercises the 23503 path.")
	}

	org := testhelpers.NewTestOrg(t, db)

	t.Cleanup(func() {
		db.Where("org_id = ?", org.ID).Delete(&ragmodel.RAGSearchSettings{})
	})

	settings := &ragmodel.RAGSearchSettings{
		OrgID:              org.ID,
		EmbeddingModelID:   "nonexistent:model-id-xyz",
		EmbeddingDim:       2560,
		IndexName:          "rag_chunks_test",
		EmbeddingPrecision: ragmodel.EmbeddingPrecisionFloat,
	}

	err := db.Create(settings).Error
	if err == nil {
		t.Fatal("insert with non-existent embedding_model_id must violate FK constraint")
	}
	if !isFKViolation(err) {
		t.Fatalf("expected 23503 foreign_key_violation, got: %v", err)
	}
}
