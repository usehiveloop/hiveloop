package scheduler

import (
	"os"
	"strconv"
	"time"
)

// Cadences and tunables. Defaults mirror Onyx's beat schedule
// (backend/onyx/background/celery/tasks/beat_schedule.py:68-200) except
// where annotated. Every value is overridable via environment variable so
// the cloud deployment can tighten cadences without a code change.
const (
	defaultIngestTick    = 15 * time.Second
	defaultPermSyncTick  = 30 * time.Second
	defaultPruneTick     = 60 * time.Second
	defaultWatchdogTick  = 60 * time.Second // Onyx runs this every 5 min; we
	// run it more often because our watchdog is the only crash-recovery path.

	// defaultWatchdogTimeout — how long an in-progress attempt may sit
	// without a heartbeat update before the watchdog fails it. Matches
	// Onyx's STUCK_INDEXING_TIMEOUT_SEC default at
	// backend/onyx/configs/app_configs.py.
	defaultWatchdogTimeout = 30 * time.Minute

	// defaultUniqueSlack — extra TTL beyond a source's tick interval
	// applied to asynq.Unique so a rare scheduler skew does not let two
	// scans co-exist with the same payload.
	defaultUniqueSlack = 30 * time.Second

	// defaultEnqueueLimit caps the number of sources processed in a
	// single scan tick. Even at thousands of sources, the scheduler
	// stays bounded — the next tick picks up where this one left off.
	defaultEnqueueLimit = 500
)

// Config bundles the scan + worker tunables. Built from the environment
// in NewConfig; tests construct it directly so they can drive cadences
// faster than 15s.
type Config struct {
	IngestTick      time.Duration
	PermSyncTick    time.Duration
	PruneTick       time.Duration
	WatchdogTick    time.Duration
	WatchdogTimeout time.Duration
	UniqueSlack     time.Duration
	EnqueueLimit    int
}

// NewConfig reads RAG_*_TICK / RAG_WATCHDOG_TIMEOUT environment overrides
// and falls back to the documented defaults.
func NewConfig() Config {
	return Config{
		IngestTick:      envDuration("RAG_INGEST_TICK", defaultIngestTick),
		PermSyncTick:    envDuration("RAG_PERM_SYNC_TICK", defaultPermSyncTick),
		PruneTick:       envDuration("RAG_PRUNE_TICK", defaultPruneTick),
		WatchdogTick:    envDuration("RAG_WATCHDOG_TICK", defaultWatchdogTick),
		WatchdogTimeout: envDuration("RAG_WATCHDOG_TIMEOUT", defaultWatchdogTimeout),
		UniqueSlack:     envDuration("RAG_UNIQUE_SLACK", defaultUniqueSlack),
		EnqueueLimit:    envInt("RAG_ENQUEUE_LIMIT", defaultEnqueueLimit),
	}
}

func envDuration(key string, def time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil || d <= 0 {
		return def
	}
	return d
}

func envInt(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return def
	}
	return n
}
