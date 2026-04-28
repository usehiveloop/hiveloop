package model

import (
	"time"

	"github.com/google/uuid"
)

// Subscription represents an org's subscription to a Plan, stored in a
// provider-agnostic way. (Provider, ExternalSubscriptionID) uniquely
// identifies the subscription in the upstream system.
type Subscription struct {
	ID                     uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	OrgID                  uuid.UUID `gorm:"type:uuid;not null;index"`
	Org                    Org       `gorm:"foreignKey:OrgID;constraint:OnDelete:CASCADE"`
	PlanID                 uuid.UUID `gorm:"type:uuid;not null;index"`
	Plan                   Plan      `gorm:"foreignKey:PlanID"`
	Provider               string    `gorm:"not null;size:32"`
	ExternalSubscriptionID string    `gorm:"not null;size:128"`
	ExternalCustomerID     string    `gorm:"not null;size:128;index"`
	Status                 string    `gorm:"not null;size:32;default:'active'"`
	CurrentPeriodStart     time.Time
	CurrentPeriodEnd       time.Time
	CanceledAt             *time.Time

	// Payment-method snapshot — lifted off the most recent charge.success
	// webhook so the billing UI can show "Visa ending in 4242 last charged
	// Apr 28" without round-tripping the provider for every render.
	LastChargeReference string     `gorm:"not null;default:'';size:128"`
	LastChargeAmount    int64      `gorm:"not null;default:0"` // minor units (kobo for NGN, cents for USD)
	LastChargedAt       *time.Time
	CardLast4           string `gorm:"not null;default:'';size:4"`
	CardBrand           string `gorm:"not null;default:'';size:32"`
	CardExpMonth        string `gorm:"not null;default:'';size:2"`
	CardExpYear         string `gorm:"not null;default:'';size:4"`
	AuthorizationCode   string `gorm:"not null;default:'';size:128"` // reusable token for off-session re-charges

	CreatedAt time.Time
	UpdatedAt time.Time
}

// TableName returns the subscriptions table name.
func (Subscription) TableName() string { return "subscriptions" }
