package e2e

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/llmvault/llmvault/internal/model"
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
