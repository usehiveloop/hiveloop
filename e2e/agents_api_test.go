package e2e

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/ziraloop/ziraloop/internal/handler"
	"github.com/ziraloop/ziraloop/internal/middleware"
	"github.com/ziraloop/ziraloop/internal/model"
	"github.com/ziraloop/ziraloop/internal/registry"
)

// agentAPIHarness extends testHarness with agent-related routes.
type agentAPIHarness struct {
	*testHarness
	org      model.Org
	cred     model.Credential
	identity model.Identity
	router   *chi.Mux
}

func newAgentAPIHarness(t *testing.T) *agentAPIHarness {
	t.Helper()
	h := newHarness(t)

	suffix := uuid.New().String()[:8]

	// Create org
	org := model.Org{Name: "e2e-agent-api-" + suffix}
	if err := h.db.Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	t.Cleanup(func() {
		h.db.Where("id = ?", org.ID).Delete(&model.Org{})
	})

	// Create credential (for agents to reference)
	cred := model.Credential{
		OrgID:        org.ID,
		Label:        "test-openai-" + suffix,
		BaseURL:      "https://api.openai.com",
		AuthScheme:   "bearer",
		ProviderID:   "openai",
		EncryptedKey: []byte("encrypted"),
		WrappedDEK:   []byte("wrapped"),
	}
	if err := h.db.Create(&cred).Error; err != nil {
		t.Fatalf("create credential: %v", err)
	}
	t.Cleanup(func() {
		h.db.Where("id = ?", cred.ID).Delete(&model.Credential{})
	})

	// Create identity (agents must be tied to an identity)
	identity := model.Identity{OrgID: org.ID, ExternalID: "test-user-" + suffix}
	if err := h.db.Create(&identity).Error; err != nil {
		t.Fatalf("create identity: %v", err)
	}
	t.Cleanup(func() {
		h.db.Where("id = ?", identity.ID).Delete(&model.Identity{})
	})

	// Build router with agent routes (no auth middleware — we inject org context directly)
	reg := registry.Global()
	sandboxTemplateHandler := handler.NewSandboxTemplateHandler(h.db, nil, nil)
	agentHandler := handler.NewAgentHandler(h.db, reg, nil, nil)

	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r = middleware.WithOrg(r, &org)
			next.ServeHTTP(w, r)
		})
	})

	r.Route("/v1/sandbox-templates", func(r chi.Router) {
		r.Post("/", sandboxTemplateHandler.Create)
		r.Get("/", sandboxTemplateHandler.List)
		r.Get("/{id}", sandboxTemplateHandler.Get)
		r.Put("/{id}", sandboxTemplateHandler.Update)
		r.Delete("/{id}", sandboxTemplateHandler.Delete)
	})
	r.Route("/v1/agents", func(r chi.Router) {
		r.Post("/", agentHandler.Create)
		r.Get("/", agentHandler.List)
		r.Get("/{id}", agentHandler.Get)
		r.Put("/{id}", agentHandler.Update)
		r.Delete("/{id}", agentHandler.Delete)
	})

	return &agentAPIHarness{
		testHarness: h,
		org:         org,
		cred:        cred,
		identity:    identity,
		router:      r,
	}
}

