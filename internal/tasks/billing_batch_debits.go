package tasks

import (
	"fmt"
	"math"
	"sort"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/billing"
	"github.com/usehivy/hivy/internal/model"
)

func planCumulativeDebits(tx *gorm.DB, rows []unbilledRow, perRowResults map[string]rowBillingResult, batchRefID string) ([]model.CreditLedgerEntry, error) {
	rowsByOrg := successfulRowsByOrg(rows, perRowResults)
	orgIDs := make([]uuid.UUID, 0, len(rowsByOrg))
	for orgID := range rowsByOrg {
		orgIDs = append(orgIDs, orgID)
	}
	sort.Slice(orgIDs, func(i, j int) bool { return orgIDs[i].String() < orgIDs[j].String() })

	entries := make([]model.CreditLedgerEntry, 0, len(orgIDs))
	for _, orgID := range orgIDs {
		if err := lockOrgForBilling(tx, orgID); err != nil {
			return nil, err
		}

		currentCost := currentRowsCost(rowsByOrg[orgID], perRowResults)
		existingCost, err := successfulBilledCost(tx, orgID)
		if err != nil {
			return nil, err
		}
		alreadyDebited, err := llmCreditsDebited(tx, orgID)
		if err != nil {
			return nil, err
		}

		target := billing.CostUSDToCredits(existingCost + currentCost)
		delta := target - alreadyDebited
		if delta <= 0 {
			continue
		}

		balance, err := ledgerBalance(tx, orgID)
		if err != nil {
			return nil, err
		}
		if balance < delta {
			markRowsInsufficient(rowsByOrg[orgID], perRowResults)
			continue
		}

		allocateCurrentRowCredits(rowsByOrg[orgID], perRowResults, delta)
		entries = append(entries, model.CreditLedgerEntry{
			OrgID:   orgID,
			Amount:  -delta,
			Reason:  billing.ReasonLLMTokens,
			RefType: "generation_batch",
			RefID:   batchRefID,
		})
	}
	return entries, nil
}

func successfulRowsByOrg(rows []unbilledRow, perRowResults map[string]rowBillingResult) map[uuid.UUID][]unbilledRow {
	out := map[uuid.UUID][]unbilledRow{}
	for _, row := range rows {
		result := perRowResults[row.ID]
		if result.BillingErr != "" || result.CostUSD <= 0 {
			continue
		}
		out[row.OrgID] = append(out[row.OrgID], row)
	}
	return out
}

func currentRowsCost(rows []unbilledRow, perRowResults map[string]rowBillingResult) float64 {
	var total float64
	for _, row := range rows {
		total += perRowResults[row.ID].CostUSD
	}
	return total
}

func lockOrgForBilling(tx *gorm.DB, orgID uuid.UUID) error {
	if err := tx.Exec(`SELECT id FROM orgs WHERE id = ? FOR UPDATE`, orgID).Error; err != nil {
		return fmt.Errorf("lock org %s for billing: %w", orgID, err)
	}
	return nil
}

func successfulBilledCost(tx *gorm.DB, orgID uuid.UUID) (float64, error) {
	var cost float64
	err := tx.Raw(`
		SELECT COALESCE(SUM(cost), 0)::float8
		FROM generations
		WHERE org_id = ?
		  AND is_system = TRUE
		  AND billed_at IS NOT NULL
		  AND billing_error = ''
		  AND cost > 0
	`, orgID).Scan(&cost).Error
	return cost, err
}

func llmCreditsDebited(tx *gorm.DB, orgID uuid.UUID) (int64, error) {
	var debited int64
	err := tx.Raw(`
		SELECT COALESCE(SUM(-amount), 0)
		FROM credit_ledger_entries
		WHERE org_id = ?
		  AND reason = ?
		  AND amount < 0
	`, orgID, billing.ReasonLLMTokens).Scan(&debited).Error
	return debited, err
}

func ledgerBalance(tx *gorm.DB, orgID uuid.UUID) (int64, error) {
	var balance int64
	err := tx.Raw(`SELECT COALESCE(SUM(amount), 0) FROM credit_ledger_entries WHERE org_id = ?`, orgID).Scan(&balance).Error
	return balance, err
}

func markRowsInsufficient(rows []unbilledRow, perRowResults map[string]rowBillingResult) {
	for _, row := range rows {
		result := perRowResults[row.ID]
		result.BillingErr = billingErrInsufFunds
		result.Credits = 0
		perRowResults[row.ID] = result
	}
}

func allocateCurrentRowCredits(rows []unbilledRow, perRowResults map[string]rowBillingResult, credits int64) {
	if credits <= 0 {
		return
	}
	remaining := credits
	rowIDs := make([]string, 0, len(rows))
	for _, row := range rows {
		rowIDs = append(rowIDs, row.ID)
	}
	sort.SliceStable(rowIDs, func(i, j int) bool {
		left := perRowResults[rowIDs[i]].ExactCredit - math.Floor(perRowResults[rowIDs[i]].ExactCredit)
		right := perRowResults[rowIDs[j]].ExactCredit - math.Floor(perRowResults[rowIDs[j]].ExactCredit)
		return left > right
	})
	for _, rowID := range rowIDs {
		if remaining == 0 {
			return
		}
		result := perRowResults[rowID]
		whole := int64(math.Floor(result.ExactCredit))
		if whole > remaining {
			whole = remaining
		}
		result.Credits = whole
		perRowResults[rowID] = result
		remaining -= whole
	}
	for _, rowID := range rowIDs {
		if remaining == 0 {
			return
		}
		result := perRowResults[rowID]
		result.Credits++
		perRowResults[rowID] = result
		remaining--
	}
}
