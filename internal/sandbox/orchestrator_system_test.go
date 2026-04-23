package sandbox

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/model"
)

func seedSystemAgent(t *testing.T, db *gorm.DB, name, providerGroup string) model.Agent {
	t.Helper()
	agent := model.Agent{
		Name:          name,
		IsSystem:      true,
		ProviderGroup: providerGroup,
		SandboxType:   "shared",
		SystemPrompt:  "test",
		Model:         "test-model",
		Status:        "active",
	}
	if err := db.Create(&agent).Error; err != nil {
		t.Fatalf("seed system agent: %v", err)
	}
	t.Cleanup(func() { db.Where("id = ?", agent.ID).Delete(&model.Agent{}) })
	return agent
}

func seedSystemSandboxRow(t *testing.T, db *gorm.DB, externalID string, status string) model.Sandbox {
	t.Helper()
	encKey := testEncKey(t)
	apiKey, _ := generateRandomHex(32)
	encrypted, _ := encKey.EncryptString(apiKey)

	sb := model.Sandbox{
		SandboxType:           "system",
		ExternalID:            externalID,
		BridgeURL:             "https://mock:25434",
		EncryptedBridgeAPIKey: encrypted,
		Status:                status,
	}
	if err := db.Create(&sb).Error; err != nil {
		t.Fatalf("seed system sandbox: %v", err)
	}
	t.Cleanup(func() { db.Where("id = ?", sb.ID).Delete(&model.Sandbox{}) })
	return sb
}

func TestEnsureSystemSandbox_CreatesWhenMissing(t *testing.T) {
	orch, provider, db := setupOrchestrator(t)
	db.Where("sandbox_type = ?", "system").Delete(&model.Sandbox{})

	ctx := context.Background()
	sb, err := orch.EnsureSystemSandbox(ctx)
	if err != nil {
		t.Fatalf("EnsureSystemSandbox: %v", err)
	}
	t.Cleanup(func() { db.Where("id = ?", sb.ID).Delete(&model.Sandbox{}) })

	if sb.SandboxType != "system" {
		t.Errorf("sandbox type: got %q, want system", sb.SandboxType)
	}
	if sb.OrgID != nil {
		t.Error("system sandbox should have nil OrgID")
	}
	if sb.Status != "running" {
		t.Errorf("status: got %q, want running", sb.Status)
	}
	if sb.ExternalID == "" {
		t.Error("external_id should be populated from provider")
	}
	if provider.count() != 1 {
		t.Fatalf("provider should have 1 sandbox, got %d", provider.count())
	}

	var persisted model.Sandbox
	if err := db.Where("sandbox_type = ?", "system").First(&persisted).Error; err != nil {
		t.Fatalf("system sandbox should be persisted: %v", err)
	}
	if persisted.ID != sb.ID {
		t.Errorf("persisted ID mismatch: got %s, want %s", persisted.ID, sb.ID)
	}
}

func TestEnsureSystemSandbox_DisablesAutoStop(t *testing.T) {
	orch, provider, db := setupOrchestrator(t)
	db.Where("sandbox_type = ?", "system").Delete(&model.Sandbox{})

	ctx := context.Background()
	sb, err := orch.EnsureSystemSandbox(ctx)
	if err != nil {
		t.Fatalf("EnsureSystemSandbox: %v", err)
	}
	t.Cleanup(func() { db.Where("id = ?", sb.ID).Delete(&model.Sandbox{}) })

	provider.mu.Lock()
	calls := append([]autoPolicyCall(nil), provider.setAutoStopCalls...)
	provider.mu.Unlock()

	if len(calls) != 1 {
		t.Fatalf("SetAutoStop calls: got %d, want 1", len(calls))
	}
	if calls[0].externalID != sb.ExternalID {
		t.Errorf("SetAutoStop externalID: got %q, want %q", calls[0].externalID, sb.ExternalID)
	}
	if calls[0].intervalMinutes != 0 {
		t.Errorf("SetAutoStop intervalMinutes: got %d, want 0 (disabled)", calls[0].intervalMinutes)
	}
}

func TestEnsureSystemSandbox_ReturnsExistingRunning(t *testing.T) {
	orch, provider, db := setupOrchestrator(t)
	db.Where("sandbox_type = ?", "system").Delete(&model.Sandbox{})

	existing := seedSystemSandboxRow(t, db, "mock-existing-system", "running")
	provider.registerSandbox(existing.ExternalID, StatusRunning)

	ctx := context.Background()
	sb, err := orch.EnsureSystemSandbox(ctx)
	if err != nil {
		t.Fatalf("EnsureSystemSandbox: %v", err)
	}

	if sb.ID != existing.ID {
		t.Errorf("returned ID: got %s, want %s (existing)", sb.ID, existing.ID)
	}
	if provider.count() != 1 {
		t.Errorf("provider should still have 1 sandbox (no new create), got %d", provider.count())
	}
}

