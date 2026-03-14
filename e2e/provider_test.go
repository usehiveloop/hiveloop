package e2e

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/useportal/llmvault/internal/middleware"
	"github.com/useportal/llmvault/internal/registry"
)

// --------------------------------------------------------------------------
// E2E: Provider list endpoint
// --------------------------------------------------------------------------

func TestE2E_Provider_List(t *testing.T) {
	h := newHarness(t)

	req := httptest.NewRequest(http.MethodGet, "/v1/providers", nil)
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var providers []struct {
		ID         string `json:"id"`
		Name       string `json:"name"`
		ModelCount int    `json:"model_count"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&providers); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if len(providers) < 50 {
		t.Fatalf("expected at least 50 providers, got %d", len(providers))
	}

	// Verify well-known providers are present
	known := map[string]bool{"openai": false, "anthropic": false, "google": false, "deepseek": false}
	for _, p := range providers {
		if _, ok := known[p.ID]; ok {
			known[p.ID] = true
		}
	}
	for id, found := range known {
		if !found {
			t.Errorf("expected provider %q in list", id)
		}
	}

	// Verify model_count > 0 for openai
	for _, p := range providers {
		if p.ID == "openai" && p.ModelCount == 0 {
			t.Error("openai should have model_count > 0")
		}
	}

	t.Logf("Listed %d providers", len(providers))
}

// --------------------------------------------------------------------------
// E2E: Provider detail endpoint
// --------------------------------------------------------------------------

func TestE2E_Provider_Get(t *testing.T) {
	h := newHarness(t)

	req := httptest.NewRequest(http.MethodGet, "/v1/providers/openai", nil)
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var provider struct {
		ID     string `json:"id"`
		Name   string `json:"name"`
		Models []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&provider); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if provider.ID != "openai" {
		t.Fatalf("expected openai, got %s", provider.ID)
	}
	if len(provider.Models) == 0 {
		t.Fatal("expected models for openai")
	}

	// Check that gpt-4o exists in models
	found := false
	for _, m := range provider.Models {
		if strings.Contains(m.ID, "gpt-4o") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected gpt-4o model in openai provider")
	}

	t.Logf("OpenAI has %d models", len(provider.Models))
}

// --------------------------------------------------------------------------
// E2E: Provider not found
// --------------------------------------------------------------------------

func TestE2E_Provider_NotFound(t *testing.T) {
	h := newHarness(t)

	req := httptest.NewRequest(http.MethodGet, "/v1/providers/nonexistent-provider", nil)
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
}

// --------------------------------------------------------------------------
// E2E: Provider models endpoint
// --------------------------------------------------------------------------

func TestE2E_Provider_Models(t *testing.T) {
	h := newHarness(t)

	req := httptest.NewRequest(http.MethodGet, "/v1/providers/anthropic/models", nil)
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var models []struct {
		ID        string `json:"id"`
		Name      string `json:"name"`
		Reasoning bool   `json:"reasoning"`
		Cost      *struct {
			Input  float64 `json:"input"`
			Output float64 `json:"output"`
		} `json:"cost"`
		Limit *struct {
			Context int64 `json:"context"`
			Output  int64 `json:"output"`
		} `json:"limit"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&models); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if len(models) == 0 {
		t.Fatal("expected models for anthropic")
	}

	// Verify Claude model exists and has cost/limit data
	for _, m := range models {
		if strings.Contains(m.ID, "claude") {
			if m.Cost == nil {
				t.Errorf("expected cost data for %s", m.ID)
			}
			if m.Limit == nil {
				t.Errorf("expected limit data for %s", m.ID)
			}
			t.Logf("Model: %s (context: %d, cost: $%.2f/$%.2f per 1M tokens)",
				m.ID, m.Limit.Context, m.Cost.Input, m.Cost.Output)
			break
		}
	}

	t.Logf("Anthropic has %d models", len(models))
}

// --------------------------------------------------------------------------
// E2E: Registry URL matching
// --------------------------------------------------------------------------

