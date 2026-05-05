package handler

import (
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/logging"
	"github.com/usehiveloop/hiveloop/internal/middleware"
	"github.com/usehiveloop/hiveloop/internal/model"
)

type chatListResponse struct {
	Data []chatSessionDTO `json:"data"`
}

type chatDetailResponse struct {
	Session  chatSessionDTO   `json:"session"`
	Messages []chatMessageDTO `json:"messages"`
}

// @Summary List chat sessions for the caller
// @Tags chats
// @Produce json
// @Success 200 {object} chatListResponse
// @Security BearerAuth
// @Router /v1/chats [get]
func (h *ChatHandler) List(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	org, ok := middleware.OrgFromContext(ctx)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}
	user, ok := middleware.UserFromContext(ctx)
	if !ok || user == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing user context"})
		return
	}

	var sessions []model.ChatSession
	if err := h.db.WithContext(ctx).
		Where("org_id = ? AND user_id = ? AND deleted_at IS NULL", org.ID, user.ID).
		Order("updated_at DESC").Limit(100).
		Find(&sessions).Error; err != nil {
		logging.FromContext(ctx).ErrorContext(ctx, "list chat sessions", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list sessions"})
		return
	}

	out := make([]chatSessionDTO, len(sessions))
	for i, s := range sessions {
		out[i] = sessionDTO(s)
	}
	writeJSON(w, http.StatusOK, chatListResponse{Data: out})
}

// @Summary Get a chat session with its messages
// @Tags chats
// @Produce json
// @Param id path string true "Chat session UUID"
// @Success 200 {object} chatDetailResponse
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /v1/chats/{id} [get]
func (h *ChatHandler) Get(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	org, ok := middleware.OrgFromContext(ctx)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}
	user, ok := middleware.UserFromContext(ctx)
	if !ok || user == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing user context"})
		return
	}

	sessionID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid session id"})
		return
	}

	var session model.ChatSession
	if err := h.db.WithContext(ctx).
		Where("id = ? AND org_id = ? AND user_id = ? AND deleted_at IS NULL",
			sessionID, org.ID, user.ID).
		First(&session).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "chat session not found"})
			return
		}
		logging.FromContext(ctx).ErrorContext(ctx, "load chat session", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load session"})
		return
	}

	var msgs []model.ChatMessage
	if err := h.db.WithContext(ctx).
		Where("session_id = ?", session.ID).Order("created_at ASC").
		Find(&msgs).Error; err != nil {
		logging.FromContext(ctx).ErrorContext(ctx, "load chat messages", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load messages"})
		return
	}

	out := chatDetailResponse{
		Session:  sessionDTO(session),
		Messages: make([]chatMessageDTO, len(msgs)),
	}
	for i, m := range msgs {
		out.Messages[i] = chatMessageDTO{
			ID: m.ID.String(), Role: m.Role, Content: m.Content,
			CreatedAt: m.CreatedAt.UTC().Format(time.RFC3339),
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func sessionDTO(s model.ChatSession) chatSessionDTO {
	return chatSessionDTO{
		ID:        s.ID.String(),
		AgentID:   s.AgentID.String(),
		CreatedAt: s.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt: s.UpdatedAt.UTC().Format(time.RFC3339),
	}
}
