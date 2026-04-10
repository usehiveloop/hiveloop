package tasks

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/hibiken/asynq"
	"gorm.io/gorm"

	"github.com/ziraloop/ziraloop/internal/model"
	"github.com/ziraloop/ziraloop/internal/sandbox"
)

type SandboxTemplateBuildHandler struct {
	db           *gorm.DB
	orchestrator *sandbox.Orchestrator
}

func NewSandboxTemplateBuildHandler(db *gorm.DB, orchestrator *sandbox.Orchestrator) *SandboxTemplateBuildHandler {
	return &SandboxTemplateBuildHandler{
		db:           db,
		orchestrator: orchestrator,
	}
}

func (h *SandboxTemplateBuildHandler) Handle(ctx context.Context, t *asynq.Task) error {
	var payload SandboxTemplateBuildPayload
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		return fmt.Errorf("unmarshal payload: %w", err)
	}

	var tmpl model.SandboxTemplate
	if err := h.db.First(&tmpl, "id = ?", payload.TemplateID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			slog.Warn("sandbox template build: template not found", "template_id", payload.TemplateID)
			return nil
		}
		return fmt.Errorf("loading template: %w", err)
	}

	// Update status to building
	h.db.Model(&tmpl).Update("build_status", "building")

	onLog := func(line string) {
		// Flush immediately so logs appear in DB while building
		h.db.Model(&tmpl).Update("build_logs", gorm.Expr("build_logs || ?", "\n"+line))
	}

	// Build the template with polling
	externalID, err := h.orchestrator.BuildTemplateWithPolling(ctx, &tmpl, onLog)

	if err != nil {
		errMsg := err.Error()
		h.db.Model(&tmpl).Updates(map[string]any{
			"build_status": "failed",
			"build_error":  errMsg,
		})
		slog.Error("sandbox template build failed", "template_id", payload.TemplateID, "error", err)
		return nil
	}

	h.db.Model(&tmpl).Updates(map[string]any{
		"build_status": "ready",
		"external_id":  externalID,
		"build_error":  nil,
	})
	slog.Info("sandbox template built", "template_id", payload.TemplateID, "external_id", externalID)

	return nil
}
