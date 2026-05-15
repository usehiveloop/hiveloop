package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/usehiveloop/hiveloop/internal/handler"
	"github.com/usehiveloop/hiveloop/internal/mcp/catalog"
	"github.com/usehiveloop/hiveloop/internal/nango"
)

func newAdminInIntegrationHarness(t *testing.T) (*chi.Mux, *nangoMockConfig) {
	t.Helper()
	db := connectTestDB(t)
	cleanupInIntegrations(t, db)

	mockCfg := &nangoMockConfig{}
	nangoSrv := httptest.NewServer(newNangoMock(mockCfg))
	t.Cleanup(nangoSrv.Close)

	nangoClient := nango.NewClient(nangoSrv.URL, "test-secret-key")
	if err := nangoClient.FetchProviders(context.Background()); err != nil {
		t.Fatalf("fetch nango providers: %v", err)
	}

	adminHandler := handler.NewAdminHandler(db, nil, nangoClient, catalog.Global(), nil, nil, "", "", 0, 0, nil)
	r := chi.NewRouter()
	r.Get("/admin/v1/in-integration-providers", adminHandler.ListInIntegrationProviders)
	r.Post("/admin/v1/in-integrations", adminHandler.CreateInIntegration)
	return r, mockCfg
}

func TestAdminListInIntegrationProvidersIncludesBugsink(t *testing.T) {
	router, _ := newAdminInIntegrationHarness(t)

	req := httptest.NewRequest(http.MethodGet, "/admin/v1/in-integration-providers", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var providers []map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&providers); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	for _, provider := range providers {
		if provider["name"] == "bugsink" {
			if provider["display_name"] != "Bugsink" {
				t.Fatalf("display_name = %v, want Bugsink", provider["display_name"])
			}
			if provider["auth_mode"] != "API_KEY" {
				t.Fatalf("auth_mode = %v, want API_KEY", provider["auth_mode"])
			}
			return
		}
	}
	t.Fatalf("expected bugsink provider in admin list, got %#v", providers)
}

func TestAdminCreateInIntegrationBugsink(t *testing.T) {
	router, mockCfg := newAdminInIntegrationHarness(t)

	body := strings.NewReader(`{"provider":"bugsink","display_name":"Bugsink"}`)
	req := httptest.NewRequest(http.MethodPost, "/admin/v1/in-integrations", body)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["provider"] != "bugsink" {
		t.Fatalf("provider = %v, want bugsink", resp["provider"])
	}
	if uniqueKey, _ := resp["unique_key"].(string); !strings.HasPrefix(uniqueKey, "bugsink-") {
		t.Fatalf("unique_key = %q, want bugsink-*", uniqueKey)
	}

	cfg, ok := resp["nango_config"].(map[string]any)
	if !ok {
		t.Fatalf("expected nango_config object, got %#v", resp["nango_config"])
	}
	if cfg["auth_mode"] != "API_KEY" {
		t.Fatalf("nango_config.auth_mode = %v, want API_KEY", cfg["auth_mode"])
	}
	if _, ok := cfg["connection_config"].(map[string]any); !ok {
		t.Fatalf("expected connection_config copied from provider template, got %#v", cfg)
	}

	mockCfg.mu.Lock()
	defer mockCfg.mu.Unlock()
	createdInNango := false
	for i, path := range mockCfg.capturedPaths {
		if path == "/integrations" && mockCfg.capturedMethods[i] == http.MethodPost {
			createdInNango = true
			break
		}
	}
	if !createdInNango {
		t.Fatal("expected Bugsink integration to be created in Nango")
	}
}
