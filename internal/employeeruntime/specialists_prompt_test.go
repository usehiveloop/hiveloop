package employeeruntime

import (
	"strings"
	"testing"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/specialists"
)

func TestBuildAvailableSpecialistsSection_ListsAttachedSpecialists(t *testing.T) {
	catalog, err := specialists.NewCatalog([]specialists.Definition{
		{
			Slug:           "software-engineering-specialist",
			Name:           "Software Engineering",
			Description:    "Build and verify code changes.",
			SpecialistType: "engineering",
			Version:        1,
			DefaultModel:   "deepseek-v4-pro",
			SystemPrompt:   "Do engineering work.",
		},
		{
			Slug:           "business-research-specialist",
			Name:           "Business Research",
			Description:    "Research business questions.",
			SpecialistType: "research",
			Version:        1,
			DefaultModel:   "deepseek-v4-pro",
			SystemPrompt:   "Do research work.",
		},
	})
	if err != nil {
		t.Fatalf("catalog: %v", err)
	}
	agent := &model.Employee{
		AttachedSpecialists: []string{"software-engineering-specialist"},
	}

	section := buildAvailableSpecialistsSection(agent, catalog)

	if section.Title != "Specialist agents" {
		t.Fatalf("section title = %q", section.Title)
	}
	if !strings.Contains(section.Content, "Software Engineering (software-engineering-specialist): Build and verify code changes.") {
		t.Fatalf("attached specialist missing from section: %q", section.Content)
	}
	if strings.Contains(section.Content, "business-research-specialist") {
		t.Fatalf("unattached specialist leaked into section: %q", section.Content)
	}
	if !strings.Contains(section.Content, "Specialist agents are attached coworkers") {
		t.Fatalf("section missing dispatch guidance: %q", section.Content)
	}
	if !strings.Contains(section.Content, "Choose based on the names and descriptions below") {
		t.Fatalf("section missing selection guidance: %q", section.Content)
	}
	if !strings.Contains(section.Content, "more than 30 seconds") {
		t.Fatalf("section missing heavy task guidance: %q", section.Content)
	}
	if !strings.Contains(section.Content, "disk or network reads and writes") {
		t.Fatalf("section missing resource guidance: %q", section.Content)
	}
	if !strings.Contains(section.Content, "matches a dedicated specialist") {
		t.Fatalf("section missing dedicated specialist guidance: %q", section.Content)
	}
	if !strings.Contains(section.Content, "when the request gives enough context to act") {
		t.Fatalf("section missing context gating guidance: %q", section.Content)
	}
	if !strings.Contains(section.Content, "If a request is vague, ask for the missing context before dispatching") {
		t.Fatalf("section missing clarification guidance: %q", section.Content)
	}
	if !strings.Contains(section.Content, "Make assumptions only for trivial, low-risk details") {
		t.Fatalf("section missing assumption guidance: %q", section.Content)
	}
	if !strings.Contains(section.Content, "schedule a wake instead of repeatedly polling status") {
		t.Fatalf("section missing wake guidance: %q", section.Content)
	}
	staleDelegationTerm := "sub" + "agent"
	if strings.Contains(section.Content, staleDelegationTerm) {
		t.Fatalf("section must use product terminology only: %q", section.Content)
	}
}

func TestDefaultRuntimeSurfaceDoesNotExposeGenericDelegation(t *testing.T) {
	for _, tool := range defaultTools() {
		if got, _ := tool["type"].(string); got == "builtin.delegate" || got == "builtin.check_delegated_status" {
			t.Fatalf("defaultTools exposed generic delegation tool %q", got)
		}
	}
	for key := range defaultLimits() {
		if strings.Contains(key, "sub"+"agent") {
			t.Fatalf("defaultLimits exposed stale key %q", key)
		}
	}
}
