package model_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"

	coremodel "github.com/usehiveloop/hiveloop/internal/model"
	"github.com/usehiveloop/hiveloop/internal/rag"
	ragmodel "github.com/usehiveloop/hiveloop/internal/rag/model"
	"github.com/usehiveloop/hiveloop/internal/rag/testhelpers"
)

// isCheckViolation pins on Postgres error 23514 so we don't
// false-positive on unique / FK failures.
func TestRAGSource_NeedsIngestPartialIndex(t *testing.T) {
	db := testhelpers.ConnectTestDB(t)

	// Part A: the partial index exists with the expected WHERE
	// predicate. Index definition is the load-bearing invariant; the
	// planner choice at test-scale row counts is noisy.
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

	// Part B: the planner picks the partial index over a seq scan
	// once seq scan is disabled. We drop the broader composite index
	// inside the transaction so the planner has no choice but the
	// partial — the transaction rollback restores the dropped index.
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
	// planner has only the partial index available.
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
		// Rollback to restore the dropped index.
		return errRollback
	})
	if !errors.Is(err, errRollback) {
		t.Fatalf("tx: %v", err)
	}
	if !strings.Contains(plan, "idx_rag_sources_needs_ingest") {
		t.Fatalf("planner did not use idx_rag_sources_needs_ingest; plan=%s", plan)
	}
}

// errRollback is a sentinel used to roll back a transaction without
// signalling a true error.
var errRollback = errors.New("intentional rollback")

// Business value: the admin API validation rejects refresh frequencies
// below 60s so a misconfiguration can't overload upstream APIs.
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

// Business value: the admin API validation rejects prune frequencies
// below 300s so a misconfiguration can't thrash the prune worker.
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

// Business value: the scheduler's "should I schedule work against this
// source?" gate. A wrong answer either stalls ingestion (false
// negative) or thrashes a disabled source (false positive).
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

// Business value: API input validation — typo'd kind strings must be
// rejected at the edge before any DB write.
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
		{ragmodel.RAGSourceKind("integration"), false}, // case-sensitive
	}
	for _, tc := range cases {
		t.Run(string(tc.kind), func(t *testing.T) {
			if got := tc.kind.IsValid(); got != tc.want {
				t.Fatalf("%s.IsValid() = %v, want %v", tc.kind, got, tc.want)
			}
		})
	}
}

// Business value: the admin UI's "Add RAG source" picker filters
// in_integrations by supports_rag_source. If the seed doesn't flip the
// flag on the known-good providers, admins can't add a source at all.
func TestRAGSource_SeedsSupportsRAGSourceForKnownIntegrations(t *testing.T) {
	db := testhelpers.ConnectTestDB(t)

	// Insert a real github integration row. ConnectTestDB already ran
	// rag.AutoMigrate which sets supports_rag_source=true for a curated
	// provider allowlist. Existing rows for "github" are already
	// flipped; new rows must also flip on the next migration pass.
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

	// Run rag.AutoMigrate again; the seed step must flip the flag for
	// our newly inserted row.
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

// Business value: per-source config must persist cleanly. Silent JSONB
// corruption on a field like config would be unrecoverable at scale.
