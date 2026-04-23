package model_test

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

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
