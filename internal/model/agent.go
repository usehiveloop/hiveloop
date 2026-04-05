package model

import (
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

type Agent struct {
	ID                uuid.UUID        `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	OrgID             *uuid.UUID       `gorm:"type:uuid;index:idx_agent_org_id"` // nil for system agents
	Org               *Org             `gorm:"foreignKey:OrgID;constraint:OnDelete:CASCADE"`
	IdentityID        *uuid.UUID       `gorm:"type:uuid;index"`
	Identity          *Identity        `gorm:"foreignKey:IdentityID;constraint:OnDelete:SET NULL"`
	Name              string           `gorm:"not null"`
	Description       *string          `gorm:"type:text"`
	CredentialID      *uuid.UUID       `gorm:"type:uuid;index"` // nil for system agents
	Credential        *Credential      `gorm:"foreignKey:CredentialID;constraint:OnDelete:SET NULL"`
	SandboxType       string           `gorm:"not null"` // "dedicated" or "shared"
	SandboxID         *uuid.UUID       `gorm:"type:uuid;index"` // set for shared agents (points to pool sandbox)
	SandboxTemplateID *uuid.UUID       `gorm:"type:uuid"`
	SandboxTemplate   *SandboxTemplate `gorm:"foreignKey:SandboxTemplateID;constraint:OnDelete:SET NULL"`

	// Bridge AgentDefinition fields
	SystemPrompt string  `gorm:"type:text;not null"`
	Instructions *string `gorm:"type:text"` // optional markdown instructions for auto-starting runs
	Model        string `gorm:"not null"` // must match credential's provider
	Tools        JSON   `gorm:"type:jsonb;not null;default:'{}'"`
	McpServers   JSON   `gorm:"type:jsonb;not null;default:'{}'"`
	Skills       JSON   `gorm:"type:jsonb;not null;default:'{}'"`
	Integrations JSON   `gorm:"type:jsonb;not null;default:'{}'"` // selected integration IDs/configs
	Subagents    JSON   `gorm:"type:jsonb;not null;default:'{}'"`
	AgentConfig  JSON   `gorm:"type:jsonb;not null;default:'{}'"` // max_tokens, max_turns, temperature, etc.
	Permissions  JSON   `gorm:"type:jsonb;not null;default:'{}'"` // tool permission overrides
	Team         string `gorm:"not null;default:''"` // team tag for memory scoping (e.g. "engineering", "sales")
	SharedMemory bool   `gorm:"not null;default:false"` // can store shared memories visible to all agents in identity

	// Sandbox setup (dedicated agents only — ignored for shared agents)
	SetupCommands    pq.StringArray `gorm:"type:text[];default:'{}'"`  // shell commands run on dedicated sandbox creation
	EncryptedEnvVars []byte         `gorm:"type:bytea"`                // AES-256-GCM encrypted JSON map of env vars

	Status        string `gorm:"not null;default:'active'"` // active, archived
	IsSystem      bool   `gorm:"not null;default:false;index"`
	ProviderGroup string `gorm:"not null;default:''"` // e.g. "anthropic", "openai", "gemini" — set for system agents
	DeletedAt     *time.Time `gorm:"index"`
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

func (Agent) TableName() string { return "agents" }
