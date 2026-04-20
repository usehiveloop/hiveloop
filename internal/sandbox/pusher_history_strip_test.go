package sandbox

import (
	"testing"

	bridgepkg "github.com/usehiveloop/hiveloop/internal/bridge"
)

// applyHistoryStripDefault caps history growth on long agent conversations
// by stripping old tool-result bodies from the LLM-visible prompt. Missing
// this on Kira's 24-turn run let history balloon to 143k tokens; an
// auto-configured strip would have kept it closer to 30k.

func TestApplyHistoryStripDefault_PopulatesWhenAbsent(t *testing.T) {
	cfg := &bridgepkg.AgentConfig{}
	applyHistoryStripDefault(cfg)

	if cfg.HistoryStrip == nil {
		t.Fatalf("HistoryStrip should be populated")
	}
	hs := cfg.HistoryStrip
	if hs.Enabled == nil || !*hs.Enabled {
		t.Errorf("Enabled should be true, got %v", hs.Enabled)
	}
	if hs.PinErrors == nil || !*hs.PinErrors {
		t.Errorf("PinErrors should be true so error context stays visible, got %v", hs.PinErrors)
	}
	if hs.PinRecentCount == nil || *hs.PinRecentCount != historyStripPinRecent {
		t.Errorf("PinRecentCount should be %d, got %v", historyStripPinRecent, hs.PinRecentCount)
	}
	if hs.AgeThreshold == nil || *hs.AgeThreshold != historyStripAgeThreshold {
		t.Errorf("AgeThreshold should be %d, got %v", historyStripAgeThreshold, hs.AgeThreshold)
	}
}

func TestApplyHistoryStripDefault_RespectsAuthorOverride(t *testing.T) {
	// Author-specified HistoryStrip wins — we never overwrite it. This also
	// covers the "explicit disable" case: author sets Enabled=false to opt
	// out, and we leave it alone.
	customEnabled := false
	custom := &bridgepkg.HistoryStripConfig{Enabled: &customEnabled}
	cfg := &bridgepkg.AgentConfig{HistoryStrip: custom}
	applyHistoryStripDefault(cfg)

	if cfg.HistoryStrip != custom {
		t.Errorf("author-supplied HistoryStrip was overwritten")
	}
	if cfg.HistoryStrip.Enabled == nil || *cfg.HistoryStrip.Enabled {
		t.Errorf("author's Enabled=false was flipped to true")
	}
}

func TestApplyHistoryStripDefault_NilConfigIsNoOp(t *testing.T) {
	applyHistoryStripDefault(nil) // must not panic
}
