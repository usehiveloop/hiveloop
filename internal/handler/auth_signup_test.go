package handler

// Internal test (package handler, not handler_test) so it can exercise the
// unexported createUserDefaultOrg helper directly. Tests run against the
// same Postgres used by handler_test (mirrored connectInternalTestDB below
// because the package_test helper isn't reachable from this package).

import (
	"os"
	"testing"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/billing"
	"github.com/usehiveloop/hiveloop/internal/model"
)

//nolint:gosec // G101: local-dev DSN, mirrors api_keys_test.go testDBURL.
const internalTestDBURL = "postgres://hiveloop:localdev@localhost:5433/hiveloop_test?sslmode=disable"

func connectInternalTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = internalTestDBURL
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("connect Postgres: %v", err)
	}
	sqlDB, _ := db.DB()
	if err := sqlDB.Ping(); err != nil {
		t.Fatalf("Postgres not reachable: %v", err)
	}
	if err := model.AutoMigrate(db); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	return db
}

func seedFreePlan(t *testing.T, db *gorm.DB, welcome int64) {
	t.Helper()
	plan := model.Plan{
		ID:             uuid.New(),
		Slug:           billing.FreePlanSlug,
		Name:           "Free",
		WelcomeCredits: welcome,
		Active:         true,
	}
	if err := db.Create(&plan).Error; err != nil {
		t.Fatalf("seed free plan: %v", err)
	}
	t.Cleanup(func() { db.Unscoped().Delete(&plan) })
}

func seedSignupUser(t *testing.T, db *gorm.DB) *model.User {
	t.Helper()
	user := &model.User{
		Email: "signup-" + uuid.NewString() + "@test.hiveloop.com",
		Name:  "Signup Test",
	}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}
	t.Cleanup(func() {
		db.Unscoped().Where("user_id = ?", user.ID).Delete(&model.OrgMembership{})
		db.Unscoped().Delete(user)
	})
	return user
}

func cleanupOrgAndLedger(t *testing.T, db *gorm.DB, orgID uuid.UUID) {
	t.Helper()
	t.Cleanup(func() {
		db.Unscoped().Where("org_id = ?", orgID).Delete(&model.CreditLedgerEntry{})
		db.Unscoped().Where("org_id = ?", orgID).Delete(&model.OrgMembership{})
		db.Unscoped().Where("id = ?", orgID).Delete(&model.Org{})
	})
}

func TestCreateUserDefaultOrg_GrantsWelcomeCredits(t *testing.T) {
	db := connectInternalTestDB(t)
	credits := billing.NewCreditsService(db)
	seedFreePlan(t, db, 5000)
	user := seedSignupUser(t, db)

	var org model.Org
	err := db.Transaction(func(tx *gorm.DB) error {
		var e error
		org, e = createUserDefaultOrg(tx, credits, user)
		return e
	})
	if err != nil {
		t.Fatalf("createUserDefaultOrg: %v", err)
	}
	cleanupOrgAndLedger(t, db, org.ID)

	bal, err := credits.Balance(org.ID)
	if err != nil {
		t.Fatalf("balance: %v", err)
	}
	if bal != 5000 {
		t.Errorf("balance = %d, want 5000", bal)
	}

	var entries []model.CreditLedgerEntry
	if err := db.Where("org_id = ? AND reason = ?", org.ID, billing.ReasonWelcomeGrant).Find(&entries).Error; err != nil {
		t.Fatalf("query welcome entries: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("want one welcome_grant entry, got %d", len(entries))
	}
	if entries[0].RefType != billing.RefTypeSignup || entries[0].RefID != user.ID.String() {
		t.Errorf("unexpected ref tagging: type=%q id=%q", entries[0].RefType, entries[0].RefID)
	}
	if entries[0].ExpiresAt != nil {
		t.Errorf("welcome credits must be permanent, got expires_at=%v", entries[0].ExpiresAt)
	}
}

func TestCreateUserDefaultOrg_ZeroWelcomeCreditsSkipsGrant(t *testing.T) {
	db := connectInternalTestDB(t)
	credits := billing.NewCreditsService(db)
	seedFreePlan(t, db, 0)
	user := seedSignupUser(t, db)

	var org model.Org
	err := db.Transaction(func(tx *gorm.DB) error {
		var e error
		org, e = createUserDefaultOrg(tx, credits, user)
		return e
	})
	if err != nil {
		t.Fatalf("createUserDefaultOrg: %v", err)
	}
	cleanupOrgAndLedger(t, db, org.ID)

	bal, _ := credits.Balance(org.ID)
	if bal != 0 {
		t.Errorf("balance = %d, want 0 (no welcome grant configured)", bal)
	}

	var count int64
	db.Model(&model.CreditLedgerEntry{}).Where("org_id = ?", org.ID).Count(&count)
	if count != 0 {
		t.Errorf("ledger should be empty, got %d entries", count)
	}
}

func TestCreateUserDefaultOrg_NoFreePlanRowSucceeds(t *testing.T) {
	// Self-hosted deployments may not seed the plan catalog. The helper must
	// still succeed: signup completes, just without a welcome grant.
	db := connectInternalTestDB(t)
	credits := billing.NewCreditsService(db)

	// Make sure no free-plan row exists for this test.
	db.Unscoped().Where("slug = ?", billing.FreePlanSlug).Delete(&model.Plan{})

	user := seedSignupUser(t, db)

	var org model.Org
	err := db.Transaction(func(tx *gorm.DB) error {
		var e error
		org, e = createUserDefaultOrg(tx, credits, user)
		return e
	})
	if err != nil {
		t.Fatalf("createUserDefaultOrg should not fail when free plan is missing: %v", err)
	}
	cleanupOrgAndLedger(t, db, org.ID)

	bal, _ := credits.Balance(org.ID)
	if bal != 0 {
		t.Errorf("balance = %d, want 0 (no plan, no grant)", bal)
	}
}
