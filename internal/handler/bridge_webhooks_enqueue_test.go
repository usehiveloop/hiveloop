package handler

import (
	"testing"

	"github.com/google/uuid"

	"github.com/usehiveloop/hiveloop/internal/enqueue"
	"github.com/usehiveloop/hiveloop/internal/model"
	"github.com/usehiveloop/hiveloop/internal/tasks"
)

// maybeEnqueueConversationNaming is best-effort — it should no-op (not
// panic, not return an error, simply do nothing) when the enqueuer is not
// configured, and should skip enqueuing when the conversation already has
// a name. When the name is empty and an enqueuer is present, it should
// enqueue exactly one conversation:name task.

func TestMaybeEnqueueConversationNaming_NoEnqueuer(t *testing.T) {
	handler := &BridgeWebhookHandler{enqueuer: nil}
	handler.maybeEnqueueConversationNaming(&model.AgentConversation{
		ID:   uuid.New(),
		Name: "",
	})
	// No panic, no error — just a silent no-op. Nothing to assert beyond that.
}

func TestMaybeEnqueueConversationNaming_AlreadyNamed(t *testing.T) {
	mock := &enqueue.MockClient{}
	handler := &BridgeWebhookHandler{enqueuer: mock}

	handler.maybeEnqueueConversationNaming(&model.AgentConversation{
		ID:   uuid.New(),
		Name: "Already Named",
	})

	if got := len(mock.Tasks()); got != 0 {
		t.Errorf("expected 0 tasks enqueued for already-named conversation, got %d", got)
	}
}

func TestMaybeEnqueueConversationNaming_Enqueues(t *testing.T) {
	mock := &enqueue.MockClient{}
	handler := &BridgeWebhookHandler{enqueuer: mock}
	convID := uuid.New()

	handler.maybeEnqueueConversationNaming(&model.AgentConversation{
		ID:   convID,
		Name: "",
	})

	got := mock.Tasks()
	if len(got) != 1 {
		t.Fatalf("expected 1 task enqueued, got %d", len(got))
	}
	if got[0].TypeName != tasks.TypeConversationName {
		t.Errorf("type = %q, want %q", got[0].TypeName, tasks.TypeConversationName)
	}
}
