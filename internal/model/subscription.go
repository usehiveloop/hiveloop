package model

import (
	"time"

	"github.com/google/uuid"
)

type Subscription struct {
	ID                  uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	OrgID               uuid.UUID  `gorm:"type:uuid;not null;index"`
	Org                 Org        `gorm:"foreignKey:OrgID;constraint:OnDelete:CASCADE"`
	PolarSubscriptionID string     `gorm:"not null;uniqueIndex"`
	PolarProductID      string     `gorm:"not null"`
	ProductType         string     `gorm:"not null"` // "free", "pro_shared", "pro_dedicated"
	Status              string     `gorm:"not null;default:'active'"` // active, canceled, past_due, revoked
	CurrentPeriodStart  time.Time
	CurrentPeriodEnd    time.Time
	CanceledAt          *time.Time
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

func (Subscription) TableName() string { return "subscriptions" }
