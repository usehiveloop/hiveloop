package handler_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestInIntegrationHandler_Get_Success(t *testing.T) {
	h := newInIntegHarness(t, nil)
	user := createTestUser(t, h.db, fmt.Sprintf("admin-%s@test.com", uuid.New().String()[:8]))
	integ := createTestInIntegration(t, h.db, "github")

	rr := h.doRequest(t, http.MethodGet, "/v1/in/integrations/"+integ.ID.String(), nil, &user)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp["id"] != integ.ID.String() {
		t.Fatalf("expected id=%s, got %v", integ.ID.String(), resp["id"])
	}
	if resp["provider"] != "github" {
		t.Fatalf("expected provider=github, got %v", resp["provider"])
	}
}

func TestInIntegrationHandler_Get_NotFound(t *testing.T) {
	h := newInIntegHarness(t, nil)
	user := createTestUser(t, h.db, fmt.Sprintf("admin-%s@test.com", uuid.New().String()[:8]))

	rr := h.doRequest(t, http.MethodGet, "/v1/in/integrations/"+uuid.New().String(), nil, &user)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
}

func TestInIntegrationHandler_Get_DeletedNotFound(t *testing.T) {
	h := newInIntegHarness(t, nil)
	user := createTestUser(t, h.db, fmt.Sprintf("admin-%s@test.com", uuid.New().String()[:8]))
	integ := createTestInIntegration(t, h.db, "github")

	now := time.Now()
	h.db.Model(&integ).Update("deleted_at", now)

	rr := h.doRequest(t, http.MethodGet, "/v1/in/integrations/"+integ.ID.String(), nil, &user)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
}
