package handler_test

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	hivecrypto "github.com/usehiveloop/hiveloop/internal/crypto"
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

	mockCfg := &nangoConnMockConfig{}
	nangoSrv := httptest.NewServer(newNangoConnMock(mockCfg))
	t.Cleanup(nangoSrv.Close)
	nangoClient := nango.NewClient(nangoSrv.URL, "test-secret-key")
	_ = nangoClient.FetchProviders(context.Background())

	h := newGitHubProfileTestHandler(t, db, nangoClient)
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
			Identity   map[string]any `json:"identity"`
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
	if createResp.Profile.Identity["email"] != "octocat@example.com" {
		t.Fatalf("expected verified email in identity, got %#v", createResp.Profile.Identity)
	}
	if got := countCapturedNangoRequests(mockCfg, http.MethodGet, "/proxy/user/emails"); got != 1 {
		t.Fatalf("github email requests = %d, want 1", got)
	}
	if createResp.Profile.Config["in_connection_id"] != conn.ID.String() {
		t.Fatalf("expected profile to store in_connection_id=%s, got %v", conn.ID, createResp.Profile.Config["in_connection_id"])
	}
	var createdProfile model.AgentProfile
	if err := db.Where("agent_id = ? AND provider = ?", agent.ID, "github").First(&createdProfile).Error; err != nil {
		t.Fatalf("load created profile: %v", err)
	}
	if len(createdProfile.EncryptedIdentity) == 0 {
		t.Fatal("expected encrypted identity to be stored")
	}
	if len(createdProfile.Identity) != 0 {
		t.Fatalf("expected plaintext identity to be empty, got %#v", createdProfile.Identity)
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
		Profile struct {
			Identity map[string]any `json:"identity"`
		} `json:"profile"`
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
	if listResp.Profile.Identity["email"] != "octocat@example.com" {
		t.Fatalf("expected decrypted identity email in list response, got %#v", listResp.Profile.Identity)
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
		Profile struct {
			Identity map[string]any `json:"identity"`
		} `json:"profile"`
		Repositories         []map[string]any `json:"repositories"`
		SelectedRepositories []map[string]any `json:"selected_repositories"`
	}{}
	if err := json.NewDecoder(rr.Body).Decode(&listResp); err != nil {
		t.Fatalf("decode second list response: %v", err)
	}
	if len(listResp.SelectedRepositories) != 1 || listResp.SelectedRepositories[0]["full_name"] != "octocat/alpha" {
		t.Fatalf("expected selected repos to survive idempotent profile update, got %#v", listResp.SelectedRepositories)
	}

	otherUser := createTestUser(t, db, fmt.Sprintf("github-profile-other-%s@test.com", uuid.New().String()[:8]))
	otherConn := model.InConnection{
		ID:                uuid.New(),
		OrgID:             org.ID,
		UserID:            otherUser.ID,
		InIntegrationID:   integ.ID,
		NangoConnectionID: "github-conn-456",
	}
	if err := db.Create(&otherConn).Error; err != nil {
		t.Fatalf("create second in-connection: %v", err)
	}
	body, _ = json.Marshal(map[string]any{"connection_id": otherConn.ID.String()})
	req = httptest.NewRequest(http.MethodPost, "/v1/agents/"+agent.ID.String()+"/profiles/github", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = middleware.WithUser(req, &user)
	req = middleware.WithOrg(req, &org)
	rr = httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected replacing employee github profile to return 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var profileCount int64
	if err := db.Model(&model.AgentProfile{}).
		Where("agent_id = ? AND provider = ? AND deleted_at IS NULL", agent.ID, "github").
		Count(&profileCount).Error; err != nil {
		t.Fatalf("count github profiles: %v", err)
	}
	if profileCount != 1 {
		t.Fatalf("expected exactly one github profile for employee, got %d", profileCount)
	}

	req = httptest.NewRequest(http.MethodGet, "/v1/agents/"+agent.ID.String()+"/profiles/github/repositories", nil)
	req = middleware.WithUser(req, &user)
	req = middleware.WithOrg(req, &org)
	rr = httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 listing repositories after profile replacement, got %d: %s", rr.Code, rr.Body.String())
	}
	listResp = struct {
		Profile struct {
			Identity map[string]any `json:"identity"`
		} `json:"profile"`
		Repositories         []map[string]any `json:"repositories"`
		SelectedRepositories []map[string]any `json:"selected_repositories"`
	}{}
	if err := json.NewDecoder(rr.Body).Decode(&listResp); err != nil {
		t.Fatalf("decode third list response: %v", err)
	}
	if len(listResp.SelectedRepositories) != 0 {
		t.Fatalf("expected selected repos to reset after switching github connection, got %#v", listResp.SelectedRepositories)
	}
}

