package handler_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

func TestIntegrationHandler_Delete_Success(t *testing.T) {
	mockCfg := &nangoMockConfig{}
	h := newIntegrationHarness(t, mockCfg)
	user := createTestUser(t, h.db, fmt.Sprintf("admin-%s@test.com", uuid.New().String()[:8]))
	integ := createTestIntegration(t, h.db, "github")

	rr := h.doRequest(t, http.MethodDelete, "/v1/integrations/"+integ.ID.String(), nil, &user)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var deleted model.Integration
	h.db.Where("id = ?", integ.ID).First(&deleted)
	if deleted.DeletedAt == nil {
		t.Fatal("expected deleted_at to be set")
	}

	mockCfg.mu.Lock()
	foundDelete := false
	for _, m := range mockCfg.capturedMethods {
		if m == http.MethodDelete {
			foundDelete = true
		}
	}
	mockCfg.mu.Unlock()
	if !foundDelete {
		t.Fatal("expected Nango to receive DELETE")
	}
}

func TestIntegrationHandler_Delete_NotFound(t *testing.T) {
	h := newIntegrationHarness(t, nil)
	user := createTestUser(t, h.db, fmt.Sprintf("admin-%s@test.com", uuid.New().String()[:8]))

	rr := h.doRequest(t, http.MethodDelete, "/v1/integrations/"+uuid.New().String(), nil, &user)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
}

func TestIntegrationHandler_Delete_NangoFailure(t *testing.T) {
	mockCfg := &nangoMockConfig{deleteStatus: http.StatusInternalServerError}
	h := newIntegrationHarness(t, mockCfg)
	user := createTestUser(t, h.db, fmt.Sprintf("admin-%s@test.com", uuid.New().String()[:8]))
	integ := createTestIntegration(t, h.db, "github")

	rr := h.doRequest(t, http.MethodDelete, "/v1/integrations/"+integ.ID.String(), nil, &user)

	if rr.Code != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestIntegrationHandler_Delete_ManagedIntegrationReadOnly(t *testing.T) {
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

	rr := h.doRequest(t, http.MethodDelete, "/v1/integrations/"+integ.ID.String(), nil, &user)

	if rr.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", rr.Code, rr.Body.String())
	}
}
