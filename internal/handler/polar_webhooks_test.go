package handler_test

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/handler"
	"github.com/usehiveloop/hiveloop/internal/model"
)

const testWebhookSecret = "test-webhook-secret-for-testing"

type polarWebhookTestHarness struct {
	db     *gorm.DB
	router *chi.Mux
}

func newPolarWebhookHarness(t *testing.T) *polarWebhookTestHarness {
	t.Helper()

	db := connectTestDB(t)
	webhookHandler := handler.NewPolarWebhookHandler(db, testWebhookSecret, "test-free-product-id")

	router := chi.NewRouter()
	router.Post("/internal/webhooks/polar", webhookHandler.Handle)

	return &polarWebhookTestHarness{db: db, router: router}
}

// signWebhookPayload creates Standard Webhooks headers for a given payload.
func signWebhookPayload(t *testing.T, secret string, payload []byte) http.Header {
	t.Helper()
	msgID := "msg_" + uuid.New().String()[:8]
	timestamp := fmt.Sprintf("%d", time.Now().Unix())

	toSign := fmt.Sprintf("%s.%s.%s", msgID, timestamp, string(payload))
	secretBytes := []byte(secret)
	mac := hmac.New(sha256.New, secretBytes)
	mac.Write([]byte(toSign))
	signature := base64.StdEncoding.EncodeToString(mac.Sum(nil))

	headers := http.Header{}
	headers.Set("webhook-id", msgID)
	headers.Set("webhook-timestamp", timestamp)
	headers.Set("webhook-signature", "v1,"+signature)
	return headers
}

func (harness *polarWebhookTestHarness) doWebhook(t *testing.T, payload []byte, signed bool) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/internal/webhooks/polar", strings.NewReader(string(payload)))
	req.Header.Set("Content-Type", "application/json")

	if signed {
		headers := signWebhookPayload(t, testWebhookSecret, payload)
		for key, values := range headers {
			for _, value := range values {
				req.Header.Set(key, value)
			}
		}
	}

	recorder := httptest.NewRecorder()
	harness.router.ServeHTTP(recorder, req)
	return recorder
}

func createWebhookTestOrg(t *testing.T, db *gorm.DB, polarCustomerID string) model.Org {
	t.Helper()
	org := model.Org{
		ID:              uuid.New(),
		Name:            fmt.Sprintf("webhook-test-%s", uuid.New().String()[:8]),
		RateLimit:       1000,
		Active:          true,
		BillingPlan:     "free",
		PolarCustomerID: &polarCustomerID,
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

// --------------------------------------------------------------------------
// Signature verification
// --------------------------------------------------------------------------

func TestPolarWebhook_RejectsUnsignedRequest(t *testing.T) {
	harness := newPolarWebhookHarness(t)

	payload := []byte(`{"type":"subscription.created","data":{}}`)
	recorder := harness.doWebhook(t, payload, false)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for unsigned request, got %d; body: %s", recorder.Code, recorder.Body.String())
	}
}

// --------------------------------------------------------------------------
// subscription.created
// --------------------------------------------------------------------------

func TestPolarWebhook_SubscriptionCreated(t *testing.T) {
	harness := newPolarWebhookHarness(t)
	polarCustomerID := "polar_cust_" + uuid.New().String()[:8]
	org := createWebhookTestOrg(t, harness.db, polarCustomerID)

	webhookPayload := map[string]any{
		"type": "subscription.created",
		"data": map[string]any{
			"id":         "polar_sub_" + uuid.New().String()[:8],
			"status":     "active",
			"product_id": "test-product-id",
			"customer_id": polarCustomerID,
			"customer": map[string]any{
				"external_id": org.ID.String(),
			},
			"current_period_start": time.Now().Format(time.RFC3339),
			"current_period_end":   time.Now().Add(30 * 24 * time.Hour).Format(time.RFC3339),
		},
	}
	payloadBytes, _ := json.Marshal(webhookPayload)

	recorder := harness.doWebhook(t, payloadBytes, true)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", recorder.Code, recorder.Body.String())
	}

	// Verify subscription was created in DB
	var subscription model.Subscription
	if err := harness.db.Where("org_id = ?", org.ID).First(&subscription).Error; err != nil {
		t.Fatalf("subscription should exist in DB: %v", err)
	}
	if subscription.Status != "active" {
		t.Fatalf("expected subscription status 'active', got %q", subscription.Status)
	}

	// Verify org billing plan was updated
	var updatedOrg model.Org
	harness.db.Where("id = ?", org.ID).First(&updatedOrg)
	if updatedOrg.BillingPlan != "pro" {
		t.Fatalf("expected org billing plan 'pro', got %q", updatedOrg.BillingPlan)
	}
}

// --------------------------------------------------------------------------
// subscription.canceled
// --------------------------------------------------------------------------

