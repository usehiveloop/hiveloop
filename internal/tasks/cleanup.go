package tasks

import (
	"context"
	"log/slog"
	"time"

	"github.com/hibiken/asynq"
	"gorm.io/gorm"

	"github.com/ziraloop/ziraloop/internal/model"
	"github.com/ziraloop/ziraloop/internal/sandbox"
	"github.com/ziraloop/ziraloop/internal/streaming"
)

// --- Token Cleanup ---

// TokenCleanupHandler deletes expired email verifications, password resets, and OAuth exchange tokens.
type TokenCleanupHandler struct {
	db *gorm.DB
}

func NewTokenCleanupHandler(db *gorm.DB) *TokenCleanupHandler {
	return &TokenCleanupHandler{db: db}
}

func (h *TokenCleanupHandler) Handle(ctx context.Context, _ *asynq.Task) error {
	cutoff := time.Now().Add(-7 * 24 * time.Hour)
	h.db.WithContext(ctx).Where("expires_at < ? OR used_at < ?", cutoff, cutoff).Delete(&model.EmailVerification{})
	h.db.WithContext(ctx).Where("expires_at < ? OR used_at < ?", cutoff, cutoff).Delete(&model.PasswordReset{})
	h.db.WithContext(ctx).Where("expires_at < ? OR used_at < ?", cutoff, cutoff).Delete(&model.OAuthExchangeToken{})
	slog.Debug("cleaned up expired verification/reset tokens")
	return nil
}

// --- Stream Cleanup ---

// StreamCleanupHandler removes idle conversation streams from Redis.
type StreamCleanupHandler struct {
	cleanup *streaming.Cleanup
}

func NewStreamCleanupHandler(cleanup *streaming.Cleanup) *StreamCleanupHandler {
	return &StreamCleanupHandler{cleanup: cleanup}
}

func (h *StreamCleanupHandler) Handle(ctx context.Context, _ *asynq.Task) error {
	h.cleanup.CleanIdle(ctx)
	return nil
}

// --- Sandbox Health Check ---

// SandboxHealthCheckHandler syncs sandbox status from the provider.
type SandboxHealthCheckHandler struct {
	orchestrator *sandbox.Orchestrator
}

func NewSandboxHealthCheckHandler(orchestrator *sandbox.Orchestrator) *SandboxHealthCheckHandler {
	return &SandboxHealthCheckHandler{orchestrator: orchestrator}
}

func (h *SandboxHealthCheckHandler) Handle(ctx context.Context, _ *asynq.Task) error {
	h.orchestrator.RunHealthCheck(ctx)
	return nil
}

// --- Sandbox Resource Check ---

// SandboxResourceCheckHandler collects cgroup resource stats from running sandboxes.
type SandboxResourceCheckHandler struct {
	orchestrator *sandbox.Orchestrator
}

func NewSandboxResourceCheckHandler(orchestrator *sandbox.Orchestrator) *SandboxResourceCheckHandler {
	return &SandboxResourceCheckHandler{orchestrator: orchestrator}
}

func (h *SandboxResourceCheckHandler) Handle(ctx context.Context, _ *asynq.Task) error {
	h.orchestrator.RunResourceCheck(ctx)
	return nil
}
