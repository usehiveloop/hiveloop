package handler_test

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/model"
)

func TestIntegration_EmployeesCreate_RouteRemoved(t *testing.T) {
	h := newEmployeeHarness(t)
	m := h.createOrg(t)

	rr := h.post(t, m, validEmployeeBody())
	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405 after employee create route removal: %s", rr.Code, rr.Body.String())
	}
}

func skillIDsFor(t *testing.T, db *gorm.DB, agentID uuid.UUID) map[uuid.UUID]bool {
	t.Helper()
	var rows []model.AgentSkill
	if err := db.Where("agent_id = ?", agentID).Find(&rows).Error; err != nil {
		t.Fatalf("load agent_skills for %v: %v", agentID, err)
	}
	out := make(map[uuid.UUID]bool, len(rows))
	for _, r := range rows {
		out[r.SkillID] = true
	}
	return out
}

func defaultSubagentsByType(t *testing.T, db *gorm.DB, agentID uuid.UUID) map[string]model.Agent {
	t.Helper()
	var links []model.AgentSubagent
	if err := db.Where("agent_id = ?", agentID).Find(&links).Error; err != nil {
		t.Fatalf("load AgentSubagent links: %v", err)
	}
	subIDs := make([]uuid.UUID, 0, len(links))
	for _, link := range links {
		subIDs = append(subIDs, link.SubagentID)
	}
	var agents []model.Agent
	if len(subIDs) > 0 {
		if err := db.Where("id IN ?", subIDs).Find(&agents).Error; err != nil {
			t.Fatalf("load subagent agents: %v", err)
		}
	}
	out := make(map[string]model.Agent, len(agents))
	for _, agent := range agents {
		typ, _ := agent.AgentConfig["default_cloud_agent_type"].(string)
		if typ != "" {
			out[typ] = agent
		}
	}
	return out
}
