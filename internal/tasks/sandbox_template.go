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
	"github.com/ziraloop/ziraloop/internal/streaming"
)

const streamKeyPrefix = "template:"

type SandboxTemplateBuildHandler struct {
	db           *gorm.DB
	orchestrator *sandbox.Orchestrator
	eventBus     *streaming.EventBus
}

func NewSandboxTemplateBuildHandler(db *gorm.DB, orchestrator *sandbox.Orchestrator, eventBus *streaming.EventBus) *SandboxTemplateBuildHandler {
	return &SandboxTemplateBuildHandler{
		db:           db,
		orchestrator: orchestrator,
		eventBus:     eventBus,
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

	streamKey := streamKeyPrefix + payload.TemplateID.String()

	h.publishStatus(ctx, streamKey, "building", "")

	h.db.Model(&tmpl).Update("build_status", "building")

	externalID, err := h.orchestrator.BuildTemplateWithLogs(ctx, &tmpl, func(line string) {
		h.publishLog(ctx, streamKey, line)
	})

	if err != nil {
		errMsg := err.Error()
		h.db.Model(&tmpl).Updates(map[string]any{
			"build_status": "failed",
			"build_error":  errMsg,
		})
		h.publishStatus(ctx, streamKey, "failed", errMsg)
		slog.Error("sandbox template build failed", "template_id", payload.TemplateID, "error", err)
		return nil
	}

	h.db.Model(&tmpl).Updates(map[string]any{
		"build_status": "ready",
		"external_id":  externalID,
		"build_error":  nil,
	})
	h.publishStatus(ctx, streamKey, "ready", "")
	slog.Info("sandbox template built", "template_id", payload.TemplateID, "external_id", externalID)

	return nil
}

func (h *SandboxTemplateBuildHandler) publishLog(ctx context.Context, streamKey, line string) {
	data, _ := json.Marshal(map[string]string{"line": line})
	_, _ = h.eventBus.Publish(ctx, streamKey, "log", data)
}

func (h *SandboxTemplateBuildHandler) publishStatus(ctx context.Context, streamKey, status, message string) {
	data, _ := json.Marshal(map[string]string{"status": status, "message": message})
	_, _ = h.eventBus.Publish(ctx, streamKey, "status", data)
}
