package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/usehiveloop/hiveloop/internal/handler"
	"github.com/usehiveloop/hiveloop/internal/model"
)

type fakeEmployeeCallbackRuntime struct {
	refreshURL     string
	refreshes      int
	forceRefresh   bool
	needsRefresh   func(*model.Sandbox) bool
	refreshSandbox func(*model.Sandbox)
}

func (f *fakeEmployeeCallbackRuntime) NeedsURLRefresh(sb *model.Sandbox) bool {
	if f.needsRefresh != nil {
		return f.needsRefresh(sb)
	}
	return f.forceRefresh
}

func (f *fakeEmployeeCallbackRuntime) RefreshEmployeeSandboxURL(_ context.Context, sb *model.Sandbox) error {
	f.refreshes++
	sb.BridgeURL = f.refreshURL
	expiresAt := time.Now().Add(time.Hour)
	sb.BridgeURLExpiresAt = &expiresAt
	if f.refreshSandbox != nil {
		f.refreshSandbox(sb)
	}
	return nil
}

func TestBridgeWebhook_RefreshesEmployeeCallbackURLBeforeForwardingCloudAgentEvent(t *testing.T) {
	h := newCloudAgentHarness(t)
	cloudAgentID := h.seedCloudAgent(t)
	task := h.seedTask(t, cloudAgentID, "Specific task")
	if err := h.db.Model(&model.CloudAgentTask{}).Where("id = ?", task.ID).Update("metadata", model.JSON{"session_id": "wrong-session"}).Error; err != nil {
		t.Fatalf("set conflicting task metadata: %v", err)
	}

	var staleHits atomic.Int64
	staleURL := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		staleHits.Add(1)
		http.Redirect(w, r, "https://dex.daytona.usehiveloop.com/dex/auth", http.StatusFound)
	}))
	t.Cleanup(staleURL.Close)

	expiredAt := time.Now().Add(-time.Minute)
	if err := h.db.Model(&model.Sandbox{}).
		Where("agent_id = ?", h.agentID).
		Updates(map[string]any{
			"bridge_url":            staleURL.URL,
			"bridge_url_expires_at": expiredAt,
		}).Error; err != nil {
		t.Fatalf("mark employee sandbox callback URL stale: %v", err)
	}

	runtime := &fakeEmployeeCallbackRuntime{
		refreshURL:   h.callbackURL,
		forceRefresh: true,
		refreshSandbox: func(sb *model.Sandbox) {
			if err := h.db.Model(sb).Updates(map[string]any{
				"bridge_url":            sb.BridgeURL,
				"bridge_url_expires_at": sb.BridgeURLExpiresAt,
			}).Error; err != nil {
				t.Fatalf("persist refreshed employee sandbox URL: %v", err)
			}
		},
	}

	var conv model.AgentConversation
	if err := h.db.Where("id = ?", task.ConversationID).First(&conv).Error; err != nil {
		t.Fatalf("load conversation: %v", err)
	}

	payload, err := json.Marshal([]map[string]any{{
		"event_id":        "evt-cloud-agent-refresh-1",
		"event_type":      "ConversationEnded",
		"agent_id":        cloudAgentID.String(),
		"conversation_id": conv.RuntimeConversationID,
		"timestamp":       time.Now().UTC().Format(time.RFC3339),
		"sequence_number": 1,
		"data": map[string]any{
			"status": "done",
		},
	}})
	if err != nil {
		t.Fatalf("marshal webhook payload: %v", err)
	}

	router := chi.NewRouter()
	webhookHandler := handler.NewBridgeWebhookHandlerWithEmployeeRuntime(h.db, h.encKey, nil, nil, runtime)
	router.Post("/internal/webhooks/bridge/{sandboxID}", webhookHandler.Handle)

	timestamp := time.Now().Unix()
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/internal/webhooks/bridge/%s", task.SandboxID), bytes.NewReader(payload))
	req.Header.Set("X-Webhook-Timestamp", fmt.Sprintf("%d", timestamp))
	req.Header.Set("X-Webhook-Signature", signBridgeWebhookPayload("cloud-agent-webhook-key", timestamp, payload))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	if runtime.refreshes != 1 {
		t.Fatalf("expected employee callback URL to be refreshed once, got %d", runtime.refreshes)
	}
	if staleHits.Load() != 0 {
		t.Fatalf("expected stale Daytona preview URL not to be used, got %d hits", staleHits.Load())
	}
	h.callbackMu.Lock()
	callbacks := append([]map[string]any(nil), h.callbacks...)
	h.callbackMu.Unlock()
	if len(callbacks) != 1 {
		t.Fatalf("expected 1 employee bridge callback, got %#v", callbacks)
	}
	if callbacks[0]["event_id"] != "evt-cloud-agent-refresh-1" {
		t.Fatalf("unexpected callback payload: %#v", callbacks[0])
	}
	if callbacks[0]["session_id"] != task.ParentConversationID {
		t.Fatalf("callback session_id = %q, want parent conversation %q", callbacks[0]["session_id"], task.ParentConversationID)
	}
	metadata, ok := callbacks[0]["metadata"].(map[string]any)
	if !ok || metadata["session_id"] != "wrong-session" {
		t.Fatalf("expected metadata to remain context-only, got %#v", callbacks[0]["metadata"])
	}
}

