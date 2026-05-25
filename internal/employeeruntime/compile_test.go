package employeeruntime

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/employeeprompts"
	"github.com/usehivy/hivy/internal/model"
)

func TestBuildPromptSections_UsesTypedFields(t *testing.T) {
	orgID := uuid.New()
	description := managedEmployeeDescription
	agent := &model.Employee{
		ID:             uuid.New(),
		OrgID:          &orgID,
		Name:           "Aria",
		Description:    &description,
		SystemPrompt:   "raw system prompt must not be forwarded",
		IdentityPrompt: "Own engineering outcomes with evidence.",
	}

	fragments := buildPromptSections(context.Background(), nil, agent, description)

	if !strings.Contains(fragments.Identity.Content, managedEmployeeName) {
		t.Fatalf("identity fragment should include employee name: %#v", fragments.Identity)
	}
	if !strings.Contains(fragments.Identity.Content, description) {
		t.Fatalf("identity fragment should include description: %#v", fragments.Identity)
	}
	if strings.Contains(fragments.Identity.Content, agent.SystemPrompt) {
		t.Fatalf("typed fragments must not include raw system prompt")
	}
	if strings.Contains(fragments.Identity.Content, agent.IdentityPrompt) {
		t.Fatalf("identity fragment should not include user-editable identity prompt")
	}
}

func TestBuildPromptSections_UpgradesDefaultManagedIdentityPrompt(t *testing.T) {
	orgID := uuid.New()
	description := "Coordinates engineering work."
	category := "engineering"
	for _, tc := range []struct {
		name           string
		identityPrompt string
	}{
		{name: "blank", identityPrompt: ""},
		{name: "legacy default", identityPrompt: employeeprompts.LegacyEngineeringIdentityPromptV1},
		{name: "current default", identityPrompt: employeeprompts.EngineeringIdentityPrompt},
	} {
		t.Run(tc.name, func(t *testing.T) {
			agent := &model.Employee{
				ID:             uuid.New(),
				OrgID:          &orgID,
				Name:           "Higu",
				Description:    &description,
				Category:       &category,
				IdentityPrompt: tc.identityPrompt,
			}

			fragments := buildPromptSections(context.Background(), nil, agent, description)

			if !strings.Contains(fragments.Identity.Content, "Communication contract") {
				t.Fatalf("identity fragment missing current communication contract: %#v", fragments.Identity)
			}
			if !strings.Contains(fragments.Identity.Content, "Do not say \"specialist runtime\"") {
				t.Fatalf("identity fragment missing specialist runtime leakage guard: %#v", fragments.Identity)
			}
		})
	}
}

func TestBuildPromptSections_PreservesCustomIdentityPrompt(t *testing.T) {
	orgID := uuid.New()
	description := "Coordinates engineering work."
	custom := "Use the team's incident voice."
	agent := &model.Employee{
		ID:             uuid.New(),
		OrgID:          &orgID,
		Name:           "Higu",
		Description:    &description,
		IdentityPrompt: custom,
	}

	fragments := buildPromptSections(context.Background(), nil, agent, description)

	if strings.Contains(fragments.Identity.Content, custom) {
		t.Fatalf("identity fragment should ignore custom identity prompt: %#v", fragments.Identity)
	}
	if !strings.Contains(fragments.Identity.Content, "Communication contract") {
		t.Fatalf("identity fragment should use backend-owned default: %#v", fragments.Identity)
	}
}

func TestBuildEmployeeSystemPrompt_CompilesAllRuntimePromptSegments(t *testing.T) {
	fragments := PromptSections{
		Identity: PromptSection{
			Title:   "Your identity",
			Content: "You are the managed employee.",
		},
		Company: PromptSection{
			Title:   "About the company",
			Content: "Company name: ExampleCo",
		},
	}

	prompt := buildEmployeeSystemPrompt(fragments)
	cacheable := requireCacheableSegments(t, prompt)
	dynamic := requireDynamicSegments(t, prompt)

	if len(cacheable) != 3 {
		t.Fatalf("cacheable segment count = %d", len(cacheable))
	}
	base := requireStaticPromptSegment(t, cacheable[0])
	if !strings.Contains(requirePromptString(t, base.Content), "Your job is to drive real team work forward.") {
		t.Fatalf("base prompt missing employee contract: %#v", base)
	}
	if got := requireDynamicContextSegmentType(t, dynamic[0]); got != "dynamic_context" {
		t.Fatalf("first dynamic segment = %q", got)
	}
	if got := requireMemorySegmentType(t, dynamic[1]); got != "memory_context" {
		t.Fatalf("second dynamic segment = %q", got)
	}
	if got := requireListSegment3Type(t, dynamic[2]); got != "skill_catalog" {
		t.Fatalf("third dynamic segment = %q", got)
	}
	if len(dynamic) != 4 {
		t.Fatalf("dynamic segment count = %d", len(dynamic))
	}
	if got := requireListSegment4Type(t, dynamic[3]); got != "mcp_tools" {
		t.Fatalf("fourth dynamic segment = %q", got)
	}
}

func requireCacheableSegments(t *testing.T, prompt SystemPromptConfig) []SystemPromptSegment {
	t.Helper()
	if prompt.CacheableSegments == nil {
		t.Fatal("cacheable segments is nil")
	}
	return *prompt.CacheableSegments
}

func requireDynamicSegments(t *testing.T, prompt SystemPromptConfig) []SystemPromptSegment {
	t.Helper()
	if prompt.DynamicSegments == nil {
		t.Fatal("dynamic segments is nil")
	}
	return *prompt.DynamicSegments
}

func requireStaticPromptSegment(t *testing.T, segment SystemPromptSegment) StaticPromptSegment {
	t.Helper()
	staticSegment, err := segment.AsSystemPromptSegment0()
	if err != nil {
		t.Fatalf("decode static prompt segment: %v", err)
	}
	if staticSegment.Type != "static_text" {
		t.Fatalf("static segment type = %q", staticSegment.Type)
	}
	return staticSegment.Config
}

func requireDynamicContextSegmentType(t *testing.T, segment SystemPromptSegment) string {
	t.Helper()
	dynamicSegment, err := segment.AsSystemPromptSegment1()
	if err != nil {
		t.Fatalf("decode dynamic context segment: %v", err)
	}
	return string(dynamicSegment.Type)
}

func requireMemorySegmentType(t *testing.T, segment SystemPromptSegment) string {
	t.Helper()
	memorySegment, err := segment.AsSystemPromptSegment2()
	if err != nil {
		t.Fatalf("decode memory segment: %v", err)
	}
	return string(memorySegment.Type)
}

func requireListSegment3Type(t *testing.T, segment SystemPromptSegment) string {
	t.Helper()
	listSegment, err := segment.AsSystemPromptSegment3()
	if err != nil {
		t.Fatalf("decode list segment: %v", err)
	}
	return string(listSegment.Type)
}

func requireListSegment4Type(t *testing.T, segment SystemPromptSegment) string {
	t.Helper()
	listSegment, err := segment.AsSystemPromptSegment4()
	if err != nil {
		t.Fatalf("decode mcp tools segment: %v", err)
	}
	return string(listSegment.Type)
}

func requirePromptString(t *testing.T, value *string) string {
	t.Helper()
	if value == nil {
		t.Fatal("prompt string is nil")
	}
	return *value
}
