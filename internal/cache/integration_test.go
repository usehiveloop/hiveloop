package cache_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/awnumar/memguard"
	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/cache"
)

func TestMemoryCache_SetGetRoundtrip(t *testing.T) {
	mc := cache.NewMemoryCache(100, 5*time.Minute)

	apiKey := []byte("sk-test-key-12345")
	enclave := memguard.NewEnclave(apiKey)
	mc.Set("cred-1", &cache.CachedCredential{
		Enclave:    enclave,
		BaseURL:    "https://api.openai.com",
		AuthScheme: "bearer",
		OrgID:      uuid.New(),
		CachedAt:   time.Now(),
		HardExpiry: time.Now().Add(time.Hour),
	})

	got, ok := mc.Get("cred-1")
	if !ok {
		t.Fatal("expected L1 cache hit")
	}
	buf, err := got.Enclave.Open()
	if err != nil {
		t.Fatalf("open enclave: %v", err)
	}
	if string(buf.Bytes()) != "sk-test-key-12345" {
		t.Fatalf("expected 'sk-test-key-12345', got %q", buf.Bytes())
	}
	buf.Destroy()
}

func TestMemoryCache_Miss(t *testing.T) {
	mc := cache.NewMemoryCache(100, 5*time.Minute)
	_, ok := mc.Get("nonexistent")
	if ok {
		t.Fatal("expected cache miss")
	}
}

func TestMemoryCache_Invalidate(t *testing.T) {
	mc := cache.NewMemoryCache(100, 5*time.Minute)
	mc.Set("cred-1", &cache.CachedCredential{
		Enclave:    memguard.NewEnclave([]byte("key")),
		HardExpiry: time.Now().Add(time.Hour),
	})
	mc.Invalidate("cred-1")
	_, ok := mc.Get("cred-1")
	if ok {
		t.Fatal("expected miss after invalidation")
	}
}

func TestMemoryCache_HardExpiry(t *testing.T) {
	mc := cache.NewMemoryCache(100, time.Hour)
	mc.Set("cred-1", &cache.CachedCredential{
		Enclave:    memguard.NewEnclave([]byte("key")),
		HardExpiry: time.Now().Add(-time.Second),
	})
	_, ok := mc.Get("cred-1")
	if ok {
		t.Fatal("expected miss for expired hard-expiry")
	}
}

func TestMemoryCache_Purge(t *testing.T) {
	mc := cache.NewMemoryCache(100, 5*time.Minute)
	for i := range 10 {
		mc.Set(fmt.Sprintf("cred-%d", i), &cache.CachedCredential{
			Enclave:    memguard.NewEnclave([]byte("key")),
			HardExpiry: time.Now().Add(time.Hour),
		})
	}
	if mc.Len() != 10 {
		t.Fatalf("expected 10 entries, got %d", mc.Len())
	}
	mc.Purge()
	if mc.Len() != 0 {
		t.Fatalf("expected 0 entries after purge, got %d", mc.Len())
	}
}
