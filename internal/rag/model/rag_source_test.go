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
func isCheckViolation(err error) bool {
	var pg *pgconn.PgError
	if errors.As(err, &pg) {
		return pg.Code == "23514"
	}
	return false
}

// Business value: GDPR tenant-deletion compliance — deleting an org
// must tear down its RAGSource rows.
func TestRAGSource_OrgCascadeDelete(t *testing.T) {
	db := testhelpers.ConnectTestDB(t)

	org := testhelpers.NewTestOrg(t, db)
	user := testhelpers.NewTestUser(t, db, org.ID)
	integ := testhelpers.NewTestInIntegration(t, db, "github")
	conn := testhelpers.NewTestInConnection(t, db, org.ID, user.ID, integ.ID)
	src := testhelpers.NewTestRAGSource(t, db, org.ID, conn.ID)
	srcID := src.ID

	// Remove memberships so the org delete can proceed; cascade from
	// orgs must then wipe rag_sources.
	if err := db.Exec(`DELETE FROM org_memberships WHERE org_id = ?`, org.ID).Error; err != nil {
		t.Fatalf("delete org_memberships: %v", err)
	}
	if err := db.Exec(`DELETE FROM orgs WHERE id = ?`, org.ID).Error; err != nil {
		t.Fatalf("delete org: %v", err)
	}

	var count int64
	if err := db.Raw(`SELECT COUNT(*) FROM rag_sources WHERE id = ?`, srcID).Scan(&count).Error; err != nil {
		t.Fatalf("count after cascade: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected rag_sources row to be cascade-deleted; %d survived", count)
	}
}

// Business value: the schema enforces the Kind invariant at write time
// so a bug in an admin API handler can't create a malformed integration
// row.
func TestRAGSource_IntegrationKindRequiresInConnection(t *testing.T) {
	db := testhelpers.ConnectTestDB(t)
	org := testhelpers.NewTestOrg(t, db)

	src := &ragmodel.RAGSource{
		OrgIDValue:     org.ID,
		KindValue:      ragmodel.RAGSourceKindIntegration,
		Name:           "integration without connection",
		Status:         ragmodel.RAGSourceStatusActive,
		Enabled:        true,
		InConnectionID: nil, // violates the CHECK: INTEGRATION must have it
		AccessType:     ragmodel.AccessTypePrivate,
	}
	err := db.Create(src).Error
	if err == nil {
		t.Fatal("expected CHECK violation on INTEGRATION with null in_connection_id; got nil")
	}
	if !isCheckViolation(err) {
		t.Fatalf("expected 23514 check violation, got: %v", err)
	}
}

// Business value: prevents an admin API bug from cross-wiring an
// InConnection FK onto a website/file-upload row.
func TestRAGSource_NonIntegrationKindRejectsInConnection(t *testing.T) {
	db := testhelpers.ConnectTestDB(t)
	org := testhelpers.NewTestOrg(t, db)
	user := testhelpers.NewTestUser(t, db, org.ID)
	integ := testhelpers.NewTestInIntegration(t, db, "github")
	conn := testhelpers.NewTestInConnection(t, db, org.ID, user.ID, integ.ID)

	connID := conn.ID
	src := &ragmodel.RAGSource{
		OrgIDValue:     org.ID,
		KindValue:      ragmodel.RAGSourceKindWebsite,
		Name:           "website with stray in_connection",
		Status:         ragmodel.RAGSourceStatusActive,
		Enabled:        true,
		InConnectionID: &connID, // violates the CHECK: non-INTEGRATION must be null
		AccessType:     ragmodel.AccessTypePrivate,
	}
	err := db.Create(src).Error
	if err == nil {
		t.Fatal("expected CHECK violation on non-INTEGRATION with in_connection_id; got nil")
	}
	if !isCheckViolation(err) {
		t.Fatalf("expected 23514 check violation, got: %v", err)
	}
}

