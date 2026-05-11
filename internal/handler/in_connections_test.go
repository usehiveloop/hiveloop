package handler_test

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

type nangoConnMockConfig struct {
	mu               sync.Mutex
	capturedPaths    []string
	capturedMethods  []string
	capturedBodies   [][]byte
	connectStatus    int
	getConnStatus    int
	deleteConnStatus int
	proxyStatus      int
}

func newNangoConnMock(cfg *nangoConnMockConfig) http.Handler {
	if cfg.connectStatus == 0 {
		cfg.connectStatus = http.StatusOK
	}
	if cfg.getConnStatus == 0 {
		cfg.getConnStatus = http.StatusOK
	}
	if cfg.deleteConnStatus == 0 {
		cfg.deleteConnStatus = http.StatusOK
	}
	if cfg.proxyStatus == 0 {
		cfg.proxyStatus = http.StatusOK
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		cfg.mu.Lock()
		cfg.capturedPaths = append(cfg.capturedPaths, r.URL.Path)
		cfg.capturedMethods = append(cfg.capturedMethods, r.Method)
		cfg.capturedBodies = append(cfg.capturedBodies, body)
		cfg.mu.Unlock()

		if r.URL.Path == "/providers" && r.Method == http.MethodGet {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]any{
					{"name": "github", "display_name": "GitHub", "auth_mode": "OAUTH2"},
					{"name": "slack", "display_name": "Slack", "auth_mode": "OAUTH2"},
					{"name": "notion", "display_name": "Notion", "auth_mode": "OAUTH2"},
				},
			})
			return
		}

		if r.URL.Path == "/connect/sessions" && r.Method == http.MethodPost {
			w.WriteHeader(cfg.connectStatus)
			if cfg.connectStatus >= 400 {
				_ = json.NewEncoder(w).Encode(map[string]any{"error": "nango error"})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"token":      "test-connect-token",
					"expires_at": time.Now().Add(15 * time.Minute).Format(time.RFC3339),
				},
			})
			return
		}

		if strings.HasPrefix(r.URL.Path, "/connection/") && r.Method == http.MethodGet {
			w.WriteHeader(cfg.getConnStatus)
			if cfg.getConnStatus >= 400 {
				_ = json.NewEncoder(w).Encode(map[string]any{"error": "nango error"})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"provider":          "github",
				"connection_config": map[string]any{"org": "hiveloop"},
				"credentials":       map[string]any{"access_token": "gho_xxxx"},
			})
			return
		}

		if strings.HasPrefix(r.URL.Path, "/connection/") && r.Method == http.MethodDelete {
			w.WriteHeader(cfg.deleteConnStatus)
			if cfg.deleteConnStatus >= 400 {
				_ = json.NewEncoder(w).Encode(map[string]any{"error": "nango error"})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok"})
			return
		}

		if r.URL.Path == "/proxy/user" && r.Method == http.MethodGet {
			w.WriteHeader(cfg.proxyStatus)
			if cfg.proxyStatus >= 400 {
				_ = json.NewEncoder(w).Encode(map[string]any{"message": "github error"})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":         12345,
				"login":      "octocat",
				"name":       "The Octocat",
				"avatar_url": "https://github.com/images/error/octocat_happy.gif",
				"html_url":   "https://github.com/octocat",
			})
			return
		}

		if r.URL.Path == "/proxy/user/repos" && r.Method == http.MethodGet {
			w.WriteHeader(cfg.proxyStatus)
			if cfg.proxyStatus >= 400 {
				_ = json.NewEncoder(w).Encode(map[string]any{"message": "github error"})
				return
			}
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{
					"id":          101,
					"node_id":     "R_kgDO101",
					"name":        "alpha",
					"full_name":   "octocat/alpha",
					"private":     false,
					"html_url":    "https://github.com/octocat/alpha",
					"description": "Alpha repo",
					"owner":       map[string]any{"login": "octocat"},
				},
				{
					"id":        102,
					"node_id":   "R_kgDO102",
					"name":      "private-beta",
					"full_name": "octocat/private-beta",
					"private":   true,
					"html_url":  "https://github.com/octocat/private-beta",
					"owner":     map[string]any{"login": "octocat"},
				},
			})
			return
		}

		w.WriteHeader(http.StatusNotFound)
	})
}
