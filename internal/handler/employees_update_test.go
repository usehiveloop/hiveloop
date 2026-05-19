package handler_test

import (
	"net/http"
	"testing"
)

func TestIntegration_EmployeeUpdate_RouteRemoved(t *testing.T) {
	h := newEmployeeHarness(t)
	m := h.createOrg(t)
	agent := h.seedEmployeeAgent(t, m)

	rr := h.putEmployee(t, m, agent.ID, map[string]any{
		"name": "Updated employee",
	}, "admin")
	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405 after employee update route removal: %s", rr.Code, rr.Body.String())
	}
}
