package model

import (
	"time"

	"github.com/google/uuid"
)

type AuditEntry struct {
	ID           int64      `gorm:"primaryKey;autoIncrement"`
	OrgID        uuid.UUID  `gorm:"type:uuid;not null;index:idx_audit_org_created"`
	CredentialID *uuid.UUID `gorm:"type:uuid;index:idx_audit_credential"`
	Action       string     `gorm:"not null"`
	Metadata     JSON       `gorm:"type:jsonb;default:'{}'"`
	IPAddress    *string    `gorm:"type:inet"`
	CreatedAt    time.Time  `gorm:"index:idx_audit_org_created"`
}

func (AuditEntry) TableName() string { return "audit_log" }

type Usage struct {
	ID            int64     `gorm:"primaryKey;autoIncrement"`
	OrgID         uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_usage_unique"`
	Org           Org       `gorm:"foreignKey:OrgID"`
	CredentialID  uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_usage_unique"`
	Credential    Credential `gorm:"foreignKey:CredentialID"`
	RequestCount  int64     `gorm:"not null;default:0"`
	PeriodStart   time.Time `gorm:"not null;uniqueIndex:idx_usage_unique"`
	PeriodEnd     time.Time `gorm:"not null"`
	CreatedAt     time.Time
}

func (Usage) TableName() string { return "usage" }
