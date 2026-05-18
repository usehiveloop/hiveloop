package handler_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/usehiveloop/hiveloop/internal/auth"
	"github.com/usehiveloop/hiveloop/internal/credentials"
	"github.com/usehiveloop/hiveloop/internal/middleware"
	"github.com/usehiveloop/hiveloop/internal/model"
)

func (h *agentCreateHarness) platformCredCleanup(t *testing.T) {
	t.Helper()
	h.db.Unscoped().Where("org_id = ?", credentials.PlatformOrgID).Delete(&model.Credential{})
	t.Cleanup(func() {
		h.db.Unscoped().Where("org_id = ?", credentials.PlatformOrgID).Delete(&model.Credential{})
	})
}

func (h *agentCreateHarness) seedSystemCred(t *testing.T, providerID string, revoked bool) model.Credential {
	t.Helper()
	cred := model.Credential{
		OrgID:        credentials.PlatformOrgID,
		Label:        "sys-" + providerID,
		BaseURL:      "https://api.example.com",
		AuthScheme:   "bearer",
		EncryptedKey: []byte("enc"),
		WrappedDEK:   []byte("dek"),
		ProviderID:   providerID,
		IsSystem:     true,
	}
	if revoked {
		now := time.Now()
		cred.RevokedAt = &now
	}
	if err := h.db.Create(&cred).Error; err != nil {
		t.Fatalf("seed system cred %s: %v", providerID, err)
	}
	t.Cleanup(func() { h.db.Unscoped().Delete(&cred) })
	return cred
}

func (h *agentCreateHarness) seedAgentWithCredential(t *testing.T, orgID uuid.UUID, credentialID *uuid.UUID, modelID string) model.Agent {
	t.Helper()
	agent := model.Agent{
		OrgID:           &orgID,
		Name:            "agent-update-" + uuid.NewString()[:8],
		SystemPrompt:    "You are helpful.",
		Model:           modelID,
		CredentialID:    credentialID,
		Tools:           model.JSON{},
		McpServers:      model.JSON{},
		Skills:          model.JSON{},
		Integrations:    model.JSON{},
		AgentConfig:     model.JSON{},
		Permissions:     model.JSON{},
		Resources:       model.JSON{},
		SandboxTools:    []string{},
		SetupCommands:   []string{},
		Status:          "active",
		IsSystem:        false,
		IsEmployee:      false,
		ProviderPrompts: model.ProviderPromptsMap{},
	}
	if err := h.db.Create(&agent).Error; err != nil {
		t.Fatalf("seed agent: %v", err)
	}
	t.Cleanup(func() { h.db.Where("id = ?", agent.ID).Delete(&model.Agent{}) })
	return agent
}

func (h *agentCreateHarness) put(t *testing.T, userID, orgID, agentID uuid.UUID, body any) *httptest.ResponseRecorder {
	t.Helper()
	buf := new(bytes.Buffer)
	_ = json.NewEncoder(buf).Encode(body)
	req := httptest.NewRequest(http.MethodPut, "/v1/agents/"+agentID.String(), buf)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Org-ID", orgID.String())

	claims := &auth.AuthClaims{UserID: userID.String(), OrgID: orgID.String(), Role: "admin"}
	req = middleware.WithAuthClaims(req, claims)

	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	return rr
}

func TestAgentUpdate_ModelUsesActiveSystemCredential(t *testing.T) {
	h := newAgentCreateHarness(t)
	h.platformCredCleanup(t)
	org, user := h.createOrgWithBYOK(t, false)
	openRouterCred := h.seedSystemCred(t, "openrouter", false)
	googleCred := h.seedSystemCred(t, "google", false)
	agent := h.seedAgentWithCredential(t, org.ID, &openRouterCred.ID, "deepseek/deepseek-v4-flash")

	rr := h.put(t, user.ID, org.ID, agent.ID, map[string]any{
		"model": " gemini-3-flash-preview ",
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var reloaded model.Agent
	if err := h.db.Where("id = ?", agent.ID).First(&reloaded).Error; err != nil {
		t.Fatalf("reload agent: %v", err)
	}
	if reloaded.Model != "gemini-3-flash-preview" {
		t.Fatalf("model = %q, want gemini-3-flash-preview", reloaded.Model)
	}
	if reloaded.CredentialID == nil || *reloaded.CredentialID != googleCred.ID {
		t.Fatalf("credential_id = %v, want %s", reloaded.CredentialID, googleCred.ID)
	}
}

func TestAgentUpdate_ModelRejectsInvalidValues(t *testing.T) {
	tests := []struct {
		name      string
		model     string
		wantError string
	}{
		{name: "empty", model: " ", wantError: "model is required"},
		{name: "unknown", model: "does-not-exist", wantError: "not in the catalog"},
		{name: "hidden", model: "openai/gpt-5-nano", wantError: "not selectable"},
		{name: "unbacked", model: "gpt-5.4", wantError: "not backed by an active system credential"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newAgentCreateHarness(t)
			h.platformCredCleanup(t)
			org, user := h.createOrgWithBYOK(t, false)
			openRouterCred := h.seedSystemCred(t, "openrouter", false)
			agent := h.seedAgentWithCredential(t, org.ID, &openRouterCred.ID, "deepseek/deepseek-v4-flash")

			rr := h.put(t, user.ID, org.ID, agent.ID, map[string]any{
				"model": tt.model,
			})
			if rr.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d: %s", rr.Code, rr.Body.String())
			}
			if msg := decodeError(t, rr); !strings.Contains(msg, tt.wantError) {
				t.Fatalf("expected %q in error, got %q", tt.wantError, msg)
			}

			var reloaded model.Agent
			if err := h.db.Where("id = ?", agent.ID).First(&reloaded).Error; err != nil {
				t.Fatalf("reload agent: %v", err)
			}
			if reloaded.Model != agent.Model {
				t.Fatalf("model changed after rejected update: got %q want %q", reloaded.Model, agent.Model)
			}
			if reloaded.CredentialID == nil || *reloaded.CredentialID != openRouterCred.ID {
				t.Fatalf("credential changed after rejected update: got %v want %s", reloaded.CredentialID, openRouterCred.ID)
			}
		})
	}
}
