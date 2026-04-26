package scheduler_test

import (
	"testing"

	"github.com/usehiveloop/hiveloop/internal/rag/scheduler"
	ragtasks "github.com/usehiveloop/hiveloop/internal/rag/tasks"
)

func TestPruneScan_GatedByLastPruned(t *testing.T) {
	f := setupScheduler(t)
	freq := 60
	due := f.makeSource(t,
		withPruneFreq(&freq),
		withLastPruned(minutesAgo(10)),
	)
	_ = f.makeSource(t,
		withPruneFreq(&freq),
		withLastPruned(minutesAgo(0)),
	)
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
