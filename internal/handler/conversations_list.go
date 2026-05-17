package handler

import (
	"fmt"
	"net/http"
	"time"
)

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
