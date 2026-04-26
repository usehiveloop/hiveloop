package scheduler_test

import (
	"testing"

	"github.com/usehiveloop/hiveloop/internal/rag/scheduler"
	ragtasks "github.com/usehiveloop/hiveloop/internal/rag/tasks"
)

// TestPruneScan_GatedByLastPruned: a source whose last_pruned +
// prune_freq_seconds is in the past gets a rag:prune task; one that
// was just pruned does not. CapabilityCheck is bypassed via an
// always-true probe so we exercise only the timing predicate.
func TestPruneScan_GatedByLastPruned(t *testing.T) {
	f := setupScheduler(t)
	freq := 60
	// Due: last_pruned 10m ago.
	due := f.makeSource(t,
		withPruneFreq(&freq),
		withLastPruned(minutesAgo(10)),
	)
	// Not due: last_pruned just now-ish.
	_ = f.makeSource(t,
		withPruneFreq(&freq),
		withLastPruned(minutesAgo(0)),
	)
	// No prune freq: must be skipped regardless.
	_ = f.makeSource(t,
		withPruneFreq(nil),
		withLastPruned(minutesAgo(120)),
	)

	supports := func(_ string) bool { return true }

	n, err := scheduler.ScanPruneDue(ctxBg(), f.DB, f.Enq, f.Cfg, supports)
	if err != nil {
		t.Fatalf("ScanPruneDue: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1 enqueue, got %d (due source = %s)", n, due.ID)
	}
	types := f.pendingTaskTypes(t)
	if len(types) != 1 || types[0] != ragtasks.TypeRagPrune {
		t.Fatalf("queue contents = %v, want [%s]", types, ragtasks.TypeRagPrune)
	}
}
