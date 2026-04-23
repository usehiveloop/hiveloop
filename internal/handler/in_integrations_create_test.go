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

func TestInIntegrationHandler_Create_Success(t *testing.T) {
	h := newInIntegHarness(t, nil)
	user := createTestUser(t, h.db, fmt.Sprintf("admin-%s@test.com", uuid.New().String()[:8]))

	rr := h.doRequest(t, http.MethodPost, "/v1/in/integrations", map[string]any{
		"provider":     "github",
		"display_name": "GitHub Built-in",
		"credentials": map[string]any{
			"type":          "OAUTH2",
			"client_id":     "test-client-id",
			"client_secret": "test-client-secret",
		},
	}, &user)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp["provider"] != "github" {
		t.Fatalf("expected provider=github, got %v", resp["provider"])
	}
	if resp["display_name"] != "GitHub Built-in" {
		t.Fatalf("expected display_name=GitHub Built-in, got %v", resp["display_name"])
	}

	h.mockCfg.mu.Lock()
	found := false
	for _, p := range h.mockCfg.capturedPaths {
		if strings.Contains(p, "/integrations") && strings.HasPrefix(p, "/integrations") {
			found = true
		}
	}
	h.mockCfg.mu.Unlock()
	if !found {
		t.Fatal("expected Nango to receive integration creation request")
	}

	var integ model.InIntegration
	if err := h.db.Where("id = ?", resp["id"]).First(&integ).Error; err != nil {
		t.Fatalf("integration not found in DB: %v", err)
	}
	if !strings.HasPrefix(integ.UniqueKey, "github-") {
		t.Fatalf("expected unique_key to start with github-, got %s", integ.UniqueKey)
	}
}

func TestInIntegrationHandler_Create_MissingProvider(t *testing.T) {
	h := newInIntegHarness(t, nil)
	user := createTestUser(t, h.db, fmt.Sprintf("admin-%s@test.com", uuid.New().String()[:8]))

	rr := h.doRequest(t, http.MethodPost, "/v1/in/integrations", map[string]any{
		"display_name": "GitHub Built-in",
	}, &user)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestInIntegrationHandler_Create_MissingDisplayName(t *testing.T) {
	h := newInIntegHarness(t, nil)
	user := createTestUser(t, h.db, fmt.Sprintf("admin-%s@test.com", uuid.New().String()[:8]))

	rr := h.doRequest(t, http.MethodPost, "/v1/in/integrations", map[string]any{
		"provider": "github",
	}, &user)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestInIntegrationHandler_Create_UnknownProvider(t *testing.T) {
	h := newInIntegHarness(t, nil)
	user := createTestUser(t, h.db, fmt.Sprintf("admin-%s@test.com", uuid.New().String()[:8]))

	rr := h.doRequest(t, http.MethodPost, "/v1/in/integrations", map[string]any{
		"provider":     "nonexistent-provider",
		"display_name": "Test",
	}, &user)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestInIntegrationHandler_Create_DuplicateProvider(t *testing.T) {
	h := newInIntegHarness(t, nil)
	user := createTestUser(t, h.db, fmt.Sprintf("admin-%s@test.com", uuid.New().String()[:8]))

	createTestInIntegration(t, h.db, "github")

	integ2 := model.InIntegration{
		ID:          uuid.New(),
		UniqueKey:   fmt.Sprintf("github-%s", uuid.New().String()[:8]),
		Provider:    "github",
		DisplayName: "GitHub 2",
	}
	err := h.db.Create(&integ2).Error
	if err == nil {
		t.Fatal("expected unique constraint error for duplicate provider")
		h.db.Where("id = ?", integ2.ID).Delete(&model.InIntegration{})
	}
	_ = user
}

func TestInIntegrationHandler_Create_InvalidCredentials(t *testing.T) {
	h := newInIntegHarness(t, nil)
	user := createTestUser(t, h.db, fmt.Sprintf("admin-%s@test.com", uuid.New().String()[:8]))

	rr := h.doRequest(t, http.MethodPost, "/v1/in/integrations", map[string]any{
		"provider":     "github",
		"display_name": "GitHub Built-in",
		"credentials": map[string]any{
			"type":      "OAUTH2",
			"client_id": "test-client-id",
		},
	}, &user)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestInIntegrationHandler_Create_NangoFailure(t *testing.T) {
	mockCfg := &nangoMockConfig{createStatus: http.StatusInternalServerError}
	h := newInIntegHarness(t, mockCfg)
	user := createTestUser(t, h.db, fmt.Sprintf("admin-%s@test.com", uuid.New().String()[:8]))

	rr := h.doRequest(t, http.MethodPost, "/v1/in/integrations", map[string]any{
		"provider":     "github",
		"display_name": "GitHub Built-in",
		"credentials": map[string]any{
			"type":          "OAUTH2",
			"client_id":     "test-client-id",
			"client_secret": "test-client-secret",
		},
	}, &user)

	if rr.Code != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d: %s", rr.Code, rr.Body.String())
	}
}
