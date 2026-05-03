package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/middleware"
	"github.com/usehiveloop/hiveloop/internal/model"
)

// ListMessages handles GET /v1/conversations/{convID}/messages.
// @Summary List conversation messages
// @Description Returns chat-ready messages aggregated from raw events. Consecutive same-name tool calls are grouped (e.g. "bash" called 11 times in a row → one group with 11 calls). Pagination is by sequence_number; aggregation is per-page so a tool call split across pages will not pair.
// @Tags conversations
// @Produce json
// @Param convID path string true "Conversation ID"
// @Param limit query int false "Max events scanned per page (default 200, max 1000). Aggregated message count may be smaller."
// @Param cursor query string false "Sequence number from the previous page's tail. Returns events with sequence_number strictly greater than this value."
// @Success 200 {object} paginatedResponse[conversationMessageResponse]
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /v1/conversations/{convID}/messages [get]
func (h *ConversationHandler) ListMessages(w http.ResponseWriter, r *http.Request) {
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

	limit, afterSeq, err := parseMessagesPagination(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	q := h.db.Where("conversation_id = ?", conv.ID).Order("sequence_number ASC").Limit(limit + 1)
	if afterSeq != nil {
		q = q.Where("sequence_number > ?", *afterSeq)
	}

	var events []model.ConversationEvent
	if err := q.Find(&events).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list messages"})
		return
	}

	hasMore := len(events) > limit
	if hasMore {
		events = events[:limit]
	}

	messages := aggregateMessages(events)

	result := paginatedResponse[conversationMessageResponse]{Data: messages, HasMore: hasMore}
	if hasMore && len(events) > 0 {
		c := strconv.FormatInt(events[len(events)-1].SequenceNumber, 10)
		result.NextCursor = &c
	}
	writeJSON(w, http.StatusOK, result)
}

func parseMessagesPagination(r *http.Request) (int, *int64, error) {
	limit := 200
	if l := r.URL.Query().Get("limit"); l != "" {
		n, err := strconv.Atoi(l)
		if err != nil || n < 1 {
			return 0, nil, fmt.Errorf("invalid limit")
		}
		if n > 1000 {
			n = 1000
		}
		limit = n
	}

	var after *int64
	if c := r.URL.Query().Get("cursor"); c != "" && c != "0" {
		n, err := strconv.ParseInt(c, 10, 64)
		if err != nil {
			return 0, nil, fmt.Errorf("invalid cursor")
		}
		after = &n
	}

	return limit, after, nil
}

func aggregateMessages(events []model.ConversationEvent) []conversationMessageResponse {
	out := make([]conversationMessageResponse, 0, len(events))
	var current *conversationMessageResponse

	ensureAgent := func(e model.ConversationEvent) *conversationMessageResponse {
		if current != nil {
			return current
		}
		out = append(out, conversationMessageResponse{
			ID:        fmt.Sprintf("agent-%d", e.SequenceNumber),
			Author:    "agent",
			Timestamp: e.Timestamp.Format("2006-01-02T15:04:05Z07:00"),
		})
		current = &out[len(out)-1]
		return current
	}

	appendCall := func(m *conversationMessageResponse, name string, call conversationToolCallResponse) {
		if n := len(m.ToolGroups); n > 0 && m.ToolGroups[n-1].Name == name {
			m.ToolGroups[n-1].Calls = append(m.ToolGroups[n-1].Calls, call)
			return
		}
		m.ToolGroups = append(m.ToolGroups, conversationToolGroupResponse{
			Name:  name,
			Calls: []conversationToolCallResponse{call},
		})
	}

	for _, e := range events {
		data := decodeEventData(e.Data)
		switch e.EventType {
		case "message_received":
			body, _ := data["content"].(string)
			out = append(out, conversationMessageResponse{
				ID:        fmt.Sprintf("msg-%d", e.SequenceNumber),
				Author:    "user",
				Timestamp: e.Timestamp.Format("2006-01-02T15:04:05Z07:00"),
				Body:      body,
			})
			current = nil

		case "tool_call_completed":
			m := ensureAgent(e)
			toolID, _ := data["tool_call_id"].(string)
			name, _ := data["title"].(string)
			if name == "" {
				name = "tool"
			}
			appendCall(m, name, conversationToolCallResponse{
				ID:      toolID,
				Title:   name,
				Status:  "completed",
				Summary: summaryFromRawOutput(data["raw_output"]),
			})

		case "turn_completed":
			current = nil
		}
	}

	return out
}

func decodeEventData(raw model.RawJSON) map[string]any {
	if len(raw) == 0 {
		return map[string]any{}
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return map[string]any{}
	}
	return m
}

func summaryFromRawOutput(raw any) string {
	if raw == nil {
		return ""
	}
	if s, ok := raw.(string); ok {
		return s
	}
	if obj, ok := raw.(map[string]any); ok {
		if s, ok := obj["output"].(string); ok {
			return s
		}
	}
	b, err := json.Marshal(raw)
	if err != nil {
		return ""
	}
	return string(b)
}
