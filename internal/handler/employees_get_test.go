package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/usehivy/hivy/internal/auth"
	"github.com/usehivy/hivy/internal/middleware"
)

func (h *employeeHarness) getEmployee(t *testing.T, m orgWithMember, agentID string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("GET", "/v1/employees/"+agentID, nil)
	req.Header.Set("X-Org-ID", m.org.ID.String())
	req = middleware.WithAuthClaims(req, &auth.AuthClaims{
		UserID: m.user.ID.String(),
		OrgID:  m.org.ID.String(),
		Role:   "admin",
	})
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	return rr
}

func TestIntegration_EmployeesGet_HappyPath_LoadsSpecialistsAndSandbox(t *testing.T) {
	h := newEmployeeHarness(t)
	h.platformCredCleanup(t)
	m := h.createOrg(t)
	emp := h.seedEmployeeAgent(t, m)
	h.seedSandbox(t, m, emp.ID)

	rr := h.getEmployee(t, m, emp.ID.String())
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rr.Code, rr.Body.String())
	}

	var item map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &item); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if item["id"] != emp.ID.String() {
		t.Fatalf("id = %v, want %s", item["id"], emp.ID)
	}
	if _, exposed := item["profiles"]; exposed {
		t.Fatalf("employee response exposed removed profiles field: %#v", item["profiles"])
	}
	if _, ok := item["sandbox"].(map[string]any); !ok {
		t.Fatalf("sandbox missing: %#v", item["sandbox"])
	}
	specialists := item["specialists"].([]any)
	if len(specialists) != 2 {
		t.Fatalf("specialists len = %d, want 2", len(specialists))
	}
}

func TestIntegration_EmployeesGet_ReportsSandboxUpgradeAvailability(t *testing.T) {
	h := newEmployeeHarness(t)
	h.platformCredCleanup(t)
	m := h.createOrg(t)
	emp := h.seedEmployeeAgent(t, m)
	sb := h.seedSandbox(t, m, emp.ID)
	h.setSandboxSnapshot(t, sb.ID, &h.cfg.SandboxesRuntimeBaseImagePrefix)

	rr := h.getEmployee(t, m, emp.ID.String())
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rr.Code, rr.Body.String())
	}
	var item map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &item); err != nil {
		t.Fatalf("decode matching snapshot response: %v", err)
	}
	if item["upgrade_available"] != false {
		t.Fatalf("matching sandbox upgrade_available = %v, want false", item["upgrade_available"])
	}
	if _, exposed := item["snapshot_id"]; exposed {
		t.Fatalf("employee response exposed snapshot_id: %#v", item)
	}

	outdatedSnapshot := "older-employee-sandbox"
	h.setSandboxSnapshot(t, sb.ID, &outdatedSnapshot)
	rr = h.getEmployee(t, m, emp.ID.String())
	if rr.Code != http.StatusOK {
		t.Fatalf("outdated status = %d, want 200: %s", rr.Code, rr.Body.String())
	}
	item = map[string]any{}
	if err := json.Unmarshal(rr.Body.Bytes(), &item); err != nil {
		t.Fatalf("decode outdated snapshot response: %v", err)
	}
	if item["upgrade_available"] != true {
		t.Fatalf("outdated sandbox upgrade_available = %v, want true", item["upgrade_available"])
	}

	h.setSandboxSnapshot(t, sb.ID, nil)
	rr = h.getEmployee(t, m, emp.ID.String())
	if rr.Code != http.StatusOK {
		t.Fatalf("legacy status = %d, want 200: %s", rr.Code, rr.Body.String())
	}
	item = map[string]any{}
	if err := json.Unmarshal(rr.Body.Bytes(), &item); err != nil {
		t.Fatalf("decode legacy snapshot response: %v", err)
	}
	if item["upgrade_available"] != true {
		t.Fatalf("legacy sandbox upgrade_available = %v, want true", item["upgrade_available"])
	}
}

func TestIntegration_EmployeesGet_ScopedToOrg(t *testing.T) {
	h := newEmployeeHarness(t)
	h.platformCredCleanup(t)
	owner := h.createOrg(t)
	stranger := h.createOrg(t)
	emp := h.seedEmployeeAgent(t, owner)

	rr := h.getEmployee(t, stranger, emp.ID.String())
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404: %s", rr.Code, rr.Body.String())
	}
}
