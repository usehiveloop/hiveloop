package goroutine_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/usehiveloop/hiveloop/internal/goroutine"
)

func TestGo_RecoversPanic(t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(1)

	goroutine.Go(context.Background(), func(context.Context) {
		defer wg.Done()
		panic("test panic")
	})

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Goroutine completed without crashing the process — panic was recovered.
	case <-time.After(2 * time.Second):
		t.Fatal("goroutine did not complete — panic recovery may have failed")
	}
}

func TestGo_RunsNormally(t *testing.T) {
	done := make(chan int, 1)

	goroutine.Go(context.Background(), func(context.Context) {
		done <- 42
	})

	select {
	case v := <-done:
		if v != 42 {
			t.Fatalf("expected 42, got %d", v)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("goroutine did not complete")
	}
}

func TestGo_PassesContextToFn(t *testing.T) {
	type key struct{}
	ctx := context.WithValue(context.Background(), key{}, "propagated")

	done := make(chan string, 1)
	goroutine.Go(ctx, func(inner context.Context) {
		v, _ := inner.Value(key{}).(string)
		done <- v
	})

	select {
	case v := <-done:
		if v != "propagated" {
			t.Fatalf("expected ctx value propagated, got %q", v)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("goroutine did not complete")
	}
}
