package goroutine_test

import (
	"sync"
	"testing"
	"time"

	"github.com/usehiveloop/hiveloop/internal/goroutine"
)

func TestGo_RecoversPanic(t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(1)

	goroutine.Go(func() {
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

	goroutine.Go(func() {
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
