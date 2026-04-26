package model_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	coremodel "github.com/usehiveloop/hiveloop/internal/model"
	"github.com/usehiveloop/hiveloop/internal/rag"
	ragmodel "github.com/usehiveloop/hiveloop/internal/rag/model"
	"github.com/usehiveloop/hiveloop/internal/rag/testhelpers"
)

func TestRAGSource_NeedsIngestPartialIndex(t *testing.T) {
	db := testhelpers.ConnectTestDB(t)

	var indexdef string
	if err := db.Raw(
		`SELECT indexdef FROM pg_indexes WHERE indexname = 'idx_rag_sources_needs_ingest'`,
	).Scan(&indexdef).Error; err != nil {
		t.Fatalf("pg_indexes lookup: %v", err)
	}
	if indexdef == "" {
		t.Fatal("idx_rag_sources_needs_ingest not found in pg_indexes")
	}
	lower := strings.ToLower(indexdef)
	if !strings.Contains(lower, "enabled = true") ||
		!strings.Contains(lower, "'active'") ||
		!strings.Contains(lower, "'initial_indexing'") {
		t.Fatalf("idx_rag_sources_needs_ingest has wrong WHERE clause: %s", indexdef)
	}

	org := testhelpers.NewTestOrg(t, db)
	user := testhelpers.NewTestUser(t, db, org.ID)
	integ := testhelpers.NewTestInIntegration(t, db, "github")

	statuses := []ragmodel.RAGSourceStatus{
		ragmodel.RAGSourceStatusActive,
		ragmodel.RAGSourceStatusInitialIndexing,
		ragmodel.RAGSourceStatusPaused,
		ragmodel.RAGSourceStatusError,
		ragmodel.RAGSourceStatusDeleting,
		ragmodel.RAGSourceStatusDisconnected,
	}
	for i := 0; i < 120; i++ {
		conn := testhelpers.NewTestInConnection(t, db, org.ID, user.ID, integ.ID)
		connID := conn.ID
		enabled := i%7 != 0
		s := &ragmodel.RAGSource{
			OrgIDValue:     org.ID,
			KindValue:      ragmodel.RAGSourceKindIntegration,
			Name:           "src-" + uuid.NewString(),
			Status:         statuses[i%len(statuses)],
			Enabled:        enabled,
			InConnectionID: &connID,
			AccessType:     ragmodel.AccessTypePrivate,
		}
		if err := db.Create(s).Error; err != nil {
			t.Fatalf("create source %d: %v", i, err)
		}
	}
	if err := db.Exec("ANALYZE rag_sources").Error; err != nil {
		t.Fatalf("ANALYZE: %v", err)
	}

	// Drop the competing composite index + seq scan inside a tx so the
	// planner has only the partial index available; the rollback restores it.
	var plan string
	err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`SET LOCAL enable_seqscan = off`).Error; err != nil {
			return err
		}
		if err := tx.Exec(`DROP INDEX IF EXISTS idx_rag_sources_org_status`).Error; err != nil {
			return err
		}
		var plans []struct {
			QueryPlan string `gorm:"column:QUERY PLAN"`
		}
		if err := tx.Raw(`EXPLAIN (FORMAT JSON)
			SELECT id FROM rag_sources
			WHERE org_id = ?
			  AND enabled = true
			  AND status IN ('ACTIVE','INITIAL_INDEXING')`, org.ID).
			Scan(&plans).Error; err != nil {
			return err
		}
		if len(plans) > 0 {
			plan = plans[0].QueryPlan
		}
		return errRollback
	})
	if !errors.Is(err, errRollback) {
		t.Fatalf("tx: %v", err)
	}
	if !strings.Contains(plan, "idx_rag_sources_needs_ingest") {
		t.Fatalf("planner did not use idx_rag_sources_needs_ingest; plan=%s", plan)
	}
}

var errRollback = errors.New("intentional rollback")

// Validates refresh frequencies cannot be set below 60s — protects
// upstream APIs from a misconfigured cadence.
func TestRAGSource_ValidateRefreshFreq(t *testing.T) {
	intPtr := func(n int) *int { return &n }

	cases := []struct {
		name    string
		refresh *int
		wantErr error
	}{
		{"nil is ok", nil, nil},
		{"59s rejected", intPtr(59), ragmodel.ErrSourceRefreshFreqTooSmall},
		{"60s ok", intPtr(60), nil},
		{"3600s ok", intPtr(3600), nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := &ragmodel.RAGSource{RefreshFreqSeconds: tc.refresh}
			got := s.ValidateRefreshFreq()
			if !errors.Is(got, tc.wantErr) {
				t.Fatalf("ValidateRefreshFreq(%v) = %v, want %v", tc.refresh, got, tc.wantErr)
			}
		})
	}
}

