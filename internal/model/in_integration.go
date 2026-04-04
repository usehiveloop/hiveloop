package model

import (
	"time"

	"github.com/google/uuid"
)

type InIntegration struct {
	ID          uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	UniqueKey   string     `gorm:"not null;uniqueIndex"`
	Provider    string     `gorm:"not null;uniqueIndex:idx_in_integ_provider"`
	DisplayName string     `gorm:"not null"`
	Meta        JSON       `gorm:"type:jsonb;default:'{}'"`
	NangoConfig JSON       `gorm:"type:jsonb;default:'{}'" json:"nango_config"`
	DeletedAt   *time.Time `gorm:"index"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (InIntegration) TableName() string { return "in_integrations" }
