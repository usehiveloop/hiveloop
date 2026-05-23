package handler_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/usehivy/hivy/internal/model"
)

func TestOrgUpdate_ResponseIncludesPlanInfo(t *testing.T) {
	h := newOrgUpdateHarness(t)
	org, user := h.createOrg(t, "admin")

	plan := model.Plan{Slug: "free", Name: "Free", PriceCents: 0, Currency: "USD", Active: true}
	if err := h.db.Where("slug = ?", plan.Slug).FirstOrCreate(&plan).Error; err != nil {
		t.Fatalf("seed plan: %v", err)
	}

	rr := h.doPatch(t, user.ID, org.ID, "admin", map[string]any{
		"name": "With Plan Info",
	})

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d body=%s, want 200", rr.Code, rr.Body.String())
	}

	var got struct {
		Plan *struct {
			Slug           string `json:"slug"`
			Name           string `json:"name"`
			MonthlyCredits int64  `json:"monthly_credits"`
			PriceCents     int64  `json:"price_cents"`
			Currency       string `json:"currency"`
		} `json:"plan"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Plan == nil {
		t.Fatalf("plan should be populated, got nil")
	}
	if got.Plan.Slug != "free" {
		t.Errorf("plan.slug: got %q, want %q", got.Plan.Slug, "free")
	}
	if got.Plan.Name != "Free" {
		t.Errorf("plan.name: got %q, want %q", got.Plan.Name, "Free")
	}
	if got.Plan.Currency != "USD" {
		t.Errorf("plan.currency: got %q, want %q", got.Plan.Currency, "USD")
	}
}
