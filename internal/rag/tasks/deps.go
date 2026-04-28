package tasks

import (
	"time"

	"github.com/hibiken/asynq"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/billing"
	"github.com/usehiveloop/hiveloop/internal/nango"
	"github.com/usehiveloop/hiveloop/internal/rag/embedclient"
	"github.com/usehiveloop/hiveloop/internal/rag/qdrant"
	"github.com/usehiveloop/hiveloop/internal/spider"
)

type Deps struct {
	DB         *gorm.DB
	Qdrant     *qdrant.Client
	Embedder   *embedclient.Embedder
	Nango      *nango.Client
	Spider     *spider.Client
	Credits    *billing.CreditsService
	Collection string

	// HeartbeatTick: the watchdog timeout must be at least 2× this value.
	HeartbeatTick time.Duration
	BatchSize     int
}

func RegisterHandlers(mux *asynq.ServeMux, deps *Deps) {
	if deps == nil {
		return
	}
	mux.HandleFunc(TypeRagIngest, deps.HandleIngest)
	mux.HandleFunc(TypeRagPermSync, deps.HandlePermSync)
	mux.HandleFunc(TypeRagPrune, deps.HandlePrune)
}

func (d *Deps) withDefaults() *Deps {
	c := *d
	if c.HeartbeatTick <= 0 {
		c.HeartbeatTick = 30 * time.Second
	}
	if c.BatchSize <= 0 {
		c.BatchSize = 100
	}
	return &c
}
