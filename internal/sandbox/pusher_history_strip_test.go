package sandbox

import (
	"testing"

	bridgepkg "github.com/usehiveloop/hiveloop/internal/bridge"
)

func unwrapHistoryStrip(t *testing.T, w *bridgepkg.AgentConfig_HistoryStrip) bridgepkg.HistoryStripConfig {
	t.Helper()
	if w == nil {
		t.Fatalf("HistoryStrip wrapper is nil")
	}
	hs, err := w.AsHistoryStripConfig()
	if err != nil {
		t.Fatalf("unwrap history strip: %v", err)
	}
	return hs
}

func TestApplyHistoryStripDefault_PopulatesWhenAbsent(t *testing.T) {
	cfg := &bridgepkg.AgentConfig{}
	applyHistoryStripDefault(cfg)

	hs := unwrapHistoryStrip(t, cfg.HistoryStrip)
	if hs.Enabled == nil || !*hs.Enabled {
		t.Errorf("Enabled should be true")
	}
	if hs.PinErrors == nil || !*hs.PinErrors {
		t.Errorf("PinErrors should be true so error context stays visible")
	}
	if hs.PinRecentCount == nil || *hs.PinRecentCount != historyStripPinRecent {
		t.Errorf("PinRecentCount should be %d", historyStripPinRecent)
	}
	if hs.AgeThreshold == nil || *hs.AgeThreshold != historyStripAgeThreshold {
		t.Errorf("AgeThreshold should be %d", historyStripAgeThreshold)
	}
}

func TestApplyHistoryStripDefault_RespectsAuthorOverride(t *testing.T) {
	customEnabled := false
	custom := bridgepkg.HistoryStripConfig{Enabled: &customEnabled}
	var existing bridgepkg.AgentConfig_HistoryStrip
	if err := existing.FromHistoryStripConfig(custom); err != nil {
		t.Fatalf("setup: %v", err)
	}
	cfg := &bridgepkg.AgentConfig{HistoryStrip: &existing}
	applyHistoryStripDefault(cfg)

	if cfg.HistoryStrip != &existing {
		t.Errorf("author-supplied HistoryStrip was overwritten")
	}
	hs := unwrapHistoryStrip(t, cfg.HistoryStrip)
	if hs.Enabled == nil || *hs.Enabled {
		t.Errorf("author's Enabled=false was flipped to true")
	}
}

func TestApplyHistoryStripDefault_NilConfigIsNoOp(t *testing.T) {
	applyHistoryStripDefault(nil)
}