func TestPolarWebhook_SubscriptionCanceled(t *testing.T) {
	harness := newPolarWebhookHarness(t)
	polarCustomerID := "polar_cust_" + uuid.New().String()[:8]
	org := createWebhookTestOrg(t, harness.db, polarCustomerID)

	// Seed an active subscription
	polarSubID := "polar_sub_" + uuid.New().String()[:8]
	subscription := model.Subscription{
		OrgID:               org.ID,
		PolarSubscriptionID: polarSubID,
		PolarProductID:      "test-product-id",
		ProductType:         "pro_shared",
		Status:              "active",
	}
	if err := harness.db.Create(&subscription).Error; err != nil {
		t.Fatalf("create subscription: %v", err)
	}

	webhookPayload := map[string]any{
		"type": "subscription.canceled",
		"data": map[string]any{
			"id":          polarSubID,
			"status":      "canceled",
			"customer_id": polarCustomerID,
			"customer": map[string]any{
				"external_id": org.ID.String(),
			},
		},
	}
	payloadBytes, _ := json.Marshal(webhookPayload)

	recorder := harness.doWebhook(t, payloadBytes, true)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", recorder.Code, recorder.Body.String())
	}

	// Verify subscription status was updated
	var updatedSub model.Subscription
	harness.db.Where("polar_subscription_id = ?", polarSubID).First(&updatedSub)
	if updatedSub.Status != "canceled" {
		t.Fatalf("expected subscription status 'canceled', got %q", updatedSub.Status)
	}
	if updatedSub.CanceledAt == nil {
		t.Fatal("expected CanceledAt to be set")
	}
}

// --------------------------------------------------------------------------
// subscription.revoked — downgrades org to free
// --------------------------------------------------------------------------

func TestPolarWebhook_SubscriptionRevoked_DowngradeToFree(t *testing.T) {
	harness := newPolarWebhookHarness(t)
	polarCustomerID := "polar_cust_" + uuid.New().String()[:8]
	org := createWebhookTestOrg(t, harness.db, polarCustomerID)

	// Mark org as pro
	harness.db.Model(&model.Org{}).Where("id = ?", org.ID).Update("billing_plan", "pro")

	// Seed an active subscription
	polarSubID := "polar_sub_" + uuid.New().String()[:8]
	subscription := model.Subscription{
		OrgID:               org.ID,
		PolarSubscriptionID: polarSubID,
		PolarProductID:      "test-product-id",
		ProductType:         "pro_shared",
		Status:              "active",
	}
	if err := harness.db.Create(&subscription).Error; err != nil {
		t.Fatalf("create subscription: %v", err)
	}

	webhookPayload := map[string]any{
		"type": "subscription.revoked",
		"data": map[string]any{
			"id":          polarSubID,
			"status":      "revoked",
			"customer_id": polarCustomerID,
			"customer": map[string]any{
				"external_id": org.ID.String(),
			},
		},
	}
	payloadBytes, _ := json.Marshal(webhookPayload)

	recorder := harness.doWebhook(t, payloadBytes, true)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", recorder.Code, recorder.Body.String())
	}

	// Verify org downgraded to free
	var updatedOrg model.Org
	harness.db.Where("id = ?", org.ID).First(&updatedOrg)
	if updatedOrg.BillingPlan != "free" {
		t.Fatalf("expected org billing plan 'free' after revocation, got %q", updatedOrg.BillingPlan)
	}
}

// --------------------------------------------------------------------------
// subscription.updated with Free product — downgrades org
// --------------------------------------------------------------------------

func TestPolarWebhook_SubscriptionUpdated_DowngradeToFree(t *testing.T) {
	harness := newPolarWebhookHarness(t)
	polarCustomerID := "polar_cust_" + uuid.New().String()[:8]
	org := createWebhookTestOrg(t, harness.db, polarCustomerID)

	// Mark org as pro
	harness.db.Model(&model.Org{}).Where("id = ?", org.ID).Update("billing_plan", "pro")

	// Seed an active subscription
	polarSubID := "polar_sub_" + uuid.New().String()[:8]
	subscription := model.Subscription{
		OrgID:               org.ID,
		PolarSubscriptionID: polarSubID,
		PolarProductID:      "test-pro-product-id",
		ProductType:         "pro_shared",
		Status:              "active",
	}
	if err := harness.db.Create(&subscription).Error; err != nil {
		t.Fatalf("create subscription: %v", err)
	}

	// Polar sends subscription.updated with product changed to Free
	webhookPayload := map[string]any{
		"type": "subscription.updated",
		"data": map[string]any{
			"id":         polarSubID,
			"status":     "active",
			"product_id": "test-free-product-id",
			"customer_id": polarCustomerID,
			"customer": map[string]any{
				"external_id": org.ID.String(),
			},
			"current_period_start": time.Now().Format(time.RFC3339),
			"current_period_end":   time.Now().Add(30 * 24 * time.Hour).Format(time.RFC3339),
		},
	}
	payloadBytes, _ := json.Marshal(webhookPayload)

	recorder := harness.doWebhook(t, payloadBytes, true)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", recorder.Code, recorder.Body.String())
	}

	// Verify org downgraded to free
	var updatedOrg model.Org
	harness.db.Where("id = ?", org.ID).First(&updatedOrg)
	if updatedOrg.BillingPlan != "free" {
		t.Fatalf("expected org billing plan 'free' after downgrade, got %q", updatedOrg.BillingPlan)
	}
}

// --------------------------------------------------------------------------
// Unhandled event type — should still return 200
// --------------------------------------------------------------------------

func TestPolarWebhook_UnhandledEventType_Returns200(t *testing.T) {
	harness := newPolarWebhookHarness(t)

	webhookPayload := map[string]any{
		"type": "order.paid",
		"data": map[string]any{"id": "some-order-id"},
	}
	payloadBytes, _ := json.Marshal(webhookPayload)

	recorder := harness.doWebhook(t, payloadBytes, true)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200 for unhandled event, got %d", recorder.Code)
	}
}
