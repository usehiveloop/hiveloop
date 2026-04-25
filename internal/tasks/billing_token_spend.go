package tasks

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"

	"github.com/usehiveloop/hiveloop/internal/billing"
)

// BillingTokenSpendPayload is the payload for TypeBillingTokenSpend tasks.
//
// Fired by the proxy's Generation middleware for every completed LLM call
// backed by a system credential (platform keys). BYOK calls don't enqueue
// this task — they don't consume credits for inference.
//
// GenerationID is the idempotency anchor: the same generation never produces
// two deductions because the credit ledger has a unique index on
// (org_id, reason, ref_type, ref_id) that rejects the second insert.
type BillingTokenSpendPayload struct {
	OrgID        uuid.UUID `json:"org_id"`
	GenerationID string    `json:"generation_id"`
	Model        string    `json:"model"`
	InputTokens  int64     `json:"input_tokens"`
	OutputTokens int64     `json:"output_tokens"`
}

// NewBillingTokenSpendTask builds an Asynq task that deducts credits for one
// LLM call. Queued on the default queue with modest retries — transient DB
// failures are handled by retry, permanent ones dead-letter so we can
// investigate.
func NewBillingTokenSpendTask(p BillingTokenSpendPayload) (*asynq.Task, error) {
	payload, err := json.Marshal(p)
	if err != nil {
		return nil, fmt.Errorf("marshal billing token spend payload: %w", err)
	}
	return asynq.NewTask(
		TypeBillingTokenSpend,
		payload,
		asynq.Queue(QueueDefault),
		asynq.MaxRetry(5),
		asynq.Timeout(15*time.Second),
	), nil
}

// BillingTokenSpendHandler applies the ledger deduction for one LLM call.
type BillingTokenSpendHandler struct {
	credits *billing.CreditsService
}

// NewBillingTokenSpendHandler constructs the handler.
func NewBillingTokenSpendHandler(credits *billing.CreditsService) *BillingTokenSpendHandler {
	return &BillingTokenSpendHandler{credits: credits}
}

// Handle implements asynq.HandlerFunc.
//
// Failure modes:
//   - Malformed payload: SkipRetry — retrying won't decode it differently.
//   - Unknown model: logged, SkipRetry. Needs billing.modelRates update.
//   - Insufficient balance: the user got the inference before balance hit
//     zero (concurrent race). We can't undo; log and SkipRetry. Next call
//     402s at the proxy gate.
//   - Already-recorded idempotent replay: INFO log, treat as success.
//   - Transient DB error: return it so asynq retries with backoff.
func (h *BillingTokenSpendHandler) Handle(ctx context.Context, task *asynq.Task) error {
	var payload BillingTokenSpendPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return fmt.Errorf("%w: %w", asynq.SkipRetry, err)
	}

	credits, err := billing.TokensToCredits(payload.Model, payload.InputTokens, payload.OutputTokens)
	if err != nil {
		if errors.Is(err, billing.ErrUnknownModel) {
			slog.ErrorContext(ctx, "billing: unknown model, skipping deduction",
				"org_id", payload.OrgID,
				"generation_id", payload.GenerationID,
				"model", payload.Model,
			)
			return fmt.Errorf("%w: %w", asynq.SkipRetry, err)
		}
		return fmt.Errorf("tokens to credits: %w", err)
	}

	if credits == 0 {
		return nil
	}

	err = h.credits.Spend(
		payload.OrgID,
		credits,
		billing.ReasonLLMTokens,
		"generation",
		payload.GenerationID,
	)
	switch {
	case err == nil:
		slog.InfoContext(ctx, "billing: spend recorded",
			"org_id", payload.OrgID,
			"generation_id", payload.GenerationID,
			"credits", credits,
			"model", payload.Model,
		)
		return nil
	case errors.Is(err, billing.ErrAlreadyRecorded):
		slog.InfoContext(ctx, "billing: spend already recorded (idempotent replay)",
			"org_id", payload.OrgID,
			"generation_id", payload.GenerationID,
		)
		return nil
	case errors.Is(err, billing.ErrInsufficientCredits):
		slog.WarnContext(ctx, "billing: insufficient credits at deduction time",
			"org_id", payload.OrgID,
			"generation_id", payload.GenerationID,
			"credits", credits,
		)
		return fmt.Errorf("%w: %w", asynq.SkipRetry, err)
	default:
		return fmt.Errorf("spend: %w", err)
	}
}
