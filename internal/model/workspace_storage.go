package model

import (
	"time"

	"github.com/google/uuid"
)

type WorkspaceStorage struct {
	ID                uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	OrgID             uuid.UUID `gorm:"type:uuid;not null;uniqueIndex"`
	Org               Org       `gorm:"foreignKey:OrgID;constraint:OnDelete:CASCADE"`
	TursoDatabaseName string    `gorm:"not null"`
	StorageURL        string    `gorm:"not null"`        // Turso database URL
	StorageAuthToken  string    `gorm:"not null"`        // encrypted at rest via KMS
	WrappedDEK        []byte    `gorm:"type:bytea"`      // KMS-wrapped DEK for StorageAuthToken encryption
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

func (WorkspaceStorage) TableName() string { return "workspace_storages" }