func TestEnsureSystemSandbox_RecreatesIfProviderLost(t *testing.T) {
	orch, provider, db := setupOrchestrator(t)
	db.Where("sandbox_type = ?", "system").Delete(&model.Sandbox{})

	stale := seedSystemSandboxRow(t, db, "mock-stale-system", "running")

	ctx := context.Background()
	sb, err := orch.EnsureSystemSandbox(ctx)
	if err != nil {
		t.Fatalf("EnsureSystemSandbox: %v", err)
	}
	t.Cleanup(func() { db.Where("id = ?", sb.ID).Delete(&model.Sandbox{}) })

	if sb.ID == stale.ID {
		t.Errorf("expected new sandbox ID after recreate, got the stale one (%s)", sb.ID)
	}
	var count int64
	db.Model(&model.Sandbox{}).Where("id = ?", stale.ID).Count(&count)
	if count != 0 {
		t.Errorf("stale sandbox row should be deleted, found %d rows", count)
	}
	if provider.count() != 1 {
		t.Errorf("provider should have 1 sandbox (the new one), got %d", provider.count())
	}
}

func TestEnsureSystemSandbox_WakesStopped(t *testing.T) {
	orch, provider, db := setupOrchestrator(t)
	db.Where("sandbox_type = ?", "system").Delete(&model.Sandbox{})

	existing := seedSystemSandboxRow(t, db, "mock-stopped-system", "stopped")
	provider.registerSandbox(existing.ExternalID, StatusStopped)

	ctx := context.Background()
	sb, err := orch.EnsureSystemSandbox(ctx)
	if err != nil {
		t.Fatalf("EnsureSystemSandbox: %v", err)
	}

	if sb.ID != existing.ID {
		t.Errorf("returned ID: got %s, want %s", sb.ID, existing.ID)
	}
	if sb.Status != "running" {
		t.Errorf("status after wake: got %q, want running", sb.Status)
	}
	if provider.getStatus(existing.ExternalID) != StatusRunning {
		t.Error("mock provider should report sandbox as running after wake")
	}
}

func TestEnsureSystemSandbox_BindsAllSystemAgents(t *testing.T) {
	orch, _, db := setupOrchestrator(t)
	db.Where("sandbox_type = ?", "system").Delete(&model.Sandbox{})

	a1 := seedSystemAgent(t, db, "test-system-agent-1-"+uuid.New().String()[:8], "anthropic")
	a2 := seedSystemAgent(t, db, "test-system-agent-2-"+uuid.New().String()[:8], "openai")
	a3 := seedSystemAgent(t, db, "test-system-agent-3-"+uuid.New().String()[:8], "gemini")

	ctx := context.Background()
	sb, err := orch.EnsureSystemSandbox(ctx)
	if err != nil {
		t.Fatalf("EnsureSystemSandbox: %v", err)
	}
	t.Cleanup(func() { db.Where("id = ?", sb.ID).Delete(&model.Sandbox{}) })

	for _, want := range []model.Agent{a1, a2, a3} {
		var reloaded model.Agent
		if err := db.Where("id = ?", want.ID).First(&reloaded).Error; err != nil {
			t.Fatalf("reload agent %s: %v", want.Name, err)
		}
		if reloaded.SandboxID == nil {
			t.Errorf("agent %s: sandbox_id should be set", want.Name)
			continue
		}
		if *reloaded.SandboxID != sb.ID {
			t.Errorf("agent %s: sandbox_id %s, want %s", want.Name, *reloaded.SandboxID, sb.ID)
		}
	}
}

func TestHealthCheck_SystemSandboxNeverStopped(t *testing.T) {
	orch, provider, db := setupOrchestrator(t)
	orch.cfg.PoolSandboxIdleTimeoutMins = 1

	sb := seedSystemSandboxRow(t, db, "mock-system-idle", "running")
	provider.registerSandbox(sb.ExternalID, StatusRunning)

	old := time.Now().Add(-2 * time.Hour)
	db.Model(&sb).Update("last_active_at", old)
	sb.LastActiveAt = &old

	ctx := context.Background()
	orch.checkSandboxHealth(ctx, &sb)

	var reloaded model.Sandbox
	db.Where("id = ?", sb.ID).First(&reloaded)
	if reloaded.Status != "running" {
		t.Errorf("system sandbox should never auto-stop, got status %q", reloaded.Status)
	}
	if provider.getStatus(sb.ExternalID) != StatusRunning {
		t.Error("provider should still report system sandbox as running")
	}
}

func TestHealthCheck_SystemSandboxStoppedWakes(t *testing.T) {
	orch, provider, db := setupOrchestrator(t)

	sb := seedSystemSandboxRow(t, db, "mock-system-stopped", "stopped")
	provider.registerSandbox(sb.ExternalID, StatusStopped)

	ctx := context.Background()
	orch.checkSandboxHealth(ctx, &sb)

	if provider.getStatus(sb.ExternalID) != StatusRunning {
		t.Errorf("system sandbox should be woken by health check, provider status %v",
			provider.getStatus(sb.ExternalID))
	}
}
