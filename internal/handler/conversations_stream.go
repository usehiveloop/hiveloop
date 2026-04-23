package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/usehiveloop/hiveloop/internal/model"
	"github.com/usehiveloop/hiveloop/internal/streaming"
)

// Stream handles GET /v1/conversations/{convID}/stream (SSE proxy).
// @Summary Stream conversation events (SSE)
// @Description Opens a Server-Sent Events stream for real-time agent responses. Defaults to live-only (cursor "$"); clients that want history should hydrate via GET /v1/conversations/{convID}/history first. Resumes from Last-Event-ID when provided.
// @Tags conversations
// @Produce text/event-stream
// @Param convID path string true "Conversation ID"
// @Success 200 {string} string "SSE event stream"
// @Failure 410 {object} errorResponse
// @Security BearerAuth
// @Router /v1/conversations/{convID}/stream [get]
func (h *ConversationHandler) Stream(w http.ResponseWriter, r *http.Request) {
	conv, ok := h.loadConversation(w, r)
	if !ok {
		return
	}

	h.streamFromRedis(w, r, conv)
}

const (
	// sseWriteDeadline bounds each individual SSE frame write. Slow clients
	// that can't accept a frame within this window are disconnected to
	// prevent them from holding a tap subscriber slot indefinitely.
	sseWriteDeadline = 10 * time.Second

	// sseMaxAge caps the lifetime of a single SSE connection. Browsers'
	// EventSource reconnects automatically with Last-Event-ID, so this is
	// transparent to users but makes deploys and long-running connection
	// tracking much cleaner.
	sseMaxAge = 1 * time.Hour

	// sseAuthRecheckInterval controls how often an active SSE stream
	// re-verifies that the caller still has access to the conversation.
	// The re-check reuses the same DB lookup as the initial auth, so the
	// auth cache's TTL gates the worst-case revocation latency.
	sseAuthRecheckInterval = 60 * time.Second

	// ssePingInterval is the interval at which we emit an SSE keep-alive
	// comment to keep intermediaries from idling the connection out.
	ssePingInterval = 15 * time.Second
)

// streamFromRedis streams events from Redis Streams (multi-subscriber, resumable).
func (h *ConversationHandler) streamFromRedis(w http.ResponseWriter, r *http.Request, conv *model.AgentConversation) {
	// Parse Last-Event-ID for resume support. Default is live-only ("$"):
	// clients should hydrate history via GET /history first, then open the
	// stream. Replaying the entire retained window on every connect is
	// wasteful and inconsistent with the DB (which keeps everything).
	cursor := r.Header.Get("Last-Event-ID")
	if cursor == "" {
		cursor = "$"
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	rc := http.NewResponseController(w)

	// Tell EventSource to wait 5s before reconnecting on disconnect (default
	// is 3s; a bit of backoff smooths deploy rollouts).
	if err := writeSSEFrame(w, rc, "retry: 5000\n\n"); err != nil {
		return
	}
	// Synthetic "ready" so the frontend can distinguish "connected but no
	// events yet" from "still connecting".
	if err := writeSSEFrame(w, rc, "event: ready\ndata: {}\n\n"); err != nil {
		return
	}

	// Subscribe to the conversation's Redis Stream. The EventBus is shared
	// across all SSE subscribers on this pod via a single per-conversation
	// tap goroutine — see internal/streaming/bus.go.
	streamCtx, cancelStream := context.WithCancel(r.Context())
	defer cancelStream()
	events := h.eventBus.Subscribe(streamCtx, conv.ID.String(), cursor)

	pingTicker := time.NewTicker(ssePingInterval)
	defer pingTicker.Stop()
	authTicker := time.NewTicker(sseAuthRecheckInterval)
	defer authTicker.Stop()
	maxAge := time.NewTimer(sseMaxAge)
	defer maxAge.Stop()

	convID := conv.ID
	orgID := conv.OrgID

	for {
		select {
		case event, ok := <-events:
			if !ok {
				return // upstream closed (ctx cancelled, tap stopped, or evicted)
			}

			// Strip redundant envelope fields for over-the-wire efficiency.
			// The browser already knows conversation_id (from URL) and
			// agent_id (from conversation metadata); they stay in Redis/DB
			// for the flusher and history endpoint.
			trimmed := trimSSEEnvelope(event.Data)

			frame := fmt.Sprintf("id: %s\nevent: %s\ndata: %s\n\n",
				event.ID, event.EventType, string(trimmed))
			if err := writeSSEFrame(w, rc, frame); err != nil {
				slog.Debug("SSE client disconnected", "conversation_id", convID)
				return
			}

		case <-pingTicker.C:
			if err := writeSSEFrame(w, rc, ": ping\n\n"); err != nil {
				return
			}

		case <-authTicker.C:
			// Re-verify that the caller still owns this conversation.
			// Drops silently if membership/key was revoked since connect.
			if !h.stillAuthorized(r.Context(), convID, orgID) {
				slog.Info("SSE auth recheck failed, closing stream",
					"conversation_id", convID)
				return
			}

		case <-maxAge.C:
			slog.Debug("SSE max age reached, closing for reconnect",
				"conversation_id", convID)
			return

		case <-r.Context().Done():
			return
		}
	}
}

// writeSSEFrame writes a single SSE frame with a bounded write deadline and
// flushes. Any error causes the caller to tear the stream down.
func writeSSEFrame(w http.ResponseWriter, rc *http.ResponseController, frame string) error {
	if err := rc.SetWriteDeadline(time.Now().Add(sseWriteDeadline)); err != nil && err != http.ErrNotSupported {
		return err
	}
	if _, err := w.Write([]byte(frame)); err != nil {
		return err
	}
	return rc.Flush()
}

// trimSSEEnvelope removes fields from the event envelope that the browser
// already knows from context (conversation_id from URL, agent_id from
// conversation metadata). The original envelope is preserved in Redis and
// Postgres for history / debugging. On parse error, returns the original
// bytes unchanged.
func trimSSEEnvelope(data json.RawMessage) json.RawMessage {
	if len(data) == 0 {
		return data
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(data, &obj); err != nil {
		return data
	}
	delete(obj, "conversation_id")
	delete(obj, "agent_id")
	out, err := json.Marshal(obj)
	if err != nil {
		return data
	}
	return out
}

// stillAuthorized re-checks that the conversation exists, belongs to the
// given org, and is still active. Returns false on any lookup failure so
// we err on the side of dropping the stream when auth state is uncertain.
func (h *ConversationHandler) stillAuthorized(ctx context.Context, convID uuid.UUID, orgID uuid.UUID) bool {
	var count int64
	if err := h.db.WithContext(ctx).
		Model(&model.AgentConversation{}).
		Where("id = ? AND org_id = ? AND status = ?", convID, orgID, "active").
		Count(&count).Error; err != nil {
		return false
	}
	return count == 1
}
