//go:build integration

package sandbox

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/llmvault/llmvault/internal/config"
	"github.com/llmvault/llmvault/internal/crypto"
	"github.com/llmvault/llmvault/internal/model"
	"github.com/llmvault/llmvault/internal/sandbox/daytona"
	"github.com/llmvault/llmvault/internal/turso"
)

// TestRealDaytona_SharedSandboxLifecycle tests the full orchestrator flow against real Daytona + Turso.
//
// Run with:
//
//	source .env && go test ./internal/sandbox/ -v -tags=integration -run TestRealDaytona -timeout=10m
func TestRealDaytona_SharedSandboxLifecycle(t *testing.T) {
	providerKey := os.Getenv("SANDBOX_PROVIDER_KEY")
	providerURL := os.Getenv("SANDBOX_PROVIDER_URL")
	encKeyB64 := os.Getenv("SANDBOX_ENCRYPTION_KEY")
	tursoToken := os.Getenv("TURSO_API_TOKEN")
	tursoOrg := os.Getenv("TURSO_ORG_SLUG")

	if providerKey == "" || encKeyB64 == "" || tursoToken == "" {
		t.Skip("skipping: SANDBOX_PROVIDER_KEY, SANDBOX_ENCRYPTION_KEY, TURSO_API_TOKEN required")
	}

	db := setupTestDB(t)
	suffix := uuid.New().String()[:8]

	// Create test org + identity
	org := model.Org{Name: "real-daytona-" + suffix}
	db.Create(&org)
	t.Cleanup(func() { db.Where("id = ?", org.ID).Delete(&model.Org{}) })

	identity := model.Identity{OrgID: org.ID, ExternalID: "real-test-" + suffix}
	db.Create(&identity)
	t.Cleanup(func() { db.Where("id = ?", identity.ID).Delete(&model.Identity{}) })

	// Build real dependencies
	encKey, err := crypto.NewSymmetricKey(encKeyB64)
	if err != nil {
		t.Fatalf("enc key: %v", err)
	}

	provider, err := daytona.NewDriver(daytona.Config{
		APIURL: providerURL,
		APIKey: providerKey,
		Target: os.Getenv("SANDBOX_TARGET"),
	})
	if err != nil {
		t.Fatalf("daytona driver: %v", err)
	}

	tursoClient := turso.NewClient(tursoToken, tursoOrg)
	tursoGroup := os.Getenv("TURSO_GROUP")
	if tursoGroup == "" {
		tursoGroup = "default"
	}
	tursoProvisioner := turso.NewProvisioner(tursoClient, tursoGroup, db)

	cfg := &config.Config{
		BridgeBaseImagePrefix:           "llmvault-bridge-0-10-0",
		BridgeHost:                      os.Getenv("BRIDGE_HOST"),
		SharedSandboxIdleTimeoutMins:    30,
		DedicatedSandboxGracePeriodMins: 5,
	}

	orch := NewOrchestrator(db, provider, tursoProvisioner, encKey, cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// --- Test 1: Create shared sandbox ---
	t.Log("Creating shared sandbox...")
	sb, err := orch.EnsureSharedSandbox(ctx, &org, &identity)
	if err != nil {
		t.Fatalf("EnsureSharedSandbox: %v", err)
	}
	t.Logf("Sandbox created: id=%s external_id=%s status=%s", sb.ID, sb.ExternalID, sb.Status)
	t.Logf("Bridge URL: %s", sb.BridgeURL)

	t.Cleanup(func() {
		t.Log("Cleaning up sandbox...")
		orch.DeleteSandbox(context.Background(), sb)
		db.Where("org_id = ?", org.ID).Delete(&model.WorkspaceStorage{})
		// Clean up Turso database
		tursoClient.DeleteDatabase(context.Background(), "llmv-"+shortID(org.ID))
	})

	if sb.Status != "running" {
		t.Fatalf("expected running, got %s", sb.Status)
	}
	if sb.BridgeURL == "" {
		t.Fatal("bridge_url should be set")
	}
	if sb.ExternalID == "" {
		t.Fatal("external_id should be set")
	}

	// --- Test 2: Verify Bridge is healthy ---
	t.Log("Checking Bridge health...")
	healthURL := sb.BridgeURL + "/health"
	resp, err := http.Get(healthURL)
	if err != nil {
		t.Fatalf("Bridge health check failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Bridge health: expected 200, got %d", resp.StatusCode)
	}
	t.Log("Bridge is healthy!")

	// --- Test 3: Verify Turso storage was created ---
	var ws model.WorkspaceStorage
	if err := db.Where("org_id = ?", org.ID).First(&ws).Error; err != nil {
		t.Fatalf("workspace storage not found: %v", err)
	}
	t.Logf("Turso DB: %s (%s)", ws.TursoDatabaseName, ws.StorageURL)

	// --- Test 4: Return existing on second call ---
	t.Log("Calling EnsureSharedSandbox again (should return existing)...")
	sb2, err := orch.EnsureSharedSandbox(ctx, &org, &identity)
	if err != nil {
		t.Fatalf("second EnsureSharedSandbox: %v", err)
	}
	if sb2.ID != sb.ID {
		t.Fatalf("expected same sandbox: got %s and %s", sb.ID, sb2.ID)
	}
	t.Log("Returned existing sandbox (correct)")

	// --- Test 5: GetBridgeClient ---
	t.Log("Getting Bridge client...")
	client, err := orch.GetBridgeClient(ctx, sb)
	if err != nil {
		t.Fatalf("GetBridgeClient: %v", err)
	}
	if err := client.HealthCheck(ctx); err != nil {
		t.Fatalf("Bridge client health check: %v", err)
	}
	t.Log("Bridge client works!")

	// --- Test 6: Stop and wake ---
	t.Log("Stopping sandbox...")
	if err := orch.StopSandbox(ctx, sb); err != nil {
		t.Fatalf("StopSandbox: %v", err)
	}

	var stopped model.Sandbox
	db.Where("id = ?", sb.ID).First(&stopped)
	if stopped.Status != "stopped" {
		t.Fatalf("expected stopped, got %s", stopped.Status)
	}
	t.Log("Sandbox stopped")

	t.Log("Waking sandbox via EnsureSharedSandbox...")
	woken, err := orch.EnsureSharedSandbox(ctx, &org, &identity)
	if err != nil {
		t.Fatalf("wake: %v", err)
	}
	if woken.Status != "running" {
		t.Fatalf("expected running after wake, got %s", woken.Status)
	}
	t.Log("Sandbox woken successfully")

	// Verify Bridge is healthy again after wake
	client2, err := orch.GetBridgeClient(ctx, woken)
	if err != nil {
		t.Fatalf("GetBridgeClient after wake: %v", err)
	}

	// Bridge may need a moment to start after wake
	var healthErr error
	for i := 0; i < 10; i++ {
		healthErr = client2.HealthCheck(ctx)
		if healthErr == nil {
			break
		}
		t.Logf("Bridge not ready yet, retrying in 3s... (%v)", healthErr)
		time.Sleep(3 * time.Second)
	}
	if healthErr != nil {
		t.Fatalf("Bridge not healthy after wake: %v", healthErr)
	}
	t.Log("Bridge healthy after wake!")

	fmt.Println("\n========================================")
	fmt.Println("  ALL REAL INTEGRATION TESTS PASSED")
	fmt.Println("========================================")
}
