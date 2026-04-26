package tasks

import (
	"time"

	"github.com/hibiken/asynq"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/nango"
	"github.com/usehiveloop/hiveloop/internal/rag/ragclient"
)

// Deps bundles the dependencies the per-source handlers need. All fields
// must be non-nil at registration time.
type Deps struct {
	DB        *gorm.DB
	RagClient *ragclient.Client
	Nango     *nango.Client

	// HeartbeatTick is the cadence at which the in-progress attempt's
	// last_heartbeat_time / last_progress_time columns are touched.
	// Defaults to 30s; the watchdog must be configured to tolerate at
	// least 2× this value.
	HeartbeatTick time.Duration

	// BatchSize is the soft cap on documents per ragclient.IngestBatch
	// call. The Rust server enforces a hard cap; this stays under it.
	BatchSize int

	// DatasetName is the LanceDB dataset every IngestBatch is routed to.
	// Sourced from the active embedding-model registry entry by the
	// caller (worker.go) so the scheduler doesn't import the embedder
	// package.
	DatasetName string

	// DeclaredVectorDim mirrors DatasetName — the vector dimension the
	// active embedding model produces. Validated server-side.
	DeclaredVectorDim uint32
}

// RegisterHandlers wires the per-source handlers onto the supplied mux.
// Called from internal/tasks/registry.go.
func RegisterHandlers(mux *asynq.ServeMux, deps *Deps) {
	if deps == nil {
		return
	}
	mux.HandleFunc(TypeRagIngest, deps.HandleIngest)
	mux.HandleFunc(TypeRagPermSync, deps.HandlePermSync)
	mux.HandleFunc(TypeRagPrune, deps.HandlePrune)
}

// defaults returns the same Deps with zero-valued tunables filled in.
func (d *Deps) withDefaults() *Deps {
	c := *d
	if c.HeartbeatTick <= 0 {
		c.HeartbeatTick = 30 * time.Second
	}
	if c.BatchSize <= 0 {
		c.BatchSize = 500
	}
	return &c
}
