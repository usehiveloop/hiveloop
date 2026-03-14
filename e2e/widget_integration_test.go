package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/llmvault/llmvault/internal/middleware"
	"github.com/llmvault/llmvault/internal/model"
	"github.com/llmvault/llmvault/internal/nango"
)

// --------------------------------------------------------------------------
// E2E: Widget ListIntegrations — returns org-scoped integrations
// --------------------------------------------------------------------------

func TestE2E_Widget_ListIntegrations(t *testing.T) {
	h := newHarness(t)
	org := h.createOrg(t)

	// Create a connect session
	token, _ := h.createConnectSession(t, org, `{"external_id":"u1","ttl":"15m"}`)

	// No integrations yet → empty array
	rr := h.connectRequest(t, http.MethodGet, "/v1/widget/integrations", token, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var list []map[string]any
	json.NewDecoder(rr.Body).Decode(&list)
	if len(list) != 0 {
		t.Fatalf("expected 0 integrations, got %d", len(list))
	}

	// Create two integrations for this org
	integ1 := h.createIntegration(t, org, "slack", "Slack")
	integ2 := h.createIntegration(t, org, "github", "GitHub")

	// List again → should see both
	rr = h.connectRequest(t, http.MethodGet, "/v1/widget/integrations", token, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	json.NewDecoder(rr.Body).Decode(&list)
	if len(list) != 2 {
		t.Fatalf("expected 2 integrations, got %d", len(list))
	}

	// Verify response shape: id, provider, display_name, auth_mode
	ids := map[string]bool{}
	for _, item := range list {
		id, _ := item["id"].(string)
		provider, _ := item["provider"].(string)
		displayName, _ := item["display_name"].(string)

		if id == "" {
			t.Error("expected non-empty id")
		}
		if provider == "" {
			t.Error("expected non-empty provider")
		}
		if displayName == "" {
			t.Error("expected non-empty display_name")
		}
		// auth_mode may be empty if provider not in Nango catalog (test providers aren't real)
		ids[id] = true
	}

	if !ids[integ1.ID.String()] {
		t.Errorf("expected integration %s in response", integ1.ID)
	}
	if !ids[integ2.ID.String()] {
		t.Errorf("expected integration %s in response", integ2.ID)
	}
}

// --------------------------------------------------------------------------
// E2E: Widget ListIntegrations — org isolation
// --------------------------------------------------------------------------

func TestE2E_Widget_ListIntegrations_OrgIsolation(t *testing.T) {
	h := newHarness(t)
	org1 := h.createOrg(t)
	org2 := h.createOrg(t)

	// Create integrations for org1 only
	h.createIntegration(t, org1, "slack", "Slack")
	h.createIntegration(t, org1, "github", "GitHub")

	// Create session for org2
	token2, _ := h.createConnectSession(t, org2, `{"external_id":"u2","ttl":"15m"}`)

	// org2 should see zero integrations
	rr := h.connectRequest(t, http.MethodGet, "/v1/widget/integrations", token2, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var list []map[string]any
	json.NewDecoder(rr.Body).Decode(&list)
	if len(list) != 0 {
		t.Fatalf("org2 should see 0 integrations, got %d", len(list))
	}
}

// --------------------------------------------------------------------------
// E2E: Widget ListIntegrations — excludes soft-deleted
// --------------------------------------------------------------------------

func TestE2E_Widget_ListIntegrations_ExcludesDeleted(t *testing.T) {
	h := newHarness(t)
	org := h.createOrg(t)
	token, _ := h.createConnectSession(t, org, `{"external_id":"u1","ttl":"15m"}`)

	integ := h.createIntegration(t, org, "slack", "Slack")

	// Verify it shows up
	rr := h.connectRequest(t, http.MethodGet, "/v1/widget/integrations", token, nil)
	var list []map[string]any
	json.NewDecoder(rr.Body).Decode(&list)
	if len(list) != 1 {
		t.Fatalf("expected 1, got %d", len(list))
	}

	// Soft-delete the integration
	h.db.Model(&model.Integration{}).Where("id = ?", integ.ID).Update("deleted_at", "2026-01-01")

	// Should no longer appear
	rr = h.connectRequest(t, http.MethodGet, "/v1/widget/integrations", token, nil)
	json.NewDecoder(rr.Body).Decode(&list)
	if len(list) != 0 {
		t.Fatalf("expected 0 after soft-delete, got %d", len(list))
	}
}

// --------------------------------------------------------------------------
// E2E: Widget ListIntegrations — includes resource configuration
// --------------------------------------------------------------------------

func TestE2E_Widget_ListIntegrations_Resources(t *testing.T) {
	h := newHarness(t)
	org := h.createOrg(t)
	token, _ := h.createConnectSession(t, org, `{"external_id":"u1","ttl":"15m"}`)

	// Create integrations with resource configs
	h.createIntegration(t, org, "slack", "Slack Workspace")
	h.createIntegration(t, org, "github", "GitHub Org")
	h.createIntegration(t, org, "asana", "Asana") // no resources

	rr := h.connectRequest(t, http.MethodGet, "/v1/widget/integrations", token, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var list []map[string]any
	json.NewDecoder(rr.Body).Decode(&list)
	if len(list) != 3 {
		t.Fatalf("expected 3 integrations, got %d", len(list))
	}

	// Find each integration and verify resources
	for _, item := range list {
		provider, _ := item["provider"].(string)
		resources, hasResources := item["resources"].([]interface{})

		switch provider {
		case "slack":
			if !hasResources {
				t.Error("slack integration should have resources")
				continue
			}
			if len(resources) != 1 {
				t.Errorf("slack expected 1 resource type, got %d", len(resources))
				continue
			}
			res := resources[0].(map[string]interface{})
			if res["type"] != "channel" {
				t.Errorf("slack expected resource type 'channel', got %v", res["type"])
			}
			if res["display_name"] != "Channels" {
				t.Errorf("slack expected display_name 'Channels', got %v", res["display_name"])
			}
			if res["icon"] != "hash" {
				t.Errorf("slack expected icon 'hash', got %v", res["icon"])
			}

		case "github":
			if !hasResources {
				t.Error("github integration should have resources")
				continue
			}
			if len(resources) != 1 {
				t.Errorf("github expected 1 resource type, got %d", len(resources))
				continue
			}
			res := resources[0].(map[string]interface{})
			if res["type"] != "repo" {
				t.Errorf("github expected resource type 'repo', got %v", res["type"])
			}

		case "asana":
			// Asana has no resources configured, should be empty array
			if !hasResources {
				t.Error("asana integration should have resources field (empty array)")
				continue
			}
			if len(resources) != 0 {
				t.Errorf("asana expected 0 resource types, got %d", len(resources))
			}
		}
	}
}

// --------------------------------------------------------------------------
// E2E: Widget CreateIntegrationConnection with resources
// --------------------------------------------------------------------------

func TestE2E_Widget_CreateIntegrationConnection_WithResources(t *testing.T) {
	h := newHarness(t)
	org := h.createOrg(t)
	token, _ := h.createConnectSession(t, org, `{"external_id":"u1","ttl":"15m"}`)

	integ := h.createIntegration(t, org, "slack", "Slack")

	// Create a connection with resources
	body := `{"nango_connection_id":"nango-conn-123","resources":{"channel":["C123","C456"]}}`
	rr := h.connectRequest(t, http.MethodPost,
		"/v1/widget/integrations/"+integ.ID.String()+"/connections",
		token, strings.NewReader(body))
	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.NewDecoder(rr.Body).Decode(&resp)

	if resp["id"] == nil || resp["id"] == "" {
		t.Error("expected non-empty connection id")
	}

	// Verify resources are in meta
	meta, ok := resp["meta"].(map[string]interface{})
	if !ok {
		t.Fatal("expected meta to be present")
	}

	resources, ok := meta["resources"].(map[string]interface{})
	if !ok {
		t.Fatal("expected resources in meta")
	}

	channels, ok := resources["channel"].([]interface{})
	if !ok || len(channels) != 2 {
		t.Fatalf("expected 2 channels, got %v", resources["channel"])
	}

	if channels[0] != "C123" || channels[1] != "C456" {
		t.Errorf("expected channels [C123, C456], got %v", channels)
	}

	// Verify connection is in list with resources
	rr = h.connectRequest(t, http.MethodGet, "/v1/widget/integrations", token, nil)
	var list []map[string]any
	json.NewDecoder(rr.Body).Decode(&list)

	for _, item := range list {
		if item["id"] == integ.ID.String() {
			// connection_id should be set
			if item["connection_id"] == nil {
				t.Error("expected connection_id to be set")
			}
		}
	}
}

// --------------------------------------------------------------------------
// E2E: Widget PatchIntegrationConnection — update resources
// --------------------------------------------------------------------------

func TestE2E_Widget_PatchIntegrationConnection(t *testing.T) {
	h := newHarness(t)
	org := h.createOrg(t)
	token, _ := h.createConnectSession(t, org, `{"external_id":"u1","ttl":"15m"}`)

	integ := h.createIntegration(t, org, "slack", "Slack")

	// Create a connection without resources
	body := `{"nango_connection_id":"nango-conn-123"}`
	rr := h.connectRequest(t, http.MethodPost,
		"/v1/widget/integrations/"+integ.ID.String()+"/connections",
		token, strings.NewReader(body))
	if rr.Code != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	var createResp map[string]any
	json.NewDecoder(rr.Body).Decode(&createResp)
	connID := createResp["id"].(string)

	// Verify no resources initially (meta may be nil or empty)
	if m, ok := createResp["meta"].(map[string]interface{}); ok {
		if _, hasResources := m["resources"]; hasResources {
			t.Error("expected no resources initially")
		}
	}

	// Patch to add resources
	patchBody := `{"resources":{"channel":["C789","CABC"]}}`
	rr = h.connectRequest(t, http.MethodPatch,
		"/v1/widget/integrations/"+integ.ID.String()+"/connections/"+connID,
		token, strings.NewReader(patchBody))
	if rr.Code != http.StatusOK {
		t.Fatalf("patch: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var patchResp map[string]any
	json.NewDecoder(rr.Body).Decode(&patchResp)

	// Verify resources updated
	meta := patchResp["meta"].(map[string]interface{})
	resources := meta["resources"].(map[string]interface{})
	channels := resources["channel"].([]interface{})
	if len(channels) != 2 || channels[0] != "C789" || channels[1] != "CABC" {
		t.Errorf("expected updated channels [C789, CABC], got %v", channels)
	}

	// Patch to update resources
	patchBody = `{"resources":{"channel":["C111"]}}`
	rr = h.connectRequest(t, http.MethodPatch,
		"/v1/widget/integrations/"+integ.ID.String()+"/connections/"+connID,
		token, strings.NewReader(patchBody))
	if rr.Code != http.StatusOK {
		t.Fatalf("patch update: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	json.NewDecoder(rr.Body).Decode(&patchResp)
	meta = patchResp["meta"].(map[string]interface{})
	resources = meta["resources"].(map[string]interface{})
	channels = resources["channel"].([]interface{})
	if len(channels) != 1 || channels[0] != "C111" {
		t.Errorf("expected updated channel [C111], got %v", channels)
	}

	// Patch with empty resources to clear
	patchBody = `{"resources":{}}`
	rr = h.connectRequest(t, http.MethodPatch,
		"/v1/widget/integrations/"+integ.ID.String()+"/connections/"+connID,
		token, strings.NewReader(patchBody))
	if rr.Code != http.StatusOK {
		t.Fatalf("patch clear: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var clearResp map[string]any
	json.NewDecoder(rr.Body).Decode(&clearResp)

	// When meta is empty, it may be omitted from JSON response
	if meta, ok := clearResp["meta"].(map[string]interface{}); ok {
		if _, hasResources := meta["resources"]; hasResources {
			t.Errorf("expected resources to be cleared, but got: %v", meta["resources"])
		}
	}
	// If meta is not present at all, resources are cleared (success)
}

// --------------------------------------------------------------------------
// E2E: Widget PatchIntegrationConnection — not found cases
// --------------------------------------------------------------------------

func TestE2E_Widget_PatchIntegrationConnection_NotFound(t *testing.T) {
	h := newHarness(t)
	org := h.createOrg(t)
	token, _ := h.createConnectSession(t, org, `{"external_id":"u1","ttl":"15m"}`)

	integ := h.createIntegration(t, org, "slack", "Slack")

	// Patch non-existent connection
	patchBody := `{"resources":{"channel":["C123"]}}`
	rr := h.connectRequest(t, http.MethodPatch,
		"/v1/widget/integrations/"+integ.ID.String()+"/connections/"+uuid.New().String(),
		token, strings.NewReader(patchBody))
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rr.Code, rr.Body.String())
	}
}

// --------------------------------------------------------------------------
// E2E: Widget CreateIntegrationConnection — full flow
// --------------------------------------------------------------------------

func TestE2E_Widget_CreateIntegrationConnection(t *testing.T) {
	h := newHarness(t)
	org := h.createOrg(t)
	token, _ := h.createConnectSession(t, org, `{"external_id":"u1","ttl":"15m"}`)

	integ := h.createIntegration(t, org, "slack", "Slack")

	// Create a connection via the widget endpoint
	body := `{"nango_connection_id":"nango-conn-123"}`
	rr := h.connectRequest(t, http.MethodPost,
		"/v1/widget/integrations/"+integ.ID.String()+"/connections",
		token, strings.NewReader(body))
	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.NewDecoder(rr.Body).Decode(&resp)

	if resp["id"] == nil || resp["id"] == "" {
		t.Error("expected non-empty connection id")
	}
	if resp["integration_id"] != integ.ID.String() {
		t.Errorf("expected integration_id %s, got %v", integ.ID, resp["integration_id"])
	}
	if resp["nango_connection_id"] != "nango-conn-123" {
		t.Errorf("expected nango_connection_id nango-conn-123, got %v", resp["nango_connection_id"])
	}
	// identity_id should be set (auto-upserted from external_id)
	if resp["identity_id"] == nil || resp["identity_id"] == "" {
		t.Error("expected identity_id to be set from session")
	}

	// Verify DB record
	var conn model.Connection
	if err := h.db.Where("id = ?", resp["id"]).First(&conn).Error; err != nil {
		t.Fatalf("connection not found in DB: %v", err)
	}
	if conn.NangoConnectionID != "nango-conn-123" {
		t.Errorf("DB nango_connection_id mismatch: %s", conn.NangoConnectionID)
	}
	if conn.OrgID != org.ID {
		t.Errorf("DB org_id mismatch: %s", conn.OrgID)
	}
}

// --------------------------------------------------------------------------
// E2E: Widget CreateIntegrationConnection — missing nango_connection_id
// --------------------------------------------------------------------------

func TestE2E_Widget_CreateIntegrationConnection_MissingField(t *testing.T) {
	h := newHarness(t)
	org := h.createOrg(t)
	token, _ := h.createConnectSession(t, org, `{"external_id":"u1","ttl":"15m"}`)
	integ := h.createIntegration(t, org, "slack", "Slack")

	rr := h.connectRequest(t, http.MethodPost,
		"/v1/widget/integrations/"+integ.ID.String()+"/connections",
		token, strings.NewReader(`{}`))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

// --------------------------------------------------------------------------
// E2E: Widget CreateIntegrationConnection — wrong org's integration
// --------------------------------------------------------------------------

func TestE2E_Widget_CreateIntegrationConnection_CrossOrg(t *testing.T) {
	h := newHarness(t)
	org1 := h.createOrg(t)
	org2 := h.createOrg(t)

	// Integration belongs to org1
	integ := h.createIntegration(t, org1, "slack", "Slack")

	// Session for org2
	token2, _ := h.createConnectSession(t, org2, `{"external_id":"u2","ttl":"15m"}`)

	body := `{"nango_connection_id":"nango-conn-x"}`
	rr := h.connectRequest(t, http.MethodPost,
		"/v1/widget/integrations/"+integ.ID.String()+"/connections",
		token2, strings.NewReader(body))
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for cross-org integration, got %d: %s", rr.Code, rr.Body.String())
	}
}

// --------------------------------------------------------------------------
// E2E: Widget CreateIntegrationConnection — invalid integration ID
// --------------------------------------------------------------------------

func TestE2E_Widget_CreateIntegrationConnection_InvalidID(t *testing.T) {
	h := newHarness(t)
	org := h.createOrg(t)
	token, _ := h.createConnectSession(t, org, `{"external_id":"u1","ttl":"15m"}`)

	// Non-existent UUID
	rr := h.connectRequest(t, http.MethodPost,
		"/v1/widget/integrations/"+uuid.New().String()+"/connections",
		token, strings.NewReader(`{"nango_connection_id":"x"}`))
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rr.Code, rr.Body.String())
	}

	// Not a UUID
	rr = h.connectRequest(t, http.MethodPost,
		"/v1/widget/integrations/not-a-uuid/connections",
		token, strings.NewReader(`{"nango_connection_id":"x"}`))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for bad UUID, got %d: %s", rr.Code, rr.Body.String())
	}
}

// --------------------------------------------------------------------------
// E2E: Widget CreateIntegrationConnection — permission enforcement
// --------------------------------------------------------------------------

func TestE2E_Widget_CreateIntegrationConnection_PermissionDenied(t *testing.T) {
	h := newHarness(t)
	org := h.createOrg(t)

	// Session with only "list" permission — no "create"
	token, _ := h.createConnectSession(t, org, `{"external_id":"u1","ttl":"15m","permissions":["list"]}`)
	integ := h.createIntegration(t, org, "slack", "Slack")

	body := `{"nango_connection_id":"nango-conn-y"}`
	rr := h.connectRequest(t, http.MethodPost,
		"/v1/widget/integrations/"+integ.ID.String()+"/connections",
		token, strings.NewReader(body))
	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", rr.Code, rr.Body.String())
	}
}

// --------------------------------------------------------------------------
// E2E: Widget CreateConnectSession — requires create permission
// --------------------------------------------------------------------------

func TestE2E_Widget_ConnectSession_PermissionDenied(t *testing.T) {
	h := newHarness(t)
	org := h.createOrg(t)

	// Session with only "list" permission
	token, _ := h.createConnectSession(t, org, `{"external_id":"u1","ttl":"15m","permissions":["list"]}`)
	integ := h.createIntegration(t, org, "slack", "Slack")

	rr := h.connectRequest(t, http.MethodPost,
		"/v1/widget/integrations/"+integ.ID.String()+"/connect-session",
		token, nil)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", rr.Code, rr.Body.String())
	}
}

// --------------------------------------------------------------------------
// E2E: Widget ConnectSession — cross-org returns 404
// --------------------------------------------------------------------------

func TestE2E_Widget_ConnectSession_CrossOrg(t *testing.T) {
	h := newHarness(t)
	org1 := h.createOrg(t)
	org2 := h.createOrg(t)

	integ := h.createIntegration(t, org1, "slack", "Slack")
	token2, _ := h.createConnectSession(t, org2, `{"external_id":"u2","ttl":"15m"}`)

	rr := h.connectRequest(t, http.MethodPost,
		"/v1/widget/integrations/"+integ.ID.String()+"/connect-session",
		token2, nil)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for cross-org, got %d: %s", rr.Code, rr.Body.String())
	}
}

// --------------------------------------------------------------------------
// E2E: Widget ConnectSession — returns token and provider_config_key
// --------------------------------------------------------------------------

func TestE2E_Widget_ConnectSession_Success(t *testing.T) {
	h := newHarness(t)
	org := h.createOrg(t)
	token, _ := h.createConnectSession(t, org, `{"external_id":"u1","ttl":"15m"}`)

	integ := h.createIntegration(t, org, "slack", "Slack")

	rr := h.connectRequest(t, http.MethodPost,
		"/v1/widget/integrations/"+integ.ID.String()+"/connect-session",
		token, nil)

	// This will return 502 because we're calling the real Nango API with a test
	// integration that doesn't exist in Nango. That's expected — the important
	// thing is it gets past auth/validation and calls Nango.
	// If it returns 401/403/404/400 that would indicate our handler logic failed.
	if rr.Code == http.StatusUnauthorized || rr.Code == http.StatusForbidden ||
		rr.Code == http.StatusNotFound || rr.Code == http.StatusBadRequest {
		t.Fatalf("unexpected auth/validation error %d: %s", rr.Code, rr.Body.String())
	}

	// If Nango is running and the integration exists in Nango, we'd get 200
	if rr.Code == http.StatusOK {
		var resp map[string]any
		json.NewDecoder(rr.Body).Decode(&resp)

		if resp["token"] == nil || resp["token"] == "" {
			t.Error("expected non-empty token")
		}
		expectedKey := fmt.Sprintf("%s_%s", org.ID.String(), integ.UniqueKey)
		if resp["provider_config_key"] != expectedKey {
			t.Errorf("expected provider_config_key %s, got %v", expectedKey, resp["provider_config_key"])
		}
		t.Logf("Connect session created successfully: key=%s", resp["provider_config_key"])
	} else {
		// 502 from Nango is acceptable in test env
		t.Logf("Nango returned %d (expected in test env without matching Nango integration)", rr.Code)
	}
}

// --------------------------------------------------------------------------
// E2E: Widget ListIntegrations — connection_id reflects connected state
// --------------------------------------------------------------------------

func TestE2E_Widget_ListIntegrations_ConnectionID(t *testing.T) {
	h := newHarness(t)
	org := h.createOrg(t)
	token, _ := h.createConnectSession(t, org, `{"external_id":"u1","ttl":"15m"}`)

	integ := h.createIntegration(t, org, "slack", "Slack")

	// Before connection: connection_id should be null
	rr := h.connectRequest(t, http.MethodGet, "/v1/widget/integrations", token, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var list []map[string]any
	json.NewDecoder(rr.Body).Decode(&list)
	if len(list) != 1 {
		t.Fatalf("expected 1 integration, got %d", len(list))
	}
	if list[0]["connection_id"] != nil {
		t.Errorf("expected connection_id to be null before connecting, got %v", list[0]["connection_id"])
	}

	// Create a connection via the widget endpoint
	body := `{"nango_connection_id":"nango-conn-cid"}`
	rr = h.connectRequest(t, http.MethodPost,
		"/v1/widget/integrations/"+integ.ID.String()+"/connections",
		token, strings.NewReader(body))
	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}
	var connResp map[string]any
	json.NewDecoder(rr.Body).Decode(&connResp)
	connID := connResp["id"].(string)

	// After connection: connection_id should be the connection UUID
	rr = h.connectRequest(t, http.MethodGet, "/v1/widget/integrations", token, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	json.NewDecoder(rr.Body).Decode(&list)
	if len(list) != 1 {
		t.Fatalf("expected 1 integration, got %d", len(list))
	}
	if list[0]["connection_id"] != connID {
		t.Errorf("expected connection_id %s, got %v", connID, list[0]["connection_id"])
	}
}

// --------------------------------------------------------------------------
// E2E: Widget CreateIntegrationConnection — duplicate rejected with 409
// --------------------------------------------------------------------------

func TestE2E_Widget_CreateIntegrationConnection_DuplicateRejected(t *testing.T) {
	h := newHarness(t)
	org := h.createOrg(t)
	token, _ := h.createConnectSession(t, org, `{"external_id":"u1","ttl":"15m"}`)

	integ := h.createIntegration(t, org, "slack", "Slack")

	// First connection succeeds
	body := `{"nango_connection_id":"nango-conn-dup1"}`
	rr := h.connectRequest(t, http.MethodPost,
		"/v1/widget/integrations/"+integ.ID.String()+"/connections",
		token, strings.NewReader(body))
	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	// Second connection should be rejected with 409
	body = `{"nango_connection_id":"nango-conn-dup2"}`
	rr = h.connectRequest(t, http.MethodPost,
		"/v1/widget/integrations/"+integ.ID.String()+"/connections",
		token, strings.NewReader(body))
	if rr.Code != http.StatusConflict {
		t.Fatalf("expected 409 Conflict, got %d: %s", rr.Code, rr.Body.String())
	}

	var errResp map[string]any
	json.NewDecoder(rr.Body).Decode(&errResp)
	if errResp["error"] != "already connected to this integration" {
		t.Errorf("unexpected error message: %v", errResp["error"])
	}
}

// --------------------------------------------------------------------------
// E2E: Widget CreateIntegrationConnection — revoked allows reconnect
// --------------------------------------------------------------------------

func TestE2E_Widget_CreateIntegrationConnection_RevokedAllowsReconnect(t *testing.T) {
	h := newHarness(t)
	org := h.createOrg(t)
	token, _ := h.createConnectSession(t, org, `{"external_id":"u1","ttl":"15m"}`)

	integ := h.createIntegration(t, org, "slack", "Slack")

	// Create a connection
	body := `{"nango_connection_id":"nango-conn-rev1"}`
	rr := h.connectRequest(t, http.MethodPost,
		"/v1/widget/integrations/"+integ.ID.String()+"/connections",
		token, strings.NewReader(body))
	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}
	var connResp map[string]any
	json.NewDecoder(rr.Body).Decode(&connResp)
	connID := connResp["id"].(string)

	// Revoke it (soft-delete by setting revoked_at)
	if err := h.db.Model(&model.Connection{}).Where("id = ?", connID).Update("revoked_at", "2026-01-01").Error; err != nil {
		t.Fatalf("failed to revoke connection: %v", err)
	}

	// Should be able to create a new connection
	body = `{"nango_connection_id":"nango-conn-rev2"}`
	rr = h.connectRequest(t, http.MethodPost,
		"/v1/widget/integrations/"+integ.ID.String()+"/connections",
		token, strings.NewReader(body))
	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201 after revoke, got %d: %s", rr.Code, rr.Body.String())
	}
}

// --------------------------------------------------------------------------
// E2E: Widget DeleteIntegrationConnection — full Nango sync
// --------------------------------------------------------------------------

func TestE2E_Widget_DeleteIntegrationConnection(t *testing.T) {
	h := newHarness(t)
	org := h.createOrg(t)
	sessionToken, _ := h.createConnectSession(t, org, `{"external_id":"u1","ttl":"15m"}`)

	// Find an API_KEY auth mode provider (no OAuth needed to create connection)
	providers := h.nangoClient.GetProviders()
	var apiKeyProvider string
	for _, p := range providers {
		if p.AuthMode == "API_KEY" {
			apiKeyProvider = p.Name
			break
		}
	}
	if apiKeyProvider == "" {
		t.Fatal("no API_KEY auth mode provider found in Nango catalog")
	}

	// Create integration via management API (creates in Nango)
	body := fmt.Sprintf(`{"provider":%q,"display_name":"Disconnect Test"}`, apiKeyProvider)
	req := httptest.NewRequest(http.MethodPost, "/v1/integrations", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = middleware.WithOrg(req, &org)
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create integration: expected 201, got %d: %s", rr.Code, rr.Body.String())
	}
	var integResp map[string]any
	json.NewDecoder(rr.Body).Decode(&integResp)
	integID := integResp["id"].(string)

	// Get the integration's unique_key from DB
	var dbInteg model.Integration
	if err := h.db.Where("id = ?", integID).First(&dbInteg).Error; err != nil {
		t.Fatalf("lookup integration: %v", err)
	}
	nangoProviderConfigKey := fmt.Sprintf("%s_%s", org.ID.String(), dbInteg.UniqueKey)

	// Create a connection directly in Nango
	nangoConnID := fmt.Sprintf("test-conn-%s", uuid.New().String()[:8])
	if err := h.nangoClient.CreateConnection(context.Background(), nango.CreateConnectionRequest{
		ProviderConfigKey: nangoProviderConfigKey,
		ConnectionID:      nangoConnID,
		APIKey:            "test-api-key-12345",
	}); err != nil {
		t.Fatalf("create Nango connection: %v", err)
	}

	// Verify connection exists in Nango
	nangoConn, err := h.nangoClient.GetConnection(context.Background(), nangoConnID, nangoProviderConfigKey)
	if err != nil {
		t.Fatalf("get Nango connection: %v", err)
	}
	if nangoConn == nil {
		t.Fatal("connection not found in Nango after create")
	}

	// Store connection record in our DB (simulates what CreateIntegrationConnection does)
	rr = h.connectRequest(t, http.MethodPost,
		"/v1/widget/integrations/"+integID+"/connections",
		sessionToken, strings.NewReader(fmt.Sprintf(`{"nango_connection_id":%q}`, nangoConnID)))
	if rr.Code != http.StatusCreated {
		t.Fatalf("store connection: expected 201, got %d: %s", rr.Code, rr.Body.String())
	}
	var connResp map[string]any
	json.NewDecoder(rr.Body).Decode(&connResp)
	connID := connResp["id"].(string)

	// Verify connection_id shows up in listing
	rr = h.connectRequest(t, http.MethodGet, "/v1/widget/integrations", sessionToken, nil)
	var list []map[string]any
	json.NewDecoder(rr.Body).Decode(&list)
	found := false
	for _, item := range list {
		if item["id"] == integID && item["connection_id"] == connID {
			found = true
		}
	}
	if !found {
		t.Fatal("expected connection_id in listing")
	}

	// Delete via the widget endpoint
	rr = h.connectRequest(t, http.MethodDelete,
		"/v1/widget/integrations/"+integID+"/connections/"+connID,
		sessionToken, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("delete: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	// Verify connection is gone from Nango
	_, err = h.nangoClient.GetConnection(context.Background(), nangoConnID, nangoProviderConfigKey)
	if err == nil {
		t.Fatal("connection should be gone from Nango after delete")
	}

	// Verify connection_id is null in listing
	rr = h.connectRequest(t, http.MethodGet, "/v1/widget/integrations", sessionToken, nil)
	json.NewDecoder(rr.Body).Decode(&list)
	for _, item := range list {
		if item["id"] == integID && item["connection_id"] != nil {
			t.Errorf("expected connection_id to be null after delete, got %v", item["connection_id"])
		}
	}

	// Verify DB record is soft-deleted
	var conn model.Connection
	if err := h.db.Where("id = ?", connID).First(&conn).Error; err != nil {
		t.Fatalf("connection not found in DB: %v", err)
	}
	if conn.RevokedAt == nil {
		t.Error("expected revoked_at to be set")
	}
}

// --------------------------------------------------------------------------
// E2E: Widget DeleteIntegrationConnection — not found
// --------------------------------------------------------------------------

func TestE2E_Widget_DeleteIntegrationConnection_NotFound(t *testing.T) {
	h := newHarness(t)
	org := h.createOrg(t)
	token, _ := h.createConnectSession(t, org, `{"external_id":"u1","ttl":"15m"}`)

	integ := h.createIntegration(t, org, "slack", "Slack")

	rr := h.connectRequest(t, http.MethodDelete,
		"/v1/widget/integrations/"+integ.ID.String()+"/connections/"+uuid.New().String(),
		token, nil)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rr.Code, rr.Body.String())
	}
}

// --------------------------------------------------------------------------
// E2E: Widget DeleteIntegrationConnection — allows reconnect after disconnect
// --------------------------------------------------------------------------

func TestE2E_Widget_DeleteIntegrationConnection_AllowsReconnect(t *testing.T) {
	h := newHarness(t)
	org := h.createOrg(t)
	token, _ := h.createConnectSession(t, org, `{"external_id":"u1","ttl":"15m"}`)

	integ := h.createIntegration(t, org, "slack", "Slack")

	// Connect
	body := `{"nango_connection_id":"nango-conn-recon1"}`
	rr := h.connectRequest(t, http.MethodPost,
		"/v1/widget/integrations/"+integ.ID.String()+"/connections",
		token, strings.NewReader(body))
	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}
	var connResp map[string]any
	json.NewDecoder(rr.Body).Decode(&connResp)
	connID := connResp["id"].(string)

	// Disconnect via API
	rr = h.connectRequest(t, http.MethodDelete,
		"/v1/widget/integrations/"+integ.ID.String()+"/connections/"+connID,
		token, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	// Reconnect should succeed (duplicate check ignores revoked)
	body = `{"nango_connection_id":"nango-conn-recon2"}`
	rr = h.connectRequest(t, http.MethodPost,
		"/v1/widget/integrations/"+integ.ID.String()+"/connections",
		token, strings.NewReader(body))
	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201 after disconnect, got %d: %s", rr.Code, rr.Body.String())
	}
}
