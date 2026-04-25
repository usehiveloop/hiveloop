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
	CreatedAt              time.Time
	UpdatedAt              time.Time
}

// TableName returns the subscriptions table name.
func (Subscription) TableName() string { return "subscriptions" }
