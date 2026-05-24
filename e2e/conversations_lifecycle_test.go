package e2e

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/usehivy/hivy/e2e/fakebridge"
	"github.com/usehivy/hivy/internal/model"
)

// TestConversationLifecycle_PushSendStreamEnd drives the full
// webhook -> Redis -> SSE pipeline against the fakebridge: webhook batch
// arrives, hivy publishes events to Redis, the SSE consumer drains
// them with monotonic sequence numbers, and the conversation_ended webhook
// flips status="ended".
func TestConversationLifecycle_PushSendStreamEnd(t *testing.T) {
	fbh := newFakeBridgeHarness(t)

	// Run on a real httptest server so SSE Flusher works (recorder doesn't).
	srv := httptest.NewServer(fbh.router)
	t.Cleanup(srv.Close)

	sseReq, _ := http.NewRequestWithContext(t.Context(), http.MethodGet,
		srv.URL+"/v1/conversations/"+fbh.conv.ID.String()+"/stream", nil)
	sseReq.Header.Set("Accept", "text/event-stream")
	sseClient := &http.Client{Timeout: 30 * time.Second}

	gotTypes := make(chan string, 16)
	go func() {
		defer close(gotTypes)
		sseResp, err := sseClient.Do(sseReq)
		if err != nil {
			return
		}
		defer sseResp.Body.Close()
		buf := make([]byte, 4096)
		acc := ""
		for {
			n, err := sseResp.Body.Read(buf)
			if n > 0 {
				acc += string(buf[:n])
				for {
					idx := strings.Index(acc, "\n\n")
					if idx == -1 {
						break
					}
					frame := acc[:idx]
					acc = acc[idx+2:]
					for line := range strings.SplitSeq(frame, "\n") {
						if rest, ok := strings.CutPrefix(line, "event:"); ok {
							gotTypes <- strings.TrimSpace(rest)
						}
					}
				}
			}
			if err != nil {
				return
			}
		}
	}()

	// Wait for the Redis XREAD BLOCK to attach with cursor "$" before
	// publishing — events posted before XREAD starts get missed.
	time.Sleep(200 * time.Millisecond)

	now := time.Now()
	events := []fakebridge.BridgeEvent{
		{
			EventID: "ev1", EventType: "message_received",
			AgentID: fbh.agent.ID.String(), ConversationID: fbh.conv.RuntimeConversationID,
			Timestamp: now, SequenceNumber: 1, Data: json.RawMessage(`{"content":"hi"}`),
		},
		{
			EventID: "ev2", EventType: "response_chunk",
			AgentID: fbh.agent.ID.String(), ConversationID: fbh.conv.RuntimeConversationID,
			Timestamp: now.Add(10 * time.Millisecond), SequenceNumber: 2, Data: json.RawMessage(`{"text":"hello"}`),
		},
		{
			EventID: "ev3", EventType: "response_chunk",
			AgentID: fbh.agent.ID.String(), ConversationID: fbh.conv.RuntimeConversationID,
			Timestamp: now.Add(20 * time.Millisecond), SequenceNumber: 3, Data: json.RawMessage(`{"text":" world"}`),
		},
		{
			EventID: "ev4", EventType: "response_chunk",
			AgentID: fbh.agent.ID.String(), ConversationID: fbh.conv.RuntimeConversationID,
			Timestamp: now.Add(30 * time.Millisecond), SequenceNumber: 4, Data: json.RawMessage(`{"text":"!"}`),
		},
		{
			EventID: "ev5", EventType: "turn_completed",
			AgentID: fbh.agent.ID.String(), ConversationID: fbh.conv.RuntimeConversationID,
			Timestamp: now.Add(40 * time.Millisecond), SequenceNumber: 5, Data: json.RawMessage(`{"stop_reason":"end_turn"}`),
		},
	}
	status, body := fbh.bridge.PostWebhook(t, events)
	if status != http.StatusOK {
		t.Fatalf("webhook POST: status=%d body=%s", status, body)
	}

	expectedTypes := []string{"message_received", "response_chunk", "response_chunk", "response_chunk", "turn_completed"}
	collected := make([]string, 0, len(expectedTypes))
	deadline := time.After(5 * time.Second)
collectLoop:
	for len(collected) < len(expectedTypes) {
		select {
		case ev, ok := <-gotTypes:
			if !ok {
				break collectLoop
			}
			if ev == "ready" {
				continue
			}
			collected = append(collected, ev)
		case <-deadline:
			break collectLoop
		}
	}
	if len(collected) != len(expectedTypes) {
		t.Errorf("got %d SSE events, want %d: %v", len(collected), len(expectedTypes), collected)
	}
	for i := 0; i < len(collected) && i < len(expectedTypes); i++ {
		if collected[i] != expectedTypes[i] {
			t.Errorf("event[%d] = %q, want %q", i, collected[i], expectedTypes[i])
		}
	}

	// End via conversation_ended webhook — driving DELETE through hivy's
	// End() handler would require a real orchestrator.
	endEvents := []fakebridge.BridgeEvent{
		{
			EventID: "ev-end", EventType: "conversation_ended",
			AgentID: fbh.agent.ID.String(), ConversationID: fbh.conv.RuntimeConversationID,
			Timestamp: time.Now(), SequenceNumber: 6, Data: json.RawMessage(`{}`),
		},
	}
	if status, body := fbh.bridge.PostWebhook(t, endEvents); status != http.StatusOK {
		t.Fatalf("end webhook: status=%d body=%s", status, body)
	}

	var refreshed model.EmployeeConversation
	fbh.db.Where("id = ?", fbh.conv.ID).First(&refreshed)
	if refreshed.Status != "ended" {
		t.Errorf("conversation status: got %q, want ended", refreshed.Status)
	}
}

func TestConversationLifecycle_UpsertAgentNewWireShape(t *testing.T) {
	fb := fakebridge.New(t)

	body := `{
		"id": "agent-1",
		"name": "test",
		"harness": "claude",
		"system_prompt": "you are test",
		"provider": {"provider_type":"anthropic","model":"claude-sonnet-4-5","api_key":"sk-x"}
	}`
	req, _ := http.NewRequestWithContext(t.Context(), http.MethodPut, fb.URL+"/push/agents/agent-1", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer x")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT: %v", err)
	}
	respBody, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("PUT status=%d body=%s", resp.StatusCode, respBody)
	}

	cap := fb.CapturedSnapshot()
	if len(cap.UpsertAgents) != 1 {
		t.Fatalf("expected 1 captured upsert, got %d", len(cap.UpsertAgents))
	}
	def := cap.UpsertAgents[0]
	if string(def.Harness) != "claude" {
		t.Errorf("harness: got %q, want claude", def.Harness)
	}
	if def.Provider.Model != "claude-sonnet-4-5" {
		t.Errorf("model: got %q", def.Provider.Model)
	}

	var raw map[string]any
	if err := json.Unmarshal(cap.UpsertAgentsRaw[0], &raw); err != nil {
		t.Fatalf("decode raw: %v", err)
	}
	for _, dead := range []string{"tools", "subagents", "immortal", "history_strip", "tool_requirements", "verifier"} {
		if _, present := raw[dead]; present {
			t.Errorf("forbidden field %q present in upsert body", dead)
		}
	}
}
