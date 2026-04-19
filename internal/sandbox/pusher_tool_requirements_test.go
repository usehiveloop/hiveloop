package sandbox

import (
	"encoding/json"
	"testing"

	bridgepkg "github.com/ziraloop/ziraloop/internal/bridge"
)

// applyToolRequirementsDefault is the single point where the memory/journal
// loop becomes an enforced server-side check. A regression here would mean
// agents silently stop running their bookkeeping tools — so we pin:
//   - which tools are in the default list,
//   - the cadence (every 5 turns, as `every_n_turns` variant),
//   - memory_recall's turn_start position,
//   - the override contract (author list wins),
//   - the disable-safe contract (tools the author disabled must never be
//     required — Bridge rejects such a push with a 400).

func TestApplyToolRequirementsDefault_AutoInjectsMemoryAndJournalLoop(t *testing.T) {
	cfg := &bridgepkg.AgentConfig{}
	applyToolRequirementsDefault(cfg, nil)

	if cfg.ToolRequirements == nil {
		t.Fatalf("ToolRequirements should be populated")
	}
	reqs := *cfg.ToolRequirements
	if len(reqs) != 3 {
		t.Fatalf("expected 3 requirements, got %d", len(reqs))
	}

	byTool := map[string]bridgepkg.ToolRequirement{}
	for _, req := range reqs {
		byTool[req.Tool] = req
	}
	for _, tool := range []string{"memory_recall", "memory_retain", "journal_write"} {
		if _, ok := byTool[tool]; !ok {
			t.Errorf("missing requirement for %q", tool)
		}
	}
}

func TestApplyToolRequirementsDefault_MemoryRecallIsTurnStart(t *testing.T) {
	cfg := &bridgepkg.AgentConfig{}
	applyToolRequirementsDefault(cfg, nil)

	for _, req := range *cfg.ToolRequirements {
		if req.Tool != "memory_recall" {
			continue
		}
		if req.Position == nil || *req.Position != bridgepkg.TurnStart {
			t.Errorf("memory_recall should be TurnStart, got %v", req.Position)
		}
		return
	}
	t.Fatalf("memory_recall requirement missing")
}

func TestApplyToolRequirementsDefault_MemoryRetainAndJournalAreAnywhere(t *testing.T) {
	// memory_retain and journal_write carry no explicit Position so Bridge
	// applies the default (Anywhere) — the agent can call them at any
	// point in the 5-turn window.
	cfg := &bridgepkg.AgentConfig{}
	applyToolRequirementsDefault(cfg, nil)

	for _, req := range *cfg.ToolRequirements {
		if req.Tool == "memory_recall" {
			continue
		}
		if req.Position != nil {
			t.Errorf("%s should leave Position unset (default Anywhere), got %v", req.Tool, *req.Position)
		}
	}
}

func TestApplyToolRequirementsDefault_CadenceIsEvery5Turns(t *testing.T) {
	// The cadence field is a tagged union — the only way to verify the
	// `every_n_turns` variant + n=5 serialization is to round-trip it
	// through JSON, which is what actually hits Bridge.
	cfg := &bridgepkg.AgentConfig{}
	applyToolRequirementsDefault(cfg, nil)

	for _, req := range *cfg.ToolRequirements {
		if req.Cadence == nil {
			t.Errorf("%s: Cadence should be set", req.Tool)
			continue
		}
		encoded, err := json.Marshal(req.Cadence)
		if err != nil {
			t.Fatalf("%s: marshal cadence: %v", req.Tool, err)
		}
		var probe struct {
			Type string `json:"type"`
			N    int32  `json:"n"`
		}
		if err := json.Unmarshal(encoded, &probe); err != nil {
			t.Fatalf("%s: unmarshal cadence: %v — got %s", req.Tool, err, encoded)
		}
		if probe.Type != string(bridgepkg.EveryNTurns) {
			t.Errorf("%s: cadence type = %q, want %q", req.Tool, probe.Type, bridgepkg.EveryNTurns)
		}
		if probe.N != defaultRequirementCadenceTurns {
			t.Errorf("%s: cadence.n = %d, want %d", req.Tool, probe.N, defaultRequirementCadenceTurns)
		}
	}
}

