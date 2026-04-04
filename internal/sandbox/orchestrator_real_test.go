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

	"github.com/ziraloop/ziraloop/internal/config"
	"github.com/ziraloop/ziraloop/internal/crypto"
	"github.com/ziraloop/ziraloop/internal/model"
	"github.com/ziraloop/ziraloop/internal/sandbox/daytona"
	"github.com/ziraloop/ziraloop/internal/turso"
)

// TestRealDaytona_PoolSandboxLifecycle tests the pool sandbox orchestrator flow against real Daytona + Turso.
//
// Run with:
//
//	source .env && go test ./internal/sandbox/ -v -tags=integration -run TestRealDaytona -timeout=10m
func TestRealDaytona_PoolSandboxLifecycle(t *testing.T) {
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

	// Create test org + identity + credential + agent
	org := model.Org{Name: "real-daytona-" + suffix}
	db.Create(&org)
	t.Cleanup(func() { db.Where("id = ?", org.ID).Delete(&model.Org{}) })

	identity := model.Identity{OrgID: org.ID, ExternalID: "real-test-" + suffix}
	db.Create(&identity)
	t.Cleanup(func() { db.Where("id = ?", identity.ID).Delete(&model.Identity{}) })

	cred := createTestCred(t, db, org.ID)
	agent := createTestAgent(t, db, org.ID, identity.ID, cred.ID, "shared")

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
		BridgeBaseImagePrefix:           "ziraloop-bridge-0-10-0",
		BridgeHost:                      os.Getenv("BRIDGE_HOST"),
		SharedSandboxIdleTimeoutMins:    30,
		DedicatedSandboxGracePeriodMins: 5,
		PoolSandboxResourceThreshold:    80.0,
		PoolSandboxIdleTimeoutMins:      30,
	}

	orch := NewOrchestrator(db, provider, tursoProvisioner, encKey, cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// --- Test 1: Assign pool sandbox ---
	t.Log("Assigning pool sandbox...")
	sb, err := orch.AssignPoolSandbox(ctx, &agent)
	if err != nil {
		t.Fatalf("AssignPoolSandbox: %v", err)
	}
	t.Logf("Sandbox created: id=%s external_id=%s status=%s", sb.ID, sb.ExternalID, sb.Status)
	t.Logf("Bridge URL: %s", sb.BridgeURL)

	t.Cleanup(func() {
		t.Log("Cleaning up sandbox...")
		orch.DeleteSandbox(context.Background(), sb)
		db.Where("org_id = ?", org.ID).Delete(&model.WorkspaceStorage{})
		tursoClient.DeleteDatabase(context.Background(), "zira-"+shortID(org.ID))
	})

	if sb.Status != "running" {
		t.Fatalf("expected running, got %s", sb.Status)
	}
	if sb.BridgeURL == "" {
		t.Fatal("bridge_url should be set")
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

	// --- Test 3: Second agent reuses same pool sandbox ---
	agent2 := createTestAgent(t, db, org.ID, identity.ID, cred.ID, "shared")
	t.Log("Assigning second agent to pool...")
	sb2, err := orch.AssignPoolSandbox(ctx, &agent2)
	if err != nil {
		t.Fatalf("second AssignPoolSandbox: %v", err)
	}
	if sb2.ID != sb.ID {
		t.Fatalf("expected same sandbox: got %s and %s", sb.ID, sb2.ID)
	}
	t.Log("Second agent reused same pool sandbox (correct)")

	// --- Test 4: GetBridgeClient ---
	t.Log("Getting Bridge client...")
	client, err := orch.GetBridgeClient(ctx, sb)
	if err != nil {
		t.Fatalf("GetBridgeClient: %v", err)
	}
	if err := client.HealthCheck(ctx); err != nil {
		t.Fatalf("Bridge client health check: %v", err)
	}
	t.Log("Bridge client works!")

	// --- Test 5: Release and verify agent count ---
	t.Log("Releasing second agent...")
	if err := orch.ReleasePoolSandbox(ctx, &agent2); err != nil {
		t.Fatalf("ReleasePoolSandbox: %v", err)
	}
	var agentCount int64
	db.Model(&model.Agent{}).Where("sandbox_id = ?", sb.ID).Count(&agentCount)
	if agentCount != 1 {
		t.Fatalf("expected agent count 1, got %d", agentCount)
	}
	t.Log("Agent released, count = 1")

	// --- Test 6: Stop and wake via re-assign ---
	t.Log("Stopping sandbox...")
	if err := orch.StopSandbox(ctx, sb); err != nil {
		t.Fatalf("StopSandbox: %v", err)
	}

	t.Log("Re-assigning (should wake stopped sandbox)...")
	woken, err := orch.AssignPoolSandbox(ctx, &agent)
	if err != nil {
		t.Fatalf("re-assign after stop: %v", err)
	}
	if woken.Status != "running" {
		t.Fatalf("expected running after wake, got %s", woken.Status)
	}
	t.Log("Sandbox woken successfully")

	// Verify Bridge is healthy again
	client2, err := orch.GetBridgeClient(ctx, woken)
	if err != nil {
		t.Fatalf("GetBridgeClient after wake: %v", err)
	}

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
