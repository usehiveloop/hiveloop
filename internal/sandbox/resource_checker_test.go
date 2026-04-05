package sandbox

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/ziraloop/ziraloop/internal/model"
)

// --- parseCgroupOutput unit tests ---

func TestParseCgroupOutput_Valid(t *testing.T) {
	output := "2147483648\n24600576\n24739840\n100000 100000\nusage_usec 1607312\nnr_throttled 0\n18\n"

	stats, err := parseCgroupOutput(output)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if stats.MemoryLimitBytes != 2147483648 {
		t.Errorf("MemoryLimitBytes: got %d, want 2147483648", stats.MemoryLimitBytes)
	}
	if stats.MemoryUsedBytes != 24600576 {
		t.Errorf("MemoryUsedBytes: got %d, want 24600576", stats.MemoryUsedBytes)
	}
	if stats.MemoryPeakBytes != 24739840 {
		t.Errorf("MemoryPeakBytes: got %d, want 24739840", stats.MemoryPeakBytes)
	}
	if stats.CPUQuota != "100000 100000" {
		t.Errorf("CPUQuota: got %q, want %q", stats.CPUQuota, "100000 100000")
	}
	if stats.CPUUsageUsec != 1607312 {
		t.Errorf("CPUUsageUsec: got %d, want 1607312", stats.CPUUsageUsec)
	}
	if stats.CPUThrottledCount != 0 {
		t.Errorf("CPUThrottledCount: got %d, want 0", stats.CPUThrottledCount)
	}
	if stats.PIDCount != 18 {
		t.Errorf("PIDCount: got %d, want 18", stats.PIDCount)
	}
}

func TestParseCgroupOutput_HighUsage(t *testing.T) {
	output := "4294967296\n3221225472\n3758096384\n200000 100000\nusage_usec 9999999999\nnr_throttled 12345\n150"

	stats, err := parseCgroupOutput(output)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if stats.MemoryLimitBytes != 4294967296 {
		t.Errorf("MemoryLimitBytes: got %d, want 4294967296", stats.MemoryLimitBytes)
	}
	if stats.MemoryUsedBytes != 3221225472 {
		t.Errorf("MemoryUsedBytes: got %d, want 3221225472", stats.MemoryUsedBytes)
	}
	if stats.MemoryPeakBytes != 3758096384 {
		t.Errorf("MemoryPeakBytes: got %d, want 3758096384", stats.MemoryPeakBytes)
	}
	if stats.CPUQuota != "200000 100000" {
		t.Errorf("CPUQuota: got %q, want %q", stats.CPUQuota, "200000 100000")
	}
	if stats.CPUUsageUsec != 9999999999 {
		t.Errorf("CPUUsageUsec: got %d, want 9999999999", stats.CPUUsageUsec)
	}
	if stats.CPUThrottledCount != 12345 {
		t.Errorf("CPUThrottledCount: got %d, want 12345", stats.CPUThrottledCount)
	}
	if stats.PIDCount != 150 {
		t.Errorf("PIDCount: got %d, want 150", stats.PIDCount)
	}
}

func TestParseCgroupOutput_UnlimitedMemory(t *testing.T) {
	output := "max\n24600576\n24739840\n100000 100000\nusage_usec 100\nnr_throttled 0\n5"

	stats, err := parseCgroupOutput(output)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if stats.MemoryLimitBytes != 0 {
		t.Errorf("MemoryLimitBytes: got %d, want 0 for unlimited", stats.MemoryLimitBytes)
	}
}

func TestParseCgroupOutput_TooFewLines(t *testing.T) {
	output := "2147483648\n24600576\n24739840"

	_, err := parseCgroupOutput(output)
	if err == nil {
		t.Fatal("expected error for too few lines")
	}
}

func TestParseCgroupOutput_InvalidMemoryValue(t *testing.T) {
	output := "not-a-number\n24600576\n24739840\n100000 100000\nusage_usec 100\nnr_throttled 0\n5"

	_, err := parseCgroupOutput(output)
	if err == nil {
		t.Fatal("expected error for invalid memory.max")
	}
}

