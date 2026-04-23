package handler

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/middleware"
	"github.com/usehiveloop/hiveloop/internal/model"
)

// ListEvents handles GET /v1/conversations/{convID}/events.
// @Summary List conversation events
// @Description Returns webhook events persisted for the conversation. Filterable by event type.
// @Tags conversations
// @Produce json
// @Param convID path string true "Conversation ID"
// @Param type query string false "Filter by event type (e.g. MessageReceived, ResponseCompleted)"
// @Param limit query int false "Page size"
// @Param cursor query string false "Pagination cursor"
// @Success 200 {object} paginatedResponse[conversationEventResponse]
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /v1/conversations/{convID}/events [get]
func (h *ConversationHandler) ListEvents(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	convID := chi.URLParam(r, "convID")
	var conv model.AgentConversation
	if err := h.db.Where("id = ? AND org_id = ?", convID, org.ID).First(&conv).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "conversation not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load conversation"})
		return
	}

	limit, cursor, err := parsePagination(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	q := h.db.Where("conversation_id = ?", conv.ID)
	if eventType := r.URL.Query().Get("type"); eventType != "" {
		q = q.Where("event_type = ?", eventType)
	}
	q = applyPagination(q, cursor, limit)

	var events []model.ConversationEvent
	if err := q.Find(&events).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list events"})
		return
	}

	hasMore := len(events) > limit
	if hasMore {
		events = events[:limit]
	}

	resp := make([]conversationEventResponse, len(events))
	for i, e := range events {
		resp[i] = conversationEventResponse{
			ID:                   e.ID.String(),
			EventID:              e.EventID,
			EventType:            e.EventType,
			AgentID:              e.AgentID,
			BridgeConversationID: e.BridgeConversationID,
			Timestamp:            e.Timestamp.Format("2006-01-02T15:04:05Z07:00"),
			SequenceNumber:       e.SequenceNumber,
			Data:                 json.RawMessage(e.Data),
			CreatedAt:            e.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		}
	}

	result := paginatedResponse[conversationEventResponse]{Data: resp, HasMore: hasMore}
	if hasMore {
		last := events[len(events)-1]
		c := encodeCursor(last.CreatedAt, last.ID)
		result.NextCursor = &c
	}
	writeJSON(w, http.StatusOK, result)
}

// History handles GET /v1/conversations/{convID}/history.
// @Summary Hydrate conversation history
// @Description Returns persisted conversation events in chronological order, intended for hydrating a UI before opening the SSE stream. Paginated via since=<event_id>. Unlike GET /events, this endpoint sorts events ASC by sequence_number so the caller can render in order.
// @Tags conversations
// @Produce json
// @Param convID path string true "Conversation ID"
// @Param since query string false "Return events with sequence_number greater than this event_id"
// @Param limit query int false "Page size (default 200, max 1000)"
// @Success 200 {object} conversationHistoryResponse
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /v1/conversations/{convID}/history [get]
func (h *ConversationHandler) History(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	convID := chi.URLParam(r, "convID")
	var conv model.AgentConversation
	if err := h.db.Where("id = ? AND org_id = ?", convID, org.ID).First(&conv).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "conversation not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load conversation"})
		return
	}

	limit := 200
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconvAtoiPositive(l); err == nil {
			if n > 1000 {
				n = 1000
			}
			limit = n
		}
	}

	q := h.db.Where("conversation_id = ?", conv.ID).Order("sequence_number ASC, created_at ASC").Limit(limit + 1)
	if since := r.URL.Query().Get("since"); since != "" {
		var anchor model.ConversationEvent
		if err := h.db.Where("conversation_id = ? AND event_id = ?", conv.ID, since).
			First(&anchor).Error; err == nil {
			q = q.Where("sequence_number > ?", anchor.SequenceNumber)
		}
	}

	var events []model.ConversationEvent
	if err := q.Find(&events).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load history"})
		return
	}

	hasMore := len(events) > limit
	if hasMore {
		events = events[:limit]
	}

	resp := conversationHistoryResponse{
		ConversationID: conv.ID.String(),
		HasMore:        hasMore,
		Events:         make([]conversationEventResponse, len(events)),
	}
	for i, e := range events {
		resp.Events[i] = conversationEventResponse{
			ID:                   e.ID.String(),
			EventID:              e.EventID,
			EventType:            e.EventType,
			AgentID:              e.AgentID,
			BridgeConversationID: e.BridgeConversationID,
			Timestamp:            e.Timestamp.Format("2006-01-02T15:04:05Z07:00"),
			SequenceNumber:       e.SequenceNumber,
			Data:                 json.RawMessage(e.Data),
			CreatedAt:            e.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		}
	}
	if len(events) > 0 {
		resp.LastEventID = events[len(events)-1].EventID
	}
	writeJSON(w, http.StatusOK, resp)
}
