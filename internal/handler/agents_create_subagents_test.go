package handler_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/usehiveloop/hiveloop/internal/model"
)

func (h *agentCreateHarness) seedAgent(t *testing.T, orgID uuid.UUID, name string, isEmployee bool) model.Agent {
	t.Helper()
	agent := model.Agent{
		OrgID:        &orgID,
		Name:         name,
		Model:        "claude-sonnet-4-6",
		SystemPrompt: "you are a test agent",
		Status:       "active",
		IsEmployee:   isEmployee,
	}
	if err := h.db.Create(&agent).Error; err != nil {
		t.Fatalf("seed agent %q: %v", name, err)
	}
	return agent
}

func TestAgentCreate_Employee_Succeeds(t *testing.T) {
	h := newAgentCreateHarness(t)
	org, user := h.createOrgWithBYOK(t, false)

	rr := h.post(t, user.ID, org.ID, map[string]any{
		"name":        "employee-" + uuid.New().String()[:8],
		"is_employee": true,
	})
	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["is_employee"] != true {
		t.Fatalf("expected is_employee=true in response, got: %v", resp["is_employee"])
	}
	if subs, ok := resp["subagent_ids"]; ok && subs != nil {
		if list, _ := subs.([]any); len(list) != 0 {
			t.Fatalf("expected no subagent_ids when none requested, got: %v", subs)
		}
	}
}

func TestAgentCreate_SubagentIDs_WithoutIsEmployee_Rejected(t *testing.T) {
	h := newAgentCreateHarness(t)
	org, user := h.createOrgWithBYOK(t, false)
	sub := h.seedAgent(t, org.ID, "sub-"+uuid.New().String()[:8], false)

	rr := h.post(t, user.ID, org.ID, map[string]any{
		"name":         "rejects-" + uuid.New().String()[:8],
		"subagent_ids": []string{sub.ID.String()},
	})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rr.Code, rr.Body.String())
	}
	if msg := decodeError(t, rr); !strings.Contains(msg, "is_employee") {
		t.Fatalf("expected is_employee mention, got: %q", msg)
	}
}

func TestAgentCreate_Employee_WithValidSubagents_Succeeds(t *testing.T) {
	h := newAgentCreateHarness(t)
	org, user := h.createOrgWithBYOK(t, false)
	subA := h.seedAgent(t, org.ID, "sub-a-"+uuid.New().String()[:8], false)
	subB := h.seedAgent(t, org.ID, "sub-b-"+uuid.New().String()[:8], false)

	rr := h.post(t, user.ID, org.ID, map[string]any{
		"name":         "boss-" + uuid.New().String()[:8],
		"is_employee":  true,
		"subagent_ids": []string{subA.ID.String(), subB.ID.String(), subA.ID.String()},
	})
	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp struct {
		ID          string   `json:"id"`
		IsEmployee  bool     `json:"is_employee"`
		SubagentIDs []string `json:"subagent_ids"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !resp.IsEmployee {
		t.Fatalf("expected is_employee=true, got false")
	}
	if len(resp.SubagentIDs) != 2 {
		t.Fatalf("expected 2 deduped subagent_ids in response, got %d: %v", len(resp.SubagentIDs), resp.SubagentIDs)
	}

	employeeID := uuid.MustParse(resp.ID)
	var count int64
	if err := h.db.Model(&model.AgentSubagent{}).Where("agent_id = ?", employeeID).Count(&count).Error; err != nil {
		t.Fatalf("count join rows: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected 2 agent_subagents rows for employee, got %d", count)
	}

	t.Cleanup(func() {
		h.db.Where("agent_id = ?", employeeID).Delete(&model.AgentSubagent{})
	})
}

func TestAgentCreate_Employee_RejectsEmployeeAsSubagent(t *testing.T) {
	h := newAgentCreateHarness(t)
	org, user := h.createOrgWithBYOK(t, false)
	otherEmployee := h.seedAgent(t, org.ID, "other-emp-"+uuid.New().String()[:8], true)

	rr := h.post(t, user.ID, org.ID, map[string]any{
		"name":         "boss-" + uuid.New().String()[:8],
		"is_employee":  true,
		"subagent_ids": []string{otherEmployee.ID.String()},
	})
	if rr.Code == http.StatusCreated {
		t.Fatalf("expected non-201 (employee as subagent should be rejected), got 201")
	}

	var count int64
	h.db.Model(&model.Agent{}).Where("org_id = ? AND is_employee = ? AND id <> ?", org.ID, true, otherEmployee.ID).Count(&count)
	if count != 0 {
		t.Fatalf("expected no new employees after rejection, found %d", count)
	}
}

func TestAgentCreate_Employee_RejectsCrossOrgSubagent(t *testing.T) {
	h := newAgentCreateHarness(t)
	org, user := h.createOrgWithBYOK(t, false)
	otherOrg, _ := h.createOrgWithBYOK(t, false)
	foreign := h.seedAgent(t, otherOrg.ID, "foreign-"+uuid.New().String()[:8], false)

	rr := h.post(t, user.ID, org.ID, map[string]any{
		"name":         "boss-" + uuid.New().String()[:8],
		"is_employee":  true,
		"subagent_ids": []string{foreign.ID.String()},
	})
	if rr.Code == http.StatusCreated {
		t.Fatalf("expected non-201 (cross-org subagent should be rejected), got 201")
	}

	var count int64
	h.db.Model(&model.Agent{}).Where("org_id = ? AND is_employee = ?", org.ID, true).Count(&count)
	if count != 0 {
		t.Fatalf("expected no employee created in caller org, found %d", count)
	}
}

func TestAgentCreate_Employee_RejectsInvalidSubagentUUID(t *testing.T) {
	h := newAgentCreateHarness(t)
	org, user := h.createOrgWithBYOK(t, false)

	rr := h.post(t, user.ID, org.ID, map[string]any{
		"name":         "boss-" + uuid.New().String()[:8],
		"is_employee":  true,
		"subagent_ids": []string{"not-a-uuid"},
	})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rr.Code, rr.Body.String())
	}
	if msg := decodeError(t, rr); !strings.Contains(msg, "invalid subagent_id") {
		t.Fatalf("expected invalid-subagent-id error, got: %q", msg)
	}
}
