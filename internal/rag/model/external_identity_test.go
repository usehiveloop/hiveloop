package model_test

import (
	"strings"
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
// in, leaving the test free to override before insert.
func mkIdentity(orgID, userID, connID uuid.UUID, provider, extID string) *ragmodel.RAGExternalIdentity {
	login := "octocat-" + extID
	return &ragmodel.RAGExternalIdentity{
		OrgID:              orgID,
		UserID:             userID,
		InConnectionID:     connID,
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

	first := mkIdentity(org.ID, user.ID, conn.ID, "github", "100001")
	if err := db.Create(first).Error; err != nil {
		t.Fatalf("first insert: %v", err)
	}

	// Second row for the same (user_id, in_connection_id). Provider +
	// ExternalUserID are deliberately different so only the other unique
	// can fire.
	second := mkIdentity(org.ID, user.ID, conn.ID, "github", "200002")
	err := db.Create(second).Error
	if err == nil {
		t.Fatal("expected unique-violation on (user_id, in_connection_id); got nil")
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

	// Seed: (provider=github, external_user_id=42, org=A) → userA1/connA1.
	seed := mkIdentity(orgA.ID, userA1.ID, connA1.ID, "github", "42")
	if err := db.Create(seed).Error; err != nil {
		t.Fatalf("seed insert: %v", err)
	}

	// Same (provider, external_user_id) in the SAME org but different
	// user+connection → must violate.
	dup := mkIdentity(orgA.ID, userA2.ID, connA2.ID, "github", "42")
	if err := db.Create(dup).Error; err == nil {
		t.Fatal("expected unique violation on (provider, external_user_id, org_id); got nil")
	} else if !isUniqueViolation(err) {
		t.Fatalf("expected unique violation, got: %v", err)
	}

	// Same (provider, external_user_id) in a DIFFERENT org → must succeed.
	// This is the cross-org-isolation business guarantee.
	cross := mkIdentity(orgB.ID, userB.ID, connB.ID, "github", "42")
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

	id := mkIdentity(org.ID, user.ID, conn.ID, "github", "cascade-user")
	if err := db.Create(id).Error; err != nil {
		t.Fatalf("insert: %v", err)
	}
	rowID := id.ID

	// Drop in_connections and org_memberships referencing this user so we
	// can drop the user row itself without tripping unrelated FKs. We want
	// to isolate the USER→identity cascade path — the connection-cascade
	// path has its own test.
	if err := db.Exec(`DELETE FROM in_connections WHERE id = ?`, conn.ID).Error; err != nil {
		t.Fatalf("delete connection: %v", err)
	}
	if err := db.Exec(`DELETE FROM org_memberships WHERE user_id = ?`, user.ID).Error; err != nil {
		t.Fatalf("delete org_memberships: %v", err)
	}
	if err := db.Exec(`DELETE FROM users WHERE id = ?`, user.ID).Error; err != nil {
		t.Fatalf("delete user: %v", err)
	}

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

// TestRAGExternalIdentity_InConnectionCascade verifies that deleting an
// InConnection wipes every cached identity pinned to that connection.
// This matches the production path where an admin disconnects an
// integration and we must stop resolving ACLs against source data that
// can no longer be refreshed.
func TestRAGExternalIdentity_InConnectionCascade(t *testing.T) {
	db := setup1E(t)

	org := testhelpers.NewTestOrg(t, db)
	user := testhelpers.NewTestUser(t, db, org.ID)
	integ := testhelpers.NewTestInIntegration(t, db, "github")
	conn := testhelpers.NewTestInConnection(t, db, org.ID, user.ID, integ.ID)

	id := mkIdentity(org.ID, user.ID, conn.ID, "github", "cascade-conn")
	if err := db.Create(id).Error; err != nil {
		t.Fatalf("insert: %v", err)
	}
	rowID := id.ID

	if err := db.Exec(`DELETE FROM in_connections WHERE id = ?`, conn.ID).Error; err != nil {
		t.Fatalf("delete connection: %v", err)
	}

	var remaining int64
	if err := db.Raw(
		`SELECT COUNT(*) FROM rag_external_identities WHERE id = ?`, rowID,
	).Scan(&remaining).Error; err != nil {
		t.Fatalf("count after conn delete: %v", err)
	}
	if remaining != 0 {
		t.Fatalf("expected RAGExternalIdentity removed by connection cascade; %d row(s) remain", remaining)
	}
}

// isUniqueViolation keeps the test independent of the pq driver's
// error-wrapping strategy — gorm wraps pgx/pq errors in a generic form,
// so matching on the SQLSTATE-ish substring is the most stable pattern.
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "duplicate key") ||
		strings.Contains(msg, "unique constraint") ||
		strings.Contains(msg, "SQLSTATE 23505") ||
		strings.Contains(msg, "23505")
}
