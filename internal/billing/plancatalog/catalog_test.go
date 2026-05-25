package plancatalog

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadDefaultCatalog(t *testing.T) {
	catalog, err := Load("")
	if err != nil {
		t.Fatalf("load default catalog: %v", err)
	}
	if catalog.Version != 1 {
		t.Fatalf("version got %d, want 1", catalog.Version)
	}
	if len(catalog.Plans) == 0 {
		t.Fatalf("expected at least one plan")
	}

	var foundFree, foundBusiness bool
	for _, plan := range catalog.Plans {
		if plan.Slug == "free" {
			foundFree = true
			if plan.WelcomeCredits != 500 {
				t.Fatalf("free welcome credits got %d, want 500", plan.WelcomeCredits)
			}
		}
		if strings.HasPrefix(plan.Slug, "business-") {
			foundBusiness = true
			if plan.PriceCents <= 0 {
				t.Fatalf("business plan %q should have a positive price", plan.Slug)
			}
			if plan.Provider != "paystack" {
				t.Fatalf("business plan %q provider got %q, want paystack", plan.Slug, plan.Provider)
			}
		}
	}
	if !foundFree || !foundBusiness {
		t.Fatalf("catalog should include free and business plans")
	}
}

func TestLoadRejectsDuplicateSlugs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "catalog.json")
	body := `{
		"version": 1,
		"plans": [
			{"slug":"free","name":"Free","provider":"","visible":true,"active":true,"monthly_credits":0,"welcome_credits":500,"price_cents":0,"currency":"NGN","features":[]},
			{"slug":"free","name":"Free Copy","provider":"","visible":true,"active":true,"monthly_credits":0,"welcome_credits":500,"price_cents":0,"currency":"NGN","features":[]}
		]
	}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write catalog: %v", err)
	}
	_, err := Load(path)
	if err == nil || !strings.Contains(err.Error(), `duplicate plan slug "free"`) {
		t.Fatalf("error got %v, want duplicate slug", err)
	}
}
