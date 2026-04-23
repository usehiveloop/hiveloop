package streaming

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"
)

func TestSubscribe_SingleTapPerConversation(t *testing.T) {
	rc := setupTestRedis(t)
	bus := NewEventBus(rc)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	const numSubs = 5
	chs := make([]<-chan StreamEvent, numSubs)
	for i := range chs {
		chs[i] = bus.Subscribe(ctx, "shared-conv", "$")
	}

	time.Sleep(200 * time.Millisecond)

	if got := bus.ActiveTaps(); got != 1 {
		t.Fatalf("expected 1 active tap for the shared conversation, got %d", got)
	}

	bus.Publish(ctx, "shared-conv", "shared_event", json.RawMessage(`{"n":1}`))

	var wg sync.WaitGroup
	received := make([]string, numSubs)
	for i, ch := range chs {
		wg.Add(1)
		go func(idx int, c <-chan StreamEvent) {
			defer wg.Done()
			select {
			case ev := <-c:
				received[idx] = ev.EventType
			case <-time.After(5 * time.Second):
				t.Errorf("subscriber %d timed out", idx)
			}
		}(i, ch)
	}
	wg.Wait()

	for i, got := range received {
		if got != "shared_event" {
			t.Errorf("subscriber %d: got %q, want shared_event", i, got)
		}
	}
}

func TestSubscribe_TapTornDownWhenLastSubscriberLeaves(t *testing.T) {
	rc := setupTestRedis(t)
	bus := NewEventBus(rc)

	ctx1, cancel1 := context.WithCancel(context.Background())
	ctx2, cancel2 := context.WithCancel(context.Background())

	_ = bus.Subscribe(ctx1, "teardown-test", "$")
	_ = bus.Subscribe(ctx2, "teardown-test", "$")

	time.Sleep(150 * time.Millisecond)
	if got := bus.ActiveTaps(); got != 1 {
		t.Fatalf("expected 1 tap, got %d", got)
	}

	cancel1()
	time.Sleep(100 * time.Millisecond)
	if got := bus.ActiveTaps(); got != 1 {
		t.Fatalf("tap should still be alive while one sub remains, got %d", got)
	}

	cancel2()

	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		if bus.ActiveTaps() == 0 {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("tap not torn down after last subscriber left, taps=%d", bus.ActiveTaps())
}
