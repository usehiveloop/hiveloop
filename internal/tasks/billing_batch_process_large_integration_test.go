package tasks_test

import (
	"context"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"

	"github.com/usehivy/hivy/internal/billing"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/tasks"
)

func TestBatch_LargeBatchEndToEnd(t *testing.T) {
	if testing.Short() {
		t.Skip("large batch test")
	}
	db := connectDB(t)

	// 5 orgs × 250 generations = 1250 rows. The handler caps at billingBatchSize (1000)
	// per call, so we expect the first run to bill 1000, the second to finish the rest.
	const orgsCount = 5
	const perOrg = 250
	type orgFixture struct {
		id    uuid.UUID
		cred  uuid.UUID
		grant int64
	}
	orgs := make([]orgFixture, orgsCount)
	for i := 0; i < orgsCount; i++ {
		orgs[i].grant = 1_000_000
		orgs[i].id, orgs[i].cred = seedOrgWithCredentialAndCredits(t, db, orgs[i].grant)
		for j := 0; j < perOrg; j++ {
			insertGeneration(t, db, orgs[i].id, orgs[i].cred, defaultGenOpts())
		}
	}

	totalRows := orgsCount * perOrg
	orgIDs := make([]uuid.UUID, len(orgs))
	for i, o := range orgs {
		orgIDs[i] = o.id
	}

	for runs := 0; runs < 5; runs++ {
		var unbilled int64
		db.Model(&model.Generation{}).
			Where("org_id IN ? AND billed_at IS NULL AND is_system = TRUE", orgIDs).
			Count(&unbilled)
		if unbilled == 0 {
			break
		}
		runBatch(t, db)
	}

	var unbilled int64
	db.Model(&model.Generation{}).
		Where("org_id IN ? AND billed_at IS NULL AND is_system = TRUE", orgIDs).
		Count(&unbilled)
	if unbilled != 0 {
		t.Errorf("after multiple batches, %d rows still unbilled (started with %d)", unbilled, totalRows)
	}

	credits := billing.NewCreditsService(db)
	expectedDeduction := int64(creditsForTestCost(perOrg))
	for i, o := range orgs {
		bal, _ := credits.Balance(o.id)
		if bal != o.grant-expectedDeduction {
			t.Errorf("org %d: balance = %d, want %d", i, bal, o.grant-expectedDeduction)
		}
	}
}

func TestBatch_ConcurrentRunsDoNotDoubleDeduct(t *testing.T) {
	db := connectDB(t)
	orgID, credID := seedOrgWithCredentialAndCredits(t, db, 100_000)

	const N = 50
	for i := 0; i < N; i++ {
		insertGeneration(t, db, orgID, credID, defaultGenOpts())
	}

	const workers = 4
	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			credits := billing.NewCreditsService(db)
			handler := tasks.NewBillingBatchProcessHandler(db, credits)
			_ = handler.Handle(context.Background(), asynq.NewTask(tasks.TypeBillingBatchProcess, nil))
		}()
	}
	wg.Wait()

	bal, _ := billing.NewCreditsService(db).Balance(orgID)
	want := int64(100_000 - creditsForTestCost(N))
	if bal != want {
		t.Errorf("concurrent runs deducted incorrectly: balance = %d, want %d", bal, want)
	}

	var ledgerCount int64
	db.Model(&model.CreditLedgerEntry{}).
		Where("org_id = ? AND reason = ?", orgID, billing.ReasonLLMTokens).
		Count(&ledgerCount)
	if ledgerCount > workers {
		t.Errorf("expected at most %d ledger entries (one per worker that claimed work), got %d", workers, ledgerCount)
	}

	var unbilled int64
	db.Model(&model.Generation{}).Where("org_id = ? AND billed_at IS NULL", orgID).Count(&unbilled)
	if unbilled != 0 {
		t.Errorf("%d rows still unbilled after concurrent runs", unbilled)
	}
}
