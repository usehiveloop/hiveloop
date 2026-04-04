package model

import (
	"time"

	"github.com/google/uuid"
)

type InConnection struct {
	ID                uuid.UUID     `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	UserID            uuid.UUID     `gorm:"type:uuid;not null;index;uniqueIndex:idx_in_conn_user_integ"`
	User              User          `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE"`
	InIntegrationID   uuid.UUID     `gorm:"type:uuid;not null;index;uniqueIndex:idx_in_conn_user_integ"`
	InIntegration     InIntegration `gorm:"foreignKey:InIntegrationID;constraint:OnDelete:CASCADE"`
	NangoConnectionID string        `gorm:"not null"`
	Meta              JSON          `gorm:"type:jsonb;default:'{}'"`
	RevokedAt         *time.Time
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

func (InConnection) TableName() string { return "in_connections" }
