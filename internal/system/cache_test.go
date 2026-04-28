package system

import (
	"context"
	"testing"
	"time"
)

func TestCacheKey_Stable(t *testing.T) {
	task := validTask("k")
	args := map[string]any{"a": "x", "b": float64(7)}
	k1, err := CacheKey(task, "model-a", args)
	if err != nil {
		t.Fatalf("k1: %v", err)
	}
	k2, err := CacheKey(task, "model-a", args)
	if err != nil {
		t.Fatalf("k2: %v", err)
	}
	if k1 != k2 {
		t.Fatalf("repeated calls produced different keys")
	}
}

func TestCacheKey_OrderInsensitive(t *testing.T) {
	task := validTask("k")
	a := map[string]any{"shape": "haiku", "topic": "go"}
	b := map[string]any{"topic": "go", "shape": "haiku"}
	ka, _ := CacheKey(task, "m", a)
	kb, _ := CacheKey(task, "m", b)
	if ka != kb {
		t.Fatalf("argument order changed key:\n  a=%s\n  b=%s", ka, kb)
	}
}

func TestCacheKey_VersionBumpDifferent(t *testing.T) {
	task := validTask("k")
	args := map[string]any{"shape": "haiku"}
	k1, _ := CacheKey(task, "m", args)
	task.Version = "v2"
	k2, _ := CacheKey(task, "m", args)
	if k1 == k2 {
		t.Fatalf("Version bump must change cache key")
	}
}

func TestCacheKey_DifferentTaskDifferent(t *testing.T) {
	a := validTask("alpha")
	b := validTask("beta")
	args := map[string]any{}
	ka, _ := CacheKey(a, "m", args)
	kb, _ := CacheKey(b, "m", args)
	if ka == kb {
		t.Fatalf("different tasks must hash to different keys")
	}
}

func TestCacheKey_DifferentModelDifferent(t *testing.T) {
	task := validTask("k")
	args := map[string]any{}
	k1, _ := CacheKey(task, "m1", args)
	k2, _ := CacheKey(task, "m2", args)
	if k1 == k2 {
		t.Fatalf("different model must change key")
	}
}

func TestMemCache_RoundTrip(t *testing.T) {
	c := NewMemCache()
	ctx := context.Background()

	if _, ok, _ := c.Get(ctx, "x"); ok {
		t.Fatalf("empty cache returned hit")
	}
	want := &CompletionResult{Text: "hi", Model: "m"}
	if err := c.Set(ctx, "x", want, time.Minute); err != nil {
		t.Fatalf("set: %v", err)
	}
	got, ok, err := c.Get(ctx, "x")
	if err != nil || !ok {
		t.Fatalf("expected hit, got ok=%v err=%v", ok, err)
	}
	if got.Text != "hi" || got.Model != "m" {
		t.Fatalf("round-trip mismatch: %+v", got)
	}
}

func TestMemCache_TTLZeroNoStore(t *testing.T) {
	c := NewMemCache()
	ctx := context.Background()
	_ = c.Set(ctx, "k", &CompletionResult{Text: "x"}, 0)
	if _, ok, _ := c.Get(ctx, "k"); ok {
		t.Fatalf("TTL=0 must not store")
	}
}
