package handler_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/auth"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/tasks"
)

func (h *employeeHarness) postUpgrade(t *testing.T, m orgWithMember, agentID uuid.UUID, body any, role string) *httptest.ResponseRecorder {
	t.Helper()
	buf := new(bytes.Buffer)
	if body != nil {
		_ = json.NewEncoder(buf).Encode(body)
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/employees/"+agentID.String()+"/sandbox/upgrade", buf)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Org-ID", m.org.ID.String())
	req = middleware.WithAuthClaims(req, &auth.AuthClaims{
		UserID: m.user.ID.String(),
		OrgID:  m.org.ID.String(),
		Role:   role,
	})
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	return rr
}

func (h *employeeHarness) getUpgrade(t *testing.T, m orgWithMember, agentID, upgradeID uuid.UUID, role string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/v1/employees/"+agentID.String()+"/sandbox/upgrades/"+upgradeID.String(), nil)
	req.Header.Set("X-Org-ID", m.org.ID.String())
	req = middleware.WithAuthClaims(req, &auth.AuthClaims{
		UserID: m.user.ID.String(),
		OrgID:  m.org.ID.String(),
		Role:   role,
	})
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	return rr
}

func TestEmployeeSandboxUpgrade_StartRequiresAdmin(t *testing.T) {
	h := newEmployeeHarness(t)
	m := h.createOrgWithRole(t, "member")
	agent := h.seedEmployeeAgent(t, m)
	h.seedSlackProfile(t, m, agent.ID)
	h.seedSandbox(t, m, agent.ID)

	rr := h.postUpgrade(t, m, agent.ID, nil, "member")
	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestEmployeeSandboxUpgrade_StartEnqueuesOperation(t *testing.T) {
	h := newEmployeeHarness(t)
	m := h.createOrg(t)
	agent := h.seedEmployeeAgent(t, m)
	h.seedSlackProfile(t, m, agent.ID)
	oldSandbox := h.seedSandbox(t, m, agent.ID)

	rr := h.postUpgrade(t, m, agent.ID, nil, "admin")
	if rr.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	upgradeID, err := uuid.Parse(resp["upgrade_id"].(string))
	if err != nil {
		t.Fatalf("parse upgrade id: %v", err)
	}
	var upgrade model.EmployeeSandboxUpgrade
	if err := h.db.First(&upgrade, "id = ?", upgradeID).Error; err != nil {
		t.Fatalf("load upgrade: %v", err)
	}
	if upgrade.OldSandboxID == nil || *upgrade.OldSandboxID != oldSandbox.ID {
		t.Fatalf("old sandbox id = %v, want %s", upgrade.OldSandboxID, oldSandbox.ID)
	}
	if upgrade.Status != model.EmployeeSandboxUpgradeStatusQueued || upgrade.Phase != model.EmployeeSandboxUpgradePhaseQueued {
		t.Fatalf("status/phase = %s/%s", upgrade.Status, upgrade.Phase)
	}
	h.enqueuer.AssertEnqueued(t, tasks.TypeEmployeeSandboxUpgrade)
	deleted := h.enqueuer.DeletedTasks()
	if len(deleted) != 1 {
		t.Fatalf("deleted stale tasks = %d, want 1", len(deleted))
	}
	if deleted[0].Queue != tasks.QueueBulk || deleted[0].ID != tasks.EmployeeSandboxUpgradeTaskID(agent.ID) {
		t.Fatalf("deleted stale task = %#v", deleted[0])
	}
}

func TestEmployeeSandboxUpgrade_DuplicateActiveReturnsExisting(t *testing.T) {
	h := newEmployeeHarness(t)
	m := h.createOrg(t)
	agent := h.seedEmployeeAgent(t, m)
	h.seedSlackProfile(t, m, agent.ID)
	oldSandbox := h.seedSandbox(t, m, agent.ID)
	existing := model.EmployeeSandboxUpgrade{
		OrgID:        m.org.ID,
		AgentID:      agent.ID,
		OldSandboxID: &oldSandbox.ID,
		Status:       model.EmployeeSandboxUpgradeStatusRunning,
		Phase:        model.EmployeeSandboxUpgradePhaseBackup,
	}
	if err := h.db.Create(&existing).Error; err != nil {
		t.Fatalf("create upgrade: %v", err)
	}

	rr := h.postUpgrade(t, m, agent.ID, nil, "admin")
	if rr.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["upgrade_id"] != existing.ID.String() {
		t.Fatalf("upgrade_id = %s, want %s", resp["upgrade_id"], existing.ID)
	}
	if got := h.enqueuer.Tasks(); len(got) != 0 {
		t.Fatalf("expected no task enqueued for duplicate, got %d", len(got))
	}
	if got := h.enqueuer.DeletedTasks(); len(got) != 0 {
		t.Fatalf("expected no task deletion for active upgrade, got %d", len(got))
	}
}

func TestEmployeeSandboxUpgrade_DeletesStaleTaskAfterFailedUpgrade(t *testing.T) {
	h := newEmployeeHarness(t)
	m := h.createOrg(t)
	agent := h.seedEmployeeAgent(t, m)
	h.seedSlackProfile(t, m, agent.ID)
	oldSandbox := h.seedSandbox(t, m, agent.ID)
	failed := model.EmployeeSandboxUpgrade{
		OrgID:        m.org.ID,
		AgentID:      agent.ID,
		OldSandboxID: &oldSandbox.ID,
		Status:       model.EmployeeSandboxUpgradeStatusFailed,
		Phase:        model.EmployeeSandboxUpgradePhaseCreatingNew,
	}
	if err := h.db.Create(&failed).Error; err != nil {
		t.Fatalf("create failed upgrade: %v", err)
	}

	rr := h.postUpgrade(t, m, agent.ID, nil, "admin")
	if rr.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", rr.Code, rr.Body.String())
	}
	deleted := h.enqueuer.DeletedTasks()
	if len(deleted) != 1 {
		t.Fatalf("deleted stale tasks = %d, want 1", len(deleted))
	}
	if deleted[0].Queue != tasks.QueueBulk || deleted[0].ID != tasks.EmployeeSandboxUpgradeTaskID(agent.ID) {
		t.Fatalf("deleted stale task = %#v", deleted[0])
	}
	h.enqueuer.AssertEnqueued(t, tasks.TypeEmployeeSandboxUpgrade)
}

