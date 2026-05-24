package sandbox

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
)

func (o *Orchestrator) waitForEmployeeRuntimeLive(ctx context.Context, sb *model.Sandbox) error {
	healthURL := strings.TrimRight(sb.BridgeURL, "/") + "/healthz"
	deadline := time.Now().Add(employeeHealthTimeout)
	client := &http.Client{Timeout: 5 * time.Second}
	attempt := 0

	logging.FromContext(ctx).InfoContext(ctx, "waiting for employee runtime", "sandbox_id", sb.ID)
	for time.Now().Before(deadline) {
		attempt++
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, healthURL, nil)
		if err != nil {
			return fmt.Errorf("build request: %w", err)
		}
		resp, doErr := client.Do(req)
		if doErr != nil {
			logging.FromContext(ctx).DebugContext(ctx, "employee runtime probe transport error",
				"sandbox_id", sb.ID, "attempt", attempt, "error", doErr)
		} else {
			status := resp.StatusCode
			resp.Body.Close()
			if status == http.StatusOK {
				logging.FromContext(ctx).InfoContext(ctx, "employee runtime live",
					"sandbox_id", sb.ID, "attempts", attempt)
				return nil
			}
			logging.FromContext(ctx).DebugContext(ctx, "employee runtime probe non-200",
				"sandbox_id", sb.ID, "attempt", attempt, "status", status)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(employeeHealthInterval):
		}
	}
	return fmt.Errorf("employee runtime not live within %s (%d attempts)", employeeHealthTimeout, attempt)
}
