package handler_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/ziraloop/ziraloop/internal/auth"
	"github.com/ziraloop/ziraloop/internal/enqueue"
	"github.com/ziraloop/ziraloop/internal/handler"
	"github.com/ziraloop/ziraloop/internal/middleware"
	"github.com/ziraloop/ziraloop/internal/model"
	"github.com/ziraloop/ziraloop/internal/tasks"
)

type agentDeleteHarness struct {
	db       *gorm.DB
	router   *chi.Mux
	enqueuer *enqueue.MockClient
}

func newAgentDeleteHarness(t *testing.T) *agentDeleteHarness {
	t.Helper()
	db := connectTestDB(t)

	mock := &enqueue.MockClient{}
	agentHandler := handler.NewAgentHandler(db, nil, nil, nil, mock)

	r := chi.NewRouter()
	r.Route("/v1/agents", func(r chi.Router) {
		r.Use(middleware.ResolveOrgFromHeader(db))
		r.Get("/", agentHandler.List)
		r.Get("/{id}", agentHandler.Get)
		r.Put("/{id}", agentHandler.Update)
		r.Delete("/{id}", agentHandler.Delete)
		r.Get("/{id}/setup", agentHandler.GetSetup)
		r.Put("/{id}/setup", agentHandler.UpdateSetup)
	})

	return &agentDeleteHarness{db: db, router: r, enqueuer: mock}
}

func (h *agentDeleteHarness) createTestOrg(t *testing.T) (model.Org, model.User) {
	t.Helper()
	user := model.User{Email: "agent-del-" + uuid.New().String()[:8] + "@test.com", Name: "Test"}
	if err := h.db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	org := model.Org{Name: "Test Org " + uuid.New().String()[:8], Active: true}
	if err := h.db.Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	membership := model.OrgMembership{UserID: user.ID, OrgID: org.ID, Role: "admin"}
	if err := h.db.Create(&membership).Error; err != nil {
		t.Fatalf("create membership: %v", err)
	}
	t.Cleanup(func() {
		h.db.Where("org_id = ?", org.ID).Delete(&model.Agent{})
		h.db.Where("user_id = ?", user.ID).Delete(&model.OrgMembership{})
		h.db.Where("id = ?", org.ID).Delete(&model.Org{})
		h.db.Where("id = ?", user.ID).Delete(&model.User{})
	})
	return org, user
}

func (h *agentDeleteHarness) createTestAgent(t *testing.T, orgID uuid.UUID, name string) model.Agent {
	t.Helper()
	agent := model.Agent{
		OrgID:        &orgID,
		Name:         name,
		SystemPrompt: "test prompt",
		Model:        "test-model",
		SandboxType:  "shared",
		Status:       "active",
	}
	if err := h.db.Create(&agent).Error; err != nil {
		t.Fatalf("create agent: %v", err)
	}
	return agent
}

func (h *agentDeleteHarness) doRequest(t *testing.T, method, path string, userID, orgID uuid.UUID) *httptest.ResponseRecorder {
	return h.doRequestWithBody(t, method, path, userID, orgID, nil)
}

func (h *agentDeleteHarness) doRequestWithBody(t *testing.T, method, path string, userID, orgID uuid.UUID, body any) *httptest.ResponseRecorder {
	t.Helper()
	var reqBody *bytes.Buffer
	if body != nil {
		reqBody = new(bytes.Buffer)
		json.NewEncoder(reqBody).Encode(body)
	} else {
		reqBody = new(bytes.Buffer)
	}
	req := httptest.NewRequest(method, path, reqBody)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Org-ID", orgID.String())

	claims := &auth.AuthClaims{UserID: userID.String(), OrgID: orgID.String(), Role: "admin"}
	req = middleware.WithAuthClaims(req, claims)

	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	return rr
}

func TestAgentDelete_SoftDeletesAndEnqueuesCleanup(t *testing.T) {
	h := newAgentDeleteHarness(t)
	org, user := h.createTestOrg(t)
	agent := h.createTestAgent(t, org.ID, "delete-test-"+uuid.New().String()[:8])

	rr := h.doRequest(t, "DELETE", "/v1/agents/"+agent.ID.String(), user.ID, org.ID)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]string
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp["status"] != "deleted" {
		t.Fatalf("expected status=deleted, got %v", resp["status"])
	}

	// Agent should still exist with deleted_at set
	var softDeleted model.Agent
	if err := h.db.Where("id = ?", agent.ID).First(&softDeleted).Error; err != nil {
		t.Fatalf("agent should still exist in DB: %v", err)
	}
	if softDeleted.DeletedAt == nil {
		t.Fatal("deleted_at should be set")
	}

	// Cleanup task should be enqueued
	h.enqueuer.AssertEnqueued(t, tasks.TypeAgentCleanup)
}

