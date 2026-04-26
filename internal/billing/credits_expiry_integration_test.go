package billing_test

import (
	"context"
	"testing"
	"time"

	"github.com/usehiveloop/hiveloop/internal/billing"
	"github.com/usehiveloop/hiveloop/internal/model"
)

func TestIntegration_Credits_SweepExpiredGrant_FullForfeit(t *testing.T) {
	db := connectCreditsTestDB(t)
	svc := billing.NewCreditsService(db)
	orgID := seedOrg(t, db)

	past := time.Now().Add(-time.Hour)
	if err := svc.Grant(orgID, 1000, billing.ReasonPlanGrant, "subscription", "sub_a", &past); err != nil {
		t.Fatalf("grant: %v", err)
	}

	if err := svc.SweepOrgExpiredGrants(context.Background(), orgID, time.Now()); err != nil {
		t.Fatalf("sweep: %v", err)
	}

	bal, _ := svc.Balance(orgID)
	if bal != 0 {
		t.Errorf("balance after expiry sweep = %d, want 0", bal)
	}

	var expiryRows []model.CreditLedgerEntry
	if err := db.Where("org_id = ? AND reason = ?", orgID, billing.ReasonExpiry).Find(&expiryRows).Error; err != nil {
		t.Fatalf("load expiry rows: %v", err)
	}
	if len(expiryRows) != 1 || expiryRows[0].Amount != -1000 {
		t.Errorf("want one expiry row of -1000, got %+v", expiryRows)
	}
}

func TestIntegration_Credits_SweepExpiredGrant_PartialForfeit(t *testing.T) {
	db := connectCreditsTestDB(t)
	svc := billing.NewCreditsService(db)
	orgID := seedOrg(t, db)

	past := time.Now().Add(-time.Hour)
	if err := svc.Grant(orgID, 1000, billing.ReasonPlanGrant, "subscription", "sub_b", &past); err != nil {
		t.Fatalf("grant: %v", err)
	}
	if err := svc.Spend(orgID, 300, billing.ReasonLLMTokens, "generation", "gen-1"); err != nil {
		t.Fatalf("spend: %v", err)
	}

	if err := svc.SweepOrgExpiredGrants(context.Background(), orgID, time.Now()); err != nil {
		t.Fatalf("sweep: %v", err)
	}

	bal, _ := svc.Balance(orgID)
	if bal != 0 {
		t.Errorf("balance after partial spend + expiry = %d, want 0", bal)
	}
}

func TestIntegration_Credits_SweepExpiredGrant_PermanentTopupSurvives(t *testing.T) {
	db := connectCreditsTestDB(t)
	svc := billing.NewCreditsService(db)
	orgID := seedOrg(t, db)

	past := time.Now().Add(-time.Hour)
	if err := svc.Grant(orgID, 200, billing.ReasonTopup, "topup", "topup-1", nil); err != nil {
		t.Fatalf("topup: %v", err)
	}
	if err := svc.Grant(orgID, 1000, billing.ReasonPlanGrant, "subscription", "sub_c", &past); err != nil {
		t.Fatalf("grant: %v", err)
	}
	if err := svc.Spend(orgID, 150, billing.ReasonLLMTokens, "generation", "gen-2"); err != nil {
		t.Fatalf("spend: %v", err)
	}

	if err := svc.SweepOrgExpiredGrants(context.Background(), orgID, time.Now()); err != nil {
		t.Fatalf("sweep: %v", err)
	}

	bal, _ := svc.Balance(orgID)
	if bal != 200 {
		t.Errorf("permanent topup should survive: balance = %d, want 200", bal)
	}
}

func TestIntegration_Credits_SweepExpiredGrant_Idempotent(t *testing.T) {
	db := connectCreditsTestDB(t)
	svc := billing.NewCreditsService(db)
	orgID := seedOrg(t, db)

	past := time.Now().Add(-time.Hour)
	if err := svc.Grant(orgID, 500, billing.ReasonPlanGrant, "subscription", "sub_d", &past); err != nil {
		t.Fatalf("grant: %v", err)
	}

	for i := 0; i < 3; i++ {
		if err := svc.SweepOrgExpiredGrants(context.Background(), orgID, time.Now()); err != nil {
			t.Fatalf("sweep iter %d: %v", i, err)
		}
	}

	var rows []model.CreditLedgerEntry
	if err := db.Where("org_id = ? AND reason = ?", orgID, billing.ReasonExpiry).Find(&rows).Error; err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(rows) != 1 {
		t.Errorf("want exactly one expiry row across re-runs, got %d", len(rows))
	}
}

func TestIntegration_Credits_SweepExpiredGrant_FutureExpiryUntouched(t *testing.T) {
	db := connectCreditsTestDB(t)
	svc := billing.NewCreditsService(db)
	orgID := seedOrg(t, db)

	future := time.Now().Add(24 * time.Hour)
	if err := svc.Grant(orgID, 500, billing.ReasonPlanGrant, "subscription", "sub_e", &future); err != nil {
		t.Fatalf("grant: %v", err)
	}

	if err := svc.SweepOrgExpiredGrants(context.Background(), orgID, time.Now()); err != nil {
		t.Fatalf("sweep: %v", err)
	}

	bal, _ := svc.Balance(orgID)
	if bal != 500 {
		t.Errorf("non-expired grant should be untouched: balance = %d, want 500", bal)
	}
}
