package handler_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/usehiveloop/hiveloop/internal/model"
)

func TestInIntegrationHandler_Update_DisplayName(t *testing.T) {
	h := newInIntegHarness(t, nil)
	user := createTestUser(t, h.db, fmt.Sprintf("admin-%s@test.com", uuid.New().String()[:8]))
	integ := createTestInIntegration(t, h.db, "github")

	rr := h.doRequest(t, http.MethodPut, "/v1/in/integrations/"+integ.ID.String(), map[string]any{
		"display_name": "Updated GitHub",
	}, &user)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp["display_name"] != "Updated GitHub" {
		t.Fatalf("expected display_name=Updated GitHub, got %v", resp["display_name"])
	}
}

func TestInIntegrationHandler_Update_Credentials(t *testing.T) {
	mockCfg := &nangoMockConfig{}
	h := newInIntegHarness(t, mockCfg)
	user := createTestUser(t, h.db, fmt.Sprintf("admin-%s@test.com", uuid.New().String()[:8]))
	integ := createTestInIntegration(t, h.db, "github")

	rr := h.doRequest(t, http.MethodPut, "/v1/in/integrations/"+integ.ID.String(), map[string]any{
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

func TestInIntegrationHandler_Update_Meta(t *testing.T) {
	h := newInIntegHarness(t, nil)
	user := createTestUser(t, h.db, fmt.Sprintf("admin-%s@test.com", uuid.New().String()[:8]))
	integ := createTestInIntegration(t, h.db, "github")

	rr := h.doRequest(t, http.MethodPut, "/v1/in/integrations/"+integ.ID.String(), map[string]any{
		"meta": map[string]any{"custom": "value"},
	}, &user)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var updated model.InIntegration
	h.db.Where("id = ?", integ.ID).First(&updated)
	if updated.Meta == nil || updated.Meta["custom"] != "value" {
		t.Fatalf("expected meta.custom=value, got %v", updated.Meta)
	}
}

func TestInIntegrationHandler_Update_NotFound(t *testing.T) {
	h := newInIntegHarness(t, nil)
	user := createTestUser(t, h.db, fmt.Sprintf("admin-%s@test.com", uuid.New().String()[:8]))

	rr := h.doRequest(t, http.MethodPut, "/v1/in/integrations/"+uuid.New().String(), map[string]any{
		"display_name": "Updated",
	}, &user)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
}

func TestInIntegrationHandler_Update_NangoFailure(t *testing.T) {
	mockCfg := &nangoMockConfig{updateStatus: http.StatusInternalServerError}
	h := newInIntegHarness(t, mockCfg)
	user := createTestUser(t, h.db, fmt.Sprintf("admin-%s@test.com", uuid.New().String()[:8]))
	integ := createTestInIntegration(t, h.db, "github")

	rr := h.doRequest(t, http.MethodPut, "/v1/in/integrations/"+integ.ID.String(), map[string]any{
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
