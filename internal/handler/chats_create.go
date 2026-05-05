package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/logging"
	"github.com/usehiveloop/hiveloop/internal/middleware"
	"github.com/usehiveloop/hiveloop/internal/model"
)

// @Summary Start a chat with an AI employee
// @Description Creates a chat session, persists the first user message, and
// @Description returns a one-shot SSE stream URL the frontend opens to receive
// @Description the assistant reply.
// @Tags chats
// @Accept json
// @Produce json
// @Param id path string true "Agent UUID"
// @Param body body sendMessageRequest true "First message"
// @Success 201 {object} sendMessageResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /v1/employees/{id}/chats [post]
func (h *ChatHandler) Create(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logging.FromContext(ctx)

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

	agentID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid agent id"})
		return
	}

	var req sendMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	req.Message = strings.TrimSpace(req.Message)
	if req.Message == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "message is required"})
		return
	}

	var agent model.Agent
	if err := h.db.WithContext(ctx).
		Where("id = ? AND org_id = ? AND is_employee = true AND deleted_at IS NULL", agentID, org.ID).
		First(&agent).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "employee not found"})
			return
		}
		log.ErrorContext(ctx, "load employee for chat", "error", err, "agent_id", agentID)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load employee"})
		return
	}

	session := model.ChatSession{
		OrgID:   org.ID,
		AgentID: agent.ID,
		UserID:  user.ID,
	}
	userMsg := model.ChatMessage{Role: "user", Content: req.Message}

	err = h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&session).Error; err != nil {
			return fmt.Errorf("create session: %w", err)
		}
		userMsg.SessionID = session.ID
		if err := tx.Create(&userMsg).Error; err != nil {
			return fmt.Errorf("create message: %w", err)
		}
		return nil
	})
	if err != nil {
		log.ErrorContext(ctx, "create chat session", "error", err, "agent_id", agentID)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create chat"})
		return
	}

	tok, err := h.mintStreamToken(session.ID, user.ID)
	if err != nil {
		log.ErrorContext(ctx, "mint stream token", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to issue stream token"})
		return
	}

	writeJSON(w, http.StatusCreated, sendMessageResponse{
		SessionID: session.ID.String(),
		StreamURL: h.streamURL(session.ID, tok),
	})
}

// @Summary Append a user message to an existing chat
// @Description Persists a new user message and returns a fresh stream URL
// @Description the frontend opens to receive the assistant reply.
// @Tags chats
// @Accept json
// @Produce json
// @Param id path string true "Chat session UUID"
// @Param body body sendMessageRequest true "Next message"
// @Success 200 {object} sendMessageResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /v1/chats/{id}/messages [post]
func (h *ChatHandler) Send(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logging.FromContext(ctx)

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

	var req sendMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	req.Message = strings.TrimSpace(req.Message)
	if req.Message == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "message is required"})
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
		log.ErrorContext(ctx, "load chat session", "error", err, "session_id", sessionID)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load session"})
		return
	}

	if err := h.db.WithContext(ctx).Create(&model.ChatMessage{
		SessionID: session.ID, Role: "user", Content: req.Message,
	}).Error; err != nil {
		log.ErrorContext(ctx, "append user message", "error", err, "session_id", sessionID)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to append message"})
		return
	}

	tok, err := h.mintStreamToken(session.ID, user.ID)
	if err != nil {
		log.ErrorContext(ctx, "mint stream token", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to issue stream token"})
		return
	}

	writeJSON(w, http.StatusOK, sendMessageResponse{
		SessionID: session.ID.String(),
		StreamURL: h.streamURL(session.ID, tok),
	})
}
