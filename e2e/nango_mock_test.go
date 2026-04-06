package e2e

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// nangoMock is an in-memory mock of the Nango API for e2e tests.
// It tracks created integrations and connections so that GET/DELETE
// operations behave realistically.
type nangoMock struct {
	mu           sync.Mutex
	integrations map[string]map[string]any // uniqueKey → stored data
	connections  map[string]map[string]any // "connectionID|providerConfigKey" → stored data
	server       *httptest.Server
}

func newNangoMock(t *testing.T) *nangoMock {
	t.Helper()
	m := &nangoMock{
		integrations: make(map[string]map[string]any),
		connections:  make(map[string]map[string]any),
	}
	m.server = httptest.NewServer(m)
	t.Cleanup(func() { m.server.Close() })
	return m
}

func (m *nangoMock) URL() string {
	return m.server.URL
}

func (m *nangoMock) connKey(connectionID, providerConfigKey string) string {
	return connectionID + "|" + providerConfigKey
}

func (m *nangoMock) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	// Strip query params for path matching, but keep them available via r.URL.Query()

	// GET /providers
	if path == "/providers" && r.Method == http.MethodGet {
		m.handleGetProviders(w, r)
		return
	}

	// POST /connect/sessions
	if path == "/connect/sessions" && r.Method == http.MethodPost {
		m.handleCreateConnectSession(w, r)
		return
	}

	// POST /connection (create)
	if path == "/connection" && r.Method == http.MethodPost {
		m.handleCreateConnection(w, r)
		return
	}

	// GET/DELETE /connection/{id}
	if strings.HasPrefix(path, "/connection/") && !strings.Contains(path[len("/connection/"):], "/") {
		connID := path[len("/connection/"):]
		providerConfigKey := r.URL.Query().Get("provider_config_key")
		switch r.Method {
		case http.MethodGet:
			m.handleGetConnection(w, connID, providerConfigKey)
			return
		case http.MethodDelete:
			m.handleDeleteConnection(w, connID, providerConfigKey)
			return
		}
	}

	// POST /integrations (create)
	if path == "/integrations" && r.Method == http.MethodPost {
		m.handleCreateIntegration(w, r)
		return
	}

	// GET/PATCH/DELETE /integrations/{uniqueKey}
	if strings.HasPrefix(path, "/integrations/") {
		uniqueKey := path[len("/integrations/"):]
		// Strip query params from key if present
		if idx := strings.Index(uniqueKey, "?"); idx != -1 {
			uniqueKey = uniqueKey[:idx]
		}
		switch r.Method {
		case http.MethodGet:
			m.handleGetIntegration(w, uniqueKey)
			return
		case http.MethodPatch:
			m.handleUpdateIntegration(w, r, uniqueKey)
			return
		case http.MethodDelete:
			m.handleDeleteIntegration(w, uniqueKey)
			return
		}
	}

	// Proxy requests: /proxy/*
	if strings.HasPrefix(path, "/proxy/") {
		m.handleProxy(w, r)
		return
	}

	w.WriteHeader(http.StatusNotFound)
	json.NewEncoder(w).Encode(map[string]any{"error": "mock: unknown route " + r.Method + " " + path})
}

func (m *nangoMock) handleGetProviders(w http.ResponseWriter, _ *http.Request) {
	json.NewEncoder(w).Encode(map[string]any{
		"data": []map[string]any{
			{"name": "github", "display_name": "GitHub", "auth_mode": "OAUTH2", "webhook_user_defined_secret": true},
			{"name": "slack", "display_name": "Slack", "auth_mode": "OAUTH2"},
			{"name": "notion", "display_name": "Notion", "auth_mode": "OAUTH2"},
			{"name": "linear", "display_name": "Linear", "auth_mode": "OAUTH2"},
			{"name": "mailchimp", "display_name": "Mailchimp", "auth_mode": "OAUTH2"},
			{"name": "sendgrid", "display_name": "SendGrid", "auth_mode": "API_KEY"},
			{"name": "stripe", "display_name": "Stripe", "auth_mode": "API_KEY"},
			{"name": "asana", "display_name": "Asana", "auth_mode": "OAUTH2"},
			{"name": "jira", "display_name": "Jira", "auth_mode": "OAUTH2"},
			{"name": "salesforce", "display_name": "Salesforce", "auth_mode": "OAUTH2"},
			{"name": "github-app-oauth", "display_name": "GitHub App", "auth_mode": "APP"},
			{"name": "salesforce", "display_name": "Salesforce", "auth_mode": "OAUTH2"},
		},
	})
}

