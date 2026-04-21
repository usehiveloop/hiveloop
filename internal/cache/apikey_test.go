package cache_test

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/usehiveloop/hiveloop/internal/cache"
)

func TestAPIKeyCache_SetAndGet(t *testing.T) {
	c := cache.NewAPIKeyCache(100, 5*time.Minute)

	id := uuid.New()
	orgID := uuid.New()
	entry := &cache.CachedAPIKey{
		ID:    id,
		OrgID: orgID,
		Scopes: []string{"connect", "credentials"},
	}

	c.Set("hash-abc", entry)

	got, ok := c.Get("hash-abc")
	if !ok {
		t.Fatal("expected cache hit")
	}
	if got.ID != id {
		t.Fatalf("expected ID %s, got %s", id, got.ID)
	}
	if got.OrgID != orgID {
		t.Fatalf("expected OrgID %s, got %s", orgID, got.OrgID)
	}
	if len(got.Scopes) != 2 || got.Scopes[0] != "connect" || got.Scopes[1] != "credentials" {
		t.Fatalf("unexpected scopes: %v", got.Scopes)
	}
}

func TestAPIKeyCache_Miss(t *testing.T) {
	c := cache.NewAPIKeyCache(100, 5*time.Minute)

	_, ok := c.Get("nonexistent")
	if ok {
		t.Fatal("expected cache miss for nonexistent key")
	}
}

func TestAPIKeyCache_Invalidate(t *testing.T) {
	c := cache.NewAPIKeyCache(100, 5*time.Minute)

	c.Set("hash-abc", &cache.CachedAPIKey{
		ID:     uuid.New(),
		OrgID:  uuid.New(),
		Scopes: []string{"all"},
	})

	c.Invalidate("hash-abc")

	_, ok := c.Get("hash-abc")
	if ok {
		t.Fatal("expected cache miss after invalidation")
	}
}

func TestAPIKeyCache_ExpiredKey(t *testing.T) {
	c := cache.NewAPIKeyCache(100, 5*time.Minute)

	expired := time.Now().Add(-time.Hour)
	c.Set("hash-expired", &cache.CachedAPIKey{
		ID:        uuid.New(),
		OrgID:     uuid.New(),
		Scopes:    []string{"all"},
		ExpiresAt: &expired,
	})

	_, ok := c.Get("hash-expired")
	if ok {
		t.Fatal("expected cache miss for expired API key")
	}
}

func TestAPIKeyCache_NonExpiredKey(t *testing.T) {
	c := cache.NewAPIKeyCache(100, 5*time.Minute)

	future := time.Now().Add(24 * time.Hour)
	c.Set("hash-valid", &cache.CachedAPIKey{
		ID:        uuid.New(),
		OrgID:     uuid.New(),
		Scopes:    []string{"tokens"},
		ExpiresAt: &future,
	})

	got, ok := c.Get("hash-valid")
	if !ok {
		t.Fatal("expected cache hit for non-expired key")
	}
	if got.ExpiresAt == nil || !got.ExpiresAt.Equal(future) {
		t.Fatal("expected ExpiresAt to be preserved")
	}
}

func TestAPIKeyCache_NilExpiresAt(t *testing.T) {
	c := cache.NewAPIKeyCache(100, 5*time.Minute)

	c.Set("hash-forever", &cache.CachedAPIKey{
		ID:        uuid.New(),
		OrgID:     uuid.New(),
		Scopes:    []string{"all"},
		ExpiresAt: nil,
	})

	_, ok := c.Get("hash-forever")
	if !ok {
		t.Fatal("expected cache hit for key with nil ExpiresAt (never expires)")
	}
}

func TestAPIKeyCache_Purge(t *testing.T) {
	c := cache.NewAPIKeyCache(100, 5*time.Minute)

	for i := range 10 {
		c.Set("hash-"+string(rune('a'+i)), &cache.CachedAPIKey{
			ID:     uuid.New(),
			OrgID:  uuid.New(),
			Scopes: []string{"all"},
		})
	}

	c.Purge()

	for i := range 10 {
		_, ok := c.Get("hash-" + string(rune('a'+i)))
		if ok {
			t.Fatalf("expected cache miss after purge for key %d", i)
		}
	}
}

func TestAPIKeyCache_LRUEviction(t *testing.T) {
	c := cache.NewAPIKeyCache(2, 5*time.Minute) // max 2 entries

	c.Set("hash-1", &cache.CachedAPIKey{ID: uuid.New(), OrgID: uuid.New(), Scopes: []string{"all"}})
	c.Set("hash-2", &cache.CachedAPIKey{ID: uuid.New(), OrgID: uuid.New(), Scopes: []string{"all"}})
	c.Set("hash-3", &cache.CachedAPIKey{ID: uuid.New(), OrgID: uuid.New(), Scopes: []string{"all"}})

	// hash-1 should be evicted
	_, ok := c.Get("hash-1")
	if ok {
		t.Fatal("expected hash-1 to be evicted (LRU)")
	}

	// hash-2 and hash-3 should still be present
	_, ok = c.Get("hash-2")
	if !ok {
		t.Fatal("expected hash-2 to still be cached")
	}
	_, ok = c.Get("hash-3")
	if !ok {
		t.Fatal("expected hash-3 to still be cached")
	}
}
