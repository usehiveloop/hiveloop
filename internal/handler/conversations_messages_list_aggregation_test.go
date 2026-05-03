package handler_test

import (
	"net/http"
	"testing"

	"github.com/usehiveloop/hiveloop/internal/model"
)

func TestListMessages_AggregatesUserMessageAndConsecutiveBashCalls(t *testing.T) {
	h := newMessagesListHarness(t)

	h.seed(t, []model.ConversationEvent{
		{SequenceNumber: 1, EventType: "message_received", Data: mustJSON(t, map[string]any{"content": "do the thing"})},
		// Three bash completions in a row with the same title should fold into one group.
		{SequenceNumber: 2, EventType: "tool_call_completed", Data: mustJSON(t, map[string]any{
			"title": "bash", "tool_call_id": "tc-1",
			"raw_output": map[string]any{"output": "a\nb\nc"},
		})},
		{SequenceNumber: 3, EventType: "tool_call_completed", Data: mustJSON(t, map[string]any{
			"title": "bash", "tool_call_id": "tc-2",
			"raw_output": map[string]any{"output": "hi"},
		})},
		{SequenceNumber: 4, EventType: "tool_call_completed", Data: mustJSON(t, map[string]any{
			"title": "bash", "tool_call_id": "tc-3",
			"raw_output": map[string]any{"output": "/work"},
		})},
		{SequenceNumber: 5, EventType: "turn_completed", Data: mustJSON(t, map[string]any{"stop_reason": "endturn"})},
	})

	rr := h.get(t, "")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body=%s", rr.Code, rr.Body.String())
	}
	page := decodeMessagesPage(t, rr)

	if got, want := len(page.Data), 2; got != want {
		t.Fatalf("messages: got %d, want %d", got, want)
	}

	user := page.Data[0]
	if user["author"] != "user" {
		t.Errorf("first message author: got %v, want user", user["author"])
	}
	if user["body"] != "do the thing" {
		t.Errorf("user body: got %v", user["body"])
	}

	agent := page.Data[1]
	if agent["author"] != "agent" {
		t.Errorf("second message author: got %v, want agent", agent["author"])
	}
	groups, _ := agent["tool_groups"].([]any)
	if len(groups) != 1 {
		t.Fatalf("tool_groups: got %d, want 1 (consecutive bash calls should collapse)", len(groups))
	}
	g0 := groups[0].(map[string]any)
	if g0["name"] != "bash" {
		t.Errorf("group name: got %v", g0["name"])
	}
	calls, _ := g0["calls"].([]any)
	if len(calls) != 3 {
		t.Fatalf("calls in bash group: got %d, want 3", len(calls))
	}
	for i, c := range calls {
		cm := c.(map[string]any)
		if cm["status"] != "completed" {
			t.Errorf("call %d status: got %v, want completed", i, cm["status"])
		}
	}
	if calls[0].(map[string]any)["title"] != "bash" {
		t.Errorf("call 0 title: got %v, want bash", calls[0].(map[string]any)["title"])
	}
}

func TestListMessages_DifferentToolNamesProduceSeparateGroups(t *testing.T) {
	h := newMessagesListHarness(t)

	h.seed(t, []model.ConversationEvent{
		{SequenceNumber: 1, EventType: "tool_call_completed", Data: mustJSON(t, map[string]any{"title": "bash", "tool_call_id": "a"})},
		{SequenceNumber: 2, EventType: "tool_call_completed", Data: mustJSON(t, map[string]any{"title": "read", "tool_call_id": "b"})},
		{SequenceNumber: 3, EventType: "tool_call_completed", Data: mustJSON(t, map[string]any{"title": "bash", "tool_call_id": "c"})},
	})

	rr := h.get(t, "")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	page := decodeMessagesPage(t, rr)
	if len(page.Data) != 1 {
		t.Fatalf("messages: got %d, want 1", len(page.Data))
	}
	groups := page.Data[0]["tool_groups"].([]any)
	if len(groups) != 3 {
		t.Fatalf("groups: got %d, want 3 (bash → read → bash)", len(groups))
	}
	wantNames := []string{"bash", "read", "bash"}
	for i, g := range groups {
		if g.(map[string]any)["name"] != wantNames[i] {
			t.Errorf("group %d name: got %v, want %s", i, g.(map[string]any)["name"], wantNames[i])
		}
	}
}

func TestListMessages_TurnCompletedSeparatesAgentMessages(t *testing.T) {
	h := newMessagesListHarness(t)

	h.seed(t, []model.ConversationEvent{
		{SequenceNumber: 1, EventType: "tool_call_completed", Data: mustJSON(t, map[string]any{"title": "bash", "tool_call_id": "a"})},
		{SequenceNumber: 2, EventType: "turn_completed", Data: mustJSON(t, map[string]any{"stop_reason": "endturn"})},
		{SequenceNumber: 3, EventType: "tool_call_completed", Data: mustJSON(t, map[string]any{"title": "bash", "tool_call_id": "b"})},
	})

	rr := h.get(t, "")
	page := decodeMessagesPage(t, rr)
	if len(page.Data) != 2 {
		t.Fatalf("expected 2 agent messages split by turn_completed, got %d", len(page.Data))
	}
	for i, m := range page.Data {
		if m["author"] != "agent" {
			t.Errorf("message %d author: got %v", i, m["author"])
		}
	}
}

