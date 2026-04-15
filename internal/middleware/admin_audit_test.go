package middleware

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ziraloop/ziraloop/internal/model"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// ---------------------------------------------------------------------------
// Unit tests for helper functions (no DB needed)
// ---------------------------------------------------------------------------

func TestParseAdminPath(t *testing.T) {
	tests := []struct {
		method, path                     string
		wantResource, wantID, wantAction string
	}{
		{"POST", "/admin/v1/users/abc-123/ban", "users", "abc-123", "ban_user"},
		{"POST", "/admin/v1/users/abc-123/unban", "users", "abc-123", "unban_user"},
		{"POST", "/admin/v1/users/abc-123/confirm-email", "users", "abc-123", "confirm-email_user"},
		{"PUT", "/admin/v1/users/abc-123", "users", "abc-123", "update_user"},
		{"DELETE", "/admin/v1/users/abc-123", "users", "abc-123", "delete_user"},
		{"PUT", "/admin/v1/orgs/abc-123", "orgs", "abc-123", "update_org"},
		{"POST", "/admin/v1/orgs/abc-123/deactivate", "orgs", "abc-123", "deactivate_org"},
		{"POST", "/admin/v1/orgs/abc-123/activate", "orgs", "abc-123", "activate_org"},
		{"DELETE", "/admin/v1/agents/abc-123", "agents", "abc-123", "delete_agent"},
		{"POST", "/admin/v1/agents/abc-123/archive", "agents", "abc-123", "archive_agent"},
		{"POST", "/admin/v1/sandboxes/cleanup", "sandboxes", "", "cleanup_sandboxes"},
		{"POST", "/admin/v1/credentials/abc-123/revoke", "credentials", "abc-123", "revoke_credential"},
		{"POST", "/admin/v1/api-keys/abc-123/revoke", "api-keys", "abc-123", "revoke_api_key"},
		{"DELETE", "/admin/v1/sandbox-templates/abc-123", "sandbox-templates", "abc-123", "delete_sandbox_template"},
		{"DELETE", "/admin/v1/connect-sessions/abc-123", "connect-sessions", "abc-123", "delete_connect_session"},
		{"DELETE", "/admin/v1/custom-domains/abc-123", "custom-domains", "abc-123", "delete_custom_domain"},
		{"DELETE", "/admin/v1/workspace-storage/abc-123", "workspace-storage", "abc-123", "delete_workspace_storage"},
	}

	for _, tc := range tests {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			resource, id, action := parseAdminPath(tc.method, tc.path)
			if resource != tc.wantResource {
				t.Errorf("resource: got %q, want %q", resource, tc.wantResource)
			}
			if id != tc.wantID {
				t.Errorf("resourceID: got %q, want %q", id, tc.wantID)
			}
			if action != tc.wantAction {
				t.Errorf("action: got %q, want %q", action, tc.wantAction)
			}
		})
	}
}

func TestSanitizePayload(t *testing.T) {
	raw := `{"name":"Alice","email":"alice@example.com","password":"secret123","meta":{"token":"abc"}}`
	result := sanitizePayload([]byte(raw))

	if result["name"] != "Alice" {
		t.Errorf("name should not be masked, got %v", result["name"])
	}
	if result["email"] != maskValue {
		t.Errorf("email should be masked, got %v", result["email"])
	}
	if result["password"] != maskValue {
		t.Errorf("password should be masked, got %v", result["password"])
	}
	meta, ok := result["meta"].(map[string]any)
	if !ok {
		t.Fatalf("meta should be a map, got %T", result["meta"])
	}
	if meta["token"] != maskValue {
		t.Errorf("nested token should be masked, got %v", meta["token"])
	}
}

func TestSanitizePayload_NonSensitiveFieldsPreserved(t *testing.T) {
	raw := `{"name":"Bob","description":"A test agent","model":"gpt-4","sandbox_type":"dedicated","status":"active"}`
	result := sanitizePayload([]byte(raw))

	for _, field := range []string{"name", "description", "model", "sandbox_type", "status"} {
		if result[field] == maskValue {
			t.Errorf("field %q should NOT be masked, but was", field)
		}
	}
}

func TestSanitizePayload_InvalidJSON(t *testing.T) {
	result := sanitizePayload([]byte("not json"))
	if result["_raw"] != "(non-JSON body)" {
		t.Errorf("expected non-JSON marker, got %v", result)
	}
}

