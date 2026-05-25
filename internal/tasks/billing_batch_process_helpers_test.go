package tasks_test

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/billing"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/tasks"
)

const (
	testInputTokens  = 30_000
	testOutputTokens = 4_000
	testCostPerGen   = 0.03071
	testModel        = "deepseek-v4-flash"
)

// genFixture is the projection we read back to assert state.
type genFixture struct {
	ID                string
	BilledAt          *time.Time
	BillingError      string
	CreditsDebited    int64
	BillingCostSource string
	Cost              float64
}

type genOpts struct {
	Model        string
	ProviderID   string
	IsSystem     bool
	InputTokens  int
	OutputTokens int
	CachedTokens int
	Cost         float64
	TokenJTI     string
}

func defaultGenOpts() genOpts {
	return genOpts{
		Model:        testModel,
		ProviderID:   "test-provider",
		IsSystem:     true,
		InputTokens:  testInputTokens,
		OutputTokens: testOutputTokens,
		Cost:         testCostPerGen,
	}
}

func creditsForTestCost(count int) int64 {
	return int64(math.Ceil(float64(count) * testCostPerGen / billing.CreditUSDValue))
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
		ProviderID:   opts.ProviderID,
		Model:        opts.Model,
		IsSystem:     opts.IsSystem,
		InputTokens:  opts.InputTokens,
		OutputTokens: opts.OutputTokens,
		CachedTokens: opts.CachedTokens,
		Cost:         opts.Cost,
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
	return genFixture{
		ID:                g.ID,
		BilledAt:          g.BilledAt,
		BillingError:      g.BillingError,
		CreditsDebited:    g.CreditsDebited,
		BillingCostSource: g.BillingCostSource,
		Cost:              g.Cost,
	}
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
		OrgID:        &orgID,
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
	agent := model.Employee{
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
		Meta:         model.JSON{"employee_id": agent.ID.String(), "type": "employee_proxy"},
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
