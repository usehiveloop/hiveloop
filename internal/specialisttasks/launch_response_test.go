package specialisttasks

import (
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

func TestNewLaunchResponseIncludesWakeReminder(t *testing.T) {
	task := model.SpecialistTask{
		ID:                uuid.New(),
		SpecialistSlug:    "software-engineering-specialist",
		EmployeeSessionID: "runtime-session-1",
		SandboxID:         uuid.New(),
		Status:            "running",
	}

	resp := newLaunchResponse(task)

	for _, text := range []string{resp.SystemReminder, resp.NextAction} {
		if !strings.Contains(text, "wake") {
			t.Fatalf("launch response missing wake guidance: %#v", resp)
		}
		if !strings.Contains(text, "longer than 30 seconds") {
			t.Fatalf("launch response missing duration guidance: %#v", resp)
		}
		if !strings.Contains(text, "instead of polling") {
			t.Fatalf("launch response missing polling guidance: %#v", resp)
		}
	}
}
