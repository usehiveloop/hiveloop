package integrations

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/nango"
)

const testDBURL = "postgres://hivy:localdev@localhost:5433/hivy_test?sslmode=disable" // #nosec G101 -- test fixture, not a real secret

func TestSeedGlobalIntegrations_RealNangoCreateUpdateAndSkip(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	db := connectIntegrationTestDB(t)
	client := realNangoClient(t, ctx)
	suffix := strings.ReplaceAll(uuid.NewString(), "-", "")[:12]
	manifestID := "linear-seed-" + suffix
	uniqueKey := "linear-seed-" + suffix
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cleanupCancel()
		if err := client.DeleteIntegration(cleanupCtx, nangoKey(uniqueKey)); err != nil && !isNotFound(err) {
			t.Logf("cleanup Nango integration %s: %v", nangoKey(uniqueKey), err)
		}
	})
	t.Setenv("LINEAR_CLIENT_ID_"+suffix, "hivy-placeholder-client-id-"+suffix)
	t.Setenv("LINEAR_CLIENT_SECRET_"+suffix, "hivy-placeholder-client-secret-"+suffix)

	dir := t.TempDir()
	writeManifest(t, dir, map[string]any{
		"version":          1,
		"id":               manifestID,
		"provider":         "linear",
		"unique_key":       uniqueKey,
		"display_name":     "Linear Seed Test",
		"allow_no_catalog": true,
		"credentials": map[string]any{
			"type":              "OAUTH2",
			"client_id_env":     "LINEAR_CLIENT_ID_" + suffix,
			"client_secret_env": "LINEAR_CLIENT_SECRET_" + suffix,
			"scopes":            "read,write",
		},
	})

	result, err := SeedGlobalIntegrations(ctx, db, client, nil, dir)
	if err != nil {
		t.Fatalf("seed create: %v", err)
	}
	if result.Created != 1 || result.Updated != 0 || result.Skipped != 0 {
		t.Fatalf("unexpected create result: %+v", result)
	}

	var integ model.InIntegration
	if err := db.Where("managed_by = ? AND managed_id = ?", managedBy, manifestID).First(&integ).Error; err != nil {
		t.Fatalf("load seeded integration: %v", err)
	}
	if integ.UniqueKey != uniqueKey || integ.Provider != "linear" || integ.ManagedHash == "" {
		t.Fatalf("unexpected seeded integration: %+v", integ)
	}
	if got := integ.NangoConfig["auth_mode"]; got != "OAUTH2" {
		t.Fatalf("expected auth_mode from Nango template, got %v", got)
	}

	result, err = SeedGlobalIntegrations(ctx, db, client, nil, dir)
	if err != nil {
		t.Fatalf("seed unchanged: %v", err)
	}
	if result.Unchanged != 1 || result.Created != 0 || result.Updated != 0 {
		t.Fatalf("unexpected unchanged result: %+v", result)
	}

	writeManifest(t, dir, map[string]any{
		"version":          1,
		"id":               manifestID,
		"provider":         "linear",
		"unique_key":       uniqueKey,
		"display_name":     "Linear Team",
		"allow_no_catalog": true,
		"credentials": map[string]any{
			"type":              "OAUTH2",
			"client_id_env":     "LINEAR_CLIENT_ID_" + suffix,
			"client_secret_env": "LINEAR_CLIENT_SECRET_" + suffix,
		},
	})
	result, err = SeedGlobalIntegrations(ctx, db, client, nil, dir)
	if err != nil {
		t.Fatalf("seed update: %v", err)
	}
	if result.Updated != 1 {
		t.Fatalf("expected one update, got %+v", result)
	}
}

