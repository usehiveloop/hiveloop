package model

import (
	"time"

	"github.com/google/uuid"
)

// HindsightBank tracks which identities/agents have had their Hindsight memory bank
// created and configured. Used for lazy bank provisioning and config change detection.
// Banks are either identity-scoped (shared across agents) or agent-scoped (private).
type HindsightBank struct {
	ID         uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	IdentityID *uuid.UUID `gorm:"type:uuid;index"`                                // set for identity-scoped banks
	Identity   *Identity  `gorm:"foreignKey:IdentityID;constraint:OnDelete:CASCADE"`
	AgentID    *uuid.UUID `gorm:"type:uuid;index"`                                // set for agent-scoped banks
	BankID     string     `gorm:"not null;uniqueIndex"` // "identity-{uuid}" or "agent-{uuid}"
	ConfigHash string     `gorm:"not null;default:''"` // SHA256 of applied MemoryConfig
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

func (HindsightBank) TableName() string { return "hindsight_banks" }