func TestMaskEmail(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"admin@example.com", "a***n@example.com"},
		{"ab@x.com", "a***b@x.com"},
		{"a@x.com", "***@x.com"},
		{"nodomain", maskValue},
	}
	for _, tc := range tests {
		got := maskEmail(tc.input)
		if got != tc.want {
			t.Errorf("maskEmail(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestMaskMap_NestedArrays(t *testing.T) {
	m := map[string]any{
		"name": "test",
		"items": []any{
			map[string]any{"key": "visible", "secret": "hidden"},
		},
	}
	maskMap(m)

	if m["name"] != "test" {
		t.Error("name should not be masked")
	}
	items := m["items"].([]any)
	item := items[0].(map[string]any)
	if item["key"] != maskValue {
		t.Errorf("nested 'key' should be masked, got %v", item["key"])
	}
	if item["secret"] != maskValue {
		t.Errorf("nested 'secret' should be masked, got %v", item["secret"])
	}
}

// ---------------------------------------------------------------------------
// Bucket-based context test (handler → middleware communication)
// ---------------------------------------------------------------------------

func TestAdminAuditBucket(t *testing.T) {
	r := httptest.NewRequest("PUT", "/admin/v1/users/abc-123", nil)

	// Middleware allocates bucket and places it on context
	bucket := &AdminAuditBucket{}
	r = WithAdminAuditBucket(r, bucket)

	// Handler writes diff into the bucket (using the same r, no reassignment needed)
	changes := AdminAuditChanges{
		"name": map[string]any{"old": "Alice", "new": "Bob"},
	}
	SetAdminAuditChanges(r, changes)

	// Middleware reads from the bucket pointer — sees the handler's writes
	if bucket.Changes == nil {
		t.Fatal("expected changes in bucket")
	}
	if len(bucket.Changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(bucket.Changes))
	}
	nameChange := bucket.Changes["name"].(map[string]any)
	if nameChange["old"] != "Alice" || nameChange["new"] != "Bob" {
		t.Errorf("unexpected change: %v", nameChange)
	}
}

func TestAdminAuditBucket_NoBucket(t *testing.T) {
	r := httptest.NewRequest("PUT", "/admin/v1/users/abc-123", nil)
	// No bucket on context — SetAdminAuditChanges should not panic
	SetAdminAuditChanges(r, AdminAuditChanges{"name": "test"})
	bucket := AdminAuditBucketFromContext(r.Context())
	if bucket != nil {
		t.Error("expected nil bucket on fresh context")
	}
}

// ---------------------------------------------------------------------------
// Integration test helpers
// ---------------------------------------------------------------------------

func connectTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := "postgres://ziraloop:localdev@localhost:5433/ziraloop?sslmode=disable"
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Skipf("skipping: cannot connect to test DB: %v", err)
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(3)
	sqlDB.SetMaxIdleConns(1)
	if err := db.AutoMigrate(&model.AdminAuditEntry{}); err != nil {
		t.Fatalf("migration failed: %v", err)
	}
	t.Cleanup(func() { sqlDB.Close() })
	return db
}

// ---------------------------------------------------------------------------
// Integration: PUT uses handler diff (only changed fields)
// ---------------------------------------------------------------------------

func TestAdminAudit_PUT_UsesHandlerDiff(t *testing.T) {
	db := connectTestDB(t)

	innerHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Handler writes only the changed field into the bucket
		SetAdminAuditChanges(r, AdminAuditChanges{
			"name": map[string]any{"old": "Alice", "new": "Bob"},
		})
		w.WriteHeader(http.StatusOK)
	})

	mw := AdminAudit(db)
	handler := mw(innerHandler)

	body := `{"name":"Bob","email":"same@example.com"}`
	req := httptest.NewRequest("PUT", "/admin/v1/users/abc-123", bytes.NewBufferString(body))
	req.RemoteAddr = "127.0.0.1:1234"
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)
	time.Sleep(200 * time.Millisecond)

	var written model.AdminAuditEntry
	err := db.Where("path = ? AND method = ?", "/admin/v1/users/abc-123", "PUT").
		Order("created_at DESC").First(&written).Error
	if err != nil {
		t.Fatalf("failed to read audit entry: %v", err)
	}
	t.Cleanup(func() {
		db.Where("id = ?", written.ID).Delete(&model.AdminAuditEntry{})
	})

	if written.Payload == nil {
		t.Fatal("expected non-nil payload")
	}

	// Should have "name" with old/new
	nameField, exists := written.Payload["name"]
	if !exists {
		t.Fatalf("expected 'name' in payload, got: %v", written.Payload)
	}
	nameMap, ok := nameField.(map[string]any)
	if !ok {
		t.Fatalf("expected name to be a map, got %T: %v", nameField, nameField)
	}
	if nameMap["old"] != "Alice" {
		t.Errorf("expected old=Alice, got %v", nameMap["old"])
	}
	if nameMap["new"] != "Bob" {
		t.Errorf("expected new=Bob, got %v", nameMap["new"])
	}

	// Should NOT have "email" — it was in the raw body but not in the diff
	if _, exists := written.Payload["email"]; exists {
		t.Errorf("email should NOT be in payload (wasn't changed), got: %v", written.Payload["email"])
	}

	if written.Action != "update_user" {
		t.Errorf("expected action=update_user, got %q", written.Action)
	}
}

// ---------------------------------------------------------------------------
// Integration: POST uses raw body (no handler diff)
// ---------------------------------------------------------------------------

