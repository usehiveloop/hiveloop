package tasks_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/billing"
	"github.com/usehiveloop/hiveloop/internal/config"
	"github.com/usehiveloop/hiveloop/internal/model"
	"github.com/usehiveloop/hiveloop/internal/tasks"
)

// 30k input + 4k output @ glm-5.1-precision = 123 credits per pricing.go.
const (
	testInputTokens   = 30_000
	testOutputTokens  = 4_000
	testCreditsPerGen = 123
	testModel         = "glm-5.1-precision"
)

// genFixture is the projection we read back to assert state.
type genFixture struct {
	ID           string
	BilledAt     *time.Time
	BillingError string
}

type genOpts struct {
	Model        string
	IsSystem     bool
	InputTokens  int
	OutputTokens int
	TokenJTI     string
}

func defaultGenOpts() genOpts {
	return genOpts{
		Model:        testModel,
		IsSystem:     true,
		InputTokens:  testInputTokens,
		OutputTokens: testOutputTokens,
	}
}

func insertGeneration(t *testing.T, db *gorm.DB, orgID uuid.UUID, credID uuid.UUID, opts genOpts) string {
	t.Helper()
	id := "gen_" + uuid.NewString()
	if opts.TokenJTI == "" {
		opts.TokenJTI = uuid.NewString()
	}
	if err := db.Create(&model.Generation{
		ID:           id,
		OrgID:        orgID,
		CredentialID: credID,
		TokenJTI:     opts.TokenJTI,
		ProviderID:   "test-provider",
		Model:        opts.Model,
		IsSystem:     opts.IsSystem,
		InputTokens:  opts.InputTokens,
		OutputTokens: opts.OutputTokens,
		CreatedAt:    time.Now().UTC(),
	}).Error; err != nil {
		t.Fatalf("insert generation: %v", err)
	}
	return id
}

func loadGen(t *testing.T, db *gorm.DB, id string) genFixture {
	t.Helper()
	var g model.Generation
	if err := db.Where("id = ?", id).First(&g).Error; err != nil {
		t.Fatalf("load generation %s: %v", id, err)
	}
	return genFixture{ID: g.ID, BilledAt: g.BilledAt, BillingError: g.BillingError}
}

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
		db.Unscoped().Where("org_id = ?", org.ID).Delete(&model.Generation{})
		db.Unscoped().Where("org_id = ?", org.ID).Delete(&model.CreditLedgerEntry{})
		db.Unscoped().Delete(&org)
	})
	return org.ID
}

func seedOrgWithCredentialAndCredits(t *testing.T, db *gorm.DB, granted int64) (uuid.UUID, uuid.UUID) {
	t.Helper()
	orgID := seedTaskTestOrg(t, db)

	cred := model.Credential{
		ID:           uuid.New(),
		OrgID:        orgID,
		ProviderID:   "test-provider",
		BaseURL:      "https://example.test",
		AuthScheme:   "bearer",
		EncryptedKey: []byte("test"),
		WrappedDEK:   []byte("test"),
	}
	if err := db.Create(&cred).Error; err != nil {
		t.Fatalf("seed credential: %v", err)
	}
	t.Cleanup(func() { db.Unscoped().Delete(&cred) })

	if granted > 0 {
		credits := billing.NewCreditsService(db)
		if err := credits.Grant(orgID, granted, billing.ReasonPlanGrant, "plan", "test", nil); err != nil {
			t.Fatalf("grant: %v", err)
		}
	}
	return orgID, cred.ID
}

func seedAgentWithToken(t *testing.T, db *gorm.DB, orgID, credID uuid.UUID, agentModel string) string {
	t.Helper()
	agent := model.Agent{
		OrgID:        &orgID,
		Name:         "billing-test-" + uuid.NewString()[:8],
		Model:        agentModel,
		SystemPrompt: "x",
		Status:       "active",
	}
	if err := db.Create(&agent).Error; err != nil {
		t.Fatalf("seed agent: %v", err)
	}
	jti := uuid.NewString()
	tok := model.Token{
		OrgID:        orgID,
		CredentialID: credID,
		JTI:          jti,
		ExpiresAt:    time.Now().Add(time.Hour),
		Meta:         model.JSON{"agent_id": agent.ID.String(), "type": "agent_proxy"},
	}
	if err := db.Create(&tok).Error; err != nil {
		t.Fatalf("seed token: %v", err)
	}
	t.Cleanup(func() {
		db.Unscoped().Delete(&tok)
		db.Unscoped().Delete(&agent)
	})
	return jti
}

func runBatch(t *testing.T, db *gorm.DB) {
	t.Helper()
	credits := billing.NewCreditsService(db)
	handler := tasks.NewBillingBatchProcessHandler(db, credits)
	if err := handler.Handle(context.Background(), asynq.NewTask(tasks.TypeBillingBatchProcess, nil)); err != nil {
		t.Fatalf("batch handler: %v", err)
	}
}

// ----------------------------------------------------------------------------

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
	expectedDeduction := int64(perOrg * testCreditsPerGen)
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
	want := int64(100_000 - N*testCreditsPerGen)
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

func TestBatch_RegisteredAsPeriodicTask(t *testing.T) {
	configs := tasks.PeriodicTaskConfigs(&config.Config{}, nil)
	for _, c := range configs {
		if c.Task.Type() == tasks.TypeBillingBatchProcess {
			if c.Cronspec != "@every 30s" {
				t.Errorf("billing batch cronspec = %q, want @every 30s", c.Cronspec)
			}
			return
		}
	}
	t.Fatal("billing batch process not registered as a periodic task")
}
