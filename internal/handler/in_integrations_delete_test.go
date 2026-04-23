package handler_test

import (
	"net/http"
	"testing"

	"github.com/usehiveloop/hiveloop/internal/model"
)

func TestInIntegrationHandler_Delete_Success(t *testing.T) {
	mockCfg := &nangoMockConfig{}
	h := newInIntegHarness(t, mockCfg)
	user := createTestUser(t, h.db, fmt.Sprintf("admin-%s@test.com", uuid.New().String()[:8]))
	integ := createTestInIntegration(t, h.db, "github")

	rr := h.doRequest(t, http.MethodDelete, "/v1/in/integrations/"+integ.ID.String(), nil, &user)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var deleted model.InIntegration
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

func TestInIntegrationHandler_Delete_NotFound(t *testing.T) {
	h := newInIntegHarness(t, nil)
	user := createTestUser(t, h.db, fmt.Sprintf("admin-%s@test.com", uuid.New().String()[:8]))

	rr := h.doRequest(t, http.MethodDelete, "/v1/in/integrations/"+uuid.New().String(), nil, &user)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
}

func TestInIntegrationHandler_Delete_NangoFailure(t *testing.T) {
	mockCfg := &nangoMockConfig{deleteStatus: http.StatusInternalServerError}
	h := newInIntegHarness(t, mockCfg)
	user := createTestUser(t, h.db, fmt.Sprintf("admin-%s@test.com", uuid.New().String()[:8]))
	integ := createTestInIntegration(t, h.db, "github")

	rr := h.doRequest(t, http.MethodDelete, "/v1/in/integrations/"+integ.ID.String(), nil, &user)

	if rr.Code != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d: %s", rr.Code, rr.Body.String())
	}
}
