package tasks_test

import (
	"testing"

	"github.com/usehivy/hivy/internal/billing"
	"github.com/usehivy/hivy/internal/model"
)

func TestBatch_DeductsSingleRow(t *testing.T) {
	db := connectDB(t)
	orgID, credID := seedOrgWithCredentialAndCredits(t, db, 10_000)

	id := insertGeneration(t, db, orgID, credID, defaultGenOpts())
	runBatch(t, db)

	g := loadGen(t, db, id)
	if g.BilledAt == nil {
		t.Fatal("billed_at should be set after batch run")
	}
	if g.BillingError != "" {
		t.Errorf("expected no billing error, got %q", g.BillingError)
	}

	bal, _ := billing.NewCreditsService(db).Balance(orgID)
	if bal != 10_000-testCreditsPerGen {
		t.Errorf("balance after single deduction = %d, want %d", bal, 10_000-testCreditsPerGen)
	}
}

func TestBatch_GroupsPerOrgIntoOneLedgerEntry(t *testing.T) {
	db := connectDB(t)
	orgID, credID := seedOrgWithCredentialAndCredits(t, db, 10_000)

	const N = 7
	for i := 0; i < N; i++ {
		insertGeneration(t, db, orgID, credID, defaultGenOpts())
	}

	runBatch(t, db)

	var ledgerCount int64
	db.Model(&model.CreditLedgerEntry{}).
		Where("org_id = ? AND reason = ?", orgID, billing.ReasonLLMTokens).
		Count(&ledgerCount)
	if ledgerCount != 1 {
		t.Errorf("expected 1 ledger entry for the batch, got %d", ledgerCount)
	}

	bal, _ := billing.NewCreditsService(db).Balance(orgID)
	want := int64(10_000 - N*testCreditsPerGen)
	if bal != want {
		t.Errorf("balance after %d-row batch = %d, want %d", N, bal, want)
	}
}

func TestBatch_MultiOrgIndependentDeductions(t *testing.T) {
	db := connectDB(t)
	orgA, credA := seedOrgWithCredentialAndCredits(t, db, 10_000)
	orgB, credB := seedOrgWithCredentialAndCredits(t, db, 5_000)
	orgC, credC := seedOrgWithCredentialAndCredits(t, db, 1_000)

	insertGeneration(t, db, orgA, credA, defaultGenOpts())
	insertGeneration(t, db, orgA, credA, defaultGenOpts())
	insertGeneration(t, db, orgB, credB, defaultGenOpts())
	insertGeneration(t, db, orgC, credC, defaultGenOpts())

	runBatch(t, db)

	credits := billing.NewCreditsService(db)
	balA, _ := credits.Balance(orgA)
	balB, _ := credits.Balance(orgB)
	balC, _ := credits.Balance(orgC)

	if balA != 10_000-2*testCreditsPerGen {
		t.Errorf("orgA balance = %d, want %d", balA, 10_000-2*testCreditsPerGen)
	}
	if balB != 5_000-testCreditsPerGen {
		t.Errorf("orgB balance = %d, want %d", balB, 5_000-testCreditsPerGen)
	}
	if balC != 1_000-testCreditsPerGen {
		t.Errorf("orgC balance = %d, want %d", balC, 1_000-testCreditsPerGen)
	}
}

func TestBatch_IdempotentSecondRun(t *testing.T) {
	db := connectDB(t)
	orgID, credID := seedOrgWithCredentialAndCredits(t, db, 10_000)
	insertGeneration(t, db, orgID, credID, defaultGenOpts())

	runBatch(t, db)
	balAfterFirst, _ := billing.NewCreditsService(db).Balance(orgID)

	runBatch(t, db)
	balAfterSecond, _ := billing.NewCreditsService(db).Balance(orgID)

	if balAfterFirst != balAfterSecond {
		t.Errorf("second batch run double-deducted: %d → %d", balAfterFirst, balAfterSecond)
	}
}

func TestBatch_SkipsBYOKAndZeroTokenRows(t *testing.T) {
	db := connectDB(t)
	orgID, credID := seedOrgWithCredentialAndCredits(t, db, 10_000)

	byok := defaultGenOpts()
	byok.IsSystem = false
	byokID := insertGeneration(t, db, orgID, credID, byok)

	zero := defaultGenOpts()
	zero.InputTokens = 0
	zero.OutputTokens = 0
	zeroID := insertGeneration(t, db, orgID, credID, zero)

	runBatch(t, db)

	if loadGen(t, db, byokID).BilledAt != nil {
		t.Error("BYOK row should not be touched by batch")
	}
	if loadGen(t, db, zeroID).BilledAt != nil {
		t.Error("zero-token row should not be touched by batch")
	}
	bal, _ := billing.NewCreditsService(db).Balance(orgID)
	if bal != 10_000 {
		t.Errorf("balance changed despite no billable rows: %d", bal)
	}
}