func TestAgentProfileHandler_GitHubRepositorySelectionCreatesHooksForNewRepos(t *testing.T) {
	db := connectTestDB(t)
	t.Cleanup(func() {
		db.Where("provider = ?", "github").Delete(&model.AgentProfile{})
		db.Where("1=1").Delete(&model.InConnection{})
		db.Where("1=1").Delete(&model.InIntegration{})
	})

	mockCfg := &nangoConnMockConfig{}
	nangoSrv := httptest.NewServer(newNangoConnMock(mockCfg))
	t.Cleanup(nangoSrv.Close)
	nangoClient := nango.NewClient(nangoSrv.URL, "test-secret-key")
	_ = nangoClient.FetchProviders(context.Background())

	h := newGitHubProfileTestHandler(t, db, nangoClient)
	r := chi.NewRouter()
	r.Post("/v1/agents/{agentID}/profiles/github", h.CreateGitHub)
	r.Patch("/v1/agents/{agentID}/profiles/github/repositories", h.UpdateGitHubRepositories)

	user := createTestUser(t, db, fmt.Sprintf("github-hooks-%s@test.com", uuid.New().String()[:8]))
	org := createTestOrg(t, db)
	agent := createGitHubProfileTestEmployee(t, db, org.ID)
	integ := createTestInIntegration(t, db, "github")
	conn := model.InConnection{
		ID:                uuid.New(),
		OrgID:             org.ID,
		UserID:            user.ID,
		InIntegrationID:   integ.ID,
		NangoConnectionID: "github-hooks-conn",
	}
	if err := db.Create(&conn).Error; err != nil {
		t.Fatalf("create in-connection: %v", err)
	}
	createGitHubProfileForTest(t, r, user, org, agent.ID, conn.ID)

	alpha := map[string]any{
		"id":        "101",
		"node_id":   "R_kgDO101",
		"name":      "alpha",
		"full_name": "octocat/alpha",
		"private":   false,
		"html_url":  "https://github.com/octocat/alpha",
		"owner":     "octocat",
	}
	privateBeta := map[string]any{
		"id":        "102",
		"node_id":   "R_kgDO102",
		"name":      "private-beta",
		"full_name": "octocat/private-beta",
		"private":   true,
		"html_url":  "https://github.com/octocat/private-beta",
		"owner":     "octocat",
	}

	updateGitHubReposForTest(t, r, user, org, agent.ID, []map[string]any{alpha}, http.StatusOK)
	if got := countCapturedNangoRequests(mockCfg, http.MethodPost, "/proxy/repos/octocat/alpha/hooks"); got != 1 {
		t.Fatalf("alpha hook creates after first save = %d, want 1", got)
	}
	assertGitHubHookCreatePayload(t, mockCfg, "/proxy/repos/octocat/alpha/hooks", agent.ID)

	updateGitHubReposForTest(t, r, user, org, agent.ID, []map[string]any{alpha, privateBeta}, http.StatusOK)
	if got := countCapturedNangoRequests(mockCfg, http.MethodPost, "/proxy/repos/octocat/alpha/hooks"); got != 1 {
		t.Fatalf("alpha hook creates after second save = %d, want still 1", got)
	}
	if got := countCapturedNangoRequests(mockCfg, http.MethodPost, "/proxy/repos/octocat/private-beta/hooks"); got != 1 {
		t.Fatalf("private-beta hook creates after second save = %d, want 1", got)
	}

	var profile model.AgentProfile
	if err := db.Where("agent_id = ? AND provider = ?", agent.ID, "github").First(&profile).Error; err != nil {
		t.Fatalf("load profile: %v", err)
	}
	if got := selectedRepoCount(t, profile.Config); got != 2 {
		t.Fatalf("selected repos saved = %d, want 2", got)
	}
	if got := webhookMetadataCount(t, profile.Config); got != 2 {
		t.Fatalf("webhook metadata entries = %d, want 2", got)
	}
}

