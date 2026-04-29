package sandbox

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/usehiveloop/hiveloop/internal/model"
)

func (o *Orchestrator) resolveBuildOpts(tmpl *model.SandboxTemplate, snapshotName string) BuildSnapshotOpts {
	cmds := []string{}
	if tmpl.BuildCommands != "" {
		cmds = strings.Split(tmpl.BuildCommands, "\n")
	}

	opts := BuildSnapshotOpts{
		Name:          snapshotName,
		BuildCommands: cmds,
	}

	if tmpl.BaseTemplateID != nil {
		var baseTmpl model.SandboxTemplate
		if err := o.db.First(&baseTmpl, "id = ?", *tmpl.BaseTemplateID).Error; err == nil {
			switch {
			case baseTmpl.BaseImageRef != nil && *baseTmpl.BaseImageRef != "":
				opts.BaseImage = *baseTmpl.BaseImageRef
			case baseTmpl.ExternalID != nil:
				opts.BaseImage = *baseTmpl.ExternalID
			}
		}
	}

	if sz, ok := model.TemplateSizes[tmpl.Size]; ok {
		opts.CPU = sz.CPU
		opts.Memory = sz.Memory
		opts.Disk = sz.Disk
	}

	return opts
}

func (o *Orchestrator) BuildTemplate(ctx context.Context, tmpl *model.SandboxTemplate) {
	o.db.Model(tmpl).Update("build_status", "building")

	opts := o.resolveBuildOpts(tmpl, tmpl.Slug)
	externalID, err := o.provider.BuildSnapshot(ctx, opts)

	if err != nil {
		errMsg := err.Error()
		o.db.Model(tmpl).Updates(map[string]any{
			"build_status": "failed",
			"build_error":  errMsg,
		})
		slog.Error("template build failed", "template_id", tmpl.ID, "error", err)
		return
	}

	o.db.Model(tmpl).Updates(map[string]any{
		"build_status": "ready",
		"external_id":  externalID,
		"build_error":  nil,
	})
	slog.Info("template built", "template_id", tmpl.ID, "external_id", externalID)
}

func (o *Orchestrator) BuildTemplateWithLogs(ctx context.Context, tmpl *model.SandboxTemplate, onLog func(string)) (string, error) {
	opts := o.resolveBuildOpts(tmpl, tmpl.Slug)
	return o.provider.BuildSnapshotWithLogs(ctx, opts, onLog)
}

func (o *Orchestrator) BuildTemplateWithPolling(ctx context.Context, tmpl *model.SandboxTemplate, onLog func(string), onStatus func(status, message string)) (externalID string, buildErr error) {
	opts := o.resolveBuildOpts(tmpl, tmpl.Slug)

	externalID, err := o.provider.BuildSnapshotWithLogs(ctx, opts, onLog)
	if err != nil {
		return "", fmt.Errorf("starting snapshot build: %w", err)
	}

	const pollInterval = 5 * time.Second
	const maxWait = 15 * time.Minute
	deadline := time.Now().Add(maxWait)

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return externalID, ctx.Err()
		case <-time.After(pollInterval):
		}

		status, err := o.provider.GetSnapshotStatus(ctx, externalID)
		if err != nil {
			slog.Warn("failed to get snapshot status, retrying", "external_id", externalID, "error", err)
			continue
		}

		switch status.State {
		case "active", "ready":
			slog.Info("snapshot build completed", "external_id", externalID, "state", status.State)
			if onStatus != nil {
				onStatus("ready", "")
			}
			return externalID, nil
		case "error":
			errMsg := status.ErrorMsg
			if errMsg == "" {
				errMsg = "snapshot build failed with unknown error"
			}
			if logs, logErr := o.provider.GetSnapshotLogs(ctx, externalID); logErr == nil && logs != "" {
				if onLog != nil {
					onLog(logs)
				}
			}
			if status.ErrorReason != "" {
				errorLog := fmt.Sprintf("\n[ERROR REASON]: %s", status.ErrorReason)
				if onLog != nil {
					onLog(errorLog)
				}
				errMsg = fmt.Sprintf("%s\n%s", errMsg, status.ErrorReason)
			}
			slog.Error("snapshot build failed", "external_id", externalID, "error", errMsg)
			if onStatus != nil {
				onStatus("failed", errMsg)
			}
			return externalID, fmt.Errorf("%s", errMsg)
		case "building", "pending", "":
			slog.Debug("snapshot still building", "external_id", externalID, "state", status.State)
		default:
			slog.Warn("unknown snapshot state", "external_id", externalID, "state", status.State)
		}
	}

	return externalID, fmt.Errorf("snapshot build timed out after %s", maxWait)
}

func (o *Orchestrator) DeleteTemplate(ctx context.Context, externalID string) error {
	return o.provider.DeleteSnapshot(ctx, externalID)
}

func (o *Orchestrator) RetryTemplateBuild(ctx context.Context, tmpl *model.SandboxTemplate, newCommands []string, onLog func(string), onStatus func(status, message string)) (externalID string, buildErr error) {
	if tmpl.ExternalID != nil && *tmpl.ExternalID != "" {
		slog.Info("deleting existing snapshot before retry", "external_id", *tmpl.ExternalID)
		if err := o.provider.DeleteSnapshot(ctx, *tmpl.ExternalID); err != nil {
			slog.Warn("failed to delete existing snapshot", "external_id", *tmpl.ExternalID, "error", err)
		}
	}

	if len(newCommands) > 0 {
		tmpl.BuildCommands = strings.Join(newCommands, "\n")
	}

	tmpl.ExternalID = nil
	tmpl.BuildStatus = "building"
	tmpl.BuildError = nil
	tmpl.BuildLogs = ""

	return o.BuildTemplateWithPolling(ctx, tmpl, onLog, onStatus)
}
