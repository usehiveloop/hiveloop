package e2e

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/e2e/fakebridge"
	bridgepkg "github.com/usehivy/hivy/internal/bridge"
)

func runApprovalRoundtrip(t *testing.T, decision string) {
	t.Helper()
	ah := newApprovalsHarness(t)

	approvalID := "appr-" + uuid.New().String()[:8]
	ah.bridge.SetPendingApprovals([]bridgepkg.ApprovalRequest{
		{
			Id:             approvalID,
			AgentId:        ah.agent.ID.String(),
			ConversationId: ah.conv.RuntimeConversationID,
			ToolName:       "bash",
			ToolCallId:     "tc-1",
			Arguments:      map[string]any{"cmd": "ls"},
			Status:         "pending",
			CreatedAt:      time.Now(),
		},
	})

	srv := httptest.NewServer(ah.router)
	t.Cleanup(srv.Close)

	sseReq, _ := http.NewRequestWithContext(t.Context(), http.MethodGet,
		srv.URL+"/v1/conversations/"+ah.conv.ID.String()+"/stream", nil)
	sseClient := &http.Client{Timeout: 30 * time.Second}

	gotTypes := make(chan string, 32)
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

	time.Sleep(150 * time.Millisecond)

	approvalReq := []fakebridge.BridgeEvent{
		{
			EventID: "ev-tar1", EventType: "tool_approval_required",
			EmployeeID: ah.agent.ID.String(), ConversationID: ah.conv.RuntimeConversationID,
			Timestamp: time.Now(), SequenceNumber: 1,
			Data: json.RawMessage(`{"request_id":"` + approvalID + `","tool":"bash"}`),
		},
	}
	if status, body := ah.bridge.PostWebhook(t, approvalReq); status != http.StatusOK {
		t.Fatalf("approval webhook: status=%d body=%s", status, body)
	}

	if !waitForType(gotTypes, "tool_approval_required", 3*time.Second) {
		t.Fatal("SSE never delivered tool_approval_required")
	}

	body := []byte(`{"decision":"` + decision + `"}`)
	req, _ := http.NewRequestWithContext(t.Context(), http.MethodPost,
		srv.URL+"/v1/conversations/"+ah.conv.ID.String()+"/approvals/"+approvalID,
		strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("resolve POST: %v", err)
	}
	respBody := readAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("resolve: status=%d body=%s", resp.StatusCode, respBody)
	}

	cap := ah.bridge.CapturedSnapshot()
	if len(cap.Approvals) != 1 {
		t.Fatalf("fakebridge approvals captured: got %d, want 1", len(cap.Approvals))
	}
	gotCall := cap.Approvals[0]
	if gotCall.EmployeeID != ah.agent.ID.String() {
		t.Errorf("employee_id: got %q, want %q", gotCall.EmployeeID, ah.agent.ID.String())
	}
	if gotCall.ConversationID != ah.conv.RuntimeConversationID {
		t.Errorf("conversation_id: got %q, want %q", gotCall.ConversationID, ah.conv.RuntimeConversationID)
	}
	if gotCall.RequestID != approvalID {
		t.Errorf("request_id: got %q, want %q", gotCall.RequestID, approvalID)
	}
	if gotCall.Decision != decision {
		t.Errorf("decision: got %q, want %q", gotCall.Decision, decision)
	}

	resolved := []fakebridge.BridgeEvent{
		{
			EventID: "ev-resolved", EventType: "tool_approval_resolved",
			EmployeeID: ah.agent.ID.String(), ConversationID: ah.conv.RuntimeConversationID,
			Timestamp: time.Now(), SequenceNumber: 2,
			Data: json.RawMessage(`{"request_id":"` + approvalID + `","decision":"` + decision + `"}`),
		},
		{
			EventID: "ev-result", EventType: "tool_call_completed",
			EmployeeID: ah.agent.ID.String(), ConversationID: ah.conv.RuntimeConversationID,
			Timestamp: time.Now(), SequenceNumber: 3,
			Data: json.RawMessage(`{"output":"ok"}`),
		},
		{
			EventID: "ev-tc", EventType: "turn_completed",
			EmployeeID: ah.agent.ID.String(), ConversationID: ah.conv.RuntimeConversationID,
			Timestamp: time.Now(), SequenceNumber: 4,
			Data: json.RawMessage(`{"stop_reason":"end_turn"}`),
		},
	}
	if status, body := ah.bridge.PostWebhook(t, resolved); status != http.StatusOK {
		t.Fatalf("resolved webhook: status=%d body=%s", status, body)
	}

	if !waitForType(gotTypes, "tool_approval_resolved", 3*time.Second) {
		t.Errorf("SSE never delivered tool_approval_resolved (decision=%s)", decision)
	}
}

func TestApprovalFlow_BridgeRequestRoundtrip(t *testing.T) {
	runApprovalRoundtrip(t, "approve")
}

func TestApprovalFlow_DenyRoundtrip(t *testing.T) {
	runApprovalRoundtrip(t, "deny")
}