func TestE2E_Registry_URLMatching(t *testing.T) {
	reg := registry.Global()

	tests := []struct {
		baseURL    string
		wantID     string
		wantMatch  bool
	}{
		{"https://api.openai.com/v1", "openai", true},
		{"https://api.anthropic.com/v1", "anthropic", true},
		{"https://openrouter.ai/api", "openrouter", true},
		{"https://api.deepseek.com", "deepseek", true},
		{"https://totally-unknown-provider.example.com", "", false},
	}

	for _, tt := range tests {
		p, ok := reg.MatchByBaseURL(tt.baseURL)
		if ok != tt.wantMatch {
			t.Errorf("MatchByBaseURL(%q): got match=%v, want %v", tt.baseURL, ok, tt.wantMatch)
			continue
		}
		if ok && p.ID != tt.wantID {
			t.Errorf("MatchByBaseURL(%q): got %q, want %q", tt.baseURL, p.ID, tt.wantID)
		}
	}
}

// --------------------------------------------------------------------------
// E2E: Credential auto-detects provider_id
// --------------------------------------------------------------------------

func TestE2E_Credential_ProviderAutoDetect(t *testing.T) {
	h := newHarness(t)
	org := h.createOrg(t)

	tests := []struct {
		baseURL      string
		wantProvider string
	}{
		{"https://api.openai.com/v1", "openai"},
		{"https://api.anthropic.com/v1", "anthropic"},
		{"https://openrouter.ai/api", "openrouter"},
		{"https://api.example.com", ""},
	}

	for _, tt := range tests {
		t.Run(tt.baseURL, func(t *testing.T) {
			body := fmt.Sprintf(`{"label":"test","base_url":%q,"auth_scheme":"bearer","api_key":"sk-test"}`, tt.baseURL)
			req := httptest.NewRequest(http.MethodPost, "/v1/credentials", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			req = middleware.WithOrg(req, &org)
			rr := httptest.NewRecorder()
			h.router.ServeHTTP(rr, req)

			if rr.Code != http.StatusCreated {
				t.Fatalf("expected 201, got %d: %s", rr.Code, rr.Body.String())
			}

			var resp struct {
				ID         string `json:"id"`
				ProviderID string `json:"provider_id"`
			}
			json.NewDecoder(rr.Body).Decode(&resp)

			if resp.ProviderID != tt.wantProvider {
				t.Errorf("provider_id: got %q, want %q", resp.ProviderID, tt.wantProvider)
			}
		})
	}
}

// --------------------------------------------------------------------------
// E2E: Provider list returns sorted by ID
// --------------------------------------------------------------------------

func TestE2E_Provider_Sorted(t *testing.T) {
	h := newHarness(t)

	req := httptest.NewRequest(http.MethodGet, "/v1/providers", nil)
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var providers []struct {
		ID string `json:"id"`
	}
	json.NewDecoder(rr.Body).Decode(&providers)

	for i := 1; i < len(providers); i++ {
		if providers[i].ID < providers[i-1].ID {
			t.Fatalf("providers not sorted: %s < %s at index %d", providers[i].ID, providers[i-1].ID, i)
		}
	}
}

// --------------------------------------------------------------------------
// E2E: Registry stats
// --------------------------------------------------------------------------

func TestE2E_Registry_Stats(t *testing.T) {
	reg := registry.Global()

	providers := reg.ProviderCount()
	models := reg.ModelCount()

	if providers < 50 {
		t.Errorf("expected at least 50 providers, got %d", providers)
	}
	if models < 1000 {
		t.Errorf("expected at least 1000 models, got %d", models)
	}

	t.Logf("Registry: %d providers, %d models", providers, models)
}

// --------------------------------------------------------------------------
// E2E: Credential list shows provider_id
// --------------------------------------------------------------------------

func TestE2E_Credential_ListShowsProviderID(t *testing.T) {
	h := newHarness(t)
	org := h.createOrg(t)

	// Create a credential with a known provider
	body := `{"label":"openai-test","base_url":"https://api.openai.com/v1","auth_scheme":"bearer","api_key":"sk-test"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/credentials", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = middleware.WithOrg(req, &org)
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	// List and verify provider_id
	req = httptest.NewRequest(http.MethodGet, "/v1/credentials", nil)
	req = middleware.WithOrg(req, &org)
	rr = httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("list: expected 200, got %d", rr.Code)
	}

	creds := decodePaginatedList(t, rr)

	found := false
	for _, c := range creds {
		if c["base_url"] == "https://api.openai.com/v1" {
			if c["provider_id"] != "openai" {
				t.Errorf("expected provider_id 'openai', got %q", c["provider_id"])
			}
			found = true
		}
	}
	if !found {
		t.Fatal("credential not in list")
	}
}
