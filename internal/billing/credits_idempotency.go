package billing

import (
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

func ledgerEntryExists(tx *gorm.DB, orgID uuid.UUID, reason, refType, refID string) (bool, error) {
	if refID == "" {
		return false, nil
	}
	var count int64
	if err := tx.Model(&model.CreditLedgerEntry{}).
		Where("org_id = ? AND reason = ? AND ref_type = ? AND ref_id = ?", orgID, reason, refType, refID).
		Limit(1).
		Count(&count).Error; err != nil {
		return false, fmt.Errorf("check credit ledger idempotency: %w", err)
	}
	return count > 0, nil
}
