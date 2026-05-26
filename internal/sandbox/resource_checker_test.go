package sandbox

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/usehivy/hivy/internal/model"
)

func TestRunResourceCheck_UpdatesSandboxFields(t *testing.T) {
	orch, provider, db := setupOrchestrator(t)

	provider.resourceUsageFn = func(_ context.Context, _ string) (*ResourceUsage, error) {
		return &ResourceUsage{
			MemoryLimitBytes:  2147483648,
			MemoryUsedBytes:   734003200,
			MemoryPeakBytes:   1048576000,
			CPUQuota:          "100000 100000",
			CPUUsageUsec:      5000000,
			CPUThrottledCount: 42,
			PIDCount:          25,
		}, nil
	}

	// Seed a running sandbox directly in DB
	sb := model.Sandbox{
		ExternalID:             "mock-rc-1",
		RuntimeURL:             "https://mock.test",
		EncryptedRuntimeSecret: []byte("enc"),
		Status:                 "running",
	}
	if err := db.Create(&sb).Error; err != nil {
		t.Fatalf("create sandbox: %v", err)
	}
	provider.registerSandbox(sb.ExternalID, StatusRunning)
	t.Cleanup(func() { db.Where("id = ?", sb.ID).Delete(&model.Sandbox{}) })

	// Run the check
	orch.RunResourceCheck(context.Background())

	// Verify DB was updated
	var reloaded model.Sandbox
	db.Where("id = ?", sb.ID).First(&reloaded)

	if reloaded.MemoryLimitBytes != 2147483648 {
		t.Errorf("MemoryLimitBytes: got %d, want 2147483648", reloaded.MemoryLimitBytes)
	}
	if reloaded.MemoryUsedBytes != 734003200 {
		t.Errorf("MemoryUsedBytes: got %d, want 734003200", reloaded.MemoryUsedBytes)
	}
	if reloaded.MemoryPeakBytes != 1048576000 {
		t.Errorf("MemoryPeakBytes: got %d, want 1048576000", reloaded.MemoryPeakBytes)
	}
	if reloaded.CPUQuota != "100000 100000" {
		t.Errorf("CPUQuota: got %q, want %q", reloaded.CPUQuota, "100000 100000")
	}
	if reloaded.CPUUsageUsec != 5000000 {
		t.Errorf("CPUUsageUsec: got %d, want 5000000", reloaded.CPUUsageUsec)
	}
	if reloaded.CPUThrottledCount != 42 {
		t.Errorf("CPUThrottledCount: got %d, want 42", reloaded.CPUThrottledCount)
	}
	if reloaded.PIDCount != 25 {
		t.Errorf("PIDCount: got %d, want 25", reloaded.PIDCount)
	}
	if reloaded.ResourceCheckedAt == nil {
		t.Fatal("ResourceCheckedAt should be set")
	}
	if time.Since(*reloaded.ResourceCheckedAt) > 10*time.Second {
		t.Error("ResourceCheckedAt should be recent")
	}
}

func TestRunResourceCheck_SkipsStoppedSandboxes(t *testing.T) {
	orch, provider, db := setupOrchestrator(t)

	targetExternalID := "mock-rc-stopped-" + uuid.New().String()[:8]
	called := false
	provider.resourceUsageFn = func(_ context.Context, externalID string) (*ResourceUsage, error) {
		if externalID == targetExternalID {
			called = true
		}
		return &ResourceUsage{}, nil
	}

	sb := model.Sandbox{
		ExternalID:             targetExternalID,
		RuntimeURL:             "https://mock.test",
		EncryptedRuntimeSecret: []byte("enc"),
		Status:                 "stopped",
	}
	db.Create(&sb)
	t.Cleanup(func() { db.Where("id = ?", sb.ID).Delete(&model.Sandbox{}) })

	orch.RunResourceCheck(context.Background())

	if called {
		t.Error("GetResourceUsage should not be called for stopped sandboxes")
	}
}

func TestRunResourceCheck_HandlesExecuteError(t *testing.T) {
	orch, provider, db := setupOrchestrator(t)

	provider.resourceUsageFn = func(_ context.Context, _ string) (*ResourceUsage, error) {
		return nil, fmt.Errorf("connection refused")
	}

	sb := model.Sandbox{
		ExternalID:             "mock-rc-err",
		RuntimeURL:             "https://mock.test",
		EncryptedRuntimeSecret: []byte("enc"),
		Status:                 "running",
	}
	db.Create(&sb)
	provider.registerSandbox(sb.ExternalID, StatusRunning)
	t.Cleanup(func() { db.Where("id = ?", sb.ID).Delete(&model.Sandbox{}) })

	// Should not panic or update DB
	orch.RunResourceCheck(context.Background())

	var reloaded model.Sandbox
	db.Where("id = ?", sb.ID).First(&reloaded)
	if reloaded.ResourceCheckedAt != nil {
		t.Error("ResourceCheckedAt should remain nil on execute error")
	}
}

func TestRunResourceCheck_MultipleSandboxes(t *testing.T) {
	orch, provider, db := setupOrchestrator(t)

	targetExternalIDs := map[string]bool{}
	callCount := 0
	provider.resourceUsageFn = func(_ context.Context, externalID string) (*ResourceUsage, error) {
		if targetExternalIDs[externalID] {
			callCount++
		}
		return &ResourceUsage{
			MemoryLimitBytes: 2147483648,
			MemoryUsedBytes:  10000000,
			MemoryPeakBytes:  10000000,
			CPUQuota:         "100000 100000",
			CPUUsageUsec:     100,
			PIDCount:         5,
		}, nil
	}

	var sandboxIDs []uuid.UUID
	for i := 0; i < 3; i++ {
		sb := model.Sandbox{
			ExternalID:             fmt.Sprintf("mock-rc-multi-%d-%s", i, uuid.New().String()[:8]),
			RuntimeURL:             "https://mock.test",
			EncryptedRuntimeSecret: []byte("enc"),
			Status:                 "running",
		}
		db.Create(&sb)
		provider.registerSandbox(sb.ExternalID, StatusRunning)
		targetExternalIDs[sb.ExternalID] = true
		sandboxIDs = append(sandboxIDs, sb.ID)
	}
	t.Cleanup(func() {
		for _, id := range sandboxIDs {
			db.Where("id = ?", id).Delete(&model.Sandbox{})
		}
	})

	orch.RunResourceCheck(context.Background())

	if callCount != 3 {
		t.Errorf("GetResourceUsage should be called 3 times, got %d", callCount)
	}

	for _, id := range sandboxIDs {
		var reloaded model.Sandbox
		db.Where("id = ?", id).First(&reloaded)
		if reloaded.ResourceCheckedAt == nil {
			t.Errorf("sandbox %s: ResourceCheckedAt should be set", id)
		}
	}
}