func TestListMessages_StartedWithoutCompletionIsHidden(t *testing.T) {
	h := newMessagesListHarness(t)

	h.seed(t, []model.ConversationEvent{
		{SequenceNumber: 1, EventType: "tool_call_started", Data: mustJSON(t, map[string]any{"title": "bash", "tool_call_id": "a"})},
	})

	rr := h.get(t, "")
	page := decodeMessagesPage(t, rr)
	if len(page.Data) != 0 {
		t.Fatalf("messages: got %d, want 0 (a started-only call should produce no message)", len(page.Data))
	}
}

func TestListMessages_StartedEventsAreIgnored(t *testing.T) {
	h := newMessagesListHarness(t)

	h.seed(t, []model.ConversationEvent{
		{SequenceNumber: 1, EventType: "tool_call_started", Data: mustJSON(t, map[string]any{"title": "bash", "tool_call_id": "a"})},
		{SequenceNumber: 2, EventType: "tool_call_started", Data: mustJSON(t, map[string]any{"title": "bash", "tool_call_id": "b"})},
		{SequenceNumber: 3, EventType: "tool_call_completed", Data: mustJSON(t, map[string]any{
			"title":        "ls",
			"tool_call_id": "a",
			"raw_output":   map[string]any{"output": "ok"},
		})},
	})

	rr := h.get(t, "")
	page := decodeMessagesPage(t, rr)
	if len(page.Data) != 1 {
		t.Fatalf("messages: got %d, want 1 (one completion → one agent message)", len(page.Data))
	}
	groups := page.Data[0]["tool_groups"].([]any)
	if len(groups) != 1 {
		t.Fatalf("groups: got %d, want 1", len(groups))
	}
	calls := groups[0].(map[string]any)["calls"].([]any)
	if len(calls) != 1 {
		t.Fatalf("calls: got %d, want 1 (only the completed event surfaces; the two started events are dropped)", len(calls))
	}
	if calls[0].(map[string]any)["status"] != "completed" {
		t.Errorf("call status: got %v, want completed", calls[0].(map[string]any)["status"])
	}
}

func TestListMessages_LatestTodosExtractedAndCallSuppressed(t *testing.T) {
	h := newMessagesListHarness(t)

	earlier := []any{
		map[string]any{"content": "old item", "status": "completed", "priority": "high"},
	}
	later := []any{
		map[string]any{"content": "do thing 1", "status": "completed", "priority": "high"},
		map[string]any{"content": "do thing 2", "status": "in_progress", "priority": "medium"},
		map[string]any{"content": "do thing 3", "status": "pending", "priority": "low"},
	}

	h.seed(t, []model.ConversationEvent{
		{SequenceNumber: 1, EventType: "tool_call_completed", Data: mustJSON(t, map[string]any{
			"title":        "1 todos",
			"tool_call_id": "tw-1",
			"raw_output":   map[string]any{"metadata": map[string]any{"todos": earlier}},
		})},
		{SequenceNumber: 2, EventType: "tool_call_completed", Data: mustJSON(t, map[string]any{
			"title":        "bash",
			"tool_call_id": "b-1",
			"raw_output":   map[string]any{"output": "ok"},
		})},
		{SequenceNumber: 3, EventType: "tool_call_completed", Data: mustJSON(t, map[string]any{
			"title":        "3 todos",
			"tool_call_id": "tw-2",
			"raw_output":   map[string]any{"metadata": map[string]any{"todos": later}},
		})},
	})

	rr := h.get(t, "")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body=%s", rr.Code, rr.Body.String())
	}
	page := decodeMessagesPage(t, rr)

	if len(page.Data) != 1 {
		t.Fatalf("messages: got %d, want 1", len(page.Data))
	}
	groups, _ := page.Data[0]["tool_groups"].([]any)
	if len(groups) != 1 || groups[0].(map[string]any)["name"] != "bash" {
		t.Fatalf("expected single bash group, got groups=%v", groups)
	}

	if len(page.LatestTodos) != 3 {
		t.Fatalf("latest_todos: got %d, want 3 (most recent todowrite)", len(page.LatestTodos))
	}
	if page.LatestTodos[0]["content"] != "do thing 1" {
		t.Errorf("first todo content: got %v, want 'do thing 1'", page.LatestTodos[0]["content"])
	}
	if page.LatestTodos[1]["status"] != "in_progress" {
		t.Errorf("second todo status: got %v, want in_progress", page.LatestTodos[1]["status"])
	}
	if page.LatestTodos[2]["priority"] != "low" {
		t.Errorf("third todo priority: got %v, want low", page.LatestTodos[2]["priority"])
	}
}
