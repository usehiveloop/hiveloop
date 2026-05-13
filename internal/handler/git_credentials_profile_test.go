package handler_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/model"
)

func TestGitCredentials_UsesAgentGitHubProfileConnection(t *testing.T) {
	requestedPaths := []string{}
	nangoHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestedPaths = append(requestedPaths, r.URL.String())
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"provider": "github",
			"credentials": map[string]any{
				"access_token": "ghs_profile_token",
			},
		})
	})

	harness := newGitCredsHarness(t, nangoHandler)
	profileConnID, profileNangoID, providerConfigKey := createGitProfileConnectionForGitCreds(t, harness.db, harness.orgID, harness.agentID, "profile")

	req := httptest.NewRequest(http.MethodPost,
		"/internal/git-credentials/"+harness.agentID.String(), nil)
	req.Header.Set("Authorization", "Bearer "+harness.bridgeKey)
	recorder := httptest.NewRecorder()
	harness.router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	if recorder.Body.String() != "username=x-access-token\npassword=ghs_profile_token\n" {
		t.Fatalf("unexpected response body: %q", recorder.Body.String())
	}
	if len(requestedPaths) != 1 {
		t.Fatalf("nango calls = %d, want 1: %#v", len(requestedPaths), requestedPaths)
	}
	want := fmt.Sprintf("/connection/%s?provider_config_key=%s", profileNangoID, providerConfigKey)
	if requestedPaths[0] != want {
		t.Fatalf("nango path = %q, want %q (profile conn %s)", requestedPaths[0], want, profileConnID)
	}
}

func TestGitCredentials_ProfileConnectionChangeBypassesCache(t *testing.T) {
	tokens := map[string]string{}
	callCount := 0
	nangoHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		token := tokens[r.URL.Path]
		if token == "" {
			token = "ghs_unknown"
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"provider": "github",
			"credentials": map[string]any{
				"access_token": token,
			},
		})
	})

	harness := newGitCredsHarness(t, nangoHandler)
	firstConnID, firstNangoID, providerConfigKey := createGitProfileConnectionForGitCreds(t, harness.db, harness.orgID, harness.agentID, "profile-a")
	tokens["/connection/"+firstNangoID] = "ghs_first_profile_token"

	req := httptest.NewRequest(http.MethodPost,
		"/internal/git-credentials/"+harness.agentID.String(), nil)
	req.Header.Set("Authorization", "Bearer "+harness.bridgeKey)
	recorder := httptest.NewRecorder()
	harness.router.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("first request expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	if recorder.Body.String() != "username=x-access-token\npassword=ghs_first_profile_token\n" {
		t.Fatalf("unexpected first response body: %q", recorder.Body.String())
	}

	secondConnID, secondNangoID := createGitConnectionForExistingIntegration(t, harness.db, harness.orgID, firstConnID, "profile-b")
	tokens["/connection/"+secondNangoID] = "ghs_second_profile_token"
	if err := harness.db.Model(&model.AgentProfile{}).
		Where("agent_id = ? AND provider = ?", harness.agentID, "github").
		Updates(map[string]any{
			"config": model.JSON{
				"in_connection_id":    secondConnID.String(),
				"provider_config_key": providerConfigKey,
			},
		}).Error; err != nil {
		t.Fatalf("update profile connection from %s to %s: %v", firstConnID, secondConnID, err)
	}

	req = httptest.NewRequest(http.MethodPost,
		"/internal/git-credentials/"+harness.agentID.String(), nil)
	req.Header.Set("Authorization", "Bearer "+harness.bridgeKey)
	recorder = httptest.NewRecorder()
	harness.router.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("second request expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	if recorder.Body.String() != "username=x-access-token\npassword=ghs_second_profile_token\n" {
		t.Fatalf("unexpected second response body: %q", recorder.Body.String())
	}
	if callCount != 2 {
		t.Fatalf("nango calls = %d, want 2 after profile connection change", callCount)
	}
}

func createGitProfileConnectionForGitCreds(t *testing.T, db *gorm.DB, orgID uuid.UUID, agentID uuid.UUID, suffix string) (uuid.UUID, string, string) {
	t.Helper()
	connID, nangoID, providerConfigKey := createGitConnectionForGitCreds(t, db, orgID, suffix)
	profile := model.AgentProfile{
		ID:         uuid.New(),
		OrgID:      orgID,
		AgentID:    agentID,
		Provider:   "github",
		ExternalID: "octocat",
		Label:      "octocat",
		Identity:   model.JSON{},
		Config: model.JSON{
			"in_connection_id":    connID.String(),
			"provider_config_key": providerConfigKey,
		},
		Status: "active",
	}
	if err := db.Create(&profile).Error; err != nil {
		t.Fatalf("create github profile: %v", err)
	}
	t.Cleanup(func() { db.Where("id = ?", profile.ID).Delete(&model.AgentProfile{}) })
	return connID, nangoID, providerConfigKey
}

func createGitConnectionForGitCreds(t *testing.T, db *gorm.DB, orgID uuid.UUID, suffix string) (uuid.UUID, string, string) {
	t.Helper()
	user := model.User{
		ID:    uuid.New(),
		Email: fmt.Sprintf("git-profile-%s-%s@example.com", suffix, uuid.New().String()[:8]),
		Name:  "Git Profile User",
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create profile user: %v", err)
	}
	t.Cleanup(func() { db.Where("id = ?", user.ID).Delete(&model.User{}) })

	uniqueKey := fmt.Sprintf("github-profile-%s-%s", suffix, uuid.New().String()[:8])
	integration := model.InIntegration{
		ID:          uuid.New(),
		UniqueKey:   uniqueKey,
		Provider:    "github",
		DisplayName: "GitHub Profile",
	}
	if err := db.Create(&integration).Error; err != nil {
		t.Fatalf("create profile integration: %v", err)
	}
	t.Cleanup(func() { db.Where("id = ?", integration.ID).Delete(&model.InIntegration{}) })

	conn := model.InConnection{
		ID:                uuid.New(),
		OrgID:             orgID,
		UserID:            user.ID,
		InIntegrationID:   integration.ID,
		NangoConnectionID: "nango-" + suffix + "-" + uuid.New().String()[:8],
	}
	if err := db.Create(&conn).Error; err != nil {
		t.Fatalf("create profile connection: %v", err)
	}
	return conn.ID, conn.NangoConnectionID, "in_" + uniqueKey
}

func createGitConnectionForExistingIntegration(t *testing.T, db *gorm.DB, orgID uuid.UUID, existingConnID uuid.UUID, suffix string) (uuid.UUID, string) {
	t.Helper()
	var existing model.InConnection
	if err := db.Where("id = ?", existingConnID).First(&existing).Error; err != nil {
		t.Fatalf("load existing connection: %v", err)
	}
	user := model.User{
		ID:    uuid.New(),
		Email: fmt.Sprintf("git-profile-%s-%s@example.com", suffix, uuid.New().String()[:8]),
		Name:  "Git Profile User",
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create second profile user: %v", err)
	}
	t.Cleanup(func() { db.Where("id = ?", user.ID).Delete(&model.User{}) })

	conn := model.InConnection{
		ID:                uuid.New(),
		OrgID:             orgID,
		UserID:            user.ID,
		InIntegrationID:   existing.InIntegrationID,
		NangoConnectionID: "nango-" + suffix + "-" + uuid.New().String()[:8],
	}
	if err := db.Create(&conn).Error; err != nil {
		t.Fatalf("create second profile connection: %v", err)
	}
	return conn.ID, conn.NangoConnectionID
}
