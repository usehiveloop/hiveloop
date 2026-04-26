package billing_test

import (
	"errors"
	"os"
	"testing"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/billing"
	"github.com/usehiveloop/hiveloop/internal/model"
)

//nolint:gosec // G101: local-dev DSN, mirrors other integration tests
const testDBURL = "postgres://hiveloop:localdev@localhost:5433/hiveloop_test?sslmode=disable"

func connectCreditsTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = testDBURL
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("connect Postgres (run `make test-setup`): %v", dsn)
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

// seedOrg creates a throwaway org and registers cleanup for it and everything
// in credit_ledger_entries pointing at it.
func seedOrg(t *testing.T, db *gorm.DB) uuid.UUID {
	t.Helper()
	org := model.Org{
		ID:     uuid.New(),
		Name:   "credits-test-" + uuid.NewString(),
		Active: true,
	}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("seed org: %v", err)
	}
	t.Cleanup(func() {
		db.Unscoped().Where("org_id = ?", org.ID).Delete(&model.CreditLedgerEntry{})
		db.Unscoped().Delete(&org)
	})
	return org.ID
}

func TestIntegration_Credits_SpendThenBalance(t *testing.T) {
	db := connectCreditsTestDB(t)
	svc := billing.NewCreditsService(db)
	orgID := seedOrg(t, db)

	if err := svc.Grant(orgID, 1000, billing.ReasonPlanGrant, "plan", "starter", nil); err != nil {
		t.Fatalf("grant: %v", err)
	}
	if err := svc.Spend(orgID, 150, billing.ReasonLLMTokens, "generation", "gen-1"); err != nil {
		t.Fatalf("spend: %v", err)
	}

	bal, err := svc.Balance(orgID)
	if err != nil {
		t.Fatalf("balance: %v", err)
	}
	if bal != 850 {
		t.Errorf("balance after 1000 grant + 150 spend = %d, want 850", bal)
	}
}

func TestIntegration_Credits_IdempotentSpend(t *testing.T) {
	db := connectCreditsTestDB(t)
	svc := billing.NewCreditsService(db)
	orgID := seedOrg(t, db)

	if err := svc.Grant(orgID, 500, billing.ReasonPlanGrant, "plan", "starter", nil); err != nil {
		t.Fatalf("grant: %v", err)
	}

	// First spend: succeeds.
	if err := svc.Spend(orgID, 100, billing.ReasonLLMTokens, "generation", "gen-abc"); err != nil {
		t.Fatalf("first spend: %v", err)
	}
	// Second spend with same (reason, ref_type, ref_id): must be rejected
	// as ErrAlreadyRecorded, and must NOT deduct a second time.
	err := svc.Spend(orgID, 100, billing.ReasonLLMTokens, "generation", "gen-abc")
	if !errors.Is(err, billing.ErrAlreadyRecorded) {
		t.Fatalf("second spend: got %v, want ErrAlreadyRecorded", err)
	}

	bal, err := svc.Balance(orgID)
	if err != nil {
		t.Fatalf("balance: %v", err)
	}
	if bal != 400 {
		t.Errorf("balance after grant 500 + idempotent-duplicate spend 100 = %d, want 400", bal)
	}
}

func TestIntegration_Credits_SpendRejectedWhenInsufficient(t *testing.T) {
	db := connectCreditsTestDB(t)
	svc := billing.NewCreditsService(db)
	orgID := seedOrg(t, db)

	if err := svc.Grant(orgID, 50, billing.ReasonPlanGrant, "plan", "starter", nil); err != nil {
		t.Fatalf("grant: %v", err)
	}

	err := svc.Spend(orgID, 100, billing.ReasonLLMTokens, "generation", "gen-nope")
	if !errors.Is(err, billing.ErrInsufficientCredits) {
		t.Fatalf("overspend: got %v, want ErrInsufficientCredits", err)
	}

	bal, _ := svc.Balance(orgID)
	if bal != 50 {
		t.Errorf("balance after rejected overspend = %d, want 50 (grant unchanged)", bal)
	}
}

func TestIntegration_Credits_SameRefIDDifferentReasonAllowed(t *testing.T) {
	// The idempotency index covers (org_id, reason, ref_type, ref_id). Two
	// spends with the same ref_id but different reasons must BOTH succeed
	// — e.g. an agent_run charge and an llm_tokens charge for the same
	// conversation.
	db := connectCreditsTestDB(t)
	svc := billing.NewCreditsService(db)
	orgID := seedOrg(t, db)

	if err := svc.Grant(orgID, 1000, billing.ReasonPlanGrant, "plan", "starter", nil); err != nil {
		t.Fatalf("grant: %v", err)
	}
	if err := svc.Spend(orgID, 10, billing.ReasonAgentRun, "conversation", "conv-1"); err != nil {
		t.Fatalf("agent_run spend: %v", err)
	}
	if err := svc.Spend(orgID, 20, billing.ReasonLLMTokens, "conversation", "conv-1"); err != nil {
		t.Fatalf("llm_tokens spend: %v", err)
	}

	bal, _ := svc.Balance(orgID)
	if bal != 970 {
		t.Errorf("balance after 1000 - 10 - 20 = %d, want 970", bal)
	}
}

func TestIntegration_Credits_EmptyRefIDNotIdempotencyScoped(t *testing.T) {
	// The idempotency unique index is partial: WHERE ref_id != ''.
	// Spends without a ref_id (e.g. manual adjustments) should not collide
	// with each other. Verify two such spends both succeed.
	db := connectCreditsTestDB(t)
	svc := billing.NewCreditsService(db)
	orgID := seedOrg(t, db)

	if err := svc.Grant(orgID, 200, billing.ReasonPlanGrant, "plan", "starter", nil); err != nil {
		t.Fatalf("grant: %v", err)
	}
	if err := svc.Spend(orgID, 10, billing.ReasonAdjustment, "", ""); err != nil {
		t.Fatalf("first adjustment: %v", err)
	}
	if err := svc.Spend(orgID, 15, billing.ReasonAdjustment, "", ""); err != nil {
		t.Fatalf("second adjustment: %v", err)
	}
	bal, _ := svc.Balance(orgID)
	if bal != 175 {
		t.Errorf("balance after 200 - 10 - 15 = %d, want 175", bal)
	}
}
