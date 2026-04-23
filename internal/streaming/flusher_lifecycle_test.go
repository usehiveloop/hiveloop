package streaming

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestFlusher_AcksAfterFlush(t *testing.T) {
	bus, flusher, db, rc := setupFlusherTest(t)
	_, convID := createTestConversation(t, db)
	ctx := context.Background()

	bus.Publish(ctx, convID.String(), "chunk", json.RawMessage(`{}`))
	flusher.flushStream(ctx, convID.String())

	pending, err := rc.XPending(ctx, bus.streamKey(convID.String()), flusherGroup).Result()
	if err != nil {
		t.Fatalf("XPending: %v", err)
	}
	if pending.Count != 0 {
		t.Fatalf("expected 0 pending entries, got %d", pending.Count)
	}
}

func TestFlusher_DoesNotAckOnDBError(t *testing.T) {
	rc := setupTestRedis(t)
	badDB, err := gorm.Open(postgres.Open("postgres://bad:bad@localhost:1/bad?sslmode=disable"), &gorm.Config{Logger: logger.Discard})
	if err == nil {
		_ = badDB
	} else {
		t.Skip("cannot test DB error scenario")
	}

	bus := NewEventBus(rc)
	flusher := NewFlusher(bus, badDB)
	ctx := context.Background()

	convID := uuid.New().String()
	bus.Publish(ctx, convID, "chunk", json.RawMessage(`{}`))

	rc.XGroupCreateMkStream(ctx, bus.streamKey(convID), flusherGroup, "0")

	flusher.flushStream(ctx, convID)

	length, _ := bus.StreamLen(ctx, convID)
	if length != 1 {
		t.Fatalf("expected 1 event still in stream, got %d", length)
	}
}

func TestFlusher_TrimsAfterFlush(t *testing.T) {
	bus, flusher, db, _ := setupFlusherTest(t)
	_, convID := createTestConversation(t, db)
	ctx := context.Background()

	for i := 0; i < 600; i++ {
		bus.Publish(ctx, convID.String(), "chunk", json.RawMessage(`{}`))
	}

	flusher.flushStream(ctx, convID.String())

	length, _ := bus.StreamLen(ctx, convID.String())
	if length > 550 {
		t.Fatalf("expected stream trimmed to ~500, got %d", length)
	}
}

func TestFlusher_GracefulShutdown(t *testing.T) {
	bus, flusher, db, _ := setupFlusherTest(t)
	_, convID := createTestConversation(t, db)
	ctx, cancel := context.WithCancel(context.Background())

	bus.Publish(ctx, convID.String(), "chunk", json.RawMessage(`{}`))

	done := make(chan struct{})
	go func() {
		flusher.Run(ctx)
		close(done)
	}()

	time.Sleep(3 * time.Second)

	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("flusher did not shut down within 5 seconds")
	}
}
