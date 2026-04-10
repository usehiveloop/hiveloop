package tasks

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

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

	return h.buildTemplate(ctx, &tmpl)
}

func (h *SandboxTemplateBuildHandler) HandleRetry(ctx context.Context, t *asynq.Task) error {
	var payload SandboxTemplateRetryBuildPayload
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		return fmt.Errorf("unmarshal retry payload: %w", err)
	}

	var tmpl model.SandboxTemplate
	if err := h.db.First(&tmpl, "id = ?", payload.TemplateID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			slog.Warn("sandbox template retry: template not found", "template_id", payload.TemplateID)
			return nil
		}
		return fmt.Errorf("loading template: %w", err)
	}

	// Delete existing snapshot if present
	if tmpl.ExternalID != nil && *tmpl.ExternalID != "" {
		slog.Info("retry: deleting existing snapshot", "external_id", *tmpl.ExternalID)
		if err := h.orchestrator.DeleteTemplate(ctx, *tmpl.ExternalID); err != nil {
			slog.Warn("retry: failed to delete existing snapshot", "external_id", *tmpl.ExternalID, "error", err)
		}
	}

	// Update commands if provided
	if len(payload.BuildCommands) > 0 {
		h.db.Model(&tmpl).Update("build_commands", strings.Join(payload.BuildCommands, "\n"))
		tmpl.BuildCommands = strings.Join(payload.BuildCommands, "\n")
	}

	// Reset template status
	h.db.Model(&tmpl).Updates(map[string]any{
		"build_status": "building",
		"external_id":  nil,
		"build_error":  nil,
		"build_logs":   "",
	})
	tmpl.BuildStatus = "building"
	tmpl.ExternalID = nil
	tmpl.BuildError = nil
	tmpl.BuildLogs = ""

	return h.buildTemplate(ctx, &tmpl)
}

func (h *SandboxTemplateBuildHandler) buildTemplate(ctx context.Context, tmpl *model.SandboxTemplate) error {
	// Buffered log channel
	logChan := make(chan string, 100)
	var logMu sync.Mutex
	var bufferedLogs []string

	// Goroutine to flush logs every 3 seconds
	done := make(chan struct{})
	go func() {
		ticker := time.NewTicker(3 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				logMu.Lock()
				if len(bufferedLogs) > 0 {
					newLogs := strings.Join(bufferedLogs, "\n")
					h.db.Model(tmpl).Update("build_logs", gorm.Expr("build_logs || ?", "\n"+newLogs))
					bufferedLogs = nil
				}
				logMu.Unlock()
			case line, ok := <-logChan:
				if !ok {
					return
				}
				logMu.Lock()
				bufferedLogs = append(bufferedLogs, line)
				logMu.Unlock()
			}
		}
	}()

	onLog := func(line string) {
		select {
		case logChan <- line:
		default:
			// Channel full, skip log
		}
	}

	onStatus := func(status, message string) {
		updates := map[string]any{
			"build_status": status,
		}
		if status == "failed" {
			updates["build_error"] = message
		}
		h.db.Model(tmpl).Updates(updates)
	}

	// Build the template with polling
	externalID, err := h.orchestrator.BuildTemplateWithPolling(ctx, tmpl, onLog, onStatus)

	// Signal flusher to stop and do final flush
	close(done)
	logMu.Lock()
	if len(bufferedLogs) > 0 {
		newLogs := strings.Join(bufferedLogs, "\n")
		h.db.Model(tmpl).Update("build_logs", gorm.Expr("build_logs || ?", "\n"+newLogs))
	}
	logMu.Unlock()

	if err != nil {
		errMsg := err.Error()
		h.db.Model(tmpl).Updates(map[string]any{
			"build_status": "failed",
			"build_error":  errMsg,
		})
		slog.Error("sandbox template build failed", "template_id", tmpl.ID, "error", err)
		return nil
	}

	h.db.Model(tmpl).Updates(map[string]any{
		"build_status": "ready",
		"external_id":  externalID,
		"build_error":  nil,
	})
	slog.Info("sandbox template built", "template_id", tmpl.ID, "external_id", externalID)

	return nil
}
