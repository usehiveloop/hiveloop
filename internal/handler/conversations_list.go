package handler

import (
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/usehiveloop/hiveloop/internal/middleware"
	"github.com/usehiveloop/hiveloop/internal/model"
)

// List handles GET /v1/agents/{agentID}/conversations.
// @Summary List conversations for an agent
// @Description Returns conversations for the specified agent.
// @Tags conversations
// @Produce json
// @Param agentID path string true "Agent ID"
// @Param status query string false "Filter by status (active, ended, error)"
// @Param limit query int false "Page size"
// @Param cursor query string false "Pagination cursor"
// @Success 200 {object} paginatedResponse[conversationResponse]
// @Security BearerAuth
// @Router /v1/agents/{agentID}/conversations [get]
func (h *ConversationHandler) List(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	agentID := chi.URLParam(r, "agentID")
	limit, cursor, err := parsePagination(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	q := h.db.Where("org_id = ? AND agent_id = ?", org.ID, agentID)
	if status := r.URL.Query().Get("status"); status != "" {
		q = q.Where("status = ?", status)
	}
	q = applyPagination(q, cursor, limit)

	var convs []model.AgentConversation
	if err := q.Find(&convs).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list conversations"})
		return
	}

	hasMore := len(convs) > limit
	if hasMore {
		convs = convs[:limit]
	}

	resp := make([]conversationResponse, len(convs))
	for i, c := range convs {
		resp[i] = conversationResponse{
			ID:        c.ID.String(),
			AgentID:   c.AgentID.String(),
			Name:      c.Name,
			Status:    c.Status,
			StreamURL: fmt.Sprintf("/v1/conversations/%s/stream", c.ID),
			CreatedAt: c.CreatedAt.Format(time.RFC3339),
		}
	}

	result := paginatedResponse[conversationResponse]{Data: resp, HasMore: hasMore}
	if hasMore {
		last := convs[len(convs)-1]
		c := encodeCursor(last.CreatedAt, last.ID)
		result.NextCursor = &c
	}
	writeJSON(w, http.StatusOK, result)
}

// Get handles GET /v1/conversations/{convID}.
// @Summary Get a conversation
// @Description Returns a conversation by ID.
// @Tags conversations
// @Produce json
// @Param convID path string true "Conversation ID"
// @Success 200 {object} conversationResponse
// @Failure 404 {object} errorResponse
// @Failure 410 {object} errorResponse "Conversation has ended"
// @Security BearerAuth
// @Router /v1/conversations/{convID} [get]
func (h *ConversationHandler) Get(w http.ResponseWriter, r *http.Request) {
	conv, ok := h.loadConversation(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, conversationResponse{
		ID:        conv.ID.String(),
		AgentID:   conv.AgentID.String(),
		Name:      conv.Name,
		Status:    conv.Status,
		StreamURL: fmt.Sprintf("/v1/conversations/%s/stream", conv.ID),
		CreatedAt: conv.CreatedAt.Format(time.RFC3339),
	})
}
