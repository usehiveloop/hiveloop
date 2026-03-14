package model

import (
	"time"

	"github.com/google/uuid"
)

type Integration struct {
	ID          uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	OrgID       uuid.UUID  `gorm:"type:uuid;not null;uniqueIndex:idx_integration_org_uk;index"`
	Org         Org        `gorm:"foreignKey:OrgID;constraint:OnDelete:CASCADE"`
	UniqueKey   string     `gorm:"not null;uniqueIndex:idx_integration_org_uk"`
	Provider    string     `gorm:"not null"`
	DisplayName string     `gorm:"not null"`
	Meta        JSON       `gorm:"type:jsonb;default:'{}'"`
	DeletedAt   *time.Time `gorm:"index"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (Integration) TableName() string { return "integrations" }
