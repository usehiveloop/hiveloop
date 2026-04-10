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
					h.db.Model(&tmpl).Update("build_logs", gorm.Expr("build_logs || ?", "\n"+newLogs))
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
		h.db.Model(&tmpl).Updates(updates)
	}

	// Build the template with polling
	externalID, err := h.orchestrator.BuildTemplateWithPolling(ctx, &tmpl, onLog, onStatus)

	// Signal flusher to stop and do final flush
	close(done)
	logMu.Lock()
	if len(bufferedLogs) > 0 {
		newLogs := strings.Join(bufferedLogs, "\n")
		h.db.Model(&tmpl).Update("build_logs", gorm.Expr("build_logs || ?", "\n"+newLogs))
	}
	logMu.Unlock()

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
