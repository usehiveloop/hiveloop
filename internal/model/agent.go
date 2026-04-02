package model

import (
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

type Agent struct {
	ID                uuid.UUID        `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	OrgID             uuid.UUID        `gorm:"type:uuid;not null;uniqueIndex:idx_agent_org_name"`
	Org               Org              `gorm:"foreignKey:OrgID;constraint:OnDelete:CASCADE"`
	IdentityID        uuid.UUID        `gorm:"type:uuid;not null;index"`
	Identity          Identity         `gorm:"foreignKey:IdentityID;constraint:OnDelete:CASCADE"`
	Name              string           `gorm:"not null;uniqueIndex:idx_agent_org_name"`
	Description       *string          `gorm:"type:text"`
	CredentialID      uuid.UUID        `gorm:"type:uuid;not null;index"`
	Credential        Credential       `gorm:"foreignKey:CredentialID;constraint:OnDelete:SET NULL"`
	SandboxType       string           `gorm:"not null"` // "dedicated" or "shared"
	SandboxTemplateID *uuid.UUID       `gorm:"type:uuid"`
	SandboxTemplate   *SandboxTemplate `gorm:"foreignKey:SandboxTemplateID;constraint:OnDelete:SET NULL"`

	// Bridge AgentDefinition fields
	SystemPrompt string `gorm:"type:text;not null"`
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

	Status    string `gorm:"not null;default:'active'"` // active, archived
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (Agent) TableName() string { return "agents" }
