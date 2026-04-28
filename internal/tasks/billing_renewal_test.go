package tasks_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/billing"
	"github.com/usehiveloop/hiveloop/internal/billing/fake"
	"github.com/usehiveloop/hiveloop/internal/billing/subscription"
	"github.com/usehiveloop/hiveloop/internal/enqueue"
	"github.com/usehiveloop/hiveloop/internal/model"
	"github.com/usehiveloop/hiveloop/internal/tasks"
)

func connectBillingTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		//nolint:gosec // local-dev DSN, mirrors other integration tests
		dsn = "postgres://hiveloop:localdev@localhost:5433/hiveloop_test?sslmode=disable"
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

// renewalHarness wires the sweep + per-sub handlers against a real DB and
// the in-memory mock enqueuer so tests can inspect what would be dispatched.
type renewalHarness struct {
	db       *gorm.DB
	provider *fake.Provider
	enqueuer *enqueue.MockClient
	service  *subscription.Service
	sweep    *tasks.BillingRenewSweepHandler
	renew    *tasks.BillingRenewSubscriptionHandler
	now      time.Time
	orgID    uuid.UUID
	plan     model.Plan
}

func newRenewalHarness(t *testing.T) *renewalHarness {
	t.Helper()
	db := connectBillingTestDB(t)

	provider := fake.New("paystack")
	registry := billing.NewRegistry()
	registry.Register(provider)
	credits := billing.NewCreditsService(db)
	enqueuer := &enqueue.MockClient{}

	now := time.Now().UTC().Truncate(time.Second)
	service := subscription.NewService(db, registry, credits)
	service.SetClock(func() time.Time { return now })

	suffix := uuid.NewString()[:8]
	plan := model.Plan{ID: uuid.New(), Slug: "ren-" + suffix, Name: "Pro", PriceCents: 2_000_000, Currency: "NGN", MonthlyCredits: 5_000, Active: true}
	if err := db.Create(&plan).Error; err != nil {
		t.Fatalf("plan: %v", err)
	}

	user := model.User{ID: uuid.New(), Email: "renw-" + suffix + "@test.com", Name: "Renew"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("user: %v", err)
	}
	org := model.Org{ID: uuid.New(), Name: "renw-" + suffix, Active: true}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("org: %v", err)
	}
	if err := db.Create(&model.OrgMembership{UserID: user.ID, OrgID: org.ID, Role: "owner"}).Error; err != nil {
		t.Fatalf("membership: %v", err)
	}

	t.Cleanup(func() {
		db.Where("org_id = ?", org.ID).Delete(&model.SubscriptionChangeQuote{})
		db.Where("org_id = ?", org.ID).Delete(&model.Subscription{})
		db.Where("org_id = ?", org.ID).Delete(&model.OrgMembership{})
		db.Where("org_id = ?", org.ID).Delete(&model.CreditLedgerEntry{})
		db.Where("id = ?", plan.ID).Delete(&model.Plan{})
		db.Where("id = ?", org.ID).Delete(&model.Org{})
		db.Where("id = ?", user.ID).Delete(&model.User{})
	})

	return &renewalHarness{
		db: db, provider: provider, enqueuer: enqueuer, service: service,
		sweep: tasks.NewBillingRenewSweepHandler(db, enqueuer),
		renew: tasks.NewBillingRenewSubscriptionHandler(service),
		now:  now,
		orgID: org.ID,
		plan: plan,
	}
}

func (h *renewalHarness) seedDueSub(t *testing.T) model.Subscription {
	t.Helper()
	end := time.Now().UTC().Add(-time.Minute)
	sub := model.Subscription{
		OrgID:              h.orgID,
		PlanID:             h.plan.ID,
		Provider:           "paystack",
		ExternalCustomerID: "CUS_renew",
		Status:             string(billing.StatusActive),
		CurrentPeriodStart: end.Add(-30 * 24 * time.Hour),
		CurrentPeriodEnd:   end,
		AuthorizationCode:  "AUTH_renew",
		PaymentChannel:     "card",
		CardLast4:          "0000",
	}
	if err := h.db.Create(&sub).Error; err != nil {
		t.Fatalf("seed sub: %v", err)
	}
	return sub
}

