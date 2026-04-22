package embedder_test

import (
	"sort"
	"testing"

	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/rag/embedder"
	ragmodel "github.com/usehiveloop/hiveloop/internal/rag/model"
	"github.com/usehiveloop/hiveloop/internal/rag/testhelpers"
)

// migrate1GAndCleanup stands up the rag_embedding_models table via the
// 1G migrator and registers a t.Cleanup that truncates it. The catalog
// is global (no org_id), so cleanup is a full TRUNCATE — isolation is
// per-test. We run seeding fresh each test so ordering cannot bleed
// state between cases.
func migrate1GAndCleanup(t *testing.T, db *gorm.DB) {
	t.Helper()
	if err := embedder.AutoMigrate1G(db); err != nil {
		t.Fatalf("AutoMigrate1G: %v", err)
	}
	t.Cleanup(func() {
		// DELETE (not TRUNCATE): the shared test Postgres may have
		// rag_search_settings / rag_index_attempts tables left over
		// from parallel tranches 1B/1C that FK-reference this table,
		// so TRUNCATE fails with 0A000. DELETE works because FKs from
		// those tables point AT us; as long as no row there references
		// a catalog row we're deleting, this succeeds.
		if err := db.Exec(`DELETE FROM rag_embedding_models`).Error; err != nil {
			t.Fatalf("delete from rag_embedding_models: %v", err)
		}
	})
	// Wipe any rows left over from a prior run BEFORE each test seeds,
	// so count assertions are deterministic.
	if err := db.Exec(`DELETE FROM rag_embedding_models`).Error; err != nil {
		t.Fatalf("pre-test delete: %v", err)
	}
}

func TestSeedRegistry_SeedsAllModels(t *testing.T) {
	db := testhelpers.ConnectTestDB(t)
	migrate1GAndCleanup(t, db)

	if err := embedder.SeedRegistry(db); err != nil {
		t.Fatalf("SeedRegistry: %v", err)
	}

	var ids []string
	if err := db.Raw(`SELECT id FROM rag_embedding_models ORDER BY id`).Scan(&ids).Error; err != nil {
		t.Fatalf("select ids: %v", err)
	}

	want := []string{
		"openai:text-embedding-3-large",
		"openai:text-embedding-3-small",
		"siliconflow:qwen3-embedding-0.6b",
		"siliconflow:qwen3-embedding-4b",
		"siliconflow:qwen3-embedding-8b",
	}
	sort.Strings(want)

	if len(ids) != len(want) {
		t.Fatalf("got %d rows, want %d: %v", len(ids), len(want), ids)
	}
	for i := range want {
		if ids[i] != want[i] {
			t.Fatalf("row %d: got %q, want %q", i, ids[i], want[i])
		}
	}
}

func TestSeedRegistry_Idempotent(t *testing.T) {
	db := testhelpers.ConnectTestDB(t)
	migrate1GAndCleanup(t, db)

	for i := 0; i < 2; i++ {
		if err := embedder.SeedRegistry(db); err != nil {
			t.Fatalf("SeedRegistry pass %d: %v", i, err)
		}
	}

	var count int64
	if err := db.Table("rag_embedding_models").Count(&count).Error; err != nil {
		t.Fatalf("count: %v", err)
	}

	// 5 = len(Registry()); re-running the seeder must not duplicate.
	if count != 5 {
		t.Fatalf("after two seed passes got %d rows, want 5", count)
	}
}

func TestSeedRegistry_UpdatesOnRegistryChange(t *testing.T) {
	db := testhelpers.ConnectTestDB(t)
	migrate1GAndCleanup(t, db)

	// Initial seed — captures whatever Registry() says right now.
	if err := embedder.SeedRegistry(db); err != nil {
		t.Fatalf("initial SeedRegistry: %v", err)
	}

	const targetID = "siliconflow:qwen3-embedding-4b"

	var before ragmodel.RAGEmbeddingModel
	if err := db.Where("id = ?", targetID).First(&before).Error; err != nil {
		t.Fatalf("fetch %s before mutation: %v", targetID, err)
	}

	// Deliberately distinct from any real price so the assertion is
	// unambiguous: this value cannot appear by accident.
	const mutatedPrice = 99.9125
	if before.PricingPer1MTokensUSD == mutatedPrice {
		t.Fatalf("test design error: mutated price collides with Registry() price")
	}

	// Build a local registry slice with the target entry's price
	// mutated. We pass this through the internal seedFromEntries
	// helper (exposed via seed_export_test.go below) — no global
	// state is mutated, so the test is safe under parallel execution.
	entries := embedder.Registry()
	var mutatedIdx = -1
	for i := range entries {
		if entries[i].ID == targetID {
			entries[i].PricingPer1MTokensUSD = mutatedPrice
			mutatedIdx = i
			break
		}
	}
	if mutatedIdx < 0 {
		t.Fatalf("registry does not contain %s — test needs updating", targetID)
	}

	if err := embedder.SeedFromEntriesForTest(db, entries); err != nil {
		t.Fatalf("re-seed with mutated entries: %v", err)
	}

	var after ragmodel.RAGEmbeddingModel
	if err := db.Where("id = ?", targetID).First(&after).Error; err != nil {
		t.Fatalf("fetch %s after mutation: %v", targetID, err)
	}

	if after.PricingPer1MTokensUSD != mutatedPrice {
		t.Fatalf("registry-as-source-of-truth violated: got pricing %v, want %v",
			after.PricingPer1MTokensUSD, mutatedPrice)
	}

	// Row count must still be 5 — a re-seed must not insert a duplicate
	// row under a different synthetic id.
	var count int64
	if err := db.Table("rag_embedding_models").Count(&count).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 5 {
		t.Fatalf("after update-seed got %d rows, want 5", count)
	}
}

