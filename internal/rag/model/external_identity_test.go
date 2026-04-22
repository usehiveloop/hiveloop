package model_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"gorm.io/gorm"

	ragmodel "github.com/usehiveloop/hiveloop/internal/rag/model"
	"github.com/usehiveloop/hiveloop/internal/rag/testhelpers"
)

// setup1E connects to the test DB, runs the 1E tranche migration (since
// 1F has not wired it into the central rag.AutoMigrate yet), and returns
// the *gorm.DB. Per-test cleanup of rag_external_identities rows piggybacks
// on the fixture-level org cleanup since the FK cascades.
func setup1E(t *testing.T) *gorm.DB {
	t.Helper()
	db := testhelpers.ConnectTestDB(t)
	if err := ragmodel.AutoMigrate1E(db); err != nil {
		t.Fatalf("AutoMigrate1E: %v", err)
	}
	// Belt-and-suspenders: if a prior test aborted mid-flight the tranche
	// table may have orphan rows whose parent orgs/users/connections were
	// already cleaned up. Scrub only the rows we can definitively identify
	// as orphan (org_id no longer present).
	t.Cleanup(func() {
		db.Exec(`DELETE FROM rag_external_identities
                 WHERE org_id NOT IN (SELECT id FROM orgs)`)
	})
	return db
}

// mkIdentity builds a RAGExternalIdentity with the required fields filled
// in, leaving the test free to override before insert. The third UUID is
// the RAGSource ID (post-Phase-3A swap; pre-3A this was the InConnection).
func mkIdentity(orgID, userID, ragSourceID uuid.UUID, provider, extID string) *ragmodel.RAGExternalIdentity {
	login := "octocat-" + extID
	return &ragmodel.RAGExternalIdentity{
		OrgID:              orgID,
		UserID:             userID,
		RAGSourceID:        ragSourceID,
		Provider:           provider,
		ExternalUserID:     extID,
		ExternalUserLogin:  &login,
		ExternalUserEmails: pq.StringArray{extID + "@example.com"},
		UpdatedAt:          time.Now(),
	}
}

// TestRAGExternalIdentity_UniquePerUserConnection pins the business rule
// that a given Hiveloop user has at most one cached external identity per
// InConnection: if perm-sync writes twice for the same (user, connection)
// we want the second write to either UPSERT or fail loudly — never create
// a duplicate row that would double-count the user in ACLs.
func TestRAGExternalIdentity_UniquePerUserConnection(t *testing.T) {
	db := setup1E(t)

	org := testhelpers.NewTestOrg(t, db)
	user := testhelpers.NewTestUser(t, db, org.ID)
	integ := testhelpers.NewTestInIntegration(t, db, "github")
	conn := testhelpers.NewTestInConnection(t, db, org.ID, user.ID, integ.ID)
	src := testhelpers.NewTestRAGSource(t, db, org.ID, conn.ID)

	first := mkIdentity(org.ID, user.ID, src.ID, "github", "100001")
	if err := db.Create(first).Error; err != nil {
		t.Fatalf("first insert: %v", err)
	}

	// Second row for the same (user_id, rag_source_id). Provider +
	// ExternalUserID are deliberately different so only the other unique
	// can fire.
	second := mkIdentity(org.ID, user.ID, src.ID, "github", "200002")
	err := db.Create(second).Error
	if err == nil {
		t.Fatal("expected unique-violation on (user_id, rag_source_id); got nil")
	}
	if !isUniqueViolation(err) {
		t.Fatalf("expected unique violation, got: %v", err)
	}
}

// TestRAGExternalIdentity_UniqueProviderExtIDInOrg verifies cross-org
// isolation: the same GitHub user (same provider + same external id) can
// belong to two Hiveloop orgs, but within a single org the pair is unique
// so a typo / race can't shadow-bind the identity to two different users.
func TestRAGExternalIdentity_UniqueProviderExtIDInOrg(t *testing.T) {
	db := setup1E(t)

	// Two orgs, two users each in their own org, two connections.
	orgA := testhelpers.NewTestOrg(t, db)
	orgB := testhelpers.NewTestOrg(t, db)
	userA1 := testhelpers.NewTestUser(t, db, orgA.ID)
	userA2 := testhelpers.NewTestUser(t, db, orgA.ID)
	userB := testhelpers.NewTestUser(t, db, orgB.ID)
	integ := testhelpers.NewTestInIntegration(t, db, "github")
	connA1 := testhelpers.NewTestInConnection(t, db, orgA.ID, userA1.ID, integ.ID)
	connA2 := testhelpers.NewTestInConnection(t, db, orgA.ID, userA2.ID, integ.ID)
	connB := testhelpers.NewTestInConnection(t, db, orgB.ID, userB.ID, integ.ID)
	srcA1 := testhelpers.NewTestRAGSource(t, db, orgA.ID, connA1.ID)
	srcA2 := testhelpers.NewTestRAGSource(t, db, orgA.ID, connA2.ID)
	srcB := testhelpers.NewTestRAGSource(t, db, orgB.ID, connB.ID)

	// Seed: (provider=github, external_user_id=42, org=A) → userA1/srcA1.
	seed := mkIdentity(orgA.ID, userA1.ID, srcA1.ID, "github", "42")
	if err := db.Create(seed).Error; err != nil {
		t.Fatalf("seed insert: %v", err)
	}

	// Same (provider, external_user_id) in the SAME org but different
	// user+source → must violate.
	dup := mkIdentity(orgA.ID, userA2.ID, srcA2.ID, "github", "42")
	if err := db.Create(dup).Error; err == nil {
		t.Fatal("expected unique violation on (provider, external_user_id, org_id); got nil")
	} else if !isUniqueViolation(err) {
		t.Fatalf("expected unique violation, got: %v", err)
	}

	// Same (provider, external_user_id) in a DIFFERENT org → must succeed.
	// This is the cross-org-isolation business guarantee.
	cross := mkIdentity(orgB.ID, userB.ID, srcB.ID, "github", "42")
	if err := db.Create(cross).Error; err != nil {
		t.Fatalf("cross-org insert with same (provider, external_user_id) must succeed, got: %v", err)
	}
}

