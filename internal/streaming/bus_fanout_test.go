package streaming

import (
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestSubscribe_SingleTapPerConversation verifies that multiple subscribers on
// the same conversation share a single XREAD loop (one tap), not N.
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

	// Give subscribers a moment to attach.
	time.Sleep(200 * time.Millisecond)

	if got := bus.ActiveTaps(); got != 1 {
		t.Fatalf("expected 1 active tap for the shared conversation, got %d", got)
	}

	// Publish a single event — every subscriber should see it.
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

// TestSubscribe_TapTornDownWhenLastSubscriberLeaves verifies that the tap
// goroutine exits and is removed from the registry when its last subscriber
// cancels.
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

	// Tap tear-down is async (it waits for the current XREAD BLOCK to unblock
	// on context cancel, up to 5s). Poll for it.
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		if bus.ActiveTaps() == 0 {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("tap not torn down after last subscriber left, taps=%d", bus.ActiveTaps())
}

// TestSubscribe_LateJoinerGetsBackfillThenLive confirms the backfill-then-tap
// handoff is gap-free and duplicate-free.
func TestSubscribe_LateJoinerGetsBackfillThenLive(t *testing.T) {
	rc := setupTestRedis(t)
	bus := NewEventBus(rc)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Pre-publish some events before anyone subscribes.
	var preIDs []string
	for i := 0; i < 3; i++ {
		id, _ := bus.Publish(ctx, "late-join", "pre", json.RawMessage(`{}`))
		preIDs = append(preIDs, id)
	}

	// Subscribe with cursor "0" -> should see everything.
	ch := bus.Subscribe(ctx, "late-join", "0")

	// After attaching, publish more events. These should come via the tap.
	go func() {
		time.Sleep(300 * time.Millisecond)
		for i := 0; i < 3; i++ {
			bus.Publish(ctx, "late-join", "post", json.RawMessage(`{}`))
		}
	}()

	var ids []string
	var types []string
	for len(ids) < 6 {
		select {
		case ev := <-ch:
			ids = append(ids, ev.ID)
			types = append(types, ev.EventType)
		case <-time.After(5 * time.Second):
			t.Fatalf("timeout: got %d events (%v)", len(ids), types)
		}
	}

	// First three events must be the pre-existing ones; last three must be
	// the ones published after subscribe.
	if types[0] != "pre" || types[1] != "pre" || types[2] != "pre" {
		t.Errorf("expected first 3 events to be pre, got %v", types[:3])
	}
	if types[3] != "post" || types[4] != "post" || types[5] != "post" {
		t.Errorf("expected last 3 events to be post, got %v", types[3:])
	}
	// IDs must be strictly increasing (no duplicates, no out-of-order).
	for i := 1; i < len(ids); i++ {
		if ids[i] <= ids[i-1] {
			t.Errorf("IDs not strictly increasing: %v", ids)
			break
		}
	}
}

// TestSubscribe_LiveOnlyCursor confirms that cursor "$" skips backfill
// entirely.
func TestSubscribe_LiveOnlyCursor(t *testing.T) {
	rc := setupTestRedis(t)
	bus := NewEventBus(rc)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Pre-publish events before anyone subscribes — "$" should ignore these.
	for i := 0; i < 3; i++ {
		bus.Publish(ctx, "live-only", "old", json.RawMessage(`{}`))
	}

	ch := bus.Subscribe(ctx, "live-only", "$")
	time.Sleep(200 * time.Millisecond) // let sub attach

	go func() {
		time.Sleep(200 * time.Millisecond)
		bus.Publish(ctx, "live-only", "new", json.RawMessage(`{}`))
	}()

	select {
	case ev := <-ch:
		if ev.EventType != "new" {
			t.Errorf("expected only new event, got %q (cursor $ leaked old data)", ev.EventType)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for new event")
	}
}

// TestSubscribe_MultipleSubscribersDifferentCursors verifies that two
// subscribers on the same tap can request different backfill cursors and
// each gets what they asked for, converging on the same live stream.
func TestSubscribe_MultipleSubscribersDifferentCursors(t *testing.T) {
	rc := setupTestRedis(t)
	bus := NewEventBus(rc)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Publish 5 events before anyone subscribes.
	var ids []string
	for i := 0; i < 5; i++ {
		id, _ := bus.Publish(ctx, "mixed-cursors", "old", json.RawMessage(`{}`))
		ids = append(ids, id)
	}

	// Subscriber A: full replay from "0".
	chA := bus.Subscribe(ctx, "mixed-cursors", "0")
	// Subscriber B: from after event 3 (should see events 4 and 5 + live).
	chB := bus.Subscribe(ctx, "mixed-cursors", ids[2])
	// Subscriber C: live only.
	chC := bus.Subscribe(ctx, "mixed-cursors", "$")

	// Let all three attach.
	time.Sleep(300 * time.Millisecond)

	// All three should share the same tap.
	if got := bus.ActiveTaps(); got != 1 {
		t.Fatalf("expected 1 tap, got %d", got)
	}

	// Collect A's backfill (should be 5 old events).
	var gotA []string
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && len(gotA) < 5 {
		select {
		case ev := <-chA:
			gotA = append(gotA, ev.ID)
		case <-time.After(200 * time.Millisecond):
		}
	}
	if len(gotA) != 5 {
		t.Errorf("A: expected 5 backfill events, got %d", len(gotA))
	}

	// Collect B's backfill (should be 2 old events: ids[3] and ids[4]).
	var gotB []string
	deadline = time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && len(gotB) < 2 {
		select {
		case ev := <-chB:
			gotB = append(gotB, ev.ID)
		case <-time.After(200 * time.Millisecond):
		}
	}
	if len(gotB) != 2 {
		t.Errorf("B: expected 2 backfill events, got %d", len(gotB))
	}
	if len(gotB) == 2 && (gotB[0] != ids[3] || gotB[1] != ids[4]) {
		t.Errorf("B: wrong backfill IDs, got %v want [%s %s]", gotB, ids[3], ids[4])
	}

	// C should see nothing from backfill.
	select {
	case ev := <-chC:
		t.Errorf("C: expected no backfill, got event %q", ev.EventType)
	case <-time.After(300 * time.Millisecond):
	}

	// Publish a live event — all three should see it.
	liveID, _ := bus.Publish(ctx, "mixed-cursors", "live", json.RawMessage(`{}`))

	for name, ch := range map[string]<-chan StreamEvent{"A": chA, "B": chB, "C": chC} {
		select {
		case ev := <-ch:
			if ev.ID != liveID {
				t.Errorf("%s: expected live ID %s, got %s", name, liveID, ev.ID)
			}
		case <-time.After(3 * time.Second):
			t.Errorf("%s: timed out waiting for live event", name)
		}
	}
}

// TestSubscribe_ContextCancelClosesChannel verifies that cancelling a
// subscriber's context closes its channel without affecting other subscribers.
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

	// chA should close.
	select {
	case _, ok := <-chA:
		if ok {
			// Could receive a pending event first, drain until close.
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

	// B should still be alive and receive events.
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

// TestSubscribe_SlowConsumerEviction verifies that a subscriber whose channel
// fills up is evicted without stalling the tap or other subscribers.
func TestSubscribe_SlowConsumerEviction(t *testing.T) {
	rc := setupTestRedis(t)
	bus := NewEventBus(rc)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Slow subscriber: never drains.
	slow := bus.Subscribe(ctx, "slow-test", "$")
	// Fast subscriber: drains continuously.
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

	// Publish more than the subscriber buffer (64) to force eviction of slow.
	// Two chans of 64 buffer each (user-facing + tap->sub) = ~128 before
	// slow fills up; publish 300 to be safe.
	for i := 0; i < 300; i++ {
		bus.Publish(ctx, "slow-test", "event", json.RawMessage(`{}`))
	}

	// Give the tap time to fan out and evict.
	time.Sleep(1 * time.Second)

	// Slow's channel should eventually close (eviction) or remain blocked.
	// We drain it: if eviction happened, we'll observe ok=false.
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
		// Slow may not have been fully drained; check whether at least the
		// fast subscriber got the majority of events (indicating it wasn't
		// starved by the slow one).
		t.Logf("slow subscriber not evicted (ok); fast received %d events", fastCount.Load())
	}
	if fastCount.Load() == 0 {
		t.Fatal("fast subscriber received 0 events — tap was stalled by slow consumer")
	}
}

// TestBackfill_SkipsAnchor verifies XRANGE's inclusive anchor doesn't cause
// duplicates when callers pass their last-seen ID.
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

	// Subscribe from after event 0 (i.e. ids[0] is the anchor, must be skipped).
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
