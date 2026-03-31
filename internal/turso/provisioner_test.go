package turso

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/llmvault/llmvault/internal/model"
)

const testDBURL = "postgres://llmvault:localdev@localhost:5433/llmvault_test?sslmode=disable"

func setupDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = testDBURL
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("connect to postgres: %v", err)
	}
	sqlDB, _ := db.DB()
	if err := sqlDB.Ping(); err != nil {
		t.Fatalf("postgres not reachable: %v", err)
	}
	if err := model.AutoMigrate(db); err != nil {
		t.Fatalf("migration: %v", err)
	}
	t.Cleanup(func() { sqlDB.Close() })
	return db
}

func TestProvisioner_EnsureStorage(t *testing.T) {
	db := setupDB(t)
	suffix := uuid.New().String()[:8]

	// Create org
	org := model.Org{Name: "turso-test-" + suffix}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	t.Cleanup(func() {
		db.Where("org_id = ?", org.ID).Delete(&model.WorkspaceStorage{})
		db.Where("id = ?", org.ID).Delete(&model.Org{})
	})

	// Mock Turso server
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/databases"):
			var body struct {
				Name  string `json:"name"`
				Group string `json:"group"`
			}
			json.NewDecoder(r.Body).Decode(&body)
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]any{
				"database": map[string]any{
					"Name":     body.Name,
					"DbId":     "db-" + body.Name,
					"Hostname": body.Name + "-test.turso.io",
				},
			})
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/auth/tokens"):
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]string{"jwt": "mock-jwt-token"})
		case r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]string{"database": "deleted"})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	client := NewClient("token", "org")
	client.baseURL = srv.URL
	provisioner := NewProvisioner(client, "default", db)

	ctx := context.Background()

	// First call should create the database
	storageURL, authToken, err := provisioner.EnsureStorage(ctx, org.ID)
	if err != nil {
		t.Fatalf("EnsureStorage (first call): %v", err)
	}
	if !strings.HasPrefix(storageURL, "libsql://") {
		t.Errorf("storage URL should start with libsql://, got %q", storageURL)
	}
	if authToken != "mock-jwt-token" {
		t.Errorf("auth token: got %q", authToken)
	}

	// Verify record in DB
	var ws model.WorkspaceStorage
	if err := db.Where("org_id = ?", org.ID).First(&ws).Error; err != nil {
		t.Fatalf("workspace storage not found in DB: %v", err)
	}
	if ws.StorageURL != storageURL {
		t.Errorf("DB storage URL mismatch: got %q", ws.StorageURL)
	}
	if !strings.HasPrefix(ws.TursoDatabaseName, "llmv-") {
		t.Errorf("DB name should start with llmv-, got %q", ws.TursoDatabaseName)
	}

	// Second call should return existing (idempotent)
	storageURL2, authToken2, err := provisioner.EnsureStorage(ctx, org.ID)
	if err != nil {
		t.Fatalf("EnsureStorage (second call): %v", err)
	}
	if storageURL2 != storageURL {
		t.Errorf("second call should return same URL: got %q, want %q", storageURL2, storageURL)
	}
	if authToken2 != authToken {
		t.Errorf("second call should return same token")
	}

	// Verify only one record exists
	var count int64
	db.Model(&model.WorkspaceStorage{}).Where("org_id = ?", org.ID).Count(&count)
	if count != 1 {
		t.Errorf("expected 1 workspace storage record, got %d", count)
	}
}

func TestProvisioner_DeleteStorage(t *testing.T) {
	db := setupDB(t)
	suffix := uuid.New().String()[:8]

	// Create org + storage record
	org := model.Org{Name: "turso-del-" + suffix}
	db.Create(&org)
	t.Cleanup(func() {
		db.Where("org_id = ?", org.ID).Delete(&model.WorkspaceStorage{})
		db.Where("id = ?", org.ID).Delete(&model.Org{})
	})

	ws := model.WorkspaceStorage{
		OrgID:             org.ID,
		TursoDatabaseName: "llmv-del-" + suffix,
		StorageURL:        "libsql://del.turso.io",
		StorageAuthToken:  "token",
	}
	db.Create(&ws)

	// Mock Turso server
	var deletedDB string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			parts := strings.Split(r.URL.Path, "/")
			deletedDB = parts[len(parts)-1]
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	client := NewClient("token", "org")
	client.baseURL = srv.URL
	provisioner := NewProvisioner(client, "default", db)

	err := provisioner.DeleteStorage(context.Background(), org.ID)
	if err != nil {
		t.Fatalf("DeleteStorage: %v", err)
	}

	// Verify Turso API was called with correct DB name
	if deletedDB != ws.TursoDatabaseName {
		t.Errorf("deleted wrong DB: got %q, want %q", deletedDB, ws.TursoDatabaseName)
	}

	// Verify record removed from DB
	var count int64
	db.Model(&model.WorkspaceStorage{}).Where("org_id = ?", org.ID).Count(&count)
	if count != 0 {
		t.Error("workspace storage should be deleted from DB")
	}
}

func TestShortID(t *testing.T) {
	id := uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")
	short := shortID(id)
	if len(short) != 12 {
		t.Errorf("expected 12 chars, got %d: %q", len(short), short)
	}
	if short != "550e8400e29b" {
		t.Errorf("shortID: got %q", short)
	}
}
