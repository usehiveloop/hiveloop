package sandbox

import "testing"

// TODO(wave-2): These tests covered the AgentConfig.HistoryStrip pipeline
// (PinErrors, PinRecentCount, AgeThreshold). The new ACP-harness OpenAPI
// dropped the field entirely. Re-enable once Wave 2 reintroduces an
// equivalent feature on the harness side.

func TestApplyHistoryStripDefault_PopulatesWhenAbsent(t *testing.T) {
	t.Skip("re-enable after Wave 2 pusher rewrite")
}

func TestApplyHistoryStripDefault_RespectsAuthorOverride(t *testing.T) {
	t.Skip("re-enable after Wave 2 pusher rewrite")
}

func TestApplyHistoryStripDefault_NilConfigIsNoOp(t *testing.T) {
	t.Skip("re-enable after Wave 2 pusher rewrite")
}