func TestSeedGlobalIntegrations_RealNangoMissingOptionalEnvSkips(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	db := connectIntegrationTestDB(t)
	client := realNangoClient(t, ctx)
	suffix := strings.ReplaceAll(uuid.NewString(), "-", "")[:12]

	dir := t.TempDir()
	writeManifest(t, dir, map[string]any{
		"version":          1,
		"id":               "linear-skip-" + suffix,
		"provider":         "linear",
		"unique_key":       "linear-skip-" + suffix,
		"display_name":     "Linear",
		"allow_no_catalog": true,
		"credentials": map[string]any{
			"type":              "OAUTH2",
			"client_id_env":     "MISSING_LINEAR_CLIENT_ID_" + suffix,
			"client_secret_env": "MISSING_LINEAR_CLIENT_SECRET_" + suffix,
		},
	})

	result, err := SeedGlobalIntegrations(ctx, db, client, nil, dir)
	if err != nil {
		t.Fatalf("seed optional missing env: %v", err)
	}
	if result.Skipped != 1 {
		t.Fatalf("expected skipped optional integration, got %+v", result)
	}
	var count int64
	if err := db.Model(&model.InIntegration{}).Count(&count).Error; err != nil {
		t.Fatalf("count integrations: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected no seeded integrations, got %d", count)
	}
}

func TestSeedGlobalIntegrations_RealNangoRequiredMissingEnvFails(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	db := connectIntegrationTestDB(t)
	client := realNangoClient(t, ctx)
	suffix := strings.ReplaceAll(uuid.NewString(), "-", "")[:12]

	dir := t.TempDir()
	writeManifest(t, dir, map[string]any{
		"version":          1,
		"id":               "linear-required-" + suffix,
		"provider":         "linear",
		"unique_key":       "linear-required-" + suffix,
		"display_name":     "Linear",
		"required":         true,
		"allow_no_catalog": true,
		"credentials": map[string]any{
			"type":              "OAUTH2",
			"client_id_env":     "MISSING_LINEAR_CLIENT_ID_" + suffix,
			"client_secret_env": "MISSING_LINEAR_CLIENT_SECRET_" + suffix,
		},
	})

	_, err := SeedGlobalIntegrations(ctx, db, client, nil, dir)
	if err == nil || !strings.Contains(err.Error(), "requires env var") {
		t.Fatalf("expected required env error, got %v", err)
	}
}

func realNangoClient(t *testing.T, ctx context.Context) *nango.Client {
	t.Helper()
	endpoint := os.Getenv("NANGO_ENDPOINT")
	if endpoint == "" {
		endpoint = "http://localhost:13003"
	}
	secret := os.Getenv("NANGO_SECRET_KEY")
	if secret == "" {
		secret = "00000000-0000-4000-8000-000000000001"
	}
	client := nango.NewClient(endpoint, secret)
	if err := client.FetchProviders(ctx); err != nil {
		t.Fatalf("real Nango unavailable at %s: %v", endpoint, err)
	}
	if _, ok := client.GetProvider("linear"); !ok {
		t.Fatalf("real Nango at %s does not expose provider linear", endpoint)
	}
	return client
}

func connectIntegrationTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = testDBURL
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{DisableForeignKeyConstraintWhenMigrating: true})
	if err != nil {
		t.Fatalf("cannot connect to Postgres: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("db handle: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)
	schema := "integrations_test_" + strings.ReplaceAll(uuid.NewString(), "-", "_")
	if err := db.Exec(`CREATE SCHEMA "` + schema + `"`).Error; err != nil {
		t.Fatalf("create schema: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Exec(`DROP SCHEMA IF EXISTS "` + schema + `" CASCADE`).Error
		_ = sqlDB.Close()
	})
	if err := db.Exec(`SET search_path TO "` + schema + `"`).Error; err != nil {
		t.Fatalf("set search_path: %v", err)
	}
	if err := db.AutoMigrate(&model.InIntegration{}, &model.InConnection{}); err != nil {
		t.Fatalf("migration failed: %v", err)
	}
	return db
}

func writeManifest(t *testing.T, dir string, body map[string]any) {
	t.Helper()
	payload, err := json.MarshalIndent(body, "", "  ")
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, fmt.Sprint(body["id"])+".json"), payload, 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
}
