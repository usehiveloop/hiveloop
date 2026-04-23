package streaming

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"

	"github.com/usehiveloop/hiveloop/internal/model"
)

func TestFlusher_RecoversChunksWhenCompletionMissing(t *testing.T) {
	bus, flusher, db, _ := setupFlusherTest(t)
	_, convID := createTestConversation(t, db)
	ctx := context.Background()

	messageID := "msg-" + uuid.New().String()[:8]
	parts := []string{"Hello", ", ", "world", "!"}
	for _, p := range parts {
		chunk, _ := json.Marshal(map[string]any{
			"data": map[string]any{"delta": p, "message_id": messageID},
		})
		bus.Publish(ctx, convID.String(), "response_chunk", chunk)
	}
	bus.Publish(ctx, convID.String(), "done", json.RawMessage(`{}`))

	flusher.flushStream(ctx, convID.String())

	var events []model.ConversationEvent
	if err := db.Where("conversation_id = ?", convID).Find(&events).Error; err != nil {
		t.Fatalf("find events: %v", err)
	}

	var recovered *model.ConversationEvent
	for i := range events {
		if events[i].EventType == "response_completed" {
			recovered = &events[i]
		}
	}
	if recovered == nil {
		t.Fatalf("expected a synthesized response_completed row, got %d events: %+v", len(events), events)
	}

	var data map[string]any
	if err := json.Unmarshal(recovered.Data, &data); err != nil {
		t.Fatalf("unmarshal data: %v", err)
	}
	if got, want := data["full_response"], "Hello, world!"; got != want {
		t.Fatalf("full_response = %q, want %q", got, want)
	}
	if data["recovered"] != true {
		t.Fatalf("expected recovered:true flag, got %v", data["recovered"])
	}

	peeked, _ := bus.PeekChunks(ctx, convID.String())
	if len(peeked) != 0 {
		t.Fatalf("expected accumulator cleared, got %v", peeked)
	}
}

func TestFlusher_DropsAccumulatorOnCompletion(t *testing.T) {
	bus, flusher, db, _ := setupFlusherTest(t)
	_, convID := createTestConversation(t, db)
	ctx := context.Background()

	messageID := "msg-" + uuid.New().String()[:8]
	chunk, _ := json.Marshal(map[string]any{
		"data": map[string]any{"delta": "hi", "message_id": messageID},
	})
	bus.Publish(ctx, convID.String(), "response_chunk", chunk)

	completion, _ := json.Marshal(map[string]any{
		"event_id": uuid.New().String(),
		"data":     map[string]any{"message_id": messageID, "full_response": "hi there"},
	})
	bus.Publish(ctx, convID.String(), "response_completed", completion)
	bus.Publish(ctx, convID.String(), "done", json.RawMessage(`{}`))

	flusher.flushStream(ctx, convID.String())

	var count int64
	db.Model(&model.ConversationEvent{}).
		Where("conversation_id = ? AND event_type = ?", convID, "response_completed").
		Count(&count)
	if count != 1 {
		t.Fatalf("expected exactly 1 response_completed row, got %d", count)
	}

	peeked, _ := bus.PeekChunks(ctx, convID.String())
	if len(peeked) != 0 {
		t.Fatalf("expected accumulator cleared on completion, got %v", peeked)
	}
}