func TestBatch_UnknownModelMarksErrorWithoutDeduction(t *testing.T) {
	db := connectDB(t)
	orgID, credID := seedOrgWithCredentialAndCredits(t, db, 10_000)

	bad := defaultGenOpts()
	bad.Model = "definitely-not-a-real-model"
	badID := insertGeneration(t, db, orgID, credID, bad)
	goodID := insertGeneration(t, db, orgID, credID, defaultGenOpts())

	runBatch(t, db)

	bg := loadGen(t, db, badID)
	if bg.BilledAt == nil {
		t.Error("unknown-model row should be marked billed_at to leave the queue")
	}
	if bg.BillingError == "" {
		t.Error("unknown-model row should have billing_error set")
	}
	if loadGen(t, db, goodID).BillingError != "" {
		t.Error("good row should not be tagged with the bad row's error")
	}
	bal, _ := billing.NewCreditsService(db).Balance(orgID)
	if bal != 10_000-testCreditsPerGen {
		t.Errorf("only the good row should have deducted: balance = %d, want %d", bal, 10_000-testCreditsPerGen)
	}
}

func TestBatch_ResolvesModelFromAgentWhenGenerationModelEmpty(t *testing.T) {
	db := connectDB(t)
	orgID, credID := seedOrgWithCredentialAndCredits(t, db, 10_000)
	jti := seedAgentWithToken(t, db, orgID, credID, testModel)

	opts := defaultGenOpts()
	opts.Model = ""
	opts.TokenJTI = jti
	id := insertGeneration(t, db, orgID, credID, opts)

	runBatch(t, db)

	g := loadGen(t, db, id)
	if g.BillingError != "" {
		t.Errorf("expected agent fallback to resolve model, got error %q", g.BillingError)
	}
	bal, _ := billing.NewCreditsService(db).Balance(orgID)
	if bal != 10_000-testCreditsPerGen {
		t.Errorf("balance after agent-resolved deduction = %d, want %d", bal, 10_000-testCreditsPerGen)
	}
}

func TestBatch_UnresolvableModelMarksError(t *testing.T) {
	db := connectDB(t)
	orgID, credID := seedOrgWithCredentialAndCredits(t, db, 10_000)

	opts := defaultGenOpts()
	opts.Model = ""
	id := insertGeneration(t, db, orgID, credID, opts)

	runBatch(t, db)

	g := loadGen(t, db, id)
	if g.BilledAt == nil {
		t.Error("unresolvable row should still leave the queue")
	}
	if g.BillingError == "" {
		t.Error("unresolvable row should be tagged with billing_error")
	}
	bal, _ := billing.NewCreditsService(db).Balance(orgID)
	if bal != 10_000 {
		t.Errorf("no deduction expected: balance = %d", bal)
	}
}

func TestBatch_InsufficientBalanceMarksRowsAndContinues(t *testing.T) {
	db := connectDB(t)
	orgPoor, credPoor := seedOrgWithCredentialAndCredits(t, db, 10) // far less than 123
	orgRich, credRich := seedOrgWithCredentialAndCredits(t, db, 10_000)

	poorID := insertGeneration(t, db, orgPoor, credPoor, defaultGenOpts())
	richID := insertGeneration(t, db, orgRich, credRich, defaultGenOpts())

	runBatch(t, db)

	pg := loadGen(t, db, poorID)
	if pg.BillingError == "" {
		t.Error("poor org row should have billing_error set")
	}
	if pg.BilledAt == nil {
		t.Error("poor org row should still be marked billed to exit the queue")
	}

	rg := loadGen(t, db, richID)
	if rg.BillingError != "" {
		t.Errorf("rich org row should not inherit poor org's error: %q", rg.BillingError)
	}

	bp, _ := billing.NewCreditsService(db).Balance(orgPoor)
	br, _ := billing.NewCreditsService(db).Balance(orgRich)
	if bp != 10 {
		t.Errorf("poor org balance changed despite insufficient credits: %d", bp)
	}
	if br != 10_000-testCreditsPerGen {
		t.Errorf("rich org should be deducted normally: %d", br)
	}
}