func TestEmployeeSandboxUpgrade_MissingProfileOrSandbox(t *testing.T) {
	h := newEmployeeHarness(t)
	m := h.createOrg(t)
	agent := h.seedEmployeeAgent(t, m)

	if rr := h.postUpgrade(t, m, agent.ID, nil, "admin"); rr.Code != http.StatusBadRequest {
		t.Fatalf("missing profile: expected 400, got %d: %s", rr.Code, rr.Body.String())
	}
	h.seedSlackProfile(t, m, agent.ID)
	if rr := h.postUpgrade(t, m, agent.ID, nil, "admin"); rr.Code != http.StatusConflict {
		t.Fatalf("missing sandbox: expected 409, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestEmployeeSandboxUpgrade_StatusScopedByOrgAndEmployee(t *testing.T) {
	h := newEmployeeHarness(t)
	owner := h.createOrg(t)
	other := h.createOrg(t)
	agent := h.seedEmployeeAgent(t, owner)
	oldSandbox := h.seedSandbox(t, owner, agent.ID)
	upgrade := model.EmployeeSandboxUpgrade{
		OrgID:        owner.org.ID,
		AgentID:      agent.ID,
		OldSandboxID: &oldSandbox.ID,
		Status:       model.EmployeeSandboxUpgradeStatusQueued,
		Phase:        model.EmployeeSandboxUpgradePhaseQueued,
	}
	if err := h.db.Create(&upgrade).Error; err != nil {
		t.Fatalf("create upgrade: %v", err)
	}

	if rr := h.getUpgrade(t, owner, agent.ID, upgrade.ID, "admin"); rr.Code != http.StatusOK {
		t.Fatalf("owner status: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	if rr := h.getUpgrade(t, other, agent.ID, upgrade.ID, "admin"); rr.Code != http.StatusNotFound {
		t.Fatalf("other org status: expected 404, got %d: %s", rr.Code, rr.Body.String())
	}
	if rr := h.getUpgrade(t, owner, uuid.New(), upgrade.ID, "admin"); rr.Code != http.StatusNotFound {
		t.Fatalf("wrong employee status: expected 404, got %d: %s", rr.Code, rr.Body.String())
	}
}
