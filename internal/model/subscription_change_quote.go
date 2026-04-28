package model

import (
	"time"

	"github.com/google/uuid"
)

// SubscriptionChangeQuote freezes the prorated amount + credit grant at
// preview-change time so apply-change can verify the customer paid what
// we quoted (not what they think the price should be).
type SubscriptionChangeQuote struct {
	ID             uuid.UUID    `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	OrgID          uuid.UUID    `gorm:"type:uuid;not null;index"`
	SubscriptionID uuid.UUID    `gorm:"type:uuid;not null;index"`
	Subscription   Subscription `gorm:"foreignKey:SubscriptionID;constraint:OnDelete:CASCADE"`

	FromPlanID uuid.UUID `gorm:"type:uuid;not null"`
	ToPlanID   uuid.UUID `gorm:"type:uuid;not null"`

	// "upgrade" or "downgrade".
	Kind string `gorm:"not null;size:16"`

	// AmountMinor: prorated charge for upgrade, 0 for downgrade.
	AmountMinor int64  `gorm:"not null"`
	Currency    string `gorm:"not null;size:8"`

	// ProrationCreditMinor is granted on apply-change for upgrades only.
	ProrationCreditMinor int64 `gorm:"not null;default:0"`

	// EffectiveAt: now() for upgrades, period_end for downgrades.
	EffectiveAt time.Time `gorm:"not null"`

	PaystackReference *string `gorm:"size:128;uniqueIndex"`

	ExpiresAt  time.Time `gorm:"not null;index"`
	ConsumedAt *time.Time

	CreatedAt time.Time
}

func (SubscriptionChangeQuote) TableName() string { return "subscription_change_quotes" }
