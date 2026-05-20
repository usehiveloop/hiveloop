package tasks

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/oklog/ulid/v2"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/billing"
	"github.com/usehiveloop/hiveloop/internal/logging"
)

const (
	billingBatchSize     = 1000
	billingBatchTimeout  = 4 * time.Minute
	billingErrUnknownMod = "unknown_model"
	billingErrUnresolved = "model_unresolved"
	billingErrInsufFunds = "insufficient_credits"
)

type BillingBatchProcessHandler struct {
	db      *gorm.DB
	credits *billing.CreditsService
}

func NewBillingBatchProcessHandler(db *gorm.DB, credits *billing.CreditsService) *BillingBatchProcessHandler {
	return &BillingBatchProcessHandler{db: db, credits: credits}
}

type unbilledRow struct {
	ID              string
	OrgID           uuid.UUID
	Model           string
	AgentModel      *string
	InputTokens     int64
	OutputTokens    int64
	CachedTokens    int64
	ReasoningTokens int64
}

// Whole batch runs in one TX so per-org Spend writes and billed_at updates
// commit atomically — a crash mid-batch leaves rows for the next tick.
func (h *BillingBatchProcessHandler) Handle(ctx context.Context, _ *asynq.Task) error {
	ctx, cancel := context.WithTimeout(ctx, billingBatchTimeout)
	defer cancel()

	var processed, deducted int

	err := h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		rows, err := selectUnbilledBatch(tx, billingBatchSize)
		if err != nil {
			return fmt.Errorf("select unbilled batch: %w", err)
		}
		if len(rows) == 0 {
			return nil
		}

		batchRefID := "batch_" + ulid.Make().String()
		ledgerWrites, perRowErrors := computeBatch(rows)

		for orgID, amount := range ledgerWrites {
			if err := billing.SpendWithTx(tx, orgID, amount, billing.ReasonLLMTokens, "generation_batch", batchRefID); err != nil {
				if billing.IsUniqueViolation(err) {
					continue
				}
				if errors.Is(err, billing.ErrInsufficientCredits) {
					for _, r := range rows {
						if r.OrgID == orgID && perRowErrors[r.ID] == "" {
							perRowErrors[r.ID] = billingErrInsufFunds
						}
					}
					delete(ledgerWrites, orgID)
					continue
				}
				return fmt.Errorf("spend org %s: %w", orgID, err)
			}
			deducted++
		}

		now := time.Now().UTC()
		for _, r := range rows {
			if err := tx.Exec(
				`UPDATE generations SET billed_at = ?, billing_error = ? WHERE id = ?`,
				now, perRowErrors[r.ID], r.ID,
			).Error; err != nil {
				return fmt.Errorf("mark billed %s: %w", r.ID, err)
			}
		}
		processed = len(rows)
		return nil
	})
	if err != nil {
		return err
	}

	if processed > 0 {
		logging.FromContext(ctx).InfoContext(ctx, "billing batch processed",
			"rows", processed,
			"orgs_deducted", deducted,
		)
	}
	return nil
}

func selectUnbilledBatch(tx *gorm.DB, limit int) ([]unbilledRow, error) {
	rows := []unbilledRow{}
	err := tx.Raw(`
		SELECT g.id, g.org_id, g.model,
		       a.model AS agent_model,
		       g.input_tokens, g.output_tokens,
		       g.cached_tokens, g.reasoning_tokens
		FROM generations AS g
		LEFT JOIN tokens AS t ON t.jti = g.token_jti
		LEFT JOIN employees AS a ON a.id = NULLIF(t.meta->>'agent_id', '')::uuid
		WHERE g.billed_at IS NULL
		  AND g.is_system = TRUE
		  AND (g.input_tokens > 0 OR g.output_tokens > 0)
		ORDER BY g.created_at
		LIMIT ?
		FOR UPDATE OF g SKIP LOCKED
	`, limit).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	return rows, nil
}

// Rows in perRowErr are still marked billed so they exit the unbilled queue.
func computeBatch(rows []unbilledRow) (map[uuid.UUID]int64, map[string]string) {
	perOrg := make(map[uuid.UUID]int64)
	perRowErr := make(map[string]string)

	for _, r := range rows {
		modelName := r.Model
		if modelName == "" && r.AgentModel != nil {
			modelName = *r.AgentModel
		}
		if modelName == "" {
			perRowErr[r.ID] = billingErrUnresolved
			continue
		}

		credits, err := billing.TokensToCredits(modelName, r.InputTokens, r.OutputTokens)
		if err != nil {
			if errors.Is(err, billing.ErrUnknownModel) {
				perRowErr[r.ID] = billingErrUnknownMod
				continue
			}
			perRowErr[r.ID] = "compute: " + err.Error()
			continue
		}
		if credits == 0 {
			continue
		}
		perOrg[r.OrgID] += credits
	}
	return perOrg, perRowErr
}
