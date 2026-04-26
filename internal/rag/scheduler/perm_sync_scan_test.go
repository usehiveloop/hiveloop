package scheduler_test

import (
	"testing"

	ragmodel "github.com/usehiveloop/hiveloop/internal/rag/model"
	"github.com/usehiveloop/hiveloop/internal/rag/scheduler"
	ragtasks "github.com/usehiveloop/hiveloop/internal/rag/tasks"
)

func TestPermSyncScan_OnlyForPermSyncCapableKinds(t *testing.T) {
	f := setupScheduler(t)
	freq := 60
	srcWith := f.makeSource(t,
		withKind("INTEGRATION"),
		withAccessType(ragmodel.AccessTypeSync),
		withPermSyncFreq(&freq),
		withLastPermSync(minutesAgo(60)),
	)
	srcWithout := f.makeSource(t,
		withKind("WEBSITE"),
		withAccessType(ragmodel.AccessTypeSync),
		withPermSyncFreq(&freq),
		withLastPermSync(minutesAgo(60)),
	)

	supports := func(kind string) bool { return kind == "INTEGRATION" }

	n, err := scheduler.ScanPermSyncDue(ctxBg(), f.DB, f.Enq, f.Cfg, supports)
	if err != nil {
		t.Fatalf("ScanPermSyncDue: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected exactly 1 enqueue, got %d (with=%s without=%s)",
			n, srcWith.ID, srcWithout.ID)
	}
	types := f.pendingTaskTypes(t)
	if len(types) != 1 || types[0] != ragtasks.TypeRagPermSync {
		t.Fatalf("queue contents = %v, want [%s]", types, ragtasks.TypeRagPermSync)
	}
}