func TestParseCgroupOutput_InvalidPIDValue(t *testing.T) {
	output := "2147483648\n24600576\n24739840\n100000 100000\nusage_usec 100\nnr_throttled 0\nabc"

	_, err := parseCgroupOutput(output)
	if err == nil {
		t.Fatal("expected error for invalid pids.current")
	}
}

// --- Integration tests (require Postgres) ---

func TestRunResourceCheck_UpdatesSandboxFields(t *testing.T) {
	orch, provider, db := setupOrchestrator(t)

	cgroupOutput := "2147483648\n734003200\n1048576000\n100000 100000\nusage_usec 5000000\nnr_throttled 42\n25"
	provider.executeCommandFn = func(_ context.Context, _ string, _ string) (string, error) {
		return cgroupOutput, nil
	}

	// Seed a running sandbox directly in DB
	sb := model.Sandbox{
		ExternalID:            "mock-rc-1",
		BridgeURL:             "https://mock.test",
		EncryptedBridgeAPIKey: []byte("enc"),
		Status:                "running",
		SandboxType:           "dedicated",
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

	called := false
	provider.executeCommandFn = func(_ context.Context, _ string, _ string) (string, error) {
		called = true
		return "", nil
	}

	sb := model.Sandbox{
		ExternalID:            "mock-rc-stopped",
		BridgeURL:             "https://mock.test",
		EncryptedBridgeAPIKey: []byte("enc"),
		Status:                "stopped",
		SandboxType:           "dedicated",
	}
	db.Create(&sb)
	t.Cleanup(func() { db.Where("id = ?", sb.ID).Delete(&model.Sandbox{}) })

	orch.RunResourceCheck(context.Background())

	if called {
		t.Error("ExecuteCommand should not be called for stopped sandboxes")
	}
}

func TestRunResourceCheck_HandlesExecuteError(t *testing.T) {
	orch, provider, db := setupOrchestrator(t)

	provider.executeCommandFn = func(_ context.Context, _ string, _ string) (string, error) {
		return "", fmt.Errorf("connection refused")
	}

	sb := model.Sandbox{
		ExternalID:            "mock-rc-err",
		BridgeURL:             "https://mock.test",
		EncryptedBridgeAPIKey: []byte("enc"),
		Status:                "running",
		SandboxType:           "dedicated",
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

	callCount := 0
	provider.executeCommandFn = func(_ context.Context, _ string, _ string) (string, error) {
		callCount++
		return "2147483648\n10000000\n10000000\n100000 100000\nusage_usec 100\nnr_throttled 0\n5", nil
	}

	var sandboxIDs []uuid.UUID
	for i := 0; i < 3; i++ {
		sb := model.Sandbox{
			ExternalID:            fmt.Sprintf("mock-rc-multi-%d-%s", i, uuid.New().String()[:8]),
			BridgeURL:             "https://mock.test",
			EncryptedBridgeAPIKey: []byte("enc"),
			Status:                "running",
			SandboxType:           "dedicated",
		}
		db.Create(&sb)
		provider.registerSandbox(sb.ExternalID, StatusRunning)
		sandboxIDs = append(sandboxIDs, sb.ID)
	}
	t.Cleanup(func() {
		for _, id := range sandboxIDs {
			db.Where("id = ?", id).Delete(&model.Sandbox{})
		}
	})

	orch.RunResourceCheck(context.Background())

	if callCount != 3 {
		t.Errorf("ExecuteCommand should be called 3 times, got %d", callCount)
	}

	for _, id := range sandboxIDs {
		var reloaded model.Sandbox
		db.Where("id = ?", id).First(&reloaded)
		if reloaded.ResourceCheckedAt == nil {
			t.Errorf("sandbox %s: ResourceCheckedAt should be set", id)
		}
	}
}
