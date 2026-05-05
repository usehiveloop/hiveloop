package handler

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/logging"
	"github.com/usehiveloop/hiveloop/internal/model"
)

const streamHTTPTimeout = 5 * time.Minute

// @Summary Stream the assistant reply for the latest user message (SSE)
// @Description Opens an SSE stream to the sandbox, replays the conversation
// @Description history through Hermes, and tees the response back to the
// @Description browser while persisting the assistant message on completion.
// @Tags chats
// @Produce text/event-stream
// @Param id path string true "Chat session UUID"
// @Param token query string true "Stream token issued by POST /chats or /chats/{id}/messages"
// @Success 200 {string} string "SSE event stream"
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Router /v1/chats/{id}/stream [get]
func (h *ChatHandler) Stream(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logging.FromContext(ctx)

	tokenStr := r.URL.Query().Get("token")
	if tokenStr == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing token"})
		return
	}
	tokSession, _, err := h.validateStreamToken(tokenStr)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid stream token"})
		return
	}
	pathSession, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil || pathSession != tokSession {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "token does not match session"})
		return
	}

	var session model.ChatSession
	if err := h.db.WithContext(ctx).
		Where("id = ? AND deleted_at IS NULL", pathSession).First(&session).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "chat session not found"})
			return
		}
		log.ErrorContext(ctx, "load session", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load session"})
		return
	}

	var messages []model.ChatMessage
	if err := h.db.WithContext(ctx).
		Where("session_id = ?", session.ID).Order("created_at ASC").
		Find(&messages).Error; err != nil {
		log.ErrorContext(ctx, "load messages", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load messages"})
		return
	}

	var sb model.Sandbox
	if err := h.db.WithContext(ctx).
		Where("agent_id = ? AND org_id = ?", session.AgentID, session.OrgID).
		Order("created_at DESC").Limit(1).First(&sb).Error; err != nil {
		log.ErrorContext(ctx, "load sandbox", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load sandbox"})
		return
	}

	apiKey, err := h.encKey.DecryptString(sb.EncryptedBridgeAPIKey)
	if err != nil {
		log.ErrorContext(ctx, "decrypt sidecar key", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to read sandbox credentials"})
		return
	}
	if err := h.orch.EnsureHermesSandboxReady(ctx, &sb, apiKey); err != nil {
		log.ErrorContext(ctx, "ensure sandbox ready", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "sandbox not ready"})
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	rc := http.NewResponseController(w)
	_ = rc.Flush()

	ctx, cancel := context.WithTimeout(ctx, streamHTTPTimeout)
	defer cancel()

	resp, err := h.postSidecarStream(ctx, sb.BridgeURL, apiKey, messages)
	if err != nil {
		log.ErrorContext(ctx, "open sidecar stream", "error", err, "session_id", session.ID)
		writeSSEError(w, rc, "stream open failed")
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		log.ErrorContext(ctx, "sidecar non-200", "status", resp.StatusCode, "body", string(body))
		writeSSEError(w, rc, "sandbox rejected stream")
		return
	}

	content, responseID := teeAssistantStream(w, rc, resp.Body)

	persistCtx, persistCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer persistCancel()
	if err := h.persistAssistantMessage(persistCtx, session.ID, content, responseID); err != nil {
		log.ErrorContext(ctx, "persist assistant message", "error", err, "session_id", session.ID)
	}
}

func (h *ChatHandler) postSidecarStream(ctx context.Context, baseURL, apiKey string, msgs []model.ChatMessage) (*http.Response, error) {
	type msgDTO struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	out := make([]msgDTO, 0, len(msgs))
	for _, m := range msgs {
		out = append(out, msgDTO{Role: m.Role, Content: m.Content})
	}
	body, err := json.Marshal(map[string]any{
		"model": chatModel, "messages": out, "stream": true,
	})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		strings.TrimRight(baseURL, "/")+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	return (&http.Client{Timeout: streamHTTPTimeout}).Do(req)
}

func teeAssistantStream(w http.ResponseWriter, rc *http.ResponseController, body io.Reader) (string, string) {
	reader := bufio.NewReaderSize(body, 64*1024)
	var content strings.Builder
	var responseID string

	for {
		line, err := reader.ReadString('\n')
		if line != "" {
			_, _ = w.Write([]byte(line))
			_ = rc.Flush()
			extractDelta(line, &content, &responseID)
		}
		if err != nil {
			return content.String(), responseID
		}
	}
}

func extractDelta(line string, content *strings.Builder, responseID *string) {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, "data:") {
		return
	}
	payload := strings.TrimSpace(strings.TrimPrefix(trimmed, "data:"))
	if payload == "" || payload == "[DONE]" {
		return
	}
	var chunk struct {
		ID      string `json:"id"`
		Choices []struct {
			Delta struct {
				Content string `json:"content"`
			} `json:"delta"`
		} `json:"choices"`
	}
	if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
		return
	}
	if *responseID == "" && chunk.ID != "" {
		*responseID = chunk.ID
	}
	for _, c := range chunk.Choices {
		content.WriteString(c.Delta.Content)
	}
}

func (h *ChatHandler) persistAssistantMessage(ctx context.Context, sessionID uuid.UUID, content, responseID string) error {
	if content == "" && responseID == "" {
		return nil
	}
	return h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&model.ChatMessage{
			SessionID: sessionID, Role: "assistant",
			Content: content, HermesResponseID: responseID,
		}).Error; err != nil {
			return err
		}
		updates := map[string]any{"updated_at": time.Now()}
		if responseID != "" {
			updates["last_response_id"] = responseID
		}
		return tx.Model(&model.ChatSession{}).Where("id = ?", sessionID).Updates(updates).Error
	})
}

func writeSSEError(w http.ResponseWriter, rc *http.ResponseController, msg string) {
	_, _ = fmt.Fprintf(w, "event: error\ndata: %q\n\n", msg)
	_ = rc.Flush()
}
