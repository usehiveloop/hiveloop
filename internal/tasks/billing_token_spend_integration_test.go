package tasks_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/billing"
	"github.com/usehiveloop/hiveloop/internal/model"
	"github.com/usehiveloop/hiveloop/internal/tasks"
)

// These tests share the testDBURL const and connectDB helper defined in
// agent_cleanup_test.go (same _test package).

func seedTaskTestOrg(t *testing.T, db *gorm.DB) uuid.UUID {
	t.Helper()
	org := model.Org{
		ID:     uuid.New(),
		Name:   "tasks-test-" + uuid.NewString(),
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

// buildTask marshals a payload into the asynq.Task format the handler expects.
func buildTask(t *testing.T, p tasks.BillingTokenSpendPayload) *asynq.Task {
	t.Helper()
	body, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	return asynq.NewTask("billing:token_spend", body)
}

func TestIntegration_TokenSpend_DeductsFromLedger(t *testing.T) {
	db := connectDB(t)
	credits := billing.NewCreditsService(db)
	orgID := seedTaskTestOrg(t, db)

	if err := credits.Grant(orgID, 10_000, billing.ReasonPlanGrant, "plan", "pro"); err != nil {
		t.Fatalf("grant: %v", err)
	}

	handler := tasks.NewBillingTokenSpendHandler(credits)
	task := buildTask(t, tasks.BillingTokenSpendPayload{
		OrgID:        orgID,
		GenerationID: "gen_" + uuid.NewString(),
		Model:        "glm-5.1-precision",
		InputTokens:  30_000,
		OutputTokens: 4_000,
	})

	if err := handler.Handle(context.Background(), task); err != nil {
		t.Fatalf("Handle: %v", err)
	}

	// Typical run (30k/4k) = 123 credits per pricing_test.go.
	// Balance starts at 10_000, expect 9_877.
	bal, err := credits.Balance(orgID)
	if err != nil {
		t.Fatalf("balance: %v", err)
	}
	if bal != 9_877 {
		t.Errorf("balance after typical spend = %d, want 9877", bal)
	}
}

func TestIntegration_TokenSpend_IdempotentOnRetry(t *testing.T) {
	db := connectDB(t)
	credits := billing.NewCreditsService(db)
	orgID := seedTaskTestOrg(t, db)

	if err := credits.Grant(orgID, 10_000, billing.ReasonPlanGrant, "plan", "pro"); err != nil {
		t.Fatalf("grant: %v", err)
	}

	handler := tasks.NewBillingTokenSpendHandler(credits)
	genID := "gen_" + uuid.NewString()
	payload := tasks.BillingTokenSpendPayload{
		OrgID:        orgID,
		GenerationID: genID,
		Model:        "glm-5.1-precision",
		InputTokens:  30_000,
		OutputTokens: 4_000,
	}

	// First attempt.
	if err := handler.Handle(context.Background(), buildTask(t, payload)); err != nil {
		t.Fatalf("first Handle: %v", err)
	}
	// Asynq retry — same payload, same generation id. Must treat as success
	// (no error returned) and MUST NOT double-deduct.
	if err := handler.Handle(context.Background(), buildTask(t, payload)); err != nil {
		t.Fatalf("retry Handle: got %v, want nil", err)
	}

	bal, _ := credits.Balance(orgID)
	if bal != 9_877 {
		t.Errorf("retried spend double-deducted: balance = %d, want 9877", bal)
	}
}

func TestIntegration_TokenSpend_ZeroTokensSkipsDeduction(t *testing.T) {
	db := connectDB(t)
	credits := billing.NewCreditsService(db)
	orgID := seedTaskTestOrg(t, db)

	if err := credits.Grant(orgID, 1_000, billing.ReasonPlanGrant, "plan", "starter"); err != nil {
		t.Fatalf("grant: %v", err)
	}
	handler := tasks.NewBillingTokenSpendHandler(credits)
	task := buildTask(t, tasks.BillingTokenSpendPayload{
		OrgID:        orgID,
		GenerationID: "gen_zero",
		Model:        "glm-5.1-precision",
		InputTokens:  0,
		OutputTokens: 0,
	})

	if err := handler.Handle(context.Background(), task); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	bal, _ := credits.Balance(orgID)
	if bal != 1_000 {
		t.Errorf("zero-token task deducted %d credits; should have skipped", 1_000-bal)
	}
}

func TestIntegration_TokenSpend_UnknownModelSkipRetries(t *testing.T) {
	db := connectDB(t)
	credits := billing.NewCreditsService(db)
	orgID := seedTaskTestOrg(t, db)
	if err := credits.Grant(orgID, 1_000, billing.ReasonPlanGrant, "plan", "starter"); err != nil {
		t.Fatalf("grant: %v", err)
	}

	handler := tasks.NewBillingTokenSpendHandler(credits)
	task := buildTask(t, tasks.BillingTokenSpendPayload{
		OrgID:        orgID,
		GenerationID: "gen_unknown_model",
		Model:        "claude-3-nonexistent",
		InputTokens:  1000,
		OutputTokens: 100,
	})

	err := handler.Handle(context.Background(), task)
	if err == nil {
		t.Fatal("expected error for unknown model")
	}
	if !errors.Is(err, asynq.SkipRetry) {
		t.Errorf("unknown model should be SkipRetry (don't retry forever), got %v", err)
	}
	// Balance untouched.
	bal, _ := credits.Balance(orgID)
	if bal != 1_000 {
		t.Errorf("unknown model deducted %d credits; should be 0", 1_000-bal)
	}
}

func TestIntegration_TokenSpend_InsufficientBalanceSkipRetries(t *testing.T) {
	// Simulates the race: proxy pre-check passed, but concurrent requests
	// drained the balance before this task ran. The handler should skip
	// retrying (we already served the inference) and log a warning.
	db := connectDB(t)
	credits := billing.NewCreditsService(db)
	orgID := seedTaskTestOrg(t, db)

	// Grant far less than the call costs.
	if err := credits.Grant(orgID, 10, billing.ReasonPlanGrant, "plan", "starter"); err != nil {
		t.Fatalf("grant: %v", err)
	}

	handler := tasks.NewBillingTokenSpendHandler(credits)
	task := buildTask(t, tasks.BillingTokenSpendPayload{
		OrgID:        orgID,
		GenerationID: "gen_broke",
		Model:        "glm-5.1-precision",
		InputTokens:  200_000,
		OutputTokens: 15_000,
	})

	err := handler.Handle(context.Background(), task)
	if err == nil {
		t.Fatal("expected error for insufficient balance")
	}
	if !errors.Is(err, asynq.SkipRetry) {
		t.Errorf("insufficient balance should be SkipRetry, got %v", err)
	}
	// Balance unchanged — spend was rejected.
	bal, _ := credits.Balance(orgID)
	if bal != 10 {
		t.Errorf("balance after rejected spend = %d, want 10", bal)
	}
}

func TestIntegration_TokenSpend_MalformedPayloadSkipRetries(t *testing.T) {
	db := connectDB(t)
	credits := billing.NewCreditsService(db)

	handler := tasks.NewBillingTokenSpendHandler(credits)
	malformed := asynq.NewTask("billing:token_spend", []byte("not json"))

	err := handler.Handle(context.Background(), malformed)
	if err == nil {
		t.Fatal("expected error for malformed payload")
	}
	if !errors.Is(err, asynq.SkipRetry) {
		t.Errorf("malformed payload should be SkipRetry, got %v", err)
	}
}
