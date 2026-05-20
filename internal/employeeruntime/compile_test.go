package employeeruntime

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/employeeprompts"
	"github.com/usehivy/hivy/internal/model"
)

func TestBuildPromptFragments_UsesTypedFields(t *testing.T) {
	orgID := uuid.New()
	description := managedEmployeeDescription
	agent := &model.Agent{
		ID:             uuid.New(),
		OrgID:          &orgID,
		Name:           "Aria",
		Description:    &description,
		SystemPrompt:   "raw system prompt must not be forwarded",
		IdentityPrompt: "Own engineering outcomes with evidence.",
	}

	fragments := buildPromptFragments(context.Background(), nil, agent, description)

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

func TestBuildPromptFragments_UpgradesDefaultManagedIdentityPrompt(t *testing.T) {
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
			agent := &model.Agent{
				ID:             uuid.New(),
				OrgID:          &orgID,
				Name:           "Higu",
				Description:    &description,
				Category:       &category,
				IdentityPrompt: tc.identityPrompt,
			}

			fragments := buildPromptFragments(context.Background(), nil, agent, description)

			if !strings.Contains(fragments.Identity.Content, "Slack communication contract") {
				t.Fatalf("identity fragment missing current Slack communication contract: %#v", fragments.Identity)
			}
			if !strings.Contains(fragments.Identity.Content, "Do not say \"specialist runtime\"") {
				t.Fatalf("identity fragment missing specialist runtime leakage guard: %#v", fragments.Identity)
			}
		})
	}
}

func TestBuildPromptFragments_PreservesCustomIdentityPrompt(t *testing.T) {
	orgID := uuid.New()
	description := "Coordinates engineering work."
	custom := "Use the team's incident voice."
	agent := &model.Agent{
		ID:             uuid.New(),
		OrgID:          &orgID,
		Name:           "Higu",
		Description:    &description,
		IdentityPrompt: custom,
	}

	fragments := buildPromptFragments(context.Background(), nil, agent, description)

	if strings.Contains(fragments.Identity.Content, custom) {
		t.Fatalf("identity fragment should ignore custom identity prompt: %#v", fragments.Identity)
	}
	if !strings.Contains(fragments.Identity.Content, "Slack communication contract") {
		t.Fatalf("identity fragment should use backend-owned default: %#v", fragments.Identity)
	}
}

func TestBuildEmployeeMCPServer_DisabledWithoutRuntimeToken(t *testing.T) {
	orgID := uuid.New()
	agent := &model.Agent{ID: uuid.New(), OrgID: &orgID}

	if got := buildEmployeeMCPServer(context.Background(), CompileDeps{}, agent); got != nil {
		t.Fatalf("expected no MCP server without DB/config/token, got %#v", got)
	}
}

func TestUpsertHivyMCPServer_ReplacesExistingHivyServer(t *testing.T) {
	servers := []any{
		map[string]any{"name": "hivy", "url": "old"},
		map[string]any{"name": "linear", "url": "keep"},
	}
	got := upsertHivyMCPServer(servers, map[string]any{"name": "hivy", "url": "new"})

	if len(got) != 2 {
		t.Fatalf("server count = %d, want 2", len(got))
	}
	if got[1].(map[string]any)["url"] != "new" {
		t.Fatalf("hivy server was not replaced: %#v", got)
	}
}
