package sandbox

import "testing"

// TODO(wave-2): These tests covered the AgentConfig.ToolRequirements pipeline
// (memory_recall / memory_retain / journal_write cadence enforcement). The
// new ACP-harness OpenAPI dropped the field. Re-enable once Wave 2 puts the
// memory/journal loop back together — likely via a bridge-side preprocessor
// or an integration MCP tool.

func TestApplyToolRequirementsDefault_AutoInjectsMemoryAndJournalLoop(t *testing.T) {
	t.Skip("re-enable after Wave 2 pusher rewrite")
}

func TestApplyToolRequirementsDefault_MemoryRecallIsTurnStart(t *testing.T) {
	t.Skip("re-enable after Wave 2 pusher rewrite")
}

func TestApplyToolRequirementsDefault_MemoryRetainAndJournalAreAnywhere(t *testing.T) {
	t.Skip("re-enable after Wave 2 pusher rewrite")
}

func TestApplyToolRequirementsDefault_CadenceIsEveryNTurnsWithDefault(t *testing.T) {
	t.Skip("re-enable after Wave 2 pusher rewrite")
}

func TestApplyToolRequirementsDefault_CadencesAreIndependentPointers(t *testing.T) {
	t.Skip("re-enable after Wave 2 pusher rewrite")
}

func TestApplyToolRequirementsDefault_RespectsAuthorOverride(t *testing.T) {
	t.Skip("re-enable after Wave 2 pusher rewrite")
}

func TestApplyToolRequirementsDefault_SkipsToolsInDisabledTools(t *testing.T) {
	t.Skip("re-enable after Wave 2 pusher rewrite")
}

func TestApplyToolRequirementsDefault_SkipsToolsDeniedViaPermissions(t *testing.T) {
	t.Skip("re-enable after Wave 2 pusher rewrite")
}

func TestApplyToolRequirementsDefault_AllDefaultsDisabledLeavesFieldNil(t *testing.T) {
	t.Skip("re-enable after Wave 2 pusher rewrite")
}

func TestApplyToolRequirementsDefault_EnforcementIsWarn(t *testing.T) {
	t.Skip("re-enable after Wave 2 pusher rewrite")
}

func TestApplyToolRequirementsDefault_DefaultCadenceIs10(t *testing.T) {
	t.Skip("re-enable after Wave 2 pusher rewrite")
}

func TestApplyToolRequirementsDefault_NilConfigIsNoOp(t *testing.T) {
	t.Skip("re-enable after Wave 2 pusher rewrite")
}
