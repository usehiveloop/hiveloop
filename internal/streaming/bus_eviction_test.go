package streaming

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"
	"time"
)

func TestSubscribe_ContextCancelClosesChannel(t *testing.T) {
	rc := setupTestRedis(t)
	bus := NewEventBus(rc)

	ctxA, cancelA := context.WithCancel(context.Background())
	ctxB, cancelB := context.WithCancel(context.Background())
	defer cancelB()

	chA := bus.Subscribe(ctxA, "cancel-one", "$")
	chB := bus.Subscribe(ctxB, "cancel-one", "$")
	time.Sleep(150 * time.Millisecond)

	cancelA()

	select {
	case _, ok := <-chA:
		if ok {
			for {
				select {
				case _, ok := <-chA:
					if !ok {
						goto closed
					}
				case <-time.After(5 * time.Second):
					t.Fatal("A channel did not close after cancel")
				}
			}
		}
	case <-time.After(5 * time.Second):
		t.Fatal("A channel did not close after cancel")
	}
closed:

	bus.Publish(context.Background(), "cancel-one", "still-alive", json.RawMessage(`{}`))
	select {
	case ev, ok := <-chB:
		if !ok {
			t.Fatal("B channel closed unexpectedly")
		}
		if ev.EventType != "still-alive" {
			t.Errorf("B got wrong event: %q", ev.EventType)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("B did not receive event")
	}
}

func TestSubscribe_SlowConsumerEviction(t *testing.T) {
	rc := setupTestRedis(t)
	bus := NewEventBus(rc)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	slow := bus.Subscribe(ctx, "slow-test", "$")
	fast := bus.Subscribe(ctx, "slow-test", "$")
	time.Sleep(150 * time.Millisecond)

	var fastCount atomic.Int64
	fastDone := make(chan struct{})
	go func() {
		defer close(fastDone)
		for {
			select {
			case _, ok := <-fast:
				if !ok {
					return
				}
				fastCount.Add(1)
			case <-ctx.Done():
				return
			}
		}
	}()

	for i := 0; i < 300; i++ {
		bus.Publish(ctx, "slow-test", "event", json.RawMessage(`{}`))
	}

	time.Sleep(1 * time.Second)

	evicted := false
	drainDeadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(drainDeadline) {
		select {
		case _, ok := <-slow:
			if !ok {
				evicted = true
				goto doneDrain
			}
		case <-time.After(200 * time.Millisecond):
			goto doneDrain
		}
	}
doneDrain:
	if !evicted {
		t.Logf("slow subscriber not evicted (ok); fast received %d events", fastCount.Load())
	}
	if fastCount.Load() == 0 {
		t.Fatal("fast subscriber received 0 events — tap was stalled by slow consumer")
	}
}

func TestBackfill_SkipsAnchor(t *testing.T) {
	rc := setupTestRedis(t)
	bus := NewEventBus(rc)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var ids []string
	for i := 0; i < 3; i++ {
		id, _ := bus.Publish(ctx, "anchor-test", "e", json.RawMessage(`{}`))
		ids = append(ids, id)
	}

	ch := bus.Subscribe(ctx, "anchor-test", ids[0])

	var got []string
	for len(got) < 2 {
		select {
		case ev := <-ch:
			got = append(got, ev.ID)
		case <-time.After(2 * time.Second):
			t.Fatalf("timeout: got %v", got)
		}
	}

	if got[0] == ids[0] {
		t.Errorf("anchor event %s was included — should be skipped", ids[0])
	}
	if got[0] != ids[1] || got[1] != ids[2] {
		t.Errorf("wrong IDs: got %v, want [%s %s]", got, ids[1], ids[2])
	}
}
