package streaming

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestSubscribe_LateJoinerGetsBackfillThenLive(t *testing.T) {
	rc := setupTestRedis(t)
	bus := NewEventBus(rc)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var preIDs []string
	for i := 0; i < 3; i++ {
		id, _ := bus.Publish(ctx, "late-join", "pre", json.RawMessage(`{}`))
		preIDs = append(preIDs, id)
	}

	ch := bus.Subscribe(ctx, "late-join", "0")

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

	if types[0] != "pre" || types[1] != "pre" || types[2] != "pre" {
		t.Errorf("expected first 3 events to be pre, got %v", types[:3])
	}
	if types[3] != "post" || types[4] != "post" || types[5] != "post" {
		t.Errorf("expected last 3 events to be post, got %v", types[3:])
	}
	for i := 1; i < len(ids); i++ {
		if ids[i] <= ids[i-1] {
			t.Errorf("IDs not strictly increasing: %v", ids)
			break
		}
	}
}

func TestSubscribe_LiveOnlyCursor(t *testing.T) {
	rc := setupTestRedis(t)
	bus := NewEventBus(rc)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	for i := 0; i < 3; i++ {
		bus.Publish(ctx, "live-only", "old", json.RawMessage(`{}`))
	}

	ch := bus.Subscribe(ctx, "live-only", "$")
	time.Sleep(200 * time.Millisecond)

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

func TestSubscribe_MultipleSubscribersDifferentCursors(t *testing.T) {
	rc := setupTestRedis(t)
	bus := NewEventBus(rc)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var ids []string
	for i := 0; i < 5; i++ {
		id, _ := bus.Publish(ctx, "mixed-cursors", "old", json.RawMessage(`{}`))
		ids = append(ids, id)
	}

	chA := bus.Subscribe(ctx, "mixed-cursors", "0")
	chB := bus.Subscribe(ctx, "mixed-cursors", ids[2])
	chC := bus.Subscribe(ctx, "mixed-cursors", "$")

	time.Sleep(300 * time.Millisecond)

	if got := bus.ActiveTaps(); got != 1 {
		t.Fatalf("expected 1 tap, got %d", got)
	}

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

	select {
	case ev := <-chC:
		t.Errorf("C: expected no backfill, got event %q", ev.EventType)
	case <-time.After(300 * time.Millisecond):
	}

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