func TestAgentProfileHandler_GitHubRepositorySelectionBlocksSaveWhenHookCreateFails(t *testing.T) {
	db := connectTestDB(t)
	t.Cleanup(func() {
		db.Where("provider = ?", "github").Delete(&model.AgentProfile{})
		db.Where("1=1").Delete(&model.InConnection{})
		db.Where("1=1").Delete(&model.InIntegration{})
	})

	mockCfg := &nangoConnMockConfig{hookCreateStatus: http.StatusBadGateway}
	nangoSrv := httptest.NewServer(newNangoConnMock(mockCfg))
	t.Cleanup(nangoSrv.Close)
	nangoClient := nango.NewClient(nangoSrv.URL, "test-secret-key")
	_ = nangoClient.FetchProviders(context.Background())

	h := newGitHubProfileTestHandler(t, db, nangoClient)
	r := chi.NewRouter()
	r.Post("/v1/agents/{agentID}/profiles/github", h.CreateGitHub)
	r.Patch("/v1/agents/{agentID}/profiles/github/repositories", h.UpdateGitHubRepositories)

	user := createTestUser(t, db, fmt.Sprintf("github-hooks-fail-%s@test.com", uuid.New().String()[:8]))
	org := createTestOrg(t, db)
	agent := createGitHubProfileTestEmployee(t, db, org.ID)
	integ := createTestInIntegration(t, db, "github")
	conn := model.InConnection{
		ID:                uuid.New(),
		OrgID:             org.ID,
		UserID:            user.ID,
		InIntegrationID:   integ.ID,
		NangoConnectionID: "github-hooks-fail-conn",
	}
	if err := db.Create(&conn).Error; err != nil {
		t.Fatalf("create in-connection: %v", err)
	}
	createGitHubProfileForTest(t, r, user, org, agent.ID, conn.ID)

	updateGitHubReposForTest(t, r, user, org, agent.ID, []map[string]any{{
		"id":        "101",
		"name":      "alpha",
		"full_name": "octocat/alpha",
	}}, http.StatusBadGateway)

	var profile model.AgentProfile
	if err := db.Where("agent_id = ? AND provider = ?", agent.ID, "github").First(&profile).Error; err != nil {
		t.Fatalf("load profile: %v", err)
	}
	if got := selectedRepoCount(t, profile.Config); got != 0 {
		t.Fatalf("selected repos saved after hook failure = %d, want 0", got)
	}
}