func TestSweepHandler_EnqueuesDueSubscriptions(t *testing.T) {
	h := newRenewalHarness(t)
	sub := h.seedDueSub(t)

	if err := h.sweep.Handle(context.Background(), nil); err != nil {
		t.Fatalf("sweep: %v", err)
	}

	enqueued := h.enqueuer.Tasks()
	if len(enqueued) != 1 {
		t.Fatalf("expected 1 enqueued task, got %d", len(enqueued))
	}
	if enqueued[0].TypeName != tasks.TypeBillingRenewSubscription {
		t.Errorf("Type = %q, want %q", enqueued[0].TypeName, tasks.TypeBillingRenewSubscription)
	}
	var payload tasks.BillingRenewSubscriptionPayload
	if err := json.Unmarshal(enqueued[0].Payload, &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if payload.SubscriptionID != sub.ID {
		t.Errorf("SubscriptionID = %v, want %v", payload.SubscriptionID, sub.ID)
	}
}

func TestSweepHandler_NoDueSubscriptionsIsNoop(t *testing.T) {
	h := newRenewalHarness(t)
	// Seed a sub but make it not due.
	end := time.Now().UTC().Add(24 * time.Hour)
	sub := model.Subscription{
		OrgID: h.orgID, PlanID: h.plan.ID, Provider: "paystack",
		ExternalCustomerID: "CUS_x", Status: string(billing.StatusActive),
		CurrentPeriodStart: end.Add(-30 * 24 * time.Hour), CurrentPeriodEnd: end,
		AuthorizationCode: "AUTH", PaymentChannel: "card",
	}
	if err := h.db.Create(&sub).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}

	if err := h.sweep.Handle(context.Background(), nil); err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if got := h.enqueuer.Tasks(); len(got) != 0 {
		t.Errorf("expected 0 enqueued, got %d", len(got))
	}
}

func TestRenewHandler_ChargesAndAdvances(t *testing.T) {
	h := newRenewalHarness(t)
	sub := h.seedDueSub(t)

	h.provider.NextChargeResult = &billing.ChargeAuthorizationResult{
		Status:          billing.StatusActive,
		Reference:       "ref_w",
		PaidAmountMinor: h.plan.PriceCents,
		Currency:        h.plan.Currency,
		PaidAt:          &h.now,
		PaymentMethod:   billing.PaymentMethod{AuthorizationCode: "AUTH", Channel: billing.ChannelCard},
	}

	payload, _ := json.Marshal(tasks.BillingRenewSubscriptionPayload{SubscriptionID: sub.ID})
	task := asynq.NewTask(tasks.TypeBillingRenewSubscription, payload)
	if err := h.renew.Handle(context.Background(), task); err != nil {
		t.Fatalf("renew: %v", err)
	}

	var fresh model.Subscription
	h.db.First(&fresh, "id = ?", sub.ID)
	if !fresh.CurrentPeriodEnd.After(time.Now()) {
		t.Errorf("period not advanced: %v", fresh.CurrentPeriodEnd)
	}
}

func TestRenewHandler_FailureSwallowedToAvoidDoubleAttempt(t *testing.T) {
	// The handler increments the attempt counter inside Renew and then
	// returns nil so asynq doesn't retry; the next sweep tick handles
	// the next attempt. This test asserts that contract.
	h := newRenewalHarness(t)
	sub := h.seedDueSub(t)
	h.provider.NextChargeError = errors.New("declined")

	payload, _ := json.Marshal(tasks.BillingRenewSubscriptionPayload{SubscriptionID: sub.ID})
	task := asynq.NewTask(tasks.TypeBillingRenewSubscription, payload)
	if err := h.renew.Handle(context.Background(), task); err != nil {
		t.Fatalf("expected handler to swallow charge errors, got %v", err)
	}

	var fresh model.Subscription
	h.db.First(&fresh, "id = ?", sub.ID)
	if fresh.RenewalAttempts != 1 {
		t.Errorf("RenewalAttempts = %d, want 1", fresh.RenewalAttempts)
	}
}
