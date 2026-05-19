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
}