func TestBridgeWebhook_ForwardsOnlyAllowedCloudAgentEvents(t *testing.T) {
	h := newCloudAgentHarness(t)
	cloudAgentID := h.seedCloudAgent(t)
	task := h.seedTask(t, cloudAgentID, "Specific task")

	postCloudAgentWebhook(t, h, task, cloudAgentID, []map[string]any{
		cloudAgentWebhookEvent("evt-allowed-ended", "ConversationEnded"),
		cloudAgentWebhookEvent("evt-allowed-done", "Done"),
		cloudAgentWebhookEvent("evt-allowed-todo", "TodoUpdated"),
		cloudAgentWebhookEvent("evt-blocked-message", "MessageReceived"),
		cloudAgentWebhookEvent("evt-blocked-tool", "ToolCallStarted"),
		cloudAgentWebhookEvent("evt-blocked-response", "ResponseCompleted"),
	})

	h.callbackMu.Lock()
	callbacks := append([]map[string]any(nil), h.callbacks...)
	h.callbackMu.Unlock()
	if len(callbacks) != 3 {
		t.Fatalf("expected 3 allowed callbacks, got %#v", callbacks)
	}

	got := map[string]bool{}
	for _, callback := range callbacks {
		got[fmt.Sprint(callback["event_type"])] = true
		if callback["session_id"] != task.ParentConversationID {
			t.Fatalf("callback session_id = %q, want %q", callback["session_id"], task.ParentConversationID)
		}
	}
	for _, eventType := range []string{"conversation_ended", "done", "todo_updated"} {
		if !got[eventType] {
			t.Fatalf("missing callback for %s in %#v", eventType, callbacks)
		}
	}
}

func postCloudAgentWebhook(t *testing.T, h *cloudAgentHarness, task model.CloudAgentTask, cloudAgentID fmt.Stringer, events []map[string]any) {
	t.Helper()

	var conv model.AgentConversation
	if err := h.db.Where("id = ?", task.ConversationID).First(&conv).Error; err != nil {
		t.Fatalf("load conversation: %v", err)
	}
	for _, event := range events {
		event["agent_id"] = cloudAgentID.String()
		event["conversation_id"] = conv.RuntimeConversationID
	}

	payload, err := json.Marshal(events)
	if err != nil {
		t.Fatalf("marshal webhook payload: %v", err)
	}

	router := chi.NewRouter()
	webhookHandler := handler.NewBridgeWebhookHandler(h.db, h.encKey, nil, nil)
	router.Post("/internal/webhooks/bridge/{sandboxID}", webhookHandler.Handle)

	timestamp := time.Now().Unix()
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/internal/webhooks/bridge/%s", task.SandboxID), bytes.NewReader(payload))
	req.Header.Set("X-Webhook-Timestamp", fmt.Sprintf("%d", timestamp))
	req.Header.Set("X-Webhook-Signature", signBridgeWebhookPayload("cloud-agent-webhook-key", timestamp, payload))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

func cloudAgentWebhookEvent(eventID, eventType string) map[string]any {
	return map[string]any{
		"event_id":        eventID,
		"event_type":      eventType,
		"timestamp":       time.Now().UTC().Format(time.RFC3339),
		"sequence_number": 1,
		"data": map[string]any{
			"status": "done",
		},
	}
}
