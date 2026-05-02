package sandbox

import "testing"

// TODO(wave-2): These tests covered the AgentConfig.Immortal pipeline
// (token-budget retention, journal_read/journal_write exposure, parent
// context-window math). The new ACP-harness OpenAPI dropped the field.
// Re-enable once Wave 2 reintroduces an equivalent feature.

func TestApplyImmortalDefault_TokenBudgetIs50PctOfParentContextWindow(t *testing.T) {
	t.Skip("re-enable after Wave 2 pusher rewrite")
}

func TestApplyImmortalDefault_AppliesRetentionAndEvictionWindow(t *testing.T) {
	t.Skip("re-enable after Wave 2 pusher rewrite")
}

func TestApplyImmortalDefault_ExposeJournalToolsTrueByDefault(t *testing.T) {
	t.Skip("re-enable after Wave 2 pusher rewrite")
}

func TestApplyImmortalDefault_ExposeJournalToolsRespectsDenial(t *testing.T) {
	t.Skip("re-enable after Wave 2 pusher rewrite")
}

func TestApplyImmortalDefault_ExposeJournalToolsRespectsDisabledTools(t *testing.T) {
	t.Skip("re-enable after Wave 2 pusher rewrite")
}

func TestApplyImmortalDefault_PartialJournalDenialKeepsExposureOn(t *testing.T) {
	t.Skip("re-enable after Wave 2 pusher rewrite")
}

func TestApplyImmortalDefault_RespectsAuthorOverride(t *testing.T) {
	t.Skip("re-enable after Wave 2 pusher rewrite")
}

func TestApplyImmortalDefault_NilConfigIsNoOp(t *testing.T) {
	t.Skip("re-enable after Wave 2 pusher rewrite")
}
