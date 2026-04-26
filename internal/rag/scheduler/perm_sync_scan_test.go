package scheduler_test

import (
	"testing"

	ragmodel "github.com/usehiveloop/hiveloop/internal/rag/model"
	"github.com/usehiveloop/hiveloop/internal/rag/scheduler"
	ragtasks "github.com/usehiveloop/hiveloop/internal/rag/tasks"
)

// TestPermSyncScan_OnlyForPermSyncCapableKinds: when two sources of
// different kinds are eligible, only the one whose kind reports
// PermSyncConnector capability gets a perm_sync task. The other is
// skipped silently — its kind is not registered to the perm-sync set.
func TestPermSyncScan_OnlyForPermSyncCapableKinds(t *testing.T) {
	f := setupScheduler(t)
	freq := 60
	// Two sources, distinct kinds. Both are perm-sync due.
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

	// Inject a CapabilityCheck that says only INTEGRATION supports
	// perm-sync. This is the production seam HasPermSyncCapability
	// fills in the worker.
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
