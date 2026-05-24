package model

import (
	"time"

	"github.com/google/uuid"
)

type Connection struct {
	ID                uuid.UUID   `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	OrgID             uuid.UUID   `gorm:"type:uuid;index"`
	Org               Org         `gorm:"foreignKey:OrgID;constraint:OnDelete:CASCADE"`
	UserID            uuid.UUID   `gorm:"type:uuid;not null;index"`
	User              User        `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE"`
	IntegrationID     uuid.UUID   `gorm:"type:uuid;not null;index"`
	Integration       Integration `gorm:"foreignKey:IntegrationID;constraint:OnDelete:CASCADE"`
	NangoConnectionID string      `gorm:"not null"`
	Meta              JSON        `gorm:"type:jsonb;default:'{}'"`
	WebhookConfigured *bool       `gorm:"not null;default:true"`
	RevokedAt         *time.Time
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

func (Connection) TableName() string { return "connections" }
