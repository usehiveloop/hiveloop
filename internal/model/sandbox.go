package model

import (
	"time"

	"github.com/google/uuid"
)

type Sandbox struct {
	ID                uuid.UUID        `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	OrgID             *uuid.UUID       `gorm:"type:uuid;index"`                          // nil for pool sandboxes
	Org               *Org             `gorm:"foreignKey:OrgID;constraint:OnDelete:CASCADE"`
	SandboxType       string           `gorm:"not null;index"` // "shared" or "dedicated"
	AgentID           *uuid.UUID       `gorm:"type:uuid;index"`
	Agent             *Agent           `gorm:"foreignKey:AgentID;constraint:OnDelete:SET NULL"`
	SandboxTemplateID *uuid.UUID       `gorm:"type:uuid"`
	SandboxTemplate   *SandboxTemplate `gorm:"foreignKey:SandboxTemplateID;constraint:OnDelete:SET NULL"`
	ExternalID        string           `gorm:"not null"`             // Daytona workspace ID
	BridgeURL         string           `gorm:"not null"`             // pre-authenticated URL to reach Bridge
	BridgeURLExpiresAt *time.Time                                    // when BridgeURL expires (nil = never)
	EncryptedBridgeAPIKey []byte       `gorm:"type:bytea;not null"`  // AES-256-GCM encrypted Bridge API key
	Status            string           `gorm:"not null;default:'creating'"` // creating, running, stopped, starting, archived, archiving, error
	ErrorMessage      *string
	LastActiveAt      *time.Time
	StoppedAt         *time.Time       // when the sandbox was last stopped (used for 24h auto-archive)

	// Resource usage (populated by resource checker cron)
	MemoryLimitBytes  int64      `gorm:"not null;default:0"`
	MemoryUsedBytes   int64      `gorm:"not null;default:0"`
	MemoryPeakBytes   int64      `gorm:"not null;default:0"`
	CPUQuota          string     `gorm:"not null;default:''"` // e.g. "100000 100000"
	CPUUsageUsec      int64      `gorm:"not null;default:0"`
	CPUThrottledCount int64      `gorm:"not null;default:0"`
	PIDCount          int64      `gorm:"column:pid_count;not null;default:0"`
	ResourceCheckedAt *time.Time // last time resource usage was collected

	CreatedAt         time.Time
	UpdatedAt         time.Time
}

func (Sandbox) TableName() string { return "sandboxes" }