func (m *nangoMock) handleCreateConnectSession(w http.ResponseWriter, _ *http.Request) {
	json.NewEncoder(w).Encode(map[string]any{
		"data": map[string]any{
			"token":      "csess_mock_" + time.Now().Format("20060102150405"),
			"expires_at": time.Now().Add(15 * time.Minute).Format(time.RFC3339),
		},
	})
}

func (m *nangoMock) handleCreateConnection(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	var req map[string]any
	json.Unmarshal(body, &req)

	connID, _ := req["connection_id"].(string)
	providerConfigKey, _ := req["provider_config_key"].(string)

	m.mu.Lock()
	m.connections[m.connKey(connID, providerConfigKey)] = map[string]any{
		"connection_id":      connID,
		"provider_config_key": providerConfigKey,
		"provider":           "github",
		"connection_config":  map[string]any{},
		"credentials":        map[string]any{"access_token": "mock_token"},
	}
	m.mu.Unlock()

	json.NewEncoder(w).Encode(map[string]any{"status": "ok"})
}

func (m *nangoMock) handleGetConnection(w http.ResponseWriter, connID, providerConfigKey string) {
	m.mu.Lock()
	conn, ok := m.connections[m.connKey(connID, providerConfigKey)]
	m.mu.Unlock()

	if !ok {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]any{"error": "connection not found"})
		return
	}
	json.NewEncoder(w).Encode(conn)
}

func (m *nangoMock) handleDeleteConnection(w http.ResponseWriter, connID, providerConfigKey string) {
	m.mu.Lock()
	delete(m.connections, m.connKey(connID, providerConfigKey))
	m.mu.Unlock()

	json.NewEncoder(w).Encode(map[string]any{"status": "ok"})
}

func (m *nangoMock) handleCreateIntegration(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	var req map[string]any
	json.Unmarshal(body, &req)

	uniqueKey, _ := req["unique_key"].(string)

	// If credentials include APP type, generate a webhook_secret like real Nango does.
	creds, _ := req["credentials"].(map[string]any)
	if creds != nil {
		if credType, _ := creds["type"].(string); credType == "APP" {
			creds["webhook_secret"] = "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6abcd"
		}
	}

	m.mu.Lock()
	m.integrations[uniqueKey] = map[string]any{
		"unique_key":   uniqueKey,
		"provider":     req["provider"],
		"display_name": req["display_name"],
		"credentials":  creds,
		"webhook_url":  "https://mock.nango.dev/webhooks/" + uniqueKey,
	}
	m.mu.Unlock()

	json.NewEncoder(w).Encode(map[string]any{"status": "ok"})
}

func (m *nangoMock) handleGetIntegration(w http.ResponseWriter, uniqueKey string) {
	m.mu.Lock()
	integ, ok := m.integrations[uniqueKey]
	m.mu.Unlock()

	if !ok {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]any{"error": "integration not found"})
		return
	}
	json.NewEncoder(w).Encode(map[string]any{"data": integ})
}

func (m *nangoMock) handleUpdateIntegration(w http.ResponseWriter, r *http.Request, uniqueKey string) {
	body, _ := io.ReadAll(r.Body)
	var req map[string]any
	json.Unmarshal(body, &req)

	m.mu.Lock()
	integ, ok := m.integrations[uniqueKey]
	if ok {
		if dn, exists := req["display_name"]; exists {
			integ["display_name"] = dn
		}
		if creds, exists := req["credentials"]; exists {
			credsMap, _ := creds.(map[string]any)
			// Regenerate webhook_secret on credential rotation for APP type
			if credsMap != nil {
				if credType, _ := credsMap["type"].(string); credType == "APP" {
					credsMap["webhook_secret"] = fmt.Sprintf("%x", time.Now().UnixNano()) + "00000000000000000000000000000000000000000000000000"
					credsMap["webhook_secret"] = credsMap["webhook_secret"].(string)[:64]
				}
			}
			integ["credentials"] = credsMap
		}
		m.integrations[uniqueKey] = integ
	}
	m.mu.Unlock()

	if !ok {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]any{"error": "integration not found"})
		return
	}

	json.NewEncoder(w).Encode(map[string]any{"status": "ok"})
}

func (m *nangoMock) handleDeleteIntegration(w http.ResponseWriter, uniqueKey string) {
	m.mu.Lock()
	delete(m.integrations, uniqueKey)
	m.mu.Unlock()

	json.NewEncoder(w).Encode(map[string]any{"status": "ok"})
}

func (m *nangoMock) handleProxy(w http.ResponseWriter, _ *http.Request) {
	// Return a generic success response for proxy requests
	json.NewEncoder(w).Encode(map[string]any{"ok": true})
}
