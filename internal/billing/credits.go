package billing

import (
	"errors"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/usehiveloop/hiveloop/internal/model"
)

// ErrInsufficientCredits is returned when Spend would drive the balance below
// zero. Handlers should translate this into HTTP 402 Payment Required.
var ErrInsufficientCredits = errors.New("billing: insufficient credits")

// ErrAlreadyRecorded is returned when Spend hits the unique index on
// (org_id, reason, ref_type, ref_id) — meaning this exact deduction has
// already been applied. Callers should treat this as success, not an error;
// it means the caller retried after the first attempt already committed.
var ErrAlreadyRecorded = errors.New("billing: spend already recorded (idempotent replay)")

// Grant reasons (stored in credit_ledger_entries.reason).
const (
	ReasonPlanGrant  = "plan_grant"
	ReasonTopup      = "topup"
	ReasonAdjustment = "adjustment"
	ReasonRefund     = "refund"
	ReasonAgentRun   = "agent_run"
	ReasonLLMTokens  = "llm_tokens"
)

// CreditsService is the append-only credit ledger. Grants are positive, spends
// are negative, balance = SUM(amount).
type CreditsService struct {
	db *gorm.DB
}

// NewCreditsService creates a credit ledger service bound to db.
func NewCreditsService(db *gorm.DB) *CreditsService {
	return &CreditsService{db: db}
}

// Balance returns the org's current credit balance.
func (s *CreditsService) Balance(orgID uuid.UUID) (int64, error) {
	return sumBalance(s.db, orgID)
}

// Grant adds credits to the org. amount must be positive.
func (s *CreditsService) Grant(orgID uuid.UUID, amount int64, reason, refType, refID string) error {
	if amount <= 0 {
		return fmt.Errorf("grant amount must be positive (got %d)", amount)
	}
	return s.db.Create(&model.CreditLedgerEntry{
		OrgID:   orgID,
		Amount:  amount,
		Reason:  reason,
		RefType: refType,
		RefID:   refID,
	}).Error
}

// Spend deducts credits. amount must be positive.
//
// Returns ErrInsufficientCredits if the org's balance would drop below zero,
// and ErrAlreadyRecorded when a deduction with the same
// (org_id, reason, ref_type, ref_id) has already been written — async task
// retries hit this and should treat it as success.
//
// Spend serialises concurrent spends for a given org by taking a row-level
// lock on the org record inside a transaction. This trades throughput per-org
// for correctness: we never oversubscribe the balance.
func (s *CreditsService) Spend(orgID uuid.UUID, amount int64, reason, refType, refID string) error {
	if amount <= 0 {
		return fmt.Errorf("spend amount must be positive (got %d)", amount)
	}

	err := s.db.Transaction(func(tx *gorm.DB) error {
		var org model.Org
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ?", orgID).First(&org).Error; err != nil {
			return fmt.Errorf("lock org: %w", err)
		}

		current, err := sumBalance(tx, orgID)
		if err != nil {
			return err
		}
		if current < amount {
			return ErrInsufficientCredits
		}

		return tx.Create(&model.CreditLedgerEntry{
			OrgID:   orgID,
			Amount:  -amount,
			Reason:  reason,
			RefType: refType,
			RefID:   refID,
		}).Error
	})
	if isUniqueViolation(err) {
		return ErrAlreadyRecorded
	}
	return err
}

// isUniqueViolation reports whether err is a Postgres unique_violation
// (SQLSTATE 23505). Used to detect idempotency-key collisions without
// requiring callers to import pgconn.
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	// The pgx driver wraps errors with a *pgconn.PgError whose Code is 23505
	// for unique violations. Match on the error message so we don't add a
	// direct dependency on pgconn here.
	msg := err.Error()
	return containsAny(msg, "SQLSTATE 23505", "duplicate key value violates unique constraint")
}

func containsAny(haystack string, needles ...string) bool {
	for _, n := range needles {
		if len(haystack) >= len(n) {
			// Manual substring scan — cheap, avoids importing strings just for this.
			for i := 0; i+len(n) <= len(haystack); i++ {
				if haystack[i:i+len(n)] == n {
					return true
				}
			}
		}
	}
	return false
}

func sumBalance(db *gorm.DB, orgID uuid.UUID) (int64, error) {
	var row struct{ Total *int64 }
	if err := db.Model(&model.CreditLedgerEntry{}).
		Select("COALESCE(SUM(amount), 0) AS total").
		Where("org_id = ?", orgID).
		Scan(&row).Error; err != nil {
		return 0, fmt.Errorf("compute balance: %w", err)
	}
	if row.Total == nil {
		return 0, nil
	}
	return *row.Total, nil
}
