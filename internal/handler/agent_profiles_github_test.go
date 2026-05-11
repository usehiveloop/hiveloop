package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/handler"
	"github.com/usehiveloop/hiveloop/internal/middleware"
	"github.com/usehiveloop/hiveloop/internal/model"
	"github.com/usehiveloop/hiveloop/internal/nango"
)

func TestAgentProfileHandler_GitHubProfileAndRepositorySelection(t *testing.T) {
	db := connectTestDB(t)
	t.Cleanup(func() {
		db.Where("provider = ?", "github").Delete(&model.AgentProfile{})
		db.Where("1=1").Delete(&model.InConnection{})
		db.Where("1=1").Delete(&model.InIntegration{})
	})

	nangoSrv := httptest.NewServer(newNangoConnMock(&nangoConnMockConfig{}))
	t.Cleanup(nangoSrv.Close)
	nangoClient := nango.NewClient(nangoSrv.URL, "test-secret-key")
	_ = nangoClient.FetchProviders(context.Background())

	h := handler.NewAgentProfileHandler(db, nil, nangoClient)
	r := chi.NewRouter()
	r.Post("/v1/agents/{agentID}/profiles/github", h.CreateGitHub)
	r.Get("/v1/agents/{agentID}/profiles/github/repositories", h.ListGitHubRepositories)
	r.Patch("/v1/agents/{agentID}/profiles/github/repositories", h.UpdateGitHubRepositories)

	user := createTestUser(t, db, fmt.Sprintf("github-profile-%s@test.com", uuid.New().String()[:8]))
	org := createTestOrg(t, db)
	agent := createGitHubProfileTestEmployee(t, db, org.ID)
	integ := createTestInIntegration(t, db, "github")
	conn := model.InConnection{
		ID:                uuid.New(),
		OrgID:             org.ID,
		UserID:            user.ID,
		InIntegrationID:   integ.ID,
		NangoConnectionID: "github-conn-123",
	}
	if err := db.Create(&conn).Error; err != nil {
		t.Fatalf("create in-connection: %v", err)
	}

	body, _ := json.Marshal(map[string]any{"connection_id": conn.ID.String()})
	req := httptest.NewRequest(http.MethodPost, "/v1/agents/"+agent.ID.String()+"/profiles/github", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = middleware.WithUser(req, &user)
	req = middleware.WithOrg(req, &org)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201 creating profile, got %d: %s", rr.Code, rr.Body.String())
	}

	var createResp struct {
		Profile struct {
			Provider   string         `json:"provider"`
			ExternalID string         `json:"external_id"`
			Config     map[string]any `json:"config"`
		} `json:"profile"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&createResp); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	if createResp.Profile.Provider != "github" {
		t.Fatalf("expected github provider, got %q", createResp.Profile.Provider)
	}
	if createResp.Profile.ExternalID != "octocat" {
		t.Fatalf("expected octocat external id, got %q", createResp.Profile.ExternalID)
	}
	if createResp.Profile.Config["in_connection_id"] != conn.ID.String() {
		t.Fatalf("expected profile to store in_connection_id=%s, got %v", conn.ID, createResp.Profile.Config["in_connection_id"])
	}

	req = httptest.NewRequest(http.MethodGet, "/v1/agents/"+agent.ID.String()+"/profiles/github/repositories", nil)
	req = middleware.WithUser(req, &user)
	req = middleware.WithOrg(req, &org)
	rr = httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 listing repositories, got %d: %s", rr.Code, rr.Body.String())
	}
	var listResp struct {
		Repositories         []map[string]any `json:"repositories"`
		SelectedRepositories []map[string]any `json:"selected_repositories"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&listResp); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if len(listResp.Repositories) != 2 {
		t.Fatalf("expected 2 repositories, got %d", len(listResp.Repositories))
	}
	if len(listResp.SelectedRepositories) != 0 {
		t.Fatalf("expected no selected repositories for returning unfinished onboarding, got %d", len(listResp.SelectedRepositories))
	}

	selected := []map[string]any{
		{
			"id":        "101",
			"node_id":   "R_kgDO101",
			"name":      "alpha",
			"full_name": "octocat/alpha",
			"private":   false,
			"html_url":  "https://github.com/octocat/alpha",
			"owner":     "octocat",
		},
	}
	body, _ = json.Marshal(map[string]any{"repositories": selected})
	req = httptest.NewRequest(http.MethodPatch, "/v1/agents/"+agent.ID.String()+"/profiles/github/repositories", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = middleware.WithUser(req, &user)
	req = middleware.WithOrg(req, &org)
	rr = httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 selecting repositories, got %d: %s", rr.Code, rr.Body.String())
	}
	var updateResp struct {
		SelectedRepositories []map[string]any `json:"selected_repositories"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&updateResp); err != nil {
		t.Fatalf("decode update response: %v", err)
	}
	if len(updateResp.SelectedRepositories) != 1 || updateResp.SelectedRepositories[0]["full_name"] != "octocat/alpha" {
		t.Fatalf("expected selected octocat/alpha, got %#v", updateResp.SelectedRepositories)
	}

	body, _ = json.Marshal(map[string]any{"connection_id": conn.ID.String()})
	req = httptest.NewRequest(http.MethodPost, "/v1/agents/"+agent.ID.String()+"/profiles/github", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = middleware.WithUser(req, &user)
	req = middleware.WithOrg(req, &org)
	rr = httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected idempotent profile update to return 200, got %d: %s", rr.Code, rr.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/v1/agents/"+agent.ID.String()+"/profiles/github/repositories", nil)
	req = middleware.WithUser(req, &user)
	req = middleware.WithOrg(req, &org)
	rr = httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 listing repositories after idempotent update, got %d: %s", rr.Code, rr.Body.String())
	}
	listResp = struct {
		Repositories         []map[string]any `json:"repositories"`
		SelectedRepositories []map[string]any `json:"selected_repositories"`
	}{}
	if err := json.NewDecoder(rr.Body).Decode(&listResp); err != nil {
		t.Fatalf("decode second list response: %v", err)
	}
	if len(listResp.SelectedRepositories) != 1 || listResp.SelectedRepositories[0]["full_name"] != "octocat/alpha" {
		t.Fatalf("expected selected repos to survive idempotent profile update, got %#v", listResp.SelectedRepositories)
	}
}

func TestAgentProfileHandler_CreateGitHubRejectsNonGitHubConnection(t *testing.T) {
	db := connectTestDB(t)
	t.Cleanup(func() {
		db.Where("provider = ?", "github").Delete(&model.AgentProfile{})
		db.Where("1=1").Delete(&model.InConnection{})
		db.Where("1=1").Delete(&model.InIntegration{})
	})

	nangoSrv := httptest.NewServer(newNangoConnMock(&nangoConnMockConfig{}))
	t.Cleanup(nangoSrv.Close)
	nangoClient := nango.NewClient(nangoSrv.URL, "test-secret-key")
	_ = nangoClient.FetchProviders(context.Background())

	h := handler.NewAgentProfileHandler(db, nil, nangoClient)
	r := chi.NewRouter()
	r.Post("/v1/agents/{agentID}/profiles/github", h.CreateGitHub)

	user := createTestUser(t, db, fmt.Sprintf("github-profile-%s@test.com", uuid.New().String()[:8]))
	org := createTestOrg(t, db)
	agent := createGitHubProfileTestEmployee(t, db, org.ID)
	integ := createTestInIntegration(t, db, "slack")
	conn := model.InConnection{
		ID:                uuid.New(),
		OrgID:             org.ID,
		UserID:            user.ID,
		InIntegrationID:   integ.ID,
		NangoConnectionID: "slack-conn-123",
	}
	if err := db.Create(&conn).Error; err != nil {
		t.Fatalf("create in-connection: %v", err)
	}

	body, _ := json.Marshal(map[string]any{"connection_id": conn.ID.String()})
	req := httptest.NewRequest(http.MethodPost, "/v1/agents/"+agent.ID.String()+"/profiles/github", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = middleware.WithUser(req, &user)
	req = middleware.WithOrg(req, &org)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for non-github connection, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestAgentProfileHandler_CreateGitHubRejectsConnectionOwnedByDifferentUser(t *testing.T) {
	db := connectTestDB(t)
	t.Cleanup(func() {
		db.Where("provider = ?", "github").Delete(&model.AgentProfile{})
		db.Where("1=1").Delete(&model.InConnection{})
		db.Where("1=1").Delete(&model.InIntegration{})
	})

	nangoSrv := httptest.NewServer(newNangoConnMock(&nangoConnMockConfig{}))
	t.Cleanup(nangoSrv.Close)
	nangoClient := nango.NewClient(nangoSrv.URL, "test-secret-key")
	_ = nangoClient.FetchProviders(context.Background())

	h := handler.NewAgentProfileHandler(db, nil, nangoClient)
	r := chi.NewRouter()
	r.Post("/v1/agents/{agentID}/profiles/github", h.CreateGitHub)

	owner := createTestUser(t, db, fmt.Sprintf("github-owner-%s@test.com", uuid.New().String()[:8]))
	attacker := createTestUser(t, db, fmt.Sprintf("github-attacker-%s@test.com", uuid.New().String()[:8]))
	org := createTestOrg(t, db)
	agent := createGitHubProfileTestEmployee(t, db, org.ID)
	integ := createTestInIntegration(t, db, "github")
	conn := model.InConnection{
		ID:                uuid.New(),
		OrgID:             org.ID,
		UserID:            owner.ID,
		InIntegrationID:   integ.ID,
		NangoConnectionID: "github-owned-by-someone-else",
	}
	if err := db.Create(&conn).Error; err != nil {
		t.Fatalf("create in-connection: %v", err)
	}

	body, _ := json.Marshal(map[string]any{"connection_id": conn.ID.String()})
	req := httptest.NewRequest(http.MethodPost, "/v1/agents/"+agent.ID.String()+"/profiles/github", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = middleware.WithUser(req, &attacker)
	req = middleware.WithOrg(req, &org)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for another user's connection, got %d: %s", rr.Code, rr.Body.String())
	}
}

func createGitHubProfileTestEmployee(t *testing.T, db interface {
	Create(value any) *gorm.DB
	Where(query any, args ...any) *gorm.DB
}, orgID uuid.UUID) model.Agent {
	t.Helper()
	agent := model.Agent{
		ID:           uuid.New(),
		OrgID:        &orgID,
		Name:         "employee-" + uuid.NewString()[:8],
		IsEmployee:   true,
		Harness:      "employee-sandbox",
		Model:        "deepseek/deepseek-v4-flash",
		SystemPrompt: "you are a test employee",
		Status:       "active",
		Tools:        model.JSON{},
		McpServers:   model.JSON{},
		Skills:       model.JSON{},
		Integrations: model.JSON{},
		Resources:    model.JSON{},
		AgentConfig:  model.JSON{},
		Permissions:  model.JSON{},
	}
	if err := db.Create(&agent).Error; err != nil {
		t.Fatalf("create agent: %v", err)
	}
	t.Cleanup(func() {
		db.Where("agent_id = ?", agent.ID).Delete(&model.AgentProfile{})
		db.Where("id = ?", agent.ID).Delete(&model.Agent{})
	})
	return agent
}
