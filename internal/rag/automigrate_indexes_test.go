package rag_test

import (
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/rag"
	ragmodel "github.com/usehiveloop/hiveloop/internal/rag/model"
	"github.com/usehiveloop/hiveloop/internal/rag/testhelpers"
)

// freshMigrate returns a *gorm.DB with the full RAG schema migrated.
//
// We deliberately DO NOT drop tables between tests: parallel test
// packages share the same Postgres test DB, and dropping rag_* tables
// here would race against their in-flight migrations and inserts.
// testhelpers.ConnectTestDB runs model.AutoMigrate + rag.AutoMigrate
// once per connection; if the schema were ever out of shape that
// migration would fail. Schema-shape assertions below use
// information_schema / pg_indexes and row-level assertions are scoped
// to per-test orgs.
func TestAutoMigrate_PartialIndexActuallyUsed_Document(t *testing.T) {
	db := freshMigrate(t)

	org := testhelpers.NewTestOrg(t, db)

	now := time.Now()
	past := now.Add(-1 * time.Hour)

	// Insert 100 docs with mixed sync states — only ~1/3 match the
	// partial-index predicate, which matches realistic production skew.
	for i := 0; i < 100; i++ {
		d := &ragmodel.RAGDocument{
			ID:           "partial-doc-" + uuid.NewString(),
			OrgID:        org.ID,
			SemanticID:   "Doc",
			LastModified: now,
		}
		switch i % 3 {
		case 0:
			// last_synced = NULL → matches predicate
			d.LastSynced = nil
		case 1:
			// last_synced in the past → last_modified > last_synced → matches
			d.LastSynced = &past
		case 2:
			// last_synced = now → last_modified == last_synced → misses
			d.LastSynced = &now
		}
		if err := db.Create(d).Error; err != nil {
			t.Fatalf("create doc %d: %v", i, err)
		}
	}
	if err := db.Exec("ANALYZE rag_documents").Error; err != nil {
		t.Fatalf("ANALYZE: %v", err)
	}

	plan := explainNoSeqScan(t, db, `
		EXPLAIN (FORMAT JSON)
		SELECT id FROM rag_documents
		WHERE last_modified > last_synced OR last_synced IS NULL
	`)
	if !strings.Contains(plan, "idx_rag_document_needs_sync") {
		t.Fatalf("planner did not use idx_rag_document_needs_sync; plan=%s", plan)
	}
}

func TestAutoMigrate_PartialIndexActuallyUsed_Watchdog(t *testing.T) {
	db := freshMigrate(t)

	org := testhelpers.NewTestOrg(t, db)
	user := testhelpers.NewTestUser(t, db, org.ID)
	integ := testhelpers.NewTestInIntegration(t, db, "github")
	conn := testhelpers.NewTestInConnection(t, db, org.ID, user.ID, integ.ID)
	src := testhelpers.NewTestRAGSource(t, db, org.ID, conn.ID)

	stale := time.Now().Add(-1 * time.Hour)
	fresh := time.Now()

	// 1000 rows with ~5% in-progress. At that skew the partial
	// heartbeat index (size ~= 50 entries) is noticeably smaller than
	// the full-table status index (size = 1000 entries), so the planner
	// reliably picks it once enable_seqscan is off.
	for i := 0; i < 1000; i++ {
		a := &ragmodel.RAGIndexAttempt{
			OrgID:       org.ID,
			RAGSourceID: src.ID,
			Status:      ragmodel.IndexingStatusSuccess, // majority branch, misses partial
		}
		if i%20 == 0 {
			a.Status = ragmodel.IndexingStatusInProgress
			if i%40 == 0 {
				a.LastProgressTime = &stale
			} else {
				a.LastProgressTime = &fresh
			}
		}
		if err := db.Create(a).Error; err != nil {
			t.Fatalf("create attempt %d: %v", i, err)
		}
	}
	if err := db.Exec("ANALYZE rag_index_attempts").Error; err != nil {
		t.Fatalf("ANALYZE: %v", err)
	}

	plan := explainNoSeqScan(t, db, `
		EXPLAIN (FORMAT JSON)
		SELECT id FROM rag_index_attempts
		WHERE status = 'in_progress'
		  AND last_progress_time < NOW() - INTERVAL '30 minutes'
	`)
	if !strings.Contains(plan, "idx_rag_index_attempt_heartbeat") {
		t.Fatalf("planner did not use idx_rag_index_attempt_heartbeat; plan=%s", plan)
	}
}

