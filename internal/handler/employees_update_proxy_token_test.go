package handler_test

import (
	"net/http"
	"testing"

	"github.com/hibiken/asynq"

	"github.com/usehiveloop/hiveloop/internal/tasks"
)

func TestIntegration_EmployeeUpdate_SchedulesImmediateProxyTokenRefreshWhenNeverRefreshed(t *testing.T) {
	h := newEmployeeHarness(t)
	m := h.createOrg(t)
	agent := h.seedEmployeeAgent(t, m)
	h.seedSandbox(t, m, agent.ID)

	rr := h.putEmployee(t, m, agent.ID, map[string]any{
		"description": "Updated description",
	}, "admin")
	if rr.Code != http.StatusOK {
		t.Fatalf("update status = %d, want 200: %s", rr.Code, rr.Body.String())
	}

	for _, task := range h.enqueuer.Tasks() {
		if task.TypeName != tasks.TypeEmployeeProxyTokenRefresh {
			continue
		}
		for _, opt := range task.Options {
			if opt.Type() == asynq.ProcessAtOpt {
				t.Fatalf("proxy token refresh should run immediately, got ProcessAt option: %v", task.Options)
			}
		}
		return
	}
	t.Fatalf("expected %s task to be enqueued", tasks.TypeEmployeeProxyTokenRefresh)
}
