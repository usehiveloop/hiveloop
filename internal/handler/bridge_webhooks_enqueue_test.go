package handler

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/enqueue"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/tasks"
)

func TestMaybeEnqueueConversationNaming_NoEnqueuer(t *testing.T) {
	handler := &BridgeWebhookHandler{enqueuer: nil}
	handler.maybeEnqueueConversationNaming(context.Background(), &model.EmployeeConversation{
		ID:   uuid.New(),
		Name: "",
	})

}

func TestMaybeEnqueueConversationNaming_AlreadyNamed(t *testing.T) {
	mock := &enqueue.MockClient{}
	handler := &BridgeWebhookHandler{enqueuer: mock}

	handler.maybeEnqueueConversationNaming(context.Background(), &model.EmployeeConversation{
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

	handler.maybeEnqueueConversationNaming(context.Background(), &model.EmployeeConversation{
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