func TestAutoMigrate_GINIndexActuallyUsed(t *testing.T) {
	db := freshMigrate(t)

	org := testhelpers.NewTestOrg(t, db)

	for i := 0; i < 100; i++ {
		emails := pq.StringArray{}
		if i%5 == 0 {
			emails = pq.StringArray{"alice@example.com"}
		} else {
			emails = pq.StringArray{"bob-" + uuid.NewString() + "@example.com"}
		}
		d := &ragmodel.RAGDocument{
			ID:                 "gin-doc-" + uuid.NewString(),
			OrgID:              org.ID,
			SemanticID:         "Doc",
			LastModified:       time.Now(),
			ExternalUserEmails: emails,
		}
		if err := db.Create(d).Error; err != nil {
			t.Fatalf("create doc %d: %v", i, err)
		}
	}
	if err := db.Exec("ANALYZE rag_documents").Error; err != nil {
		t.Fatalf("ANALYZE: %v", err)
	}

	// The plan agent noted @> is the GIN-preferred operator; ANY(...)
	// doesn't use GIN. We use the production shape here.
	plan := explainNoSeqScan(t, db, `
		EXPLAIN (FORMAT JSON)
		SELECT id FROM rag_documents
		WHERE external_user_emails @> ARRAY['alice@example.com']::text[]
	`)
	if !strings.Contains(plan, "idx_rag_document_ext_emails") {
		t.Fatalf("planner did not use idx_rag_document_ext_emails; plan=%s", plan)
	}
}

func explainNoSeqScan(t *testing.T, db *gorm.DB, sql string) string {
	t.Helper()

	var planJSON string
	err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SET LOCAL enable_seqscan = off").Error; err != nil {
			return err
		}
		var plans []struct {
			QueryPlan string `gorm:"column:QUERY PLAN"`
		}
		if err := tx.Raw(sql).Scan(&plans).Error; err != nil {
			return err
		}
		if len(plans) == 0 {
			return nil
		}
		planJSON = plans[0].QueryPlan
		return nil
	})
	if err != nil {
		t.Fatalf("explain: %v", err)
	}
	return planJSON
}

func TestAutoMigrate_SeedsEmbeddingRegistry(t *testing.T) {
	db := freshMigrate(t)

	type row struct {
		ID        string
		Provider  string
		Dimension int
	}
	var got []row
	if err := db.Raw(`
		SELECT id, provider, dimension FROM rag_embedding_models
		ORDER BY id
	`).Scan(&got).Error; err != nil {
		t.Fatalf("query rag_embedding_models: %v", err)
	}

	expected := []row{
		{"openai:text-embedding-3-large", "openai", 3072},
		{"openai:text-embedding-3-small", "openai", 1536},
		{"siliconflow:qwen3-embedding-0.6b", "siliconflow", 1024},
		{"siliconflow:qwen3-embedding-4b", "siliconflow", 2560},
		{"siliconflow:qwen3-embedding-8b", "siliconflow", 4096},
	}

	if len(got) != len(expected) {
		t.Fatalf("seed row count: got %d, want %d\n  got=%v", len(got), len(expected), got)
	}
	for i, want := range expected {
		if got[i] != want {
			t.Errorf("row[%d]: got %+v, want %+v", i, got[i], want)
		}
	}
}

