package model

import (
	"time"

	"github.com/google/uuid"
)

// Subscription represents an org's subscription to a Plan. We manage the
// recurring lifecycle ourselves (period start/end, cancel-at-period-end,
// pending plan changes) and use the provider purely to charge a saved
// payment method via Authorization. ExternalSubscriptionID is nullable
// because most provider-managed subscription rows are unused now — we
// only keep it to preserve historical data and to give us an escape
// hatch if we ever opt back into a provider-side subscription.
type Subscription struct {
	ID       uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	OrgID    uuid.UUID `gorm:"type:uuid;not null;index"`
	Org      Org       `gorm:"foreignKey:OrgID;constraint:OnDelete:CASCADE"`
	PlanID   uuid.UUID `gorm:"type:uuid;not null;index"`
	Plan     Plan      `gorm:"foreignKey:PlanID"`
	Provider string    `gorm:"not null;size:32"`

	ExternalSubscriptionID *string `gorm:"size:128"`
	ExternalCustomerID     string  `gorm:"not null;size:128;index"`

	// Status is one of: active, past_due, canceled, incomplete.
	Status string `gorm:"not null;size:32;default:'active'"`

	CurrentPeriodStart time.Time
	CurrentPeriodEnd   time.Time
	CanceledAt         *time.Time

	// CancelAtPeriodEnd is the Stripe-style soft cancel flag. When true the
	// renewal worker stops charging after CurrentPeriodEnd and the row
	// transitions to status='canceled'. Resume flips it back to false.
	CancelAtPeriodEnd bool `gorm:"not null;default:false"`

	// PendingPlanID and PendingChangeAt schedule a downgrade to apply at
	// CurrentPeriodEnd. Set by /subscription/apply-change for downgrades;
	// cleared by the renewal worker when it advances the period.
	PendingPlanID   *uuid.UUID `gorm:"type:uuid"`
	PendingPlan     *Plan      `gorm:"foreignKey:PendingPlanID"`
	PendingChangeAt *time.Time

	// Renewal worker bookkeeping. RenewalAttempts increments after every
	// failed charge attempt; the renewal worker stops trying when it
	// reaches MaxRenewalAttempts (5) and transitions the row to past_due.
	// LastRenewalAttemptAt rate-limits the sweep — the next attempt is
	// only enqueued after RenewalRetryInterval (1h) has passed.
	RenewalAttempts      int    `gorm:"not null;default:0"`
	LastRenewalAttemptAt *time.Time
	LastRenewalError     string `gorm:"not null;default:'';size:512"`

	// Saved payment method, lifted off the most recent successful charge.
	// PaymentChannel is "card" or "bank" — these are the only channels we
	// support for subscription renewals because they're the only Paystack
	// channels that issue a reusable AuthorizationCode.
	PaymentChannel        string `gorm:"not null;default:'';size:16"`
	PaymentBankName       string `gorm:"not null;default:'';size:64"`
	PaymentAccountName    string `gorm:"not null;default:'';size:128"`
	LastChargeReference   string `gorm:"not null;default:'';size:128"`
	LastChargeAmount      int64  `gorm:"not null;default:0"`
	LastChargedAt         *time.Time
	CardLast4             string `gorm:"not null;default:'';size:4"`
	CardBrand             string `gorm:"not null;default:'';size:32"`
	CardExpMonth          string `gorm:"not null;default:'';size:2"`
	CardExpYear           string `gorm:"not null;default:'';size:4"`
	AuthorizationCode     string `gorm:"not null;default:'';size:128"`

	CreatedAt time.Time
	UpdatedAt time.Time
}

// TableName returns the subscriptions table name.
func (Subscription) TableName() string { return "subscriptions" }
