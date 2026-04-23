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

// seedEmbeddingModelID ensures a row exists in rag_embedding_models
// for the given ID so search_settings inserts that reference it
// survive the FK constraint. Idempotent (ON CONFLICT DO NOTHING).
func seedEmbeddingModelID(t *testing.T, db *gorm.DB, id string) {
	t.Helper()

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
// invariant: org_id is the primary key, so a second insert for the
// same org must fail with 23505.
func TestRAGSearchSettings_OrgPK(t *testing.T) {
	db := testhelpers.ConnectTestDB(t)

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

// TestRAGSearchSettings_EmbeddingModelFK exercises the FK from
// rag_search_settings.embedding_model_id → rag_embedding_models.id.
// Inserts with a non-existent model id must fail with 23503.
func TestRAGSearchSettings_EmbeddingModelFK(t *testing.T) {
	db := testhelpers.ConnectTestDB(t)

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
