package model

import (
	"time"

	"github.com/google/uuid"
)

// CreditLedgerEntry is an append-only ledger row. Grants are positive, spends
// are negative. An org's balance is SUM(amount) WHERE org_id = ?.
//
// This table is never updated or deleted — corrections are additional rows
// with reason="adjustment".
type CreditLedgerEntry struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	OrgID     uuid.UUID `gorm:"type:uuid;not null;index"`
	Amount    int64     `gorm:"not null"`
	Reason    string    `gorm:"not null;size:64"`
	RefType   string    `gorm:"size:64"`
	RefID     string    `gorm:"size:64;index"`
	CreatedAt time.Time
}

// TableName returns the ledger table name.
func (CreditLedgerEntry) TableName() string { return "credit_ledger_entries" }
