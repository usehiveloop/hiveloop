package streaming

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
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
		_, _ = bus.Publish(ctx, convID.String(), "response_chunk", chunk)
	}
	_, _ = bus.Publish(ctx, convID.String(), "done", json.RawMessage(`{}`))

	flusher.flushStream(ctx, convID.String())

	var events []model.EmployeeSessionEvent
	if err := db.Where("employee_session_id = ?", convID).Find(&events).Error; err != nil {
		t.Fatalf("find events: %v", err)
	}

	var recovered *model.EmployeeSessionEvent
	for i := range events {
		if events[i].EventType == "response_completed" {
			recovered = &events[i]
		}
	}
	if recovered == nil {
		t.Fatalf("expected a synthesized response_completed row, got %d events: %+v", len(events), events)
	}

	var data map[string]any
	if err := json.Unmarshal(recovered.Payload, &data); err != nil {
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

func TestFlusher_RecoversRuntimeNativeChunksOnTurnCompleted(t *testing.T) {
	bus, flusher, db, _ := setupFlusherTest(t)
	_, convID := createTestConversation(t, db)
	ctx := context.Background()

	firstChunkID := uuid.New().String()
	chunk1, _ := json.Marshal(map[string]any{
		"event_id": firstChunkID,
		"data": map[string]any{
			"content": map[string]any{"type": "text", "text": "Hello"},
		},
	})
	chunk2, _ := json.Marshal(map[string]any{
		"event_id": uuid.New().String(),
		"data": map[string]any{
			"content": map[string]any{"type": "text", "text": ", Runtime"},
		},
	})
	terminal, _ := json.Marshal(map[string]any{
		"event_id":        uuid.New().String(),
		"employee_id":     "agent-1",
		"conversation_id": "runtime-" + convID.String(),
		"timestamp":       "2026-05-13T12:00:00Z",
		"sequence_number": 4,
		"data":            map[string]any{"stop_reason": "endturn"},
	})

	_, _ = bus.Publish(ctx, convID.String(), "response_chunk", chunk1)
	_, _ = bus.Publish(ctx, convID.String(), "response_chunk", chunk2)
	_, _ = bus.Publish(ctx, convID.String(), "turn_completed", terminal)

	flusher.flushStream(ctx, convID.String())

	var event model.EmployeeSessionEvent
	if err := db.Where("employee_session_id = ? AND event_type = ?", convID, "response_completed").First(&event).Error; err != nil {
		t.Fatalf("expected synthesized response_completed: %v", err)
	}
	var data map[string]any
	if err := json.Unmarshal(event.Payload, &data); err != nil {
		t.Fatalf("unmarshal data: %v", err)
	}
	if data["message_id"] != firstChunkID {
		t.Fatalf("message_id = %v, want first chunk event id %s", data["message_id"], firstChunkID)
	}
	if data["full_response"] != "Hello, Runtime" {
		t.Fatalf("full_response = %v, want Hello, Runtime", data["full_response"])
	}
	if event.SequenceNumber != 3 {
		t.Fatalf("recovered sequence_number = %d, want 3", event.SequenceNumber)
	}
}

func TestFlusher_SeparatesRuntimeNativeChunksAcrossTurns(t *testing.T) {
	bus, flusher, db, _ := setupFlusherTest(t)
	_, convID := createTestConversation(t, db)
	ctx := context.Background()

	firstTurnID := uuid.New().String()
	secondTurnID := uuid.New().String()
	firstChunk, _ := json.Marshal(map[string]any{
		"event_id": firstTurnID,
		"data": map[string]any{
			"content": map[string]any{"type": "text", "text": "first"},
		},
	})
	firstDone, _ := json.Marshal(map[string]any{
		"event_id":        uuid.New().String(),
		"employee_id":     "agent-1",
		"conversation_id": "runtime-" + convID.String(),
		"timestamp":       "2026-05-13T12:00:00Z",
		"sequence_number": 2,
		"data":            map[string]any{"stop_reason": "endturn"},
	})
	secondChunk, _ := json.Marshal(map[string]any{
		"event_id": secondTurnID,
		"data": map[string]any{
			"content": map[string]any{"type": "text", "text": "second"},
		},
	})
	secondDone, _ := json.Marshal(map[string]any{
		"event_id":        uuid.New().String(),
		"employee_id":     "agent-1",
		"conversation_id": "runtime-" + convID.String(),
		"timestamp":       "2026-05-13T12:00:01Z",
		"sequence_number": 4,
		"data":            map[string]any{"stop_reason": "endturn"},
	})

	_, _ = bus.Publish(ctx, convID.String(), "response_chunk", firstChunk)
	_, _ = bus.Publish(ctx, convID.String(), "turn_completed", firstDone)
	_, _ = bus.Publish(ctx, convID.String(), "response_chunk", secondChunk)
	_, _ = bus.Publish(ctx, convID.String(), "turn_completed", secondDone)

	flusher.flushStream(ctx, convID.String())

	var events []model.EmployeeSessionEvent
	if err := db.Where("employee_session_id = ? AND event_type = ?", convID, "response_completed").Order("sequence_number ASC").Find(&events).Error; err != nil {
		t.Fatalf("find response_completed events: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 synthesized completions, got %d", len(events))
	}

	got := map[string]string{}
	for _, event := range events {
		var data map[string]any
		if err := json.Unmarshal(event.Payload, &data); err != nil {
			t.Fatalf("unmarshal data: %v", err)
		}
		got[data["message_id"].(string)] = data["full_response"].(string)
	}
	if got[firstTurnID] != "first" || got[secondTurnID] != "second" {
		t.Fatalf("unexpected recovered responses: %#v", got)
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
	_, _ = bus.Publish(ctx, convID.String(), "response_chunk", chunk)

	completion, _ := json.Marshal(map[string]any{
		"event_id": uuid.New().String(),
		"data":     map[string]any{"message_id": messageID, "full_response": "hi there"},
	})
	_, _ = bus.Publish(ctx, convID.String(), "response_completed", completion)
	_, _ = bus.Publish(ctx, convID.String(), "done", json.RawMessage(`{}`))

	flusher.flushStream(ctx, convID.String())

	var count int64
	db.Model(&model.EmployeeSessionEvent{}).
		Where("employee_session_id = ? AND event_type = ?", convID, "response_completed").
		Count(&count)
	if count != 1 {
		t.Fatalf("expected exactly 1 response_completed row, got %d", count)
	}

	peeked, _ := bus.PeekChunks(ctx, convID.String())
	if len(peeked) != 0 {
		t.Fatalf("expected accumulator cleared on completion, got %v", peeked)
	}
}

func TestFlusher_RecoversReasoningDeltasOnTerminalEvent(t *testing.T) {
	bus, flusher, db, _ := setupFlusherTest(t)
	_, convID := createTestConversation(t, db)
	ctx := context.Background()

	firstDeltaID := uuid.New().String()
	delta1, _ := json.Marshal(map[string]any{
		"event_id": firstDeltaID,
		"data": map[string]any{
			"content": map[string]any{"type": "text", "text": "I need"},
		},
	})
	delta2, _ := json.Marshal(map[string]any{
		"event_id": uuid.New().String(),
		"data": map[string]any{
			"content": map[string]any{"type": "text", "text": " to inspect logs."},
		},
	})
	terminal, _ := json.Marshal(map[string]any{
		"event_id":        uuid.New().String(),
		"employee_id":     "agent-1",
		"conversation_id": "runtime-" + convID.String(),
		"timestamp":       "2026-05-13T12:00:00Z",
		"sequence_number": 4,
		"data":            map[string]any{"stop_reason": "endturn"},
	})

	_, _ = bus.Publish(ctx, convID.String(), "reasoning_delta", delta1)
	_, _ = bus.Publish(ctx, convID.String(), "reasoning_delta", delta2)
	_, _ = bus.Publish(ctx, convID.String(), "turn_completed", terminal)

	flusher.flushStream(ctx, convID.String())

	var event model.EmployeeSessionEvent
	if err := db.Where("employee_session_id = ? AND event_type = ?", convID, "reasoning_completed").First(&event).Error; err != nil {
		t.Fatalf("expected synthesized reasoning_completed: %v", err)
	}
	var data map[string]any
	if err := json.Unmarshal(event.Payload, &data); err != nil {
		t.Fatalf("unmarshal data: %v", err)
	}
	if data["message_id"] != firstDeltaID {
		t.Fatalf("message_id = %v, want first delta event id %s", data["message_id"], firstDeltaID)
	}
	if data["full_reasoning"] != "I need to inspect logs." {
		t.Fatalf("full_reasoning = %v, want accumulated reasoning", data["full_reasoning"])
	}
	if event.SequenceNumber != 3 {
		t.Fatalf("recovered sequence_number = %d, want 3", event.SequenceNumber)
	}

	peeked, _ := bus.PeekAccumulated(ctx, "reasoning", convID.String())
	if len(peeked) != 0 {
		t.Fatalf("expected reasoning accumulator cleared, got %v", peeked)
	}
}