func TestAdminAudit_POST_UsesRawBody(t *testing.T) {
	db := connectTestDB(t)

	innerHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	mw := AdminAudit(db)
	handler := mw(innerHandler)

	body := `{"reason":"policy violation"}`
	req := httptest.NewRequest("POST", "/admin/v1/users/abc-123/ban", bytes.NewBufferString(body))
	req.RemoteAddr = "127.0.0.1:1234"
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)
	time.Sleep(200 * time.Millisecond)

	var written model.AdminAuditEntry
	err := db.Where("path = ? AND method = ?", "/admin/v1/users/abc-123/ban", "POST").
		Order("created_at DESC").First(&written).Error
	if err != nil {
		t.Fatalf("failed to read audit entry: %v", err)
	}
	t.Cleanup(func() {
		db.Where("id = ?", written.ID).Delete(&model.AdminAuditEntry{})
	})

	if written.Payload == nil {
		t.Fatal("expected non-nil payload")
	}
	if written.Action != "ban_user" {
		t.Errorf("expected action=ban_user, got %q", written.Action)
	}
}

// ---------------------------------------------------------------------------
// Integration: GET requests not logged
// ---------------------------------------------------------------------------

func TestAdminAudit_GET_NotLogged(t *testing.T) {
	db := connectTestDB(t)

	called := false
	innerHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	mw := AdminAudit(db)
	handler := mw(innerHandler)

	uniquePath := "/admin/v1/users/get-test-" + time.Now().Format("20060102150405")
	req := httptest.NewRequest("GET", uniquePath, nil)
	req.RemoteAddr = "127.0.0.1:1234"
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)
	time.Sleep(200 * time.Millisecond)

	if !called {
		t.Error("handler should have been called")
	}

	var count int64
	db.Model(&model.AdminAuditEntry{}).Where("path = ?", uniquePath).Count(&count)
	if count != 0 {
		t.Errorf("GET requests should not be logged, found %d entries", count)
	}
}

// ---------------------------------------------------------------------------
// Integration: PUT without handler diff falls back to raw body
// ---------------------------------------------------------------------------

func TestAdminAudit_PUT_NoChanges_UsesRawBody(t *testing.T) {
	db := connectTestDB(t)

	innerHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Handler does NOT write any changes (e.g. "no fields to update")
		w.WriteHeader(http.StatusBadRequest)
	})

	mw := AdminAudit(db)
	handler := mw(innerHandler)

	body := `{"name":"same_name"}`
	uniqueID := "fallback-test-" + time.Now().Format("150405")
	req := httptest.NewRequest("PUT", "/admin/v1/users/"+uniqueID, bytes.NewBufferString(body))
	req.RemoteAddr = "127.0.0.1:1234"
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)
	time.Sleep(200 * time.Millisecond)

	var written model.AdminAuditEntry
	err := db.Where("resource_id = ? AND method = ?", uniqueID, "PUT").
		Order("created_at DESC").First(&written).Error
	if err != nil {
		t.Fatalf("failed to read audit entry: %v", err)
	}
	t.Cleanup(func() {
		db.Where("id = ?", written.ID).Delete(&model.AdminAuditEntry{})
	})

	if written.Payload == nil {
		t.Fatal("expected payload from raw body fallback")
	}
	if written.Payload["name"] != "same_name" {
		t.Errorf("expected raw body name, got %v", written.Payload["name"])
	}
}

// ---------------------------------------------------------------------------
// Integration: sensitive fields in handler diffs are masked
// ---------------------------------------------------------------------------

func TestAdminAudit_PUT_DiffFieldsMasked(t *testing.T) {
	db := connectTestDB(t)

	innerHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		SetAdminAuditChanges(r, AdminAuditChanges{
			"email": map[string]any{"old": "old@example.com", "new": "new@example.com"},
			"name":  map[string]any{"old": "Alice", "new": "Bob"},
		})
		w.WriteHeader(http.StatusOK)
	})

	mw := AdminAudit(db)
	handler := mw(innerHandler)

	uniqueID := "mask-test-" + time.Now().Format("150405")
	req := httptest.NewRequest("PUT", "/admin/v1/users/"+uniqueID, bytes.NewBufferString(`{}`))
	req.RemoteAddr = "127.0.0.1:1234"
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)
	time.Sleep(200 * time.Millisecond)

	var written model.AdminAuditEntry
	err := db.Where("resource_id = ? AND method = ?", uniqueID, "PUT").
		Order("created_at DESC").First(&written).Error
	if err != nil {
		t.Fatalf("failed to read audit entry: %v", err)
	}
	t.Cleanup(func() {
		db.Where("id = ?", written.ID).Delete(&model.AdminAuditEntry{})
	})

	// email field should be masked
	if written.Payload["email"] != maskValue {
		t.Errorf("email diff should be masked, got %v", written.Payload["email"])
	}
	// name should still have old/new
	nameField := written.Payload["name"]
	if nameField == maskValue {
		t.Error("name should NOT be masked")
	}
}