func (h *agentAPIHarness) request(t *testing.T, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	var req *http.Request
	if body != "" {
		req = httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	return rr
}

// === Sandbox Template Tests ===

func TestSandboxTemplateAPI_CRUD(t *testing.T) {
	h := newAgentAPIHarness(t)

	// Create
	rr := h.request(t, http.MethodPost, "/v1/sandbox-templates", `{
		"name": "python-ml",
		"build_commands": "pip install numpy pandas scikit-learn",
		"config": {"cpu": "2", "memory": "4096"}
	}`)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	var created struct {
		ID            string         `json:"id"`
		Name          string         `json:"name"`
		BuildCommands string         `json:"build_commands"`
		BuildStatus   string         `json:"build_status"`
		Config        map[string]any `json:"config"`
	}
	json.NewDecoder(rr.Body).Decode(&created)
	if created.Name != "python-ml" {
		t.Errorf("name: got %q", created.Name)
	}
	if created.BuildCommands != "pip install numpy pandas scikit-learn" {
		t.Errorf("build_commands: got %q", created.BuildCommands)
	}
	if created.BuildStatus != "pending" {
		t.Errorf("build_status: got %q", created.BuildStatus)
	}
	if created.Config["cpu"] != "2" {
		t.Errorf("config.cpu: got %v", created.Config["cpu"])
	}

	t.Cleanup(func() {
		h.db.Where("id = ?", created.ID).Delete(&model.SandboxTemplate{})
	})

	// Get
	rr = h.request(t, http.MethodGet, "/v1/sandbox-templates/"+created.ID, "")
	if rr.Code != http.StatusOK {
		t.Fatalf("get: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	// List
	rr = h.request(t, http.MethodGet, "/v1/sandbox-templates", "")
	if rr.Code != http.StatusOK {
		t.Fatalf("list: expected 200, got %d", rr.Code)
	}
	var listed struct {
		Data    []any `json:"data"`
		HasMore bool  `json:"has_more"`
	}
	json.NewDecoder(rr.Body).Decode(&listed)
	if len(listed.Data) < 1 {
		t.Fatal("list should return at least 1 template")
	}

	// Update
	rr = h.request(t, http.MethodPut, "/v1/sandbox-templates/"+created.ID, `{
		"name": "python-ml-v2",
		"build_commands": "pip install numpy pandas scikit-learn torch"
	}`)
	if rr.Code != http.StatusOK {
		t.Fatalf("update: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var updated struct {
		Name          string `json:"name"`
		BuildCommands string `json:"build_commands"`
		BuildStatus   string `json:"build_status"`
	}
	json.NewDecoder(rr.Body).Decode(&updated)
	if updated.Name != "python-ml-v2" {
		t.Errorf("updated name: got %q", updated.Name)
	}
	if updated.BuildStatus != "pending" {
		t.Errorf("build_status should reset to pending after command change: got %q", updated.BuildStatus)
	}

	// Delete
	rr = h.request(t, http.MethodDelete, "/v1/sandbox-templates/"+created.ID, "")
	if rr.Code != http.StatusOK {
		t.Fatalf("delete: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	// Verify deleted
	rr = h.request(t, http.MethodGet, "/v1/sandbox-templates/"+created.ID, "")
	if rr.Code != http.StatusNotFound {
		t.Errorf("get after delete: expected 404, got %d", rr.Code)
	}
}

func TestSandboxTemplateAPI_Validation(t *testing.T) {
	h := newAgentAPIHarness(t)

	// Missing name
	rr := h.request(t, http.MethodPost, "/v1/sandbox-templates", `{"build_commands": "echo hi"}`)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("missing name: expected 400, got %d", rr.Code)
	}

	// Not found
	rr = h.request(t, http.MethodGet, "/v1/sandbox-templates/"+uuid.New().String(), "")
	if rr.Code != http.StatusNotFound {
		t.Errorf("not found: expected 404, got %d", rr.Code)
	}
}

// === Agent Tests ===

func TestAgentAPI_CRUD(t *testing.T) {
	h := newAgentAPIHarness(t)
	suffix := uuid.New().String()[:8]

	// Create
	body := fmt.Sprintf(`{
		"name": "support-agent-%s",
		"description": "Customer support bot",
		"identity_id": %q,
		"credential_id": %q,
		"sandbox_type": "shared",
		"system_prompt": "You are a helpful customer support agent.",
		"model": "gpt-4o",
		"agent_config": {"max_tokens": 4096, "temperature": 0.3},
		"permissions": {"bash": "require_approval"}
	}`, suffix, h.identity.ID.String(), h.cred.ID.String())

	rr := h.request(t, http.MethodPost, "/v1/agents", body)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	var created struct {
		ID           string         `json:"id"`
		Name         string         `json:"name"`
		Description  *string        `json:"description"`
		IdentityID   *string        `json:"identity_id"`
		CredentialID string         `json:"credential_id"`
		ProviderID   string         `json:"provider_id"`
		SandboxType  string         `json:"sandbox_type"`
		SystemPrompt string         `json:"system_prompt"`
		Model        string         `json:"model"`
		AgentConfig  map[string]any `json:"agent_config"`
		Permissions  map[string]any `json:"permissions"`
		Status       string         `json:"status"`
	}
	json.NewDecoder(rr.Body).Decode(&created)

	t.Cleanup(func() {
		h.db.Where("id = ?", created.ID).Delete(&model.Agent{})
	})

	if created.Name != "support-agent-"+suffix {
		t.Errorf("name: got %q", created.Name)
	}
	if created.Description == nil || *created.Description != "Customer support bot" {
		t.Errorf("description: got %v", created.Description)
	}
	if created.IdentityID == nil || *created.IdentityID != h.identity.ID.String() {
		t.Errorf("identity_id: got %v, want %s", created.IdentityID, h.identity.ID)
	}
	if created.CredentialID != h.cred.ID.String() {
		t.Errorf("credential_id: got %q", created.CredentialID)
	}
	if created.ProviderID != "openai" {
		t.Errorf("provider_id: got %q", created.ProviderID)
	}
	if created.SandboxType != "shared" {
		t.Errorf("sandbox_type: got %q", created.SandboxType)
	}
	if created.Model != "gpt-4o" {
		t.Errorf("model: got %q", created.Model)
	}
	if created.Status != "active" {
		t.Errorf("status: got %q", created.Status)
	}
	if created.Permissions["bash"] != "require_approval" {
		t.Errorf("permissions.bash: got %v", created.Permissions["bash"])
	}

	// Get
	rr = h.request(t, http.MethodGet, "/v1/agents/"+created.ID, "")
	if rr.Code != http.StatusOK {
		t.Fatalf("get: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	// List
	rr = h.request(t, http.MethodGet, "/v1/agents", "")
	if rr.Code != http.StatusOK {
		t.Fatalf("list: expected 200, got %d", rr.Code)
	}
	var listed struct {
		Data []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"data"`
	}
	json.NewDecoder(rr.Body).Decode(&listed)
	if len(listed.Data) < 1 {
		t.Fatal("list should return at least 1 agent")
	}

	// List with filter
	rr = h.request(t, http.MethodGet, "/v1/agents?sandbox_type=shared", "")
	if rr.Code != http.StatusOK {
		t.Fatalf("list filtered: expected 200, got %d", rr.Code)
	}

	// Update
	rr = h.request(t, http.MethodPut, "/v1/agents/"+created.ID, `{
		"system_prompt": "You are an updated support agent.",
		"agent_config": {"max_tokens": 8192}
	}`)
	if rr.Code != http.StatusOK {
		t.Fatalf("update: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var updated struct {
		SystemPrompt string         `json:"system_prompt"`
		AgentConfig  map[string]any `json:"agent_config"`
	}
	json.NewDecoder(rr.Body).Decode(&updated)
	if updated.SystemPrompt != "You are an updated support agent." {
		t.Errorf("updated system_prompt: got %q", updated.SystemPrompt)
	}

	// Delete
	rr = h.request(t, http.MethodDelete, "/v1/agents/"+created.ID, "")
	if rr.Code != http.StatusOK {
		t.Fatalf("delete: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	// Verify deleted
	rr = h.request(t, http.MethodGet, "/v1/agents/"+created.ID, "")
	if rr.Code != http.StatusNotFound {
		t.Errorf("get after delete: expected 404, got %d", rr.Code)
	}
}

func TestAgentAPI_Validation(t *testing.T) {
	h := newAgentAPIHarness(t)

	// Missing required fields
	rr := h.request(t, http.MethodPost, "/v1/agents", `{"name": "test"}`)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("missing fields: expected 400, got %d: %s", rr.Code, rr.Body.String())
	}

	// Invalid sandbox_type
	rr = h.request(t, http.MethodPost, "/v1/agents", fmt.Sprintf(`{
		"name": "test", "identity_id": %q, "credential_id": %q, "sandbox_type": "invalid",
		"system_prompt": "test", "model": "gpt-4o"
	}`, h.identity.ID.String(), h.cred.ID.String()))
	if rr.Code != http.StatusBadRequest {
		t.Errorf("invalid sandbox_type: expected 400, got %d: %s", rr.Code, rr.Body.String())
	}

	// Non-existent credential
	rr = h.request(t, http.MethodPost, "/v1/agents", fmt.Sprintf(`{
		"name": "test", "identity_id": %q, "credential_id": %q, "sandbox_type": "shared",
		"system_prompt": "test", "model": "gpt-4o"
	}`, h.identity.ID.String(), uuid.New().String()))
	if rr.Code != http.StatusBadRequest {
		t.Errorf("bad credential: expected 400, got %d: %s", rr.Code, rr.Body.String())
	}

	// Model not supported by provider
	rr = h.request(t, http.MethodPost, "/v1/agents", fmt.Sprintf(`{
		"name": "test", "identity_id": %q, "credential_id": %q, "sandbox_type": "shared",
		"system_prompt": "test", "model": "claude-opus-4-20250514"
	}`, h.identity.ID.String(), h.cred.ID.String()))
	if rr.Code != http.StatusBadRequest {
		t.Errorf("wrong provider model: expected 400, got %d: %s", rr.Code, rr.Body.String())
	}
	var errResp struct{ Error string }
	json.NewDecoder(rr.Body).Decode(&errResp)
	if errResp.Error == "" {
		t.Error("expected error message about model/provider mismatch")
	}

	// Not found
	rr = h.request(t, http.MethodGet, "/v1/agents/"+uuid.New().String(), "")
	if rr.Code != http.StatusNotFound {
		t.Errorf("not found: expected 404, got %d", rr.Code)
	}
}

func TestAgentAPI_DuplicateName(t *testing.T) {
	h := newAgentAPIHarness(t)
	suffix := uuid.New().String()[:8]
	name := "dup-agent-" + suffix

	body := fmt.Sprintf(`{
		"name": %q, "identity_id": %q, "credential_id": %q, "sandbox_type": "shared",
		"system_prompt": "test", "model": "gpt-4o"
	}`, name, h.identity.ID.String(), h.cred.ID.String())

	// First create succeeds
	rr := h.request(t, http.MethodPost, "/v1/agents", body)
	if rr.Code != http.StatusCreated {
		t.Fatalf("first create: expected 201, got %d: %s", rr.Code, rr.Body.String())
	}
	var first struct{ ID string }
	json.NewDecoder(rr.Body).Decode(&first)
	t.Cleanup(func() {
		h.db.Where("id = ?", first.ID).Delete(&model.Agent{})
	})

	// Second create with same name fails
	rr = h.request(t, http.MethodPost, "/v1/agents", body)
	if rr.Code != http.StatusConflict {
		t.Errorf("duplicate name: expected 409, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestAgentAPI_WithTemplate(t *testing.T) {
	h := newAgentAPIHarness(t)
	suffix := uuid.New().String()[:8]

	// Create template first
	rr := h.request(t, http.MethodPost, "/v1/sandbox-templates", `{
		"name": "tmpl-for-agent",
		"build_commands": "echo setup"
	}`)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create template: %d: %s", rr.Code, rr.Body.String())
	}
	var tmpl struct{ ID string }
	json.NewDecoder(rr.Body).Decode(&tmpl)
	t.Cleanup(func() {
		h.db.Where("id = ?", tmpl.ID).Delete(&model.SandboxTemplate{})
	})

	// Mark template as ready (normally done by background build)
	h.db.Model(&model.SandboxTemplate{}).Where("id = ?", tmpl.ID).Updates(map[string]any{
		"build_status": "ready",
		"external_id":  "test-snapshot-id",
	})

	// Create agent with template
	body := fmt.Sprintf(`{
		"name": "agent-with-tmpl-%s",
		"identity_id": %q,
		"credential_id": %q,
		"sandbox_type": "dedicated",
		"sandbox_template_id": %q,
		"system_prompt": "test",
		"model": "gpt-4o"
	}`, suffix, h.identity.ID.String(), h.cred.ID.String(), tmpl.ID)

	rr = h.request(t, http.MethodPost, "/v1/agents", body)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create agent with template: %d: %s", rr.Code, rr.Body.String())
	}
	var agent struct {
		ID                string  `json:"id"`
		SandboxTemplateID *string `json:"sandbox_template_id"`
	}
	json.NewDecoder(rr.Body).Decode(&agent)
	t.Cleanup(func() {
		h.db.Where("id = ?", agent.ID).Delete(&model.Agent{})
	})

	if agent.SandboxTemplateID == nil || *agent.SandboxTemplateID != tmpl.ID {
		t.Errorf("sandbox_template_id: got %v, want %q", agent.SandboxTemplateID, tmpl.ID)
	}

	// Cannot delete template while agent references it
	rr = h.request(t, http.MethodDelete, "/v1/sandbox-templates/"+tmpl.ID, "")
	if rr.Code != http.StatusConflict {
		t.Errorf("delete template in use: expected 409, got %d: %s", rr.Code, rr.Body.String())
	}
}