func TestSeedFromEntries_EmptySliceIsNoOp(t *testing.T) {
	db := testhelpers.ConnectTestDB(t)
	migrate1GAndCleanup(t, db)

	// Seed so there's something in the table.
	if err := embedder.SeedRegistry(db); err != nil {
		t.Fatalf("SeedRegistry: %v", err)
	}

	// Re-seed with an empty slice: must succeed and must not touch rows.
	if err := embedder.SeedFromEntriesForTest(db, nil); err != nil {
		t.Fatalf("empty seed: %v", err)
	}

	var count int64
	if err := db.Table("rag_embedding_models").Count(&count).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 5 {
		t.Fatalf("empty seed disturbed catalog: got %d rows, want 5", count)
	}
}

// closedDB returns a *gorm.DB whose underlying *sql.DB has been closed,
// so every subsequent query errors out. Used to exercise error-return
// branches in seedFromEntries / AutoMigrate1G without racing against a
// real DB.
func closedDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := testhelpers.ConnectTestDB(t)
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("db.DB: %v", err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	return db
}

func TestSeedFromEntries_PropagatesDBError(t *testing.T) {
	// Closing the underlying sql.DB forces every later query to return
	// "sql: database is closed". This is the only way to exercise the
	// error-wrapping branch of seedFromEntries without a mock.
	db := closedDB(t)
	err := embedder.SeedFromEntriesForTest(db, embedder.Registry())
	if err == nil {
		t.Fatal("expected error from closed DB, got nil")
	}
}

func TestAutoMigrate1G_PropagatesDBError(t *testing.T) {
	// Same pattern as above — exercises the early-return branch when
	// gorm.AutoMigrate fails.
	db := closedDB(t)
	if err := embedder.AutoMigrate1G(db); err == nil {
		t.Fatal("expected error from closed DB, got nil")
	}
}

func TestDeriveDatasetName(t *testing.T) {
	cases := []struct {
		name      string
		provider  string
		modelName string
		dim       int
		want      string
	}{
		{
			name:      "siliconflow qwen3 4b — the default model",
			provider:  "siliconflow",
			modelName: "Qwen/Qwen3-Embedding-4B",
			dim:       2560,
			want:      "rag_chunks__siliconflow_qwen3_embedding_4b__2560",
		},
		{
			name:      "openai text-embedding-3-small",
			provider:  "openai",
			modelName: "text-embedding-3-small",
			dim:       1536,
			want:      "rag_chunks__openai_text_embedding_3_small__1536",
		},
		{
			name:      "uppercase provider lowercased",
			provider:  "SiliconFlow",
			modelName: "Qwen/Qwen3-Embedding-8B",
			dim:       4096,
			want:      "rag_chunks__siliconflow_qwen3_embedding_8b__4096",
		},
		{
			name:      "org namespace stripped; underscores and dots preserved",
			provider:  "custom",
			modelName: "BAAI/bge_m3.v2",
			dim:       1024,
			// BAAI/ is stripped as the org prefix; bge_m3.v2 passes
			// through because only `-` is replaced, not `_` or `.`.
			want: "rag_chunks__custom_bge_m3.v2__1024",
		},
		{
			name:      "no slash: modelname used verbatim after lowercase+dash-normalize",
			provider:  "test",
			modelName: "Nomic-Embed-Text",
			dim:       768,
			want:      "rag_chunks__test_nomic_embed_text__768",
		},
		{
			name:      "no slash in model name",
			provider:  "openai",
			modelName: "text-embedding-3-large",
			dim:       3072,
			want:      "rag_chunks__openai_text_embedding_3_large__3072",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := embedder.DeriveDatasetNameForTest(tc.provider, tc.modelName, tc.dim)
			if got != tc.want {
				t.Fatalf("deriveDatasetName(%q, %q, %d): got %q, want %q",
					tc.provider, tc.modelName, tc.dim, got, tc.want)
			}
		})
	}
}
