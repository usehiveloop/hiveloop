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
//   - the override contract (author list wins).

func TestApplyToolRequirementsDefault_AutoInjectsMemoryAndJournalLoop(t *testing.T) {
	cfg := &bridgepkg.AgentConfig{}
	applyToolRequirementsDefault(cfg)

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
	applyToolRequirementsDefault(cfg)

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
	applyToolRequirementsDefault(cfg)

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
	applyToolRequirementsDefault(cfg)

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
	applyToolRequirementsDefault(cfg)

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
	applyToolRequirementsDefault(cfg)

	if len(*cfg.ToolRequirements) != 1 || (*cfg.ToolRequirements)[0].Tool != "write_file" {
		t.Errorf("author-supplied requirements were overwritten: %#v", *cfg.ToolRequirements)
	}

	empty := []bridgepkg.ToolRequirement{}
	cfg2 := &bridgepkg.AgentConfig{ToolRequirements: &empty}
	applyToolRequirementsDefault(cfg2)
	if len(*cfg2.ToolRequirements) != 0 {
		t.Errorf("explicit empty list should be preserved, got %#v", *cfg2.ToolRequirements)
	}
}

func TestApplyToolRequirementsDefault_NilConfigIsNoOp(t *testing.T) {
	applyToolRequirementsDefault(nil) // must not panic
}
