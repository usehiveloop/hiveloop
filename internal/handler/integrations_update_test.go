package handler_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

func TestIntegrationHandler_Update_DisplayName(t *testing.T) {
	h := newIntegrationHarness(t, nil)
	user := createTestUser(t, h.db, fmt.Sprintf("admin-%s@test.com", uuid.New().String()[:8]))
	integ := createTestIntegration(t, h.db, "github")

	rr := h.doRequest(t, http.MethodPut, "/v1/integrations/"+integ.ID.String(), map[string]any{
		"display_name": "Updated GitHub",
	}, &user)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	_ = json.NewDecoder(rr.Body).Decode(&resp)
	if resp["display_name"] != "Updated GitHub" {
		t.Fatalf("expected display_name=Updated GitHub, got %v", resp["display_name"])
	}
}

func TestIntegrationHandler_Update_Credentials(t *testing.T) {
	mockCfg := &nangoMockConfig{}
	h := newIntegrationHarness(t, mockCfg)
	user := createTestUser(t, h.db, fmt.Sprintf("admin-%s@test.com", uuid.New().String()[:8]))
	integ := createTestIntegration(t, h.db, "github")

	rr := h.doRequest(t, http.MethodPut, "/v1/integrations/"+integ.ID.String(), map[string]any{
		"credentials": map[string]any{
			"type":          "OAUTH2",
			"client_id":     "new-client-id",
			"client_secret": "new-client-secret",
		},
	}, &user)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	mockCfg.mu.Lock()
	foundPatch := false
	for i, m := range mockCfg.capturedMethods {
		if m == http.MethodPatch && strings.Contains(mockCfg.capturedPaths[i], "/integrations/") {
			foundPatch = true
		}
	}
	mockCfg.mu.Unlock()
	if !foundPatch {
		t.Fatal("expected Nango to receive PATCH for credential update")
	}
}

func TestIntegrationHandler_Update_Meta(t *testing.T) {
	h := newIntegrationHarness(t, nil)
	user := createTestUser(t, h.db, fmt.Sprintf("admin-%s@test.com", uuid.New().String()[:8]))
	integ := createTestIntegration(t, h.db, "github")

	rr := h.doRequest(t, http.MethodPut, "/v1/integrations/"+integ.ID.String(), map[string]any{
		"meta": map[string]any{"custom": "value"},
	}, &user)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var updated model.Integration
	h.db.Where("id = ?", integ.ID).First(&updated)
	if updated.Meta == nil || updated.Meta["custom"] != "value" {
		t.Fatalf("expected meta.custom=value, got %v", updated.Meta)
	}
}

func TestIntegrationHandler_Update_NangoFailure(t *testing.T) {
	mockCfg := &nangoMockConfig{updateStatus: http.StatusInternalServerError}
	h := newIntegrationHarness(t, mockCfg)
	user := createTestUser(t, h.db, fmt.Sprintf("admin-%s@test.com", uuid.New().String()[:8]))
	integ := createTestIntegration(t, h.db, "github")

	rr := h.doRequest(t, http.MethodPut, "/v1/integrations/"+integ.ID.String(), map[string]any{
		"credentials": map[string]any{
			"type":          "OAUTH2",
			"client_id":     "new-client-id",
			"client_secret": "new-client-secret",
		},
	}, &user)

	if rr.Code != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestIntegrationHandler_Update_ManagedIntegrationReadOnly(t *testing.T) {
	h := newIntegrationHarness(t, nil)
	user := createTestUser(t, h.db, fmt.Sprintf("admin-%s@test.com", uuid.New().String()[:8]))
	integ := model.Integration{
		ID:          uuid.New(),
		UniqueKey:   "managed-" + uuid.New().String()[:8],
		Provider:    "github",
		DisplayName: "Managed GitHub",
		ManagedBy:   "global_integrations",
		ManagedID:   "github",
		ManagedHash: "hash",
	}
	if err := h.db.Create(&integ).Error; err != nil {
		t.Fatalf("create managed integration: %v", err)
	}

	rr := h.doRequest(t, http.MethodPut, "/v1/integrations/"+integ.ID.String(), map[string]any{
		"display_name": "Edited",
	}, &user)

	if rr.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", rr.Code, rr.Body.String())
	}
}
