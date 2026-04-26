package scheduler

import (
	"os"
	"strconv"
	"time"
)

const (
	defaultIngestTick      = 15 * time.Second
	defaultPermSyncTick    = 30 * time.Second
	defaultPruneTick       = 60 * time.Second
	defaultWatchdogTick    = 60 * time.Second
	defaultWatchdogTimeout = 30 * time.Minute
	defaultUniqueSlack     = 30 * time.Second
	defaultEnqueueLimit    = 500
)

type Config struct {
	IngestTick      time.Duration
	PermSyncTick    time.Duration
	PruneTick       time.Duration
	WatchdogTick    time.Duration
	WatchdogTimeout time.Duration
	UniqueSlack     time.Duration
	EnqueueLimit    int
}

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
