package sandbox

import (
	"testing"

	bridgepkg "github.com/usehiveloop/hiveloop/internal/bridge"
)

func unwrapImmortal(t *testing.T, w *bridgepkg.AgentConfig_Immortal) bridgepkg.ImmortalConfig {
	t.Helper()
	if w == nil {
		t.Fatalf("Immortal wrapper is nil")
	}
	imm, err := w.AsImmortalConfig()
	if err != nil {
		t.Fatalf("unwrap immortal: %v", err)
	}
	return imm
}

func TestApplyImmortalDefault_TokenBudgetIs50PctOfParentContextWindow(t *testing.T) {
	cases := []struct {
		name       string
		providerID string
		model      string
		wantBudget int32
	}{
		{"gemini-3-pro", "google", "gemini-3-pro-preview", 500000},
		{"claude-sonnet-4-5", "anthropic", "claude-sonnet-4-5", 100000},
		{"gpt-5.1", "openai", "gpt-5.1", 200000},
		{"unknown-model", "openai", "something-we-havent-curated", 64000},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &bridgepkg.AgentConfig{}
			applyImmortalDefault(cfg, bridgepkg.ProviderConfig{}, tc.providerID, tc.model, nil)
			imm := unwrapImmortal(t, cfg.Immortal)
			if imm.TokenBudget == nil || *imm.TokenBudget != tc.wantBudget {
				got := int32(-1)
				if imm.TokenBudget != nil {
					got = *imm.TokenBudget
				}
				t.Errorf("TokenBudget: got %d want %d", got, tc.wantBudget)
			}
		})
	}
}

func TestApplyImmortalDefault_AppliesRetentionAndEvictionWindow(t *testing.T) {
	cfg := &bridgepkg.AgentConfig{}
	applyImmortalDefault(cfg, bridgepkg.ProviderConfig{}, "google", "gemini-3-pro-preview", nil)
	imm := unwrapImmortal(t, cfg.Immortal)
	if imm.RetentionWindow == nil || *imm.RetentionWindow != immortalRetentionWindow {
		t.Errorf("RetentionWindow: want %d", immortalRetentionWindow)
	}
	if imm.EvictionWindow == nil || *imm.EvictionWindow != immortalEvictionWindow {
		t.Errorf("EvictionWindow: want %v", immortalEvictionWindow)
	}
}

func TestApplyImmortalDefault_ExposeJournalToolsTrueByDefault(t *testing.T) {
	cfg := &bridgepkg.AgentConfig{}
	applyImmortalDefault(cfg, bridgepkg.ProviderConfig{}, "google", "gemini-3-pro-preview", nil)
	imm := unwrapImmortal(t, cfg.Immortal)
	if imm.ExposeJournalTools == nil || !*imm.ExposeJournalTools {
		t.Errorf("ExposeJournalTools should default true when journal tools aren't denied")
	}
}

func TestApplyImmortalDefault_ExposeJournalToolsRespectsDenial(t *testing.T) {
	cfg := &bridgepkg.AgentConfig{}
	perms := map[string]bridgepkg.ToolPermission{
		"journal_read":  bridgepkg.ToolPermissionDeny,
		"journal_write": bridgepkg.ToolPermissionDeny,
	}
	applyImmortalDefault(cfg, bridgepkg.ProviderConfig{}, "google", "gemini-3-pro-preview", &perms)
	imm := unwrapImmortal(t, cfg.Immortal)
	if imm.ExposeJournalTools == nil || *imm.ExposeJournalTools {
		t.Errorf("ExposeJournalTools should be false when both journal tools are denied")
	}
}

func TestApplyImmortalDefault_ExposeJournalToolsRespectsDisabledTools(t *testing.T) {
	disabled := []string{"journal_read", "journal_write"}
	cfg := &bridgepkg.AgentConfig{DisabledTools: &disabled}
	applyImmortalDefault(cfg, bridgepkg.ProviderConfig{}, "google", "gemini-3-pro-preview", nil)
	imm := unwrapImmortal(t, cfg.Immortal)
	if imm.ExposeJournalTools == nil || *imm.ExposeJournalTools {
		t.Errorf("ExposeJournalTools should be false when journal tools are in disabled_tools")
	}
}

func TestApplyImmortalDefault_PartialJournalDenialKeepsExposureOn(t *testing.T) {
	cfg := &bridgepkg.AgentConfig{}
	perms := map[string]bridgepkg.ToolPermission{
		"journal_write": bridgepkg.ToolPermissionDeny,
	}
	applyImmortalDefault(cfg, bridgepkg.ProviderConfig{}, "google", "gemini-3-pro-preview", &perms)
	imm := unwrapImmortal(t, cfg.Immortal)
	if imm.ExposeJournalTools == nil || !*imm.ExposeJournalTools {
		t.Errorf("denying only one journal tool must keep the other exposed")
	}
}

func TestApplyImmortalDefault_RespectsAuthorOverride(t *testing.T) {
	customBudget := int32(50_000)
	custom := bridgepkg.ImmortalConfig{TokenBudget: &customBudget}
	var existing bridgepkg.AgentConfig_Immortal
	if err := existing.FromImmortalConfig(custom); err != nil {
		t.Fatalf("setup: %v", err)
	}
	cfg := &bridgepkg.AgentConfig{Immortal: &existing}
	applyImmortalDefault(cfg, bridgepkg.ProviderConfig{}, "google", "gemini-3-pro-preview", nil)
	imm := unwrapImmortal(t, cfg.Immortal)
	if imm.TokenBudget == nil || *imm.TokenBudget != 50_000 {
		t.Errorf("author-supplied TokenBudget was overwritten")
	}
	if imm.RetentionWindow != nil {
		t.Errorf("author-supplied Immortal must not be merged with defaults")
	}
}

func TestApplyImmortalDefault_NilConfigIsNoOp(t *testing.T) {
	applyImmortalDefault(nil, bridgepkg.ProviderConfig{}, "google", "gemini-3-pro-preview", nil)
}
