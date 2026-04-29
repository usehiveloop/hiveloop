package subscription_test

import (
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/billing"
	"github.com/usehiveloop/hiveloop/internal/billing/fake"
	subpkg "github.com/usehiveloop/hiveloop/internal/billing/subscription"
	"github.com/usehiveloop/hiveloop/internal/model"
)

//nolint:gosec // local-dev DSN, mirrors other integration tests
const testDBURL = "postgres://hiveloop:localdev@localhost:5433/hiveloop_test?sslmode=disable"

func connectTestDB(t *testing.T) *gorm.DB {
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

type harness struct {
	db        *gorm.DB
	provider  *fake.Provider
	registry  *billing.Registry
	credits   *billing.CreditsService
	service   *subpkg.Service
	orgID     uuid.UUID
	planFree  model.Plan
	planStart model.Plan
	planPro   model.Plan
	planPrem  model.Plan
	now       time.Time
}

// newHarness builds a fresh test harness: a unique org, four plans (free /
// starter / pro / premium, all NGN), a fake provider registered as
// "paystack", and a Service with its clock pinned to a deterministic now.
func newHarness(t *testing.T) *harness {
	t.Helper()
	db := connectTestDB(t)
	provider := fake.New("paystack")
	registry := billing.NewRegistry()
	registry.Register(provider)
	credits := billing.NewCreditsService(db)

	suffix := uuid.NewString()[:8]
	planFree := mustCreatePlan(t, db, model.Plan{Slug: "free-" + suffix, Name: "Free", PriceCents: 0, Currency: "NGN", MonthlyCredits: 100})
	planStart := mustCreatePlan(t, db, model.Plan{Slug: "starter-" + suffix, Name: "Starter", PriceCents: 500_000, Currency: "NGN", MonthlyCredits: 1_000})
	planPro := mustCreatePlan(t, db, model.Plan{Slug: "pro-" + suffix, Name: "Pro", PriceCents: 2_000_000, Currency: "NGN", MonthlyCredits: 5_000})
	planPrem := mustCreatePlan(t, db, model.Plan{Slug: "prem-" + suffix, Name: "Premium", PriceCents: 5_000_000, Currency: "NGN", MonthlyCredits: 20_000})

	org := model.Org{ID: uuid.New(), Name: "billing-test-" + suffix, Active: true, PlanSlug: planStart.Slug}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Second)

	t.Cleanup(func() {
		db.Unscoped().Where("org_id = ?", org.ID).Delete(&model.SubscriptionChangeQuote{})
		db.Unscoped().Where("org_id = ?", org.ID).Delete(&model.Subscription{})
		db.Unscoped().Where("org_id = ?", org.ID).Delete(&model.CreditLedgerEntry{})
		db.Unscoped().Delete(&org)
		for _, p := range []model.Plan{planFree, planStart, planPro, planPrem} {
			db.Unscoped().Delete(&p)
		}
	})

	service := subpkg.NewService(db, registry, credits)
	service.SetClock(func() time.Time { return now })

	return &harness{
		db: db, provider: provider, registry: registry, credits: credits,
		service: service, orgID: org.ID,
		planFree: planFree, planStart: planStart, planPro: planPro, planPrem: planPrem,
		now: now,
	}
}

func mustCreatePlan(t *testing.T, db *gorm.DB, p model.Plan) model.Plan {
	t.Helper()
	p.ID = uuid.New()
	p.Active = true
	if err := db.Create(&p).Error; err != nil {
		t.Fatalf("create plan: %v", err)
	}
	return p
}

func (h *harness) seedSub(t *testing.T, plan model.Plan, periodOffsetSecs int64) model.Subscription {
	t.Helper()
	periodStart := h.now.Add(time.Duration(-periodOffsetSecs) * time.Second)
	periodEnd := periodStart.Add(30 * 24 * time.Hour)
	sub := model.Subscription{
		OrgID:              h.orgID,
		PlanID:             plan.ID,
		Provider:           "paystack",
		ExternalCustomerID: "CUS_test",
		Status:             string(billing.StatusActive),
		CurrentPeriodStart: periodStart,
		CurrentPeriodEnd:   periodEnd,
		AuthorizationCode:  "AUTH_test",
		PaymentChannel:       "card",
	}
	if err := h.db.Create(&sub).Error; err != nil {
		t.Fatalf("seed sub: %v", err)
	}
	return sub
}