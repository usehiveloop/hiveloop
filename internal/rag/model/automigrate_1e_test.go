package model_test

import (
	"testing"

	ragmodel "github.com/usehiveloop/hiveloop/internal/rag/model"
	"github.com/usehiveloop/hiveloop/internal/rag/testhelpers"
)

// TestAutoMigrate1E_AppliesAndIsIdempotent exercises both branches of
// the guarded ALTER TABLE helper:
//
//  1. FKs missing → ADD CONSTRAINT executes.
//  2. FKs already there → information_schema short-circuit, no duplicate
//     ADD attempt (which would error on Postgres).
//
// This is the path that runs on every boot in CI + production. If it
// regresses, deploy hangs at startup.
func TestAutoMigrate1E_AppliesAndIsIdempotent(t *testing.T) {
	db := testhelpers.ConnectTestDB(t)

	// Simulate a fresh DB for the FK-creation path. DROP CONSTRAINT is
	// idempotent via IF EXISTS so this works whether or not prior tests
	// already applied them.
	drops := []string{
		`ALTER TABLE rag_external_identities DROP CONSTRAINT IF EXISTS fk_rag_external_identities_org`,
		`ALTER TABLE rag_external_identities DROP CONSTRAINT IF EXISTS fk_rag_external_identities_user`,
		`ALTER TABLE rag_external_identities DROP CONSTRAINT IF EXISTS fk_rag_external_identities_connection`,
	}
	// Sequence: (a) call AutoMigrate1E first so the table exists,
	// (b) drop the FKs, (c) call AutoMigrate1E again to exercise the
	// CREATE branch, (d) call once more for idempotency.

	// (a)
	if err := ragmodel.AutoMigrate1E(db); err != nil {
		t.Fatalf("initial AutoMigrate1E: %v", err)
	}
	// (b)
	for _, s := range drops {
		if err := db.Exec(s).Error; err != nil {
			t.Fatalf("drop constraint for fresh-apply test: %v", err)
		}
	}
	// (c) FK-creation path.
	if err := ragmodel.AutoMigrate1E(db); err != nil {
		t.Fatalf("fresh-apply AutoMigrate1E: %v", err)
	}

	// Post-condition: all three FKs exist.
	for _, name := range []string{
		"fk_rag_external_identities_org",
		"fk_rag_external_identities_user",
		"fk_rag_external_identities_connection",
	} {
		var n int64
		if err := db.Raw(
			`SELECT COUNT(*) FROM information_schema.table_constraints
             WHERE table_name = 'rag_external_identities' AND constraint_name = ?`,
			name,
		).Scan(&n).Error; err != nil {
			t.Fatalf("lookup %s: %v", name, err)
		}
		if n != 1 {
			t.Fatalf("expected FK %s to be present after AutoMigrate1E; count=%d", name, n)
		}
	}

	// (d) Idempotency: call again. The information_schema short-circuit
	// branch now runs for every FK. Must not error.
	if err := ragmodel.AutoMigrate1E(db); err != nil {
		t.Fatalf("idempotent re-run of AutoMigrate1E: %v", err)
	}
}
