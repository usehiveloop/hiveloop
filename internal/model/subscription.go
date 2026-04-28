package model

import (
	"time"

	"github.com/google/uuid"
)

// Subscription: org's subscription to a Plan. We manage the recurring
// lifecycle ourselves and use the provider purely for charge_authorization.
// ExternalSubscriptionID is nullable since we no longer rely on
// provider-side subscriptions; kept for historical rows + escape hatch.
type Subscription struct {
	ID       uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	OrgID    uuid.UUID `gorm:"type:uuid;not null;index"`
	Org      Org       `gorm:"foreignKey:OrgID;constraint:OnDelete:CASCADE"`
	PlanID   uuid.UUID `gorm:"type:uuid;not null;index"`
	Plan     Plan      `gorm:"foreignKey:PlanID"`
	Provider string    `gorm:"not null;size:32"`

	ExternalSubscriptionID *string `gorm:"size:128"`
	ExternalCustomerID     string  `gorm:"not null;size:128;index"`

	// active, past_due, canceled, incomplete.
	Status string `gorm:"not null;size:32;default:'active'"`

	CurrentPeriodStart time.Time
	CurrentPeriodEnd   time.Time
	CanceledAt         *time.Time

	// Stripe-style soft cancel: renewal worker stops charging after period_end.
	CancelAtPeriodEnd bool `gorm:"not null;default:false"`

	// Deferred plan change applied at the next renewal; cleared by the worker.
	PendingPlanID   *uuid.UUID `gorm:"type:uuid"`
	PendingPlan     *Plan      `gorm:"foreignKey:PendingPlanID"`
	PendingChangeAt *time.Time

	// Renewal worker bookkeeping. Caps at MaxRenewalAttempts → past_due.
	RenewalAttempts      int `gorm:"not null;default:0"`
	LastRenewalAttemptAt *time.Time
	LastRenewalError     string `gorm:"not null;default:'';size:512"`

	// Saved payment method (card or bank only — others can't recharge).
	PaymentChannel      string `gorm:"not null;default:'';size:16"`
	PaymentBankName     string `gorm:"not null;default:'';size:64"`
	PaymentAccountName  string `gorm:"not null;default:'';size:128"`
	LastChargeReference string `gorm:"not null;default:'';size:128"`
	LastChargeAmount    int64  `gorm:"not null;default:0"`
	LastChargedAt       *time.Time
	CardLast4           string `gorm:"not null;default:'';size:4"`
	CardBrand           string `gorm:"not null;default:'';size:32"`
	CardExpMonth        string `gorm:"not null;default:'';size:2"`
	CardExpYear         string `gorm:"not null;default:'';size:4"`
	AuthorizationCode   string `gorm:"not null;default:'';size:128"`

	CreatedAt time.Time
	UpdatedAt time.Time
}

func (Subscription) TableName() string { return "subscriptions" }
