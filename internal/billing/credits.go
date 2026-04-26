package billing

import (
	"context"
	"errors"
	"fmt"
	"time"

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
	ReasonPlanGrant    = "plan_grant"
	ReasonTopup        = "topup"
	ReasonAdjustment   = "adjustment"
	ReasonRefund       = "refund"
	ReasonAgentRun     = "agent_run"
	ReasonLLMTokens    = "llm_tokens"
	ReasonExpiry       = "expiry"
	ReasonWelcomeGrant = "welcome_grant"
)

// RefType for expiry offset entries: ref_id is the expired grant entry's ID.
const RefTypeExpiredGrant = "expired_grant"

// RefType for welcome grants: ref_id is the new user's ID. Combined with the
// idx_credit_ledger_idempotent unique index, this guarantees one welcome
// grant per user even if signup is retried.
const RefTypeSignup = "signup"

// FreePlanSlug is the slug of the default plan every new org starts on.
// Welcome credits are sourced from this plan's WelcomeCredits column.
const FreePlanSlug = "free"

// PlanGrantGracePeriod is added to a subscription's CurrentPeriodEnd when
// computing the ExpiresAt for plan-grant credits. The grace window prevents
// a cycle-boundary spend from being refused while the next invoice.paid
// webhook is still in flight.
const PlanGrantGracePeriod = 3 * 24 * time.Hour

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
//
// expiresAt is the wall-clock time after which any remaining portion of this
// grant is forfeited by the periodic sweep (see SweepExpiredGrants). Pass nil
// for permanent credits (top-ups, manual adjustments, refunds).
func (s *CreditsService) Grant(orgID uuid.UUID, amount int64, reason, refType, refID string, expiresAt *time.Time) error {
	return GrantWithTx(s.db, orgID, amount, reason, refType, refID, expiresAt)
}

// GrantWithTx writes a grant entry on the supplied DB handle, which is
// expected to be a *gorm.DB tied to an open transaction. Use this when the
// grant must be atomic with surrounding work (e.g. signup creating the user
// + org + welcome grant in one transaction).
func GrantWithTx(tx *gorm.DB, orgID uuid.UUID, amount int64, reason, refType, refID string, expiresAt *time.Time) error {
	if amount <= 0 {
		return fmt.Errorf("grant amount must be positive (got %d)", amount)
	}
	return tx.Create(&model.CreditLedgerEntry{
		OrgID:     orgID,
		Amount:    amount,
		Reason:    reason,
		RefType:   refType,
		RefID:     refID,
		ExpiresAt: expiresAt,
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

// SweepAllExpiredGrants finds every org with at least one expired grant that
// has not yet been offset by an expiry entry, and runs SweepOrgExpiredGrants
// on each. Designed to be called from a periodic Asynq task; safe to re-run.
//
// Per-org failures are logged via the returned error chain but do not stop
// processing of other orgs — a single bad row shouldn't block the cycle.
func (s *CreditsService) SweepAllExpiredGrants(ctx context.Context) error {
	now := time.Now()

	var orgIDs []uuid.UUID
	if err := s.db.WithContext(ctx).
		Raw(`
			SELECT DISTINCT le.org_id
			FROM credit_ledger_entries AS le
			WHERE le.amount > 0
			  AND le.expires_at IS NOT NULL
			  AND le.expires_at < ?
			  AND NOT EXISTS (
			    SELECT 1 FROM credit_ledger_entries AS off
			    WHERE off.org_id = le.org_id
			      AND off.reason = ?
			      AND off.ref_type = ?
			      AND off.ref_id = le.id::text
			  )
		`, now, ReasonExpiry, RefTypeExpiredGrant).
		Scan(&orgIDs).Error; err != nil {
		return fmt.Errorf("find candidate orgs: %w", err)
	}

	var firstErr error
	for _, orgID := range orgIDs {
		if err := s.SweepOrgExpiredGrants(ctx, orgID, now); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			// Continue: a single org's failure (e.g. transient lock contention)
			// shouldn't block the cycle for the rest.
		}
	}
	return firstErr
}

// SweepOrgExpiredGrants runs FIFO attribution over the org's full ledger and,
// for any grant whose ExpiresAt is now in the past with a positive remaining
// portion, writes a single negative "expiry" ledger entry that offsets the
// remaining amount.
//
// Locks the org row FOR UPDATE so concurrent Spend calls serialise behind it
// — the replay's view of the ledger is consistent for the duration of the
// transaction. Idempotent via the ledger's unique index on
// (org_id, reason, ref_type, ref_id) WHERE ref_id != ''.
func (s *CreditsService) SweepOrgExpiredGrants(ctx context.Context, orgID uuid.UUID, now time.Time) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var org model.Org
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ?", orgID).First(&org).Error; err != nil {
			return fmt.Errorf("lock org %s: %w", orgID, err)
		}

		var entries []model.CreditLedgerEntry
		if err := tx.Where("org_id = ?", orgID).
			Order("created_at ASC, id ASC").
			Find(&entries).Error; err != nil {
			return fmt.Errorf("load ledger for org %s: %w", orgID, err)
		}

		buckets := replayBuckets(entries)

		for _, b := range buckets {
			if b.expiresAt == nil || !b.expiresAt.Before(now) || b.remaining <= 0 {
				continue
			}
			err := tx.Create(&model.CreditLedgerEntry{
				OrgID:   orgID,
				Amount:  -b.remaining,
				Reason:  ReasonExpiry,
				RefType: RefTypeExpiredGrant,
				RefID:   b.entryID.String(),
			}).Error
			if err != nil && !isUniqueViolation(err) {
				return fmt.Errorf("write expiry entry for grant %s: %w", b.entryID, err)
			}
			// Unique-violation means a concurrent sweep already wrote this
			// offset — treat as success.
		}
		return nil
	})
}

