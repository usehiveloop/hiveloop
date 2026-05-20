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
	emailsStatus     int
	githubEmails     []map[string]any
	repoStatus       int
	repoPermissions  map[string]map[string]bool
	hookListStatus   int
	hookCreateStatus int
	hookUpdateStatus int
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
	if cfg.emailsStatus == 0 {
		cfg.emailsStatus = http.StatusOK
	}
	if cfg.repoStatus == 0 {
		cfg.repoStatus = http.StatusOK
	}
	if cfg.hookListStatus == 0 {
		cfg.hookListStatus = http.StatusOK
	}
	if cfg.hookCreateStatus == 0 {
		cfg.hookCreateStatus = http.StatusCreated
	}
	if cfg.hookUpdateStatus == 0 {
		cfg.hookUpdateStatus = http.StatusOK
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
					{"name": "linear", "display_name": "Linear", "auth_mode": "OAUTH2", "webhook_user_defined_secret": true},
					{"name": "notion", "display_name": "Notion", "auth_mode": "OAUTH2"},
				},
			})
			return
		}

		if strings.HasPrefix(r.URL.Path, "/integrations") {
			switch r.Method {
			case http.MethodPost:
				_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"unique_key": "test"}})
			case http.MethodGet:
				_ = json.NewEncoder(w).Encode(map[string]any{
					"data": map[string]any{
						"unique_key":       "test",
						"logo":             "https://example.com/linear.png",
						"webhook_url":      "https://webhook.nango.test/linear-profile",
						"credentials":      map[string]any{"webhook_secret": "nango-generated-secret"},
						"forward_webhooks": true,
					},
				})
			case http.MethodPatch:
				_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"unique_key": "test"}})
			case http.MethodDelete:
				_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok"})
			default:
				w.WriteHeader(http.StatusMethodNotAllowed)
			}
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

		if r.URL.Path == "/connect/sessions/reconnect" && r.Method == http.MethodPost {
			w.WriteHeader(cfg.connectStatus)
			if cfg.connectStatus >= 400 {
				_ = json.NewEncoder(w).Encode(map[string]any{"error": "nango error"})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"token":      "test-reconnect-token",
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
				"connection_config": map[string]any{"org": "hivy"},
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

		if r.URL.Path == "/proxy/user/emails" && r.Method == http.MethodGet {
			w.WriteHeader(cfg.emailsStatus)
			if cfg.emailsStatus >= 400 {
				_ = json.NewEncoder(w).Encode(map[string]any{"message": "github emails error"})
				return
			}
			emails := cfg.githubEmails
			if emails == nil {
				emails = []map[string]any{
					{"email": "octocat-unverified@example.com", "verified": false, "primary": false},
					{"email": "octocat@example.com", "verified": true, "primary": true},
				}
			}
			_ = json.NewEncoder(w).Encode(emails)
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

		if strings.HasPrefix(r.URL.Path, "/proxy/repos/") && strings.HasSuffix(r.URL.Path, "/hooks") {
			switch r.Method {
			case http.MethodGet:
				w.WriteHeader(cfg.hookListStatus)
				if cfg.hookListStatus >= 400 {
					_ = json.NewEncoder(w).Encode(map[string]any{"message": "github hook list error"})
					return
				}
				_ = json.NewEncoder(w).Encode([]map[string]any{})
				return
			case http.MethodPost:
				w.WriteHeader(cfg.hookCreateStatus)
				if cfg.hookCreateStatus >= 400 {
					_ = json.NewEncoder(w).Encode(map[string]any{"message": "github hook create error"})
					return
				}
				_ = json.NewEncoder(w).Encode(map[string]any{
					"id":         9001,
					"name":       "web",
					"active":     true,
					"events":     []string{"pull_request", "pull_request_review", "pull_request_review_comment", "pull_request_review_thread", "issue_comment", "workflow_run", "workflow_job", "commit_comment", "issues"},
					"config":     map[string]any{"url": "https://api.hivy.test/internal/webhooks/github/employees/test"},
					"created_at": time.Now().UTC().Format(time.RFC3339),
				})
				return
			}
		}

		if strings.HasPrefix(r.URL.Path, "/proxy/repos/") && r.Method == http.MethodGet {
			w.WriteHeader(cfg.repoStatus)
			if cfg.repoStatus >= 400 {
				_ = json.NewEncoder(w).Encode(map[string]any{"message": "github repo error"})
				return
			}
			fullName := strings.TrimPrefix(r.URL.Path, "/proxy/repos/")
			permissions := map[string]bool{"admin": true, "maintain": true, "push": true, "triage": true, "pull": true}
			if cfg.repoPermissions != nil {
				if override, ok := cfg.repoPermissions[fullName]; ok {
					permissions = override
				}
			}
			parts := strings.Split(fullName, "/")
			name := fullName
			if len(parts) == 2 {
				name = parts[1]
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":          101,
				"name":        name,
				"full_name":   fullName,
				"permissions": permissions,
			})
			return
		}

		if strings.Contains(r.URL.Path, "/hooks/") && r.Method == http.MethodPatch {
			w.WriteHeader(cfg.hookUpdateStatus)
			if cfg.hookUpdateStatus >= 400 {
				_ = json.NewEncoder(w).Encode(map[string]any{"message": "github hook update error"})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":         9001,
				"name":       "web",
				"active":     true,
				"events":     []string{"pull_request", "pull_request_review", "pull_request_review_comment", "pull_request_review_thread", "issue_comment", "workflow_run", "workflow_job", "commit_comment", "issues"},
				"config":     map[string]any{"url": "https://api.hivy.test/internal/webhooks/github/employees/test"},
				"created_at": time.Now().UTC().Format(time.RFC3339),
			})
			return
		}

		w.WriteHeader(http.StatusNotFound)
	})
}
