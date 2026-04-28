package system

import (
	"context"
	"sync"
	"time"
)

// MemCache is an in-memory Cache implementation, intended for tests. The
// production cache is RedisCache; do not wire MemCache into the binary.
type MemCache struct {
	mu   sync.Mutex
	data map[string]CompletionResult
}

func NewMemCache() *MemCache { return &MemCache{data: map[string]CompletionResult{}} }

func (m *MemCache) Get(_ context.Context, key string) (*CompletionResult, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.data[key]
	if !ok {
		return nil, false, nil
	}
	cp := v
	return &cp, true, nil
}

func (m *MemCache) Set(_ context.Context, key string, val *CompletionResult, ttl time.Duration) error {
	if ttl <= 0 {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[key] = *val
	return nil
}

// Len returns the number of entries (for test assertions).
func (m *MemCache) Len() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.data)
}
