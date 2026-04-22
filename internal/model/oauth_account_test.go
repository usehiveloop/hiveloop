package model_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"

	"github.com/usehiveloop/hiveloop/internal/model"
	"github.com/usehiveloop/hiveloop/internal/rag/testhelpers"
)

// TestOAuthAccount_MigrationAddsFieldsWithoutBreakingExistingRows proves
// the Tranche 1E column extension (provider_user_email, provider_user_login,
// verified_emails, last_synced_at) is safe to ship against a live
// oauth_accounts table holding production rows.
//
// The concrete production worry: shipping this change triggers
// `model.AutoMigrate(db)` on boot. If the migration tried to add these
// columns as NOT NULL, or rewrote a column type, the existing rows —
// created before the migration existed — would be corrupted or rejected.
//
// Procedure:
//  1. ConnectTestDB runs both migrations (current code).
//  2. Simulate a pre-Tranche-1E production DB by ALTER-DROPping the four
//     new columns.
//  3. Insert a row with raw SQL using only the pre-1E column set — this
//     is a realistic "row written by old code".
//  4. Re-run `model.AutoMigrate(db)` (gorm AutoMigrate is idempotent and
//     picks up the struct's new columns).
//  5. Verify the pre-existing row is still readable via gorm and the new
//     fields come back nil / empty.
//  6. Update the row with values for the new fields and verify they
//     persist + reload correctly.
//
// If this test fails, the migration is unsafe to deploy against
// production and must NOT be merged.
func TestOAuthAccount_MigrationAddsFieldsWithoutBreakingExistingRows(t *testing.T) {
	db := testhelpers.ConnectTestDB(t)

	// --- 1. Set up a parent user (oauth_accounts.user_id is NOT NULL) ---
	org := testhelpers.NewTestOrg(t, db)
	user := testhelpers.NewTestUser(t, db, org.ID)

	// --- 2. Simulate pre-1E schema: drop the new columns if present ---
	// IF EXISTS makes this work whether or not the current migration has
	// already run them in; repeats cleanly.
	dropStmts := []string{
		`ALTER TABLE oauth_accounts DROP COLUMN IF EXISTS provider_user_email`,
		`ALTER TABLE oauth_accounts DROP COLUMN IF EXISTS provider_user_login`,
		`ALTER TABLE oauth_accounts DROP COLUMN IF EXISTS verified_emails`,
		`ALTER TABLE oauth_accounts DROP COLUMN IF EXISTS last_synced_at`,
	}
	for _, stmt := range dropStmts {
		if err := db.Exec(stmt).Error; err != nil {
			t.Fatalf("simulate pre-1E drop %q: %v", stmt, err)
		}
	}

	// --- 3. Insert a "legacy" row via raw SQL using only pre-1E columns ---
	legacyID := uuid.New()
	legacyProvider := "github"
	legacyProviderUID := "legacy-" + legacyID.String()
	now := time.Now().UTC().Truncate(time.Microsecond)
	if err := db.Exec(
		`INSERT INTO oauth_accounts (id, user_id, provider, provider_user_id, created_at, updated_at)
         VALUES (?, ?, ?, ?, ?, ?)`,
		legacyID, user.ID, legacyProvider, legacyProviderUID, now, now,
	).Error; err != nil {
		t.Fatalf("insert legacy row: %v", err)
	}
	// Guaranteed row-level cleanup on success or failure.
	t.Cleanup(func() {
		db.Exec(`DELETE FROM oauth_accounts WHERE id = ?`, legacyID)
	})

	// --- 4. Re-run the migration. This is the real production upgrade
	// path: boot-time AutoMigrate picks up the 4 new columns and ALTERs
	// them in. It must not fail, and must not rewrite the legacy row.
	if err := model.AutoMigrate(db); err != nil {
		t.Fatalf("AutoMigrate after column drop must succeed against live rows; got: %v", err)
	}

	// --- 5. Legacy row still exists, new fields readable + null ---
	var reloaded model.OAuthAccount
	if err := db.Where("id = ?", legacyID).First(&reloaded).Error; err != nil {
		t.Fatalf("legacy row disappeared after migration: %v", err)
	}
	if reloaded.ID != legacyID {
		t.Fatalf("legacy row id changed: want %s, got %s", legacyID, reloaded.ID)
	}
	if reloaded.Provider != legacyProvider {
		t.Fatalf("legacy provider changed: want %q, got %q", legacyProvider, reloaded.Provider)
	}
	if reloaded.ProviderUserID != legacyProviderUID {
		t.Fatalf("legacy provider_user_id changed: want %q, got %q", legacyProviderUID, reloaded.ProviderUserID)
	}
	if reloaded.ProviderUserEmail != nil {
		t.Fatalf("expected provider_user_email to be NULL on legacy row, got %v", *reloaded.ProviderUserEmail)
	}
	if reloaded.ProviderUserLogin != nil {
		t.Fatalf("expected provider_user_login to be NULL on legacy row, got %v", *reloaded.ProviderUserLogin)
	}
	if len(reloaded.VerifiedEmails) != 0 {
		t.Fatalf("expected verified_emails empty on legacy row, got %v", reloaded.VerifiedEmails)
	}
	if reloaded.LastSyncedAt != nil {
		t.Fatalf("expected last_synced_at NULL on legacy row, got %v", *reloaded.LastSyncedAt)
	}

	// --- 6. Update with values for the new fields, assert persist + reload ---
	email := "work@example.com"
	login := "octocat"
	syncedAt := time.Now().UTC().Truncate(time.Microsecond)
	reloaded.ProviderUserEmail = &email
	reloaded.ProviderUserLogin = &login
	reloaded.VerifiedEmails = pq.StringArray{"work@example.com", "personal@example.com"}
	reloaded.LastSyncedAt = &syncedAt
	if err := db.Save(&reloaded).Error; err != nil {
		t.Fatalf("save updated row: %v", err)
	}

	var reread model.OAuthAccount
	if err := db.Where("id = ?", legacyID).First(&reread).Error; err != nil {
		t.Fatalf("re-read after update: %v", err)
	}
	if reread.ProviderUserEmail == nil || *reread.ProviderUserEmail != email {
		t.Fatalf("ProviderUserEmail not persisted; got %v", reread.ProviderUserEmail)
	}
	if reread.ProviderUserLogin == nil || *reread.ProviderUserLogin != login {
		t.Fatalf("ProviderUserLogin not persisted; got %v", reread.ProviderUserLogin)
	}
	if len(reread.VerifiedEmails) != 2 ||
		reread.VerifiedEmails[0] != "work@example.com" ||
		reread.VerifiedEmails[1] != "personal@example.com" {
		t.Fatalf("VerifiedEmails not persisted as expected; got %v", reread.VerifiedEmails)
	}
	if reread.LastSyncedAt == nil || !reread.LastSyncedAt.Equal(syncedAt) {
		t.Fatalf("LastSyncedAt not persisted; want %v, got %v", syncedAt, reread.LastSyncedAt)
	}
}
