package model

import (
	"time"

	"github.com/google/uuid"
)

type Connection struct {
	ID                uuid.UUID    `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	OrgID             uuid.UUID    `gorm:"type:uuid;not null;index"`
	Org               Org          `gorm:"foreignKey:OrgID;constraint:OnDelete:CASCADE"`
	IntegrationID     uuid.UUID    `gorm:"type:uuid;not null;index"`
	Integration       Integration  `gorm:"foreignKey:IntegrationID;constraint:OnDelete:CASCADE"`
	NangoConnectionID string       `gorm:"not null"`
	IdentityID        *uuid.UUID   `gorm:"type:uuid;index"`
	Identity          *Identity    `gorm:"foreignKey:IdentityID;constraint:OnDelete:SET NULL"`
	Meta              JSON         `gorm:"type:jsonb;default:'{}'"`
	RevokedAt         *time.Time
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

func (Connection) TableName() string { return "connections" }
