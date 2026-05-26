package sandbox

import (
	"context"
	"strings"
	"testing"

	"github.com/usehivy/hivy/internal/model"
)

func TestSandboxOperationRejectsDifferentProvider(t *testing.T) {
	orch, provider, db := setupOrchestrator(t)

	sb := model.Sandbox{
		ProviderID:             "other-provider",
		ExternalID:             "foreign-sandbox",
		RuntimeURL:             "https://foreign.test",
		EncryptedRuntimeSecret: []byte("enc"),
		Status:                 string(StatusRunning),
	}
	if err := db.Create(&sb).Error; err != nil {
		t.Fatalf("create sandbox: %v", err)
	}
	t.Cleanup(func() { db.Where("id = ?", sb.ID).Delete(&model.Sandbox{}) })

	err := orch.StopSandbox(context.Background(), &sb)
	if err == nil || !strings.Contains(err.Error(), "active provider") {
		t.Fatalf("StopSandbox error = %v, want provider mismatch", err)
	}
	if len(provider.stoppedIDs) != 0 {
		t.Fatalf("provider StopSandbox should not be called, got %v", provider.stoppedIDs)
	}
}

func TestTemplateBuildRejectsDifferentProvider(t *testing.T) {
	orch, _, db := setupOrchestrator(t)

	tmpl := model.SandboxTemplate{
		ProviderID:    "other-provider",
		Name:          "foreign-template",
		Slug:          "foreign-template",
		Size:          "small",
		BuildStatus:   "pending",
		BuildCommands: "echo hi",
		Tags:          model.JSON{},
		Config:        model.JSON{},
	}
	if err := db.Create(&tmpl).Error; err != nil {
		t.Fatalf("create template: %v", err)
	}
	t.Cleanup(func() { db.Where("id = ?", tmpl.ID).Delete(&model.SandboxTemplate{}) })

	_, err := orch.BuildTemplateWithLogs(context.Background(), &tmpl, nil)
	if err == nil || !strings.Contains(err.Error(), "active provider") {
		t.Fatalf("BuildTemplateWithLogs error = %v, want provider mismatch", err)
	}
}
