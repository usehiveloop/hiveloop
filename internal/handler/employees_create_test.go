package handler_test

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
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
	var rows []model.EmployeeSkill
	if err := db.Where("employee_id = ?", agentID).Find(&rows).Error; err != nil {
		t.Fatalf("load employee_skills for %v: %v", agentID, err)
	}
	out := make(map[uuid.UUID]bool, len(rows))
	for _, r := range rows {
		out[r.SkillID] = true
	}
	return out
}
