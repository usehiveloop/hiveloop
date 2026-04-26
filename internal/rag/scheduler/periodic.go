package scheduler

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/hibiken/asynq"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/enqueue"
	ragtasks "github.com/usehiveloop/hiveloop/internal/rag/tasks"
)

// Deps holds the dependencies the periodic scan handlers need. The
// asynq mux receives one Handle method per scan task type from this
// struct; the per-source ingest/perm_sync/prune handlers live in the
// internal/rag/tasks package.
type Deps struct {
	DB       *gorm.DB
	Enq      enqueue.TaskEnqueuer
	Cfg      Config
	Supports CapabilityCheck // optional; nil falls back to HasPermSyncCapability / HasSlimCapability per loop
}

// Configs returns the periodic-task registrations for the four scan
// loops. Cronspecs use the @every form so we can drive them at sub-minute
// cadences. The caller appends these to the existing
// internal/tasks.PeriodicTaskConfigs() slice.
func (d *Deps) Configs() []*asynq.PeriodicTaskConfig {
	cfg := d.Cfg
	return []*asynq.PeriodicTaskConfig{
		{
			Cronspec: fmt.Sprintf("@every %s", cfg.IngestTick),
			Task:     asynq.NewTask(ragtasks.TypeRagScanIngestDue, nil),
			Opts: []asynq.Option{
				asynq.Queue(ragtasks.QueueRagWork),
				asynq.MaxRetry(0),
				asynq.Timeout(cfg.IngestTick - 1),
			},
		},
		{
			Cronspec: fmt.Sprintf("@every %s", cfg.PermSyncTick),
			Task:     asynq.NewTask(ragtasks.TypeRagScanPermSyncDue, nil),
			Opts: []asynq.Option{
				asynq.Queue(ragtasks.QueueRagWork),
				asynq.MaxRetry(0),
				asynq.Timeout(cfg.PermSyncTick - 1),
			},
		},
		{
			Cronspec: fmt.Sprintf("@every %s", cfg.PruneTick),
			Task:     asynq.NewTask(ragtasks.TypeRagScanPruneDue, nil),
			Opts: []asynq.Option{
				asynq.Queue(ragtasks.QueueRagWork),
				asynq.MaxRetry(0),
				asynq.Timeout(cfg.PruneTick - 1),
			},
		},
		{
			Cronspec: fmt.Sprintf("@every %s", cfg.WatchdogTick),
			Task:     asynq.NewTask(ragtasks.TypeRagWatchdog, nil),
			Opts: []asynq.Option{
				asynq.Queue(ragtasks.QueueRagWork),
				asynq.MaxRetry(0),
				asynq.Timeout(cfg.WatchdogTick - 1),
			},
		},
	}
}

// Register wires the four scan-task handlers onto the supplied asynq
// mux. Mounted from internal/tasks/registry.go.
func (d *Deps) Register(mux *asynq.ServeMux) {
	mux.HandleFunc(ragtasks.TypeRagScanIngestDue, d.handleScanIngest)
	mux.HandleFunc(ragtasks.TypeRagScanPermSyncDue, d.handleScanPermSync)
	mux.HandleFunc(ragtasks.TypeRagScanPruneDue, d.handleScanPrune)
	mux.HandleFunc(ragtasks.TypeRagWatchdog, d.handleWatchdog)
}

func (d *Deps) handleScanIngest(ctx context.Context, _ *asynq.Task) error {
	n, err := ScanIngestDue(ctx, d.DB, d.Enq, d.Cfg)
	if err != nil {
		slog.Error("rag scheduler: ingest scan", "err", err, "enqueued", n)
		return err
	}
	if n > 0 {
		slog.Info("rag scheduler: ingest scan", "enqueued", n)
	}
	return nil
}

func (d *Deps) handleScanPermSync(ctx context.Context, _ *asynq.Task) error {
	n, err := ScanPermSyncDue(ctx, d.DB, d.Enq, d.Cfg, d.Supports)
	if err != nil {
		slog.Error("rag scheduler: perm_sync scan", "err", err, "enqueued", n)
		return err
	}
	if n > 0 {
		slog.Info("rag scheduler: perm_sync scan", "enqueued", n)
	}
	return nil
}

func (d *Deps) handleScanPrune(ctx context.Context, _ *asynq.Task) error {
	n, err := ScanPruneDue(ctx, d.DB, d.Enq, d.Cfg, d.Supports)
	if err != nil {
		slog.Error("rag scheduler: prune scan", "err", err, "enqueued", n)
		return err
	}
	if n > 0 {
		slog.Info("rag scheduler: prune scan", "enqueued", n)
	}
	return nil
}

func (d *Deps) handleWatchdog(ctx context.Context, _ *asynq.Task) error {
	n, err := ScanStuckAttempts(ctx, d.DB, d.Cfg)
	if err != nil {
		slog.Error("rag scheduler: watchdog", "err", err, "failed_attempts", n)
		return err
	}
	if n > 0 {
		slog.Warn("rag scheduler: watchdog failed stuck attempts", "count", n)
	}
	return nil
}