// TestRAGExternalIdentity_UserCascade deletes the Hiveloop user and
// verifies every cached identity row for that user is gone. This is the
// GDPR-/account-closure compliance path: when a user is removed, their
// source-side identity cache must not linger.
func TestRAGExternalIdentity_UserCascade(t *testing.T) {
	db := setup1E(t)

	org := testhelpers.NewTestOrg(t, db)
	user := testhelpers.NewTestUser(t, db, org.ID)
	integ := testhelpers.NewTestInIntegration(t, db, "github")
	conn := testhelpers.NewTestInConnection(t, db, org.ID, user.ID, integ.ID)
	src := testhelpers.NewTestRAGSource(t, db, org.ID, conn.ID)

	id := mkIdentity(org.ID, user.ID, src.ID, "github", "cascade-user")
	if err := db.Create(id).Error; err != nil {
		t.Fatalf("insert: %v", err)
	}
	rowID := id.ID

	// Drop org_memberships referencing this user so we can drop the user
	// row itself without tripping unrelated FKs. Then delete the user —
	// the direct user_id FK on rag_external_identities (CASCADE) should
	// sweep the identity row regardless of what happens to the cascaded
	// rag_source / in_connection behind it.
	if err := db.Exec(`DELETE FROM org_memberships WHERE user_id = ?`, user.ID).Error; err != nil {
		t.Fatalf("delete org_memberships: %v", err)
	}
	if err := db.Exec(`DELETE FROM users WHERE id = ?`, user.ID).Error; err != nil {
		t.Fatalf("delete user: %v", err)
	}
	_ = conn // conn.ID may have been cascade-deleted via users; not re-queried.
	_ = src

	var remaining int64
	if err := db.Raw(
		`SELECT COUNT(*) FROM rag_external_identities WHERE id = ?`, rowID,
	).Scan(&remaining).Error; err != nil {
		t.Fatalf("count after user delete: %v", err)
	}
	if remaining != 0 {
		t.Fatalf("expected RAGExternalIdentity removed by user cascade; %d row(s) remain", remaining)
	}

	// Fixture t.Cleanup may try to re-delete the membership/user; those
	// deletes become no-ops against already-deleted rows, which is fine.
}

// TestRAGExternalIdentity_RAGSourceCascade verifies that deleting a
// RAGSource wipes every cached identity pinned to that source.
// Post-Phase-3A this replaces the old InConnection-cascade test: the
// admin UI's "remove RAG source" path now goes through rag_sources, not
// in_connections, and ACL resolution must stop against source data that
// can no longer be refreshed.
func TestRAGExternalIdentity_RAGSourceCascade(t *testing.T) {
	db := setup1E(t)

	org := testhelpers.NewTestOrg(t, db)
	user := testhelpers.NewTestUser(t, db, org.ID)
	integ := testhelpers.NewTestInIntegration(t, db, "github")
	conn := testhelpers.NewTestInConnection(t, db, org.ID, user.ID, integ.ID)
	src := testhelpers.NewTestRAGSource(t, db, org.ID, conn.ID)

	id := mkIdentity(org.ID, user.ID, src.ID, "github", "cascade-src")
	if err := db.Create(id).Error; err != nil {
		t.Fatalf("insert: %v", err)
	}
	rowID := id.ID

	if err := db.Exec(`DELETE FROM rag_sources WHERE id = ?`, src.ID).Error; err != nil {
		t.Fatalf("delete rag_source: %v", err)
	}

	var remaining int64
	if err := db.Raw(
		`SELECT COUNT(*) FROM rag_external_identities WHERE id = ?`, rowID,
	).Scan(&remaining).Error; err != nil {
		t.Fatalf("count after rag_source delete: %v", err)
	}
	if remaining != 0 {
		t.Fatalf("expected RAGExternalIdentity removed by rag_source cascade; %d row(s) remain", remaining)
	}
}

// isUniqueViolation used here comes from sync_state_test.go (same
// _test package). Kept centralised so every tranche's tests use a
// single SQLSTATE-based helper.