func TestAgentDelete_HiddenFromListAndGet(t *testing.T) {
	h := newAgentDeleteHarness(t)
	org, user := h.createTestOrg(t)
	agent := h.createTestAgent(t, org.ID, "hidden-"+uuid.New().String()[:8])

	h.doRequest(t, "DELETE", "/v1/agents/"+agent.ID.String(), user.ID, org.ID)

	// List should not include it
	rr := h.doRequest(t, "GET", "/v1/agents", user.ID, org.ID)
	if rr.Code != http.StatusOK {
		t.Fatalf("list: expected 200, got %d", rr.Code)
	}
	var listResp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &listResp)
	if data, ok := listResp["data"].([]any); ok {
		for _, raw := range data {
			agentMap := raw.(map[string]any)
			if agentMap["id"] == agent.ID.String() {
				t.Fatal("soft-deleted agent should not appear in list")
			}
		}
	}

	// Get should return 404
	rr = h.doRequest(t, "GET", "/v1/agents/"+agent.ID.String(), user.ID, org.ID)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("get: expected 404, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestAgentDelete_AlreadyDeletedReturns404(t *testing.T) {
	h := newAgentDeleteHarness(t)
	org, user := h.createTestOrg(t)
	agent := h.createTestAgent(t, org.ID, "already-del-"+uuid.New().String()[:8])

	now := time.Now()
	h.db.Model(&agent).Update("deleted_at", &now)

	rr := h.doRequest(t, "DELETE", "/v1/agents/"+agent.ID.String(), user.ID, org.ID)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for already-deleted agent, got %d", rr.Code)
	}
}

func TestAgentDelete_WrongOrgReturns404(t *testing.T) {
	h := newAgentDeleteHarness(t)
	org, _ := h.createTestOrg(t)
	otherOrg, otherUser := h.createTestOrg(t)
	agent := h.createTestAgent(t, org.ID, "wrong-org-"+uuid.New().String()[:8])

	rr := h.doRequest(t, "DELETE", "/v1/agents/"+agent.ID.String(), otherUser.ID, otherOrg.ID)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for wrong org, got %d", rr.Code)
	}
}

func TestAgentDelete_UpdateReturns404ForSoftDeleted(t *testing.T) {
	h := newAgentDeleteHarness(t)
	org, user := h.createTestOrg(t)
	agent := h.createTestAgent(t, org.ID, "update-del-"+uuid.New().String()[:8])

	// Soft-delete
	now := time.Now()
	h.db.Model(&agent).Update("deleted_at", &now)

	rr := h.doRequestWithBody(t, "PUT", "/v1/agents/"+agent.ID.String(), user.ID, org.ID, map[string]string{
		"name": "new-name",
	})
	if rr.Code != http.StatusNotFound {
		t.Fatalf("update: expected 404 for soft-deleted agent, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestAgentDelete_GetSetupReturns404ForSoftDeleted(t *testing.T) {
	h := newAgentDeleteHarness(t)
	org, user := h.createTestOrg(t)
	agent := h.createTestAgent(t, org.ID, "setup-del-"+uuid.New().String()[:8])

	now := time.Now()
	h.db.Model(&agent).Update("deleted_at", &now)

	rr := h.doRequest(t, "GET", "/v1/agents/"+agent.ID.String()+"/setup", user.ID, org.ID)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("get setup: expected 404 for soft-deleted agent, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestAgentDelete_UpdateSetupReturns404ForSoftDeleted(t *testing.T) {
	h := newAgentDeleteHarness(t)
	org, user := h.createTestOrg(t)
	agent := h.createTestAgent(t, org.ID, "usetup-del-"+uuid.New().String()[:8])

	now := time.Now()
	h.db.Model(&agent).Update("deleted_at", &now)

	rr := h.doRequestWithBody(t, "PUT", "/v1/agents/"+agent.ID.String()+"/setup", user.ID, org.ID, map[string]any{
		"setup_commands": []string{"echo hello"},
	})
	if rr.Code != http.StatusNotFound {
		t.Fatalf("update setup: expected 404 for soft-deleted agent, got %d: %s", rr.Code, rr.Body.String())
	}
}
