package handler

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/crypto"
	"github.com/usehiveloop/hiveloop/internal/sandbox"
)

const (
	streamTokenTTL = 5 * time.Minute
	chatModel      = "hermes-agent"
)

type ChatHandler struct {
	db         *gorm.DB
	orch       *sandbox.Orchestrator
	encKey     *crypto.SymmetricKey
	signingKey []byte
}

func NewChatHandler(db *gorm.DB, orch *sandbox.Orchestrator, encKey *crypto.SymmetricKey, signingKey []byte) *ChatHandler {
	return &ChatHandler{db: db, orch: orch, encKey: encKey, signingKey: signingKey}
}

type chatSessionDTO struct {
	ID        string `json:"id"`
	AgentID   string `json:"agent_id"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

type chatMessageDTO struct {
	ID        string `json:"id"`
	Role      string `json:"role"`
	Content   string `json:"content"`
	CreatedAt string `json:"created_at"`
}

type sendMessageRequest struct {
	Message string `json:"message"`
}

type sendMessageResponse struct {
	SessionID string `json:"session_id"`
	StreamURL string `json:"stream_url"`
}

type streamTokenClaims struct {
	SessionID string `json:"sid"`
	UserID    string `json:"uid"`
	jwt.RegisteredClaims
}

func (h *ChatHandler) mintStreamToken(sessionID, userID uuid.UUID) (string, error) {
	now := time.Now()
	claims := streamTokenClaims{
		SessionID: sessionID.String(),
		UserID:    userID.String(),
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(streamTokenTTL)),
		},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return tok.SignedString(h.signingKey)
}

func (h *ChatHandler) validateStreamToken(tokenStr string) (sessionID, userID uuid.UUID, err error) {
	claims := &streamTokenClaims{}
	_, err = jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method")
		}
		return h.signingKey, nil
	})
	if err != nil {
		return uuid.Nil, uuid.Nil, err
	}
	sessionID, err = uuid.Parse(claims.SessionID)
	if err != nil {
		return uuid.Nil, uuid.Nil, fmt.Errorf("invalid sid: %w", err)
	}
	userID, err = uuid.Parse(claims.UserID)
	if err != nil {
		return uuid.Nil, uuid.Nil, fmt.Errorf("invalid uid: %w", err)
	}
	return sessionID, userID, nil
}

func (h *ChatHandler) streamURL(sessionID uuid.UUID, token string) string {
	return fmt.Sprintf("/v1/chats/%s/stream?token=%s", sessionID, token)
}

var errChatNotFound = errors.New("chat session not found")