// Validates prune frequencies cannot be set below 300s.
func TestRAGSource_ValidatePruneFreq(t *testing.T) {
	intPtr := func(n int) *int { return &n }

	cases := []struct {
		name    string
		prune   *int
		wantErr error
	}{
		{"nil is ok", nil, nil},
		{"299s rejected", intPtr(299), ragmodel.ErrSourcePruneFreqTooSmall},
		{"300s ok", intPtr(300), nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := &ragmodel.RAGSource{PruneFreqSeconds: tc.prune}
			got := s.ValidatePruneFreq()
			if !errors.Is(got, tc.wantErr) {
				t.Fatalf("ValidatePruneFreq(%v) = %v, want %v", tc.prune, got, tc.wantErr)
			}
		})
	}
}

func TestRAGSourceStatus_IsActive(t *testing.T) {
	cases := []struct {
		status ragmodel.RAGSourceStatus
		want   bool
	}{
		{ragmodel.RAGSourceStatusActive, true},
		{ragmodel.RAGSourceStatusInitialIndexing, true},
		{ragmodel.RAGSourceStatusPaused, false},
		{ragmodel.RAGSourceStatusDeleting, false},
		{ragmodel.RAGSourceStatusError, false},
		{ragmodel.RAGSourceStatusDisconnected, false},
	}
	for _, tc := range cases {
		t.Run(string(tc.status), func(t *testing.T) {
			if got := tc.status.IsActive(); got != tc.want {
				t.Fatalf("%s.IsActive() = %v, want %v", tc.status, got, tc.want)
			}
		})
	}
}

func TestRAGSourceKind_IsValid(t *testing.T) {
	cases := []struct {
		kind ragmodel.RAGSourceKind
		want bool
	}{
		{ragmodel.RAGSourceKindIntegration, true},
		{ragmodel.RAGSourceKindWebsite, true},
		{ragmodel.RAGSourceKindFileUpload, true},
		{ragmodel.RAGSourceKind("random"), false},
		{ragmodel.RAGSourceKind(""), false},
		{ragmodel.RAGSourceKind("integration"), false},
	}
	for _, tc := range cases {
		t.Run(string(tc.kind), func(t *testing.T) {
			if got := tc.kind.IsValid(); got != tc.want {
				t.Fatalf("%s.IsValid() = %v, want %v", tc.kind, got, tc.want)
			}
		})
	}
}

// Verifies the migration's seed flips supports_rag_source=true on a
// curated provider allowlist; without it the admin UI can't list
// addable RAG sources.
func TestRAGSource_SeedsSupportsRAGSourceForKnownIntegrations(t *testing.T) {
	db := testhelpers.ConnectTestDB(t)

	seedUnique := "github-seed-" + uuid.NewString()
	integ := &coremodel.InIntegration{
		UniqueKey:   seedUnique,
		Provider:    "github",
		DisplayName: "GitHub (seed test)",
	}
	if err := db.Create(integ).Error; err != nil {
		t.Fatalf("insert integration: %v", err)
	}
	t.Cleanup(func() {
		db.Where("id = ?", integ.ID).Delete(&coremodel.InIntegration{})
	})

	if err := rag.AutoMigrate(db); err != nil {
		t.Fatalf("rag.AutoMigrate: %v", err)
	}

	var supports bool
	if err := db.Raw(
		`SELECT supports_rag_source FROM in_integrations WHERE id = ?`,
		integ.ID,
	).Scan(&supports).Error; err != nil {
		t.Fatalf("read supports_rag_source: %v", err)
	}
	if !supports {
		t.Fatal("expected supports_rag_source=true for provider='github' after migration")
	}
}

// Validates that IndexingStart round-trips through the schema —
// silent loss to NULL would cause every run to re-ingest full history.
func TestRAGSource_IndexingStartFloor(t *testing.T) {
	db := testhelpers.ConnectTestDB(t)
	org := testhelpers.NewTestOrg(t, db)
	user := testhelpers.NewTestUser(t, db, org.ID)
	integ := testhelpers.NewTestInIntegration(t, db, "github")
	conn := testhelpers.NewTestInConnection(t, db, org.ID, user.ID, integ.ID)

	src := testhelpers.NewTestRAGSource(t, db, org.ID, conn.ID)
	var read ragmodel.RAGSource
	if err := db.First(&read, "id = ?", src.ID).Error; err != nil {
		t.Fatalf("read back: %v", err)
	}
	if read.IndexingStart != nil {
		t.Fatalf("default IndexingStart should be nil, got %v", read.IndexingStart)
	}

	floor := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := db.Model(&read).Update("indexing_start", floor).Error; err != nil {
		t.Fatalf("update IndexingStart: %v", err)
	}
	var updated ragmodel.RAGSource
	if err := db.First(&updated, "id = ?", src.ID).Error; err != nil {
		t.Fatalf("read after update: %v", err)
	}
	if updated.IndexingStart == nil || !updated.IndexingStart.Equal(floor) {
		t.Fatalf("IndexingStart round-trip mismatch: got %v want %v", updated.IndexingStart, floor)
	}
}

