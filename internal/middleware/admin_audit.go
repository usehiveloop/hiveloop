package middleware

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/enqueue"
	"github.com/usehiveloop/hiveloop/internal/model"
	"github.com/usehiveloop/hiveloop/internal/tasks"
)

// sensitiveKeys are fields whose values must be masked in admin audit logs.
// Matching is case-insensitive and applies to any nesting depth.
var sensitiveKeys = map[string]bool{
	"password":           true,
	"password_hash":      true,
	"email":              true,
	"token":              true,
	"access_token":       true,
	"refresh_token":      true,
	"secret":             true,
	"api_key":            true,
	"key":                true,
	"encrypted_key":      true,
	"wrapped_dek":        true,
	"session_token":      true,
	"nango_secret_key":   true,
	"key_hash":           true,
	"key_prefix":         true,
	"token_hash":         true,
	"ban_reason":         true,
	"ip_address":         true,
	"allowed_origins":    true,
	"encrypted_env_vars": true,
}

const maskValue = "***"

// AdminAudit returns middleware that logs every admin API request to the
// admin_audit_log table. It captures the sanitized request body for mutating
// operations (POST/PUT/DELETE) and skips body capture for GET/HEAD.
//
// The middleware extracts:
//   - admin user ID and email from AuthClaims + User context
//   - resource and resource ID from the URL path
//   - a human-readable action (e.g. "update_user", "ban_user", "delete_org")
func AdminAudit(db *gorm.DB, enqueuer ...enqueue.TaskEnqueuer) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			start := time.Now()

			var rawBody []byte
			if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Body != nil {
				rawBody, _ = io.ReadAll(r.Body)
				r.Body = io.NopCloser(bytes.NewReader(rawBody))
			}

			bucket := &AdminAuditBucket{}
			r = WithAdminAuditBucket(r, bucket)

			sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(sw, r)

			if r.Method == http.MethodGet || r.Method == http.MethodHead {
				return
			}

			entry := model.AdminAuditEntry{
				Method:     r.Method,
				Path:       r.URL.Path,
				StatusCode: sw.status,
				LatencyMs:  time.Since(start).Milliseconds(),
				CreatedAt:  time.Now(),
			}

			if claims, ok := AuthClaimsFromContext(ctx); ok {
				if uid, err := uuid.Parse(claims.UserID); err == nil {
					entry.AdminID = uid
				}
			}
			if user, ok := UserFromContext(ctx); ok {
				entry.AdminEmail = maskEmail(user.Email)
			}

			if ip, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
				entry.IPAddress = &ip
			} else {
				addr := r.RemoteAddr
				entry.IPAddress = &addr
			}

			entry.Resource, entry.ResourceID, entry.Action = parseAdminPath(r.Method, r.URL.Path)

			if len(bucket.Changes) > 0 {
				sanitized := model.JSON(bucket.Changes)
				maskMap(sanitized)
				entry.Payload = sanitized
			} else if len(rawBody) > 0 {
				entry.Payload = sanitizePayload(rawBody)
			}

			if len(enqueuer) > 0 && enqueuer[0] != nil {
				if task, err := tasks.NewAdminAuditWriteTask(entry); err == nil {
					if _, err := enqueuer[0].Enqueue(task); err != nil {
						slog.Error("failed to enqueue admin audit entry", "error", err, "path", entry.Path)
					}
				}
			} else {
				go func() {
					if err := db.Create(&entry).Error; err != nil {
						slog.Error("failed to write admin audit entry", "error", err, "path", entry.Path)
					}
				}()
			}
		})
	}
}

// parseAdminPath extracts resource, resource ID, and action from an admin API path.
// Examples:
//
//	POST /admin/v1/users/abc-123/ban  → resource="users", resourceID="abc-123", action="ban_user"
//	PUT  /admin/v1/orgs/abc-123      → resource="orgs",  resourceID="abc-123", action="update_org"
//	DELETE /admin/v1/employees/abc-123   → resource="employees", resourceID="abc-123", action="delete_employee"
func parseAdminPath(method, path string) (resource, resourceID, action string) {

	path = strings.TrimPrefix(path, "/admin/v1/")
	parts := strings.Split(path, "/")

	if len(parts) == 0 {
		return "unknown", "", "unknown"
	}

	resource = parts[0]

	singular := strings.TrimSuffix(resource, "s")
	if resource == "sandbox-templates" {
		singular = "sandbox_template"
	} else if resource == "api-keys" {
		singular = "api_key"
	} else if resource == "connect-sessions" {
	} else if resource == "custom-domains" {
		singular = "custom_domain"
	} else if resource == "workspace-storage" {
		singular = "workspace_storage"
	} else if resource == "in-integrations" {
		singular = "in_integration"
	} else if resource == "in-connections" {
		singular = "in_connection"
	}
	singular = strings.ReplaceAll(singular, "-", "_")

	switch {
	case len(parts) >= 3:

		resourceID = parts[1]
		action = parts[2] + "_" + singular
	case len(parts) == 2 && parts[1] == "cleanup":

		action = "cleanup_" + resource
	case len(parts) == 2:

		resourceID = parts[1]
		switch method {
		case http.MethodPut:
			action = "update_" + singular
		case http.MethodDelete:
			action = "delete_" + singular
		case http.MethodPost:
			action = "create_" + singular
		default:
			action = method + "_" + singular
		}
	case len(parts) == 1:

		if resource == "cleanup" {
			resource = "sandboxes"
			action = "cleanup_sandboxes"
		} else {
			action = method + "_" + singular
		}
	}

	return resource, resourceID, action
}

// sanitizePayload parses JSON body and masks sensitive fields.
func sanitizePayload(raw []byte) model.JSON {
	var data map[string]any
	if err := json.Unmarshal(raw, &data); err != nil {
		return model.JSON{"_raw": "(non-JSON body)"}
	}
	maskMap(data)
	return model.JSON(data)
}

// maskMap recursively masks sensitive fields in a map.
func maskMap(m map[string]any) {
	for key, val := range m {
		if sensitiveKeys[strings.ToLower(key)] {
			m[key] = maskValue
			continue
		}
		switch v := val.(type) {
		case map[string]any:
			maskMap(v)
		case []any:
			for _, item := range v {
				if sub, ok := item.(map[string]any); ok {
					maskMap(sub)
				}
			}
		}
	}
}

// maskEmail masks the local part of an email, keeping first and last chars.
// "admin@example.com" → "a***n@example.com"
func maskEmail(email string) string {
	parts := strings.SplitN(email, "@", 2)
	if len(parts) != 2 {
		return maskValue
	}
	local := parts[0]
	if len(local) <= 1 {
		return maskValue + "@" + parts[1]
	}
	return string(local[0]) + "***" + string(local[len(local)-1]) + "@" + parts[1]
}