func TestApplyToolRequirementsDefault_CadencesAreIndependentPointers(t *testing.T) {
	// Each requirement owns its own cadence pointer — regression guard
	// against accidentally aliasing the union state.
	cfg := &bridgepkg.AgentConfig{}
	applyToolRequirementsDefault(cfg, nil)

	reqs := *cfg.ToolRequirements
	if reqs[0].Cadence == reqs[1].Cadence || reqs[1].Cadence == reqs[2].Cadence {
		t.Errorf("cadence pointers must not be shared across requirements")
	}
}

func TestApplyToolRequirementsDefault_RespectsAuthorOverride(t *testing.T) {
	// Any non-nil author list (including an empty one — "no requirements,
	// please") wins. We only inject when the field was unset.
	custom := []bridgepkg.ToolRequirement{{Tool: "write_file"}}
	cfg := &bridgepkg.AgentConfig{ToolRequirements: &custom}
	applyToolRequirementsDefault(cfg, nil)

	if len(*cfg.ToolRequirements) != 1 || (*cfg.ToolRequirements)[0].Tool != "write_file" {
		t.Errorf("author-supplied requirements were overwritten: %#v", *cfg.ToolRequirements)
	}

	empty := []bridgepkg.ToolRequirement{}
	cfg2 := &bridgepkg.AgentConfig{ToolRequirements: &empty}
	applyToolRequirementsDefault(cfg2, nil)
	if len(*cfg2.ToolRequirements) != 0 {
		t.Errorf("explicit empty list should be preserved, got %#v", *cfg2.ToolRequirements)
	}
}

func TestApplyToolRequirementsDefault_SkipsToolsInDisabledTools(t *testing.T) {
	// Regression guard for production incident: Bridge rejected a push with
	// 400 because journal_write was in disabled_tools AND auto-required.
	// The injector must filter out any default tool that's already disabled.
	disabled := []string{"journal_write"}
	cfg := &bridgepkg.AgentConfig{DisabledTools: &disabled}
	applyToolRequirementsDefault(cfg, nil)

	if cfg.ToolRequirements == nil {
		t.Fatalf("ToolRequirements should still be populated with surviving tools")
	}
	for _, req := range *cfg.ToolRequirements {
		if req.Tool == "journal_write" {
			t.Errorf("journal_write is in DisabledTools but was auto-required")
		}
	}
	if len(*cfg.ToolRequirements) != 2 {
		t.Errorf("expected 2 surviving requirements, got %d", len(*cfg.ToolRequirements))
	}
}

func TestApplyToolRequirementsDefault_SkipsToolsDeniedViaPermissions(t *testing.T) {
	// The permissions map is the other source of tool disablement — it's
	// the user-facing way. A "deny" entry must also disqualify auto-require.
	perms := map[string]bridgepkg.ToolPermission{
		"memory_retain": bridgepkg.ToolPermissionDeny,
		"memory_recall": bridgepkg.ToolPermissionAllow,
	}
	cfg := &bridgepkg.AgentConfig{}
	applyToolRequirementsDefault(cfg, &perms)

	for _, req := range *cfg.ToolRequirements {
		if req.Tool == "memory_retain" {
			t.Errorf("memory_retain has deny permission but was auto-required")
		}
	}
	if len(*cfg.ToolRequirements) != 2 {
		t.Errorf("expected 2 surviving requirements (recall + journal), got %d", len(*cfg.ToolRequirements))
	}
}

func TestApplyToolRequirementsDefault_AllDefaultsDisabledLeavesFieldNil(t *testing.T) {
	// If every default is blocked, don't inject an empty slice — leave
	// ToolRequirements nil so Bridge treats the agent as opt-out cleanly.
	disabled := []string{"memory_recall", "memory_retain", "journal_write"}
	cfg := &bridgepkg.AgentConfig{DisabledTools: &disabled}
	applyToolRequirementsDefault(cfg, nil)

	if cfg.ToolRequirements != nil {
		t.Errorf("expected nil ToolRequirements when all defaults are disabled, got %#v", *cfg.ToolRequirements)
	}
}

func TestApplyToolRequirementsDefault_NilConfigIsNoOp(t *testing.T) {
	applyToolRequirementsDefault(nil, nil) // must not panic
}
