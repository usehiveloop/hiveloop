package model

import (
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"gorm.io/gorm"
)

type Org struct {
	ID             uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	Name           string         `gorm:"not null;uniqueIndex"`
	RateLimit      int            `gorm:"not null;default:1000"`
	Active         bool           `gorm:"not null;default:true"`
	AllowedOrigins pq.StringArray `gorm:"type:text[]"`

	// Billing (Polar)
	PolarCustomerID *string `gorm:"index"`
	BillingPlan     string  `gorm:"not null;default:'free'"` // "free", "pro"

	CreatedAt time.Time
	UpdatedAt time.Time
}

func (Org) TableName() string { return "orgs" }

func AutoMigrate(db *gorm.DB) error {
	if err := db.AutoMigrate(
		&Org{},
		&User{},
		&OrgMembership{},
		&RefreshToken{},
		&Identity{},
		&IdentityRateLimit{},
		&Credential{},
		&Token{},
		&AuditEntry{},
		&Usage{},
		&ConnectSession{},
		&APIKey{},
		&Integration{},
		&Connection{},
		&Generation{},
		&EmailVerification{},
		&PasswordReset{},
		&SandboxTemplate{},
		&Agent{},
		&Sandbox{},
		&WorkspaceStorage{},
		&AgentConversation{},
		&ConversationEvent{},
		&CustomDomain{},
		&OrgWebhookConfig{},
		&HindsightBank{},
		&ForgeRun{},
		&ForgeIteration{},
		&ForgeEvalCase{},
		&ForgeEvalResult{},
		&ForgeEvent{},
		&InIntegration{},
		&InConnection{},
		&OAuthAccount{},
		&OAuthExchangeToken{},
		&AdminAuditEntry{},
		&OTPCode{},
		&MarketplaceAgent{},
		&ToolUsage{},
		&Subscription{},
		&DriveAsset{},
		&AgentTrigger{},
		&Skill{},
		&SkillVersion{},
		&AgentSkill{},
		&AgentSubagent{},
	); err != nil {
		return err
	}

	// Partial unique: org-scoped agents have unique (org_id, name).
	db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_agent_org_name ON agents (org_id, name) WHERE org_id IS NOT NULL`)
	// Partial unique: system agents have globally unique names.
	db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_agent_system_name ON agents (name) WHERE org_id IS NULL`)

	// GIN indexes for JSONB metadata filtering
	db.Exec("CREATE INDEX IF NOT EXISTS idx_credentials_meta ON credentials USING GIN (meta jsonb_path_ops)")
	db.Exec("CREATE INDEX IF NOT EXISTS idx_tokens_meta ON tokens USING GIN (meta jsonb_path_ops)")
	db.Exec("CREATE INDEX IF NOT EXISTS idx_identities_meta ON identities USING GIN (meta jsonb_path_ops)")
	db.Exec("CREATE INDEX IF NOT EXISTS idx_integrations_meta ON integrations USING GIN (meta jsonb_path_ops)")

	db.Exec("CREATE INDEX IF NOT EXISTS idx_in_integrations_meta ON in_integrations USING GIN (meta jsonb_path_ops)")
	db.Exec("CREATE INDEX IF NOT EXISTS idx_in_connections_meta ON in_connections USING GIN (meta jsonb_path_ops)")

	// GIN index for generation tags array filtering
	db.Exec("CREATE INDEX IF NOT EXISTS idx_gen_tags ON generations USING GIN (tags)")

	// Partial index for efficient webhook → trigger lookup (only enabled triggers).
	db.Exec(`CREATE INDEX IF NOT EXISTS idx_agent_triggers_lookup ON agent_triggers (connection_id) WHERE enabled = true`)
	// Drop old single-key unique index if it exists.
	db.Exec(`DROP INDEX IF EXISTS idx_agent_triggers_unique`)
	// Unique constraint: one trigger config per agent+connection pair.
	db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_agent_triggers_agent_conn ON agent_triggers (agent_id, connection_id)`)

	// Partial unique: a git-sourced skill can only have one version per commit SHA.
	db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_skill_versions_skill_sha ON skill_versions (skill_id, commit_sha) WHERE commit_sha IS NOT NULL`)
	// GIN index for skill tag filtering in the marketplace.
	db.Exec(`CREATE INDEX IF NOT EXISTS idx_skills_tags ON skills USING GIN (tags)`)

	return nil
}
