package model

import (
	"time"

	"github.com/google/uuid"
)

// SubscriptionChangeQuote is a server-issued quote for switching a
// subscription from one plan to another. The amount and proration credit
// are computed at issue time and frozen on the row, so the apply step can
// verify the customer actually paid what we quoted — not what they think
// the price should be — without recomputing under different inputs.
//
// Lifecycle:
//   - issued by POST /v1/billing/subscription/preview-change
//   - consumed by POST /v1/billing/subscription/apply-change (sets ConsumedAt)
//   - expires after ExpiresAt, after which apply-change rejects it
type SubscriptionChangeQuote struct {
	ID             uuid.UUID    `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	OrgID          uuid.UUID    `gorm:"type:uuid;not null;index"`
	SubscriptionID uuid.UUID    `gorm:"type:uuid;not null;index"`
	Subscription   Subscription `gorm:"foreignKey:SubscriptionID;constraint:OnDelete:CASCADE"`

	FromPlanID uuid.UUID `gorm:"type:uuid;not null"`
	ToPlanID   uuid.UUID `gorm:"type:uuid;not null"`

	// Kind is "upgrade" or "downgrade". Upgrades require a Paystack reference
	// matching AmountMinor; downgrades are deferred to period end with no charge.
	Kind string `gorm:"not null;size:16"`

	// AmountMinor is the prorated charge for an upgrade (positive) or 0 for a
	// downgrade. Currency is fixed by the subscription — cross-currency
	// changes are rejected at preview time.
	AmountMinor int64  `gorm:"not null"`
	Currency    string `gorm:"not null;size:8"`

	// ProrationCreditMinor is the unused-time credit on the current plan,
	// granted to the org's credit ledger when an upgrade is applied. For
	// downgrades it's always 0 because nothing is consumed yet.
	ProrationCreditMinor int64 `gorm:"not null;default:0"`

	// EffectiveAt is when the change actually applies — now() for upgrades,
	// the subscription's CurrentPeriodEnd for downgrades.
	EffectiveAt time.Time `gorm:"not null"`

	// PaystackReference is set when apply-change consumes this quote against
	// a verified Paystack transaction. Null on downgrades and on quotes that
	// expired without being applied.
	PaystackReference *string `gorm:"size:128;uniqueIndex"`

	ExpiresAt  time.Time `gorm:"not null;index"`
	ConsumedAt *time.Time

	CreatedAt time.Time
}

// TableName returns the change-quotes table name.
func (SubscriptionChangeQuote) TableName() string { return "subscription_change_quotes" }
