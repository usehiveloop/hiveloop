package model

import (
	"time"

	"github.com/google/uuid"
)

// AdminAuditEntry records every mutating operation performed via the admin API.
// Payloads are sanitized — sensitive fields (emails, passwords, tokens, secrets)
// are masked before storage.
type AdminAuditEntry struct {
	ID         int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	AdminID    uuid.UUID `gorm:"type:uuid;not null;index:idx_admin_audit_admin" json:"admin_id"`
	AdminEmail string    `gorm:"not null" json:"admin_email"`
	Method     string    `gorm:"not null" json:"method"`
	Path       string    `gorm:"not null" json:"path"`
	Resource   string    `gorm:"not null;index:idx_admin_audit_resource" json:"resource"`
	ResourceID string    `json:"resource_id,omitempty"`
	Action     string    `gorm:"not null;index:idx_admin_audit_action" json:"action"`
	StatusCode int       `gorm:"not null" json:"status_code"`
	Payload    JSON      `gorm:"type:jsonb;default:'{}'" json:"payload"`
	IPAddress  *string   `gorm:"type:inet" json:"ip_address,omitempty"`
	LatencyMs  int64     `json:"latency_ms"`
	CreatedAt  time.Time `gorm:"index:idx_admin_audit_created" json:"created_at"`
}

func (AdminAuditEntry) TableName() string { return "admin_audit_log" }
