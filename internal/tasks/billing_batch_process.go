package tasks

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/oklog/ulid/v2"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/billing"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
)

const (
	billingBatchSize     = 1000
	billingBatchTimeout  = 4 * time.Minute
	billingErrUnknownMod = "unknown_model"
	billingErrUnresolved = "model_unresolved"
	billingErrInsufFunds = "insufficient_credits"
)

var errBillingModelUnresolved = errors.New("billing model unresolved")

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
	ProviderID      string
	Model           string
	AgentModel      *string
	InputTokens     int64
	OutputTokens    int64
	CachedTokens    int64
	ReasoningTokens int64
	Cost            float64
}

type rowBillingResult struct {
	CostUSD     float64
	CostSource  string
	Credits     int64
	BillingErr  string
	ExactCredit float64
}

// Whole batch runs in one TX so ledger writes and billed_at updates commit
// atomically. The debit is cumulative per org, so split worker ticks cannot
// change the final credits charged for the same set of generations.
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
		perRowResults := computeRows(rows)
		ledgerEntries, err := planCumulativeDebits(tx, rows, perRowResults, batchRefID)
		if err != nil {
			return err
		}
		if len(ledgerEntries) > 0 {
			if err := tx.Create(&ledgerEntries).Error; err != nil {
				return fmt.Errorf("bulk insert billing ledger entries: %w", err)
			}
			deducted = len(ledgerEntries)
		}

		now := time.Now().UTC()
		for _, r := range rows {
			result := perRowResults[r.ID]
			updates := map[string]any{
				"billed_at":           now,
				"billing_error":       result.BillingErr,
				"credits_debited":     result.Credits,
				"billing_cost_source": result.CostSource,
			}
			if result.CostUSD > 0 {
				updates["cost"] = result.CostUSD
			}
			if err := tx.Model(&model.Generation{}).Where("id = ?", r.ID).Updates(updates).Error; err != nil {
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
		SELECT g.id, g.org_id, g.provider_id, g.model,
		       a.model AS agent_model,
		       g.input_tokens, g.output_tokens,
		       g.cached_tokens, g.reasoning_tokens, g.cost
		FROM generations AS g
		LEFT JOIN tokens AS t ON t.jti = g.token_jti
		LEFT JOIN employees AS a ON a.id = NULLIF(t.meta->>'employee_id', '')::uuid
		WHERE g.billed_at IS NULL
		  AND g.is_system = TRUE
		  AND (g.cost > 0 OR g.input_tokens > 0 OR g.output_tokens > 0)
		ORDER BY g.created_at
		LIMIT ?
		FOR UPDATE OF g SKIP LOCKED
	`, limit).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	return rows, nil
}

// Rows with errors are still marked billed so they exit the unbilled queue.
func computeRows(rows []unbilledRow) map[string]rowBillingResult {
	perRowResult := make(map[string]rowBillingResult, len(rows))

	for _, r := range rows {
		cost, source, err := rowCostUSD(r)
		if err != nil {
			if errors.Is(err, errBillingModelUnresolved) {
				perRowResult[r.ID] = rowBillingResult{BillingErr: billingErrUnresolved}
				continue
			}
			if errors.Is(err, billing.ErrUnknownModel) {
				perRowResult[r.ID] = rowBillingResult{BillingErr: billingErrUnknownMod}
				continue
			}
			perRowResult[r.ID] = rowBillingResult{BillingErr: "compute: " + err.Error()}
			continue
		}
		if cost <= 0 {
			perRowResult[r.ID] = rowBillingResult{CostSource: source}
			continue
		}
		exact := cost / billing.CreditUSDValue
		base := int64(math.Floor(exact))
		perRowResult[r.ID] = rowBillingResult{
			CostUSD:     cost,
			CostSource:  source,
			Credits:     base,
			ExactCredit: exact,
		}
	}

	return perRowResult
}

func rowCostUSD(r unbilledRow) (float64, string, error) {
	if r.Cost > 0 {
		return r.Cost, billing.CostSourceProvider, nil
	}
	modelName := r.Model
	if modelName == "" && r.AgentModel != nil {
		modelName = *r.AgentModel
	}
	if modelName == "" {
		return 0, "", errBillingModelUnresolved
	}
	cost, err := billing.EstimateCostUSD(nil, r.ProviderID, modelName, r.InputTokens, r.OutputTokens, r.CachedTokens)
	if err != nil {
		return 0, "", err
	}
	return cost, billing.CostSourceRegistry, nil
}
