package handler_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/config"
	"github.com/usehiveloop/hiveloop/internal/handler"
	"github.com/usehiveloop/hiveloop/internal/middleware"
	"github.com/usehiveloop/hiveloop/internal/model"
)

// billingTestHarness provides isolated infrastructure for billing handler tests.
type billingTestHarness struct {
	db     *gorm.DB
	router *chi.Mux
}

func newBillingHarness(t *testing.T) *billingTestHarness {
	t.Helper()

	db := connectTestDB(t)

	// BillingHandler with nil Polar client — only DB-dependent endpoints are testable without Polar.
	cfg := &config.Config{
		PolarProductProSharedID:    "test-shared-product-id",
		PolarProductProDedicatedID: "test-dedicated-product-id",
	}
	billingHandler := handler.NewBillingHandler(db, nil, cfg)

	router := chi.NewRouter()
	router.Get("/v1/billing/subscription", billingHandler.GetSubscription)

	return &billingTestHarness{db: db, router: router}
}

func createBillingTestOrg(t *testing.T, db *gorm.DB, billingPlan string) model.Org {
	t.Helper()
	org := model.Org{
		ID:          uuid.New(),
		Name:        fmt.Sprintf("billing-test-%s", uuid.New().String()[:8]),
		RateLimit:   1000,
		Active:      true,
		BillingPlan: billingPlan,
	}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	t.Cleanup(func() {
		db.Where("org_id = ?", org.ID).Delete(&model.Subscription{})
		cleanupOrg(t, db, org.ID)
	})
	return org
}

func (harness *billingTestHarness) doRequest(t *testing.T, method, path string, org *model.Org) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	req.Header.Set("Content-Type", "application/json")
	if org != nil {
		req = middleware.WithOrg(req, org)
	}
	recorder := httptest.NewRecorder()
	harness.router.ServeHTTP(recorder, req)
	return recorder
}

// --------------------------------------------------------------------------
// GET /v1/billing/subscription
// --------------------------------------------------------------------------

func TestBillingHandler_GetSubscription_FreeByDefault(t *testing.T) {
	harness := newBillingHarness(t)
	org := createBillingTestOrg(t, harness.db, "free")

	recorder := harness.doRequest(t, http.MethodGet, "/v1/billing/subscription", &org)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", recorder.Code, recorder.Body.String())
	}

	var resp map[string]any
	if err := json.NewDecoder(recorder.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if resp["plan"] != "free" {
		t.Fatalf("expected plan 'free', got %v", resp["plan"])
	}
	if resp["status"] != "active" {
		t.Fatalf("expected status 'active', got %v", resp["status"])
	}
}

func TestBillingHandler_GetSubscription_WithActiveSubscription(t *testing.T) {
	harness := newBillingHarness(t)
	org := createBillingTestOrg(t, harness.db, "pro")

	// Create an active subscription
	subscription := model.Subscription{
		OrgID:               org.ID,
		PolarSubscriptionID: "polar_sub_" + uuid.New().String()[:8],
		PolarProductID:      "test-shared-product-id",
		ProductType:         "pro_shared",
		Status:              "active",
	}
	if err := harness.db.Create(&subscription).Error; err != nil {
		t.Fatalf("create subscription: %v", err)
	}

	recorder := harness.doRequest(t, http.MethodGet, "/v1/billing/subscription", &org)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", recorder.Code, recorder.Body.String())
	}

	var resp map[string]any
	if err := json.NewDecoder(recorder.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if resp["plan"] != "pro" {
		t.Fatalf("expected plan 'pro', got %v", resp["plan"])
	}
	if resp["product_type"] != "pro_shared" {
		t.Fatalf("expected product_type 'pro_shared', got %v", resp["product_type"])
	}
}

func TestBillingHandler_GetSubscription_NoOrg(t *testing.T) {
	harness := newBillingHarness(t)

	recorder := harness.doRequest(t, http.MethodGet, "/v1/billing/subscription", nil)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", recorder.Code)
	}
}