func TestAgentProfileHandler_CreateGitHubRequiresVerifiedEmail(t *testing.T) {
	db := connectTestDB(t)
	t.Cleanup(func() {
		db.Where("provider = ?", "github").Delete(&model.AgentProfile{})
		db.Where("1=1").Delete(&model.InConnection{})
		db.Where("1=1").Delete(&model.InIntegration{})
	})

	mockCfg := &nangoConnMockConfig{
		githubEmails: []map[string]any{
			{"email": "octocat-unverified@example.com", "verified": false},
			{"email": "", "verified": true},
		},
	}
	nangoSrv := httptest.NewServer(newNangoConnMock(mockCfg))
	t.Cleanup(nangoSrv.Close)
	nangoClient := nango.NewClient(nangoSrv.URL, "test-secret-key")
	_ = nangoClient.FetchProviders(context.Background())

	h := newGitHubProfileTestHandler(t, db, nangoClient)
	r := chi.NewRouter()
	r.Post("/v1/agents/{agentID}/profiles/github", h.CreateGitHub)

	user := createTestUser(t, db, fmt.Sprintf("github-no-email-%s@test.com", uuid.New().String()[:8]))
	org := createTestOrg(t, db)
	agent := createGitHubProfileTestEmployee(t, db, org.ID)
	integ := createTestInIntegration(t, db, "github")
	conn := model.InConnection{
		ID:                uuid.New(),
		OrgID:             org.ID,
		UserID:            user.ID,
		InIntegrationID:   integ.ID,
		NangoConnectionID: "github-no-email-conn",
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
	if rr.Code != http.StatusBadGateway {
		t.Fatalf("expected 502 without verified email, got %d: %s", rr.Code, rr.Body.String())
	}
	if got := countCapturedNangoRequests(mockCfg, http.MethodGet, "/proxy/user/emails"); got != 1 {
		t.Fatalf("github email requests = %d, want 1", got)
	}
	var profileCount int64
	if err := db.Model(&model.AgentProfile{}).Where("agent_id = ? AND provider = ?", agent.ID, "github").Count(&profileCount).Error; err != nil {
		t.Fatalf("count profiles: %v", err)
	}
	if profileCount != 0 {
		t.Fatalf("profiles saved without verified email = %d, want 0", profileCount)
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

	h := handler.NewAgentProfileHandler(db, nil, nil, nangoClient)
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

func TestAgentProfileHandler_CreateGitHubAllowsOrgConnectionOwnedByDifferentUser(t *testing.T) {
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

	h := handler.NewAgentProfileHandler(db, nil, testSymmetricKey(t), nangoClient)
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
	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201 for org-scoped github connection, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestGitHubEmployeeWebhookHandlerAcceptsValidSignature(t *testing.T) {
	db := connectTestDB(t)
	t.Cleanup(func() {
		db.Where("provider = ?", "github").Delete(&model.AgentProfile{})
	})

	kms := newTestKMS(t)
	org := createTestOrg(t, db)
	agent := createGitHubProfileTestEmployee(t, db, org.ID)
	createGitHubWebhookProfileForTest(t, db, kms, org.ID, agent.ID, "github-webhook-secret")

	h := handler.NewGitHubEmployeeWebhookHandler(db, kms)
	r := chi.NewRouter()
	r.Post("/internal/webhooks/github/employees/{agentID}", h.Handle)

	body := []byte(`{"action":"opened","repository":{"full_name":"octocat/alpha"}}`)
	req := httptest.NewRequest(http.MethodPost, "/internal/webhooks/github/employees/"+agent.ID.String(), bytes.NewReader(body))
	req.Header.Set("X-Hub-Signature-256", signGitHubWebhook("github-webhook-secret", body))
	req.Header.Set("X-GitHub-Event", "issues")
	req.Header.Set("X-GitHub-Delivery", uuid.NewString())
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 for valid signature, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestGitHubEmployeeWebhookHandlerRejectsInvalidSignature(t *testing.T) {
	db := connectTestDB(t)
	t.Cleanup(func() {
		db.Where("provider = ?", "github").Delete(&model.AgentProfile{})
	})

	kms := newTestKMS(t)
	org := createTestOrg(t, db)
	agent := createGitHubProfileTestEmployee(t, db, org.ID)
	createGitHubWebhookProfileForTest(t, db, kms, org.ID, agent.ID, "github-webhook-secret")

	h := handler.NewGitHubEmployeeWebhookHandler(db, kms)
	r := chi.NewRouter()
	r.Post("/internal/webhooks/github/employees/{agentID}", h.Handle)

	body := []byte(`{"action":"opened","repository":{"full_name":"octocat/alpha"}}`)
	req := httptest.NewRequest(http.MethodPost, "/internal/webhooks/github/employees/"+agent.ID.String(), bytes.NewReader(body))
	req.Header.Set("X-Hub-Signature-256", signGitHubWebhook("wrong-secret", body))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for invalid signature, got %d: %s", rr.Code, rr.Body.String())
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

func newGitHubProfileTestHandler(t *testing.T, db *gorm.DB, nangoClient *nango.Client) *handler.AgentProfileHandler {
	t.Helper()
	h := handler.NewAgentProfileHandler(db, newTestKMS(t), testSymmetricKey(t), nangoClient)
	h.SetWebhookBaseURL("https://api.hiveloop.test")
	return h
}

func createGitHubProfileForTest(t *testing.T, r http.Handler, user model.User, org model.Org, agentID uuid.UUID, connectionID uuid.UUID) {
	t.Helper()
	body, _ := json.Marshal(map[string]any{"connection_id": connectionID.String()})
	req := httptest.NewRequest(http.MethodPost, "/v1/agents/"+agentID.String()+"/profiles/github", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = middleware.WithUser(req, &user)
	req = middleware.WithOrg(req, &org)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated && rr.Code != http.StatusOK {
		t.Fatalf("create github profile: got %d: %s", rr.Code, rr.Body.String())
	}
}

func updateGitHubReposForTest(t *testing.T, r http.Handler, user model.User, org model.Org, agentID uuid.UUID, repos []map[string]any, wantStatus int) {
	t.Helper()
	body, _ := json.Marshal(map[string]any{"repositories": repos})
	req := httptest.NewRequest(http.MethodPatch, "/v1/agents/"+agentID.String()+"/profiles/github/repositories", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = middleware.WithUser(req, &user)
	req = middleware.WithOrg(req, &org)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != wantStatus {
		t.Fatalf("update github repos: got %d, want %d: %s", rr.Code, wantStatus, rr.Body.String())
	}
}

func countCapturedNangoRequests(cfg *nangoConnMockConfig, method string, path string) int {
	cfg.mu.Lock()
	defer cfg.mu.Unlock()
	count := 0
	for i := range cfg.capturedPaths {
		if cfg.capturedMethods[i] == method && cfg.capturedPaths[i] == path {
			count++
		}
	}
	return count
}

func assertGitHubHookCreatePayload(t *testing.T, cfg *nangoConnMockConfig, path string, agentID uuid.UUID) {
	t.Helper()
	cfg.mu.Lock()
	defer cfg.mu.Unlock()
	for i := range cfg.capturedPaths {
		if cfg.capturedMethods[i] != http.MethodPost || cfg.capturedPaths[i] != path {
			continue
		}
		var payload struct {
			Name   string            `json:"name"`
			Active bool              `json:"active"`
			Events []string          `json:"events"`
			Config map[string]string `json:"config"`
		}
		if err := json.Unmarshal(cfg.capturedBodies[i], &payload); err != nil {
			t.Fatalf("decode hook payload: %v", err)
		}
		if payload.Name != "web" {
			t.Fatalf("hook name = %q, want web", payload.Name)
		}
		if !payload.Active {
			t.Fatal("hook active = false, want true")
		}
		if !containsString(payload.Events, "pull_request") || !containsString(payload.Events, "issues") || !containsString(payload.Events, "workflow_job") {
			t.Fatalf("hook events missing expected values: %#v", payload.Events)
		}
		wantURL := "https://api.hiveloop.test/internal/webhooks/github/employees/" + agentID.String()
		if payload.Config["url"] != wantURL {
			t.Fatalf("hook url = %q, want %q", payload.Config["url"], wantURL)
		}
		if payload.Config["content_type"] != "json" {
			t.Fatalf("content_type = %q, want json", payload.Config["content_type"])
		}
		if payload.Config["secret"] == "" {
			t.Fatal("expected webhook secret in hook config")
		}
		return
	}
	t.Fatalf("did not find hook POST payload for %s", path)
}

func selectedRepoCount(t *testing.T, cfg model.JSON) int {
	t.Helper()
	raw := cfg["selected_repositories"]
	b, err := json.Marshal(raw)
	if err != nil {
		t.Fatalf("marshal selected repos: %v", err)
	}
	var repos []map[string]any
	if err := json.Unmarshal(b, &repos); err != nil {
		t.Fatalf("decode selected repos: %v", err)
	}
	return len(repos)
}

func webhookMetadataCount(t *testing.T, cfg model.JSON) int {
	t.Helper()
	raw := cfg["github_webhooks"]
	b, err := json.Marshal(raw)
	if err != nil {
		t.Fatalf("marshal webhook metadata: %v", err)
	}
	var metadata map[string]any
	if err := json.Unmarshal(b, &metadata); err != nil {
		t.Fatalf("decode webhook metadata: %v", err)
	}
	return len(metadata)
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func createGitHubWebhookProfileForTest(t *testing.T, db *gorm.DB, kms *hivecrypto.KeyWrapper, orgID uuid.UUID, agentID uuid.UUID, secret string) model.AgentProfile {
	t.Helper()
	plaintext, err := json.Marshal(map[string]any{"webhook_secret": secret})
	if err != nil {
		t.Fatalf("marshal webhook secret: %v", err)
	}
	dek, err := hivecrypto.GenerateDEK()
	if err != nil {
		t.Fatalf("generate DEK: %v", err)
	}
	encrypted, err := hivecrypto.EncryptCredential(plaintext, dek)
	if err != nil {
		t.Fatalf("encrypt secret: %v", err)
	}
	wrapped, err := kms.Wrap(context.Background(), dek)
	if err != nil {
		t.Fatalf("wrap DEK: %v", err)
	}
	profile := model.AgentProfile{
		ID:               uuid.New(),
		OrgID:            orgID,
		AgentID:          agentID,
		Provider:         "github",
		ExternalID:       "octocat",
		Label:            "octocat",
		Identity:         model.JSON{},
		Config:           model.JSON{},
		EncryptedSecrets: encrypted,
		WrappedDEK:       wrapped,
		Status:           "active",
	}
	if err := db.Create(&profile).Error; err != nil {
		t.Fatalf("create github profile: %v", err)
	}
	return profile
}

func signGitHubWebhook(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}