// Business value: pins the "one RAG source per InConnection"
// invariant, preventing two RAGSource rows from both claiming
// ownership of the same connection.
func TestRAGSource_UniquePerInConnection(t *testing.T) {
	db := testhelpers.ConnectTestDB(t)
	org := testhelpers.NewTestOrg(t, db)
	user := testhelpers.NewTestUser(t, db, org.ID)
	integ := testhelpers.NewTestInIntegration(t, db, "github")
	conn := testhelpers.NewTestInConnection(t, db, org.ID, user.ID, integ.ID)

	connID := conn.ID

	first := &ragmodel.RAGSource{
		OrgIDValue:     org.ID,
		KindValue:      ragmodel.RAGSourceKindIntegration,
		Name:           "first",
		Status:         ragmodel.RAGSourceStatusActive,
		Enabled:        true,
		InConnectionID: &connID,
		AccessType:     ragmodel.AccessTypePrivate,
	}
	if err := db.Create(first).Error; err != nil {
		t.Fatalf("first insert: %v", err)
	}
	t.Cleanup(func() {
		db.Where("id = ?", first.ID).Delete(&ragmodel.RAGSource{})
	})

	second := &ragmodel.RAGSource{
		OrgIDValue:     org.ID,
		KindValue:      ragmodel.RAGSourceKindIntegration,
		Name:           "second",
		Status:         ragmodel.RAGSourceStatusActive,
		Enabled:        true,
		InConnectionID: &connID,
		AccessType:     ragmodel.AccessTypePrivate,
	}
	err := db.Create(second).Error
	if err == nil {
		t.Fatal("expected unique violation on second RAGSource for same in_connection_id")
	}
	if !isUniqueViolation(err) {
		t.Fatalf("expected 23505 unique violation, got: %v", err)
	}
}

// Business value: the scheduler scans rag_sources every 30s for work.
// A missing partial index turns that hot-path scan into a full table
// walk the moment an org accumulates enough sources.
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
func TestRAGSource_PersistsConfigJSON(t *testing.T) {
	db := testhelpers.ConnectTestDB(t)
	org := testhelpers.NewTestOrg(t, db)
	user := testhelpers.NewTestUser(t, db, org.ID)
	integ := testhelpers.NewTestInIntegration(t, db, "github")
	conn := testhelpers.NewTestInConnection(t, db, org.ID, user.ID, integ.ID)

	connID := conn.ID
	cfg := coremodel.JSON{
		"repo_allowlist":     []any{"hiveloop/api", "hiveloop/web"},
		"include_issues":     true,
		"include_prs":        false,
		"batch_size":         float64(250), // JSON numbers round-trip as float64
		"nested":             map[string]any{"depth": float64(2), "enabled": true},
	}

	src := &ragmodel.RAGSource{
		OrgIDValue:     org.ID,
		KindValue:      ragmodel.RAGSourceKindIntegration,
		Name:           "config round-trip",
		Status:         ragmodel.RAGSourceStatusActive,
		Enabled:        true,
		InConnectionID: &connID,
		AccessType:     ragmodel.AccessTypePrivate,
		ConfigValue:    cfg,
	}
	if err := db.Create(src).Error; err != nil {
		t.Fatalf("create: %v", err)
	}
	t.Cleanup(func() {
		db.Where("id = ?", src.ID).Delete(&ragmodel.RAGSource{})
	})

	var reloaded ragmodel.RAGSource
	if err := db.Where("id = ?", src.ID).First(&reloaded).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}

	// Allowlist
	al, ok := reloaded.ConfigValue["repo_allowlist"].([]any)
	if !ok || len(al) != 2 || al[0] != "hiveloop/api" || al[1] != "hiveloop/web" {
		t.Fatalf("repo_allowlist round-trip failed: %#v", reloaded.ConfigValue["repo_allowlist"])
	}
	// Booleans
	if reloaded.ConfigValue["include_issues"] != true {
		t.Fatalf("include_issues round-trip: %#v", reloaded.ConfigValue["include_issues"])
	}
	if reloaded.ConfigValue["include_prs"] != false {
		t.Fatalf("include_prs round-trip: %#v", reloaded.ConfigValue["include_prs"])
	}
	// Number
	if reloaded.ConfigValue["batch_size"] != float64(250) {
		t.Fatalf("batch_size round-trip: %#v", reloaded.ConfigValue["batch_size"])
	}
	// Nested object
	nested, ok := reloaded.ConfigValue["nested"].(map[string]any)
	if !ok || nested["depth"] != float64(2) || nested["enabled"] != true {
		t.Fatalf("nested round-trip: %#v", reloaded.ConfigValue["nested"])
	}
}

// Business value: migrations must be idempotent. Running
// rag.AutoMigrate twice against a live DB must not error and must not
// multiply indexes.
func TestAutoMigrate_IsIdempotent(t *testing.T) {
	db := testhelpers.ConnectTestDB(t)

	countIndexes := func() int64 {
		var n int64
		if err := db.Raw(`
			SELECT COUNT(*) FROM pg_indexes
			WHERE schemaname = 'public' AND tablename LIKE 'rag_%'
		`).Scan(&n).Error; err != nil {
			t.Fatalf("count indexes: %v", err)
		}
		return n
	}
	before := countIndexes()

	if err := rag.AutoMigrate(db); err != nil {
		t.Fatalf("second rag.AutoMigrate: %v", err)
	}
	after := countIndexes()
	if before != after {
		t.Fatalf("rag.AutoMigrate not idempotent: index count %d -> %d", before, after)
	}
}

