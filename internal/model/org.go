package model

import (
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/rag"
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
		&OrgInvite{},
		&RefreshToken{},
		&Credential{},
		&Token{},
		&AuditEntry{},
		&Usage{},
		&APIKey{},
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
		&HindsightBank{},
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
		&Router{},
		&RouterTrigger{},
		&RoutingRule{},
		&RoutingDecision{},
		&RouterConversation{},
		&Skill{},
		&SkillVersion{},
		&AgentSkill{},
		&AgentSubagent{},
		&ConversationSubscription{},
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

	db.Exec("CREATE INDEX IF NOT EXISTS idx_in_integrations_meta ON in_integrations USING GIN (meta jsonb_path_ops)")
	db.Exec("CREATE INDEX IF NOT EXISTS idx_in_connections_meta ON in_connections USING GIN (meta jsonb_path_ops)")

	// GIN index for generation tags array filtering
	db.Exec("CREATE INDEX IF NOT EXISTS idx_gen_tags ON generations USING GIN (tags)")

	// Drop old FK constraint on router_triggers that referenced the connections table.
	// RouterTrigger.ConnectionID now references in_connections.
	db.Exec(`ALTER TABLE router_triggers DROP CONSTRAINT IF EXISTS fk_router_triggers_connection`)

	// Drop stale unique constraint on in_connections(user_id, in_integration_id).
	// Multiple connections per user+integration are now allowed (different nango connections).
	db.Exec(`DROP INDEX IF EXISTS idx_in_conn_user_integ`)

	// Partial unique: a git-sourced skill can only have one version per commit SHA.
	db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_skill_versions_skill_sha ON skill_versions (skill_id, commit_sha) WHERE commit_sha IS NOT NULL`)
	// GIN index for skill tag filtering in the marketplace.
	db.Exec(`CREATE INDEX IF NOT EXISTS idx_skills_tags ON skills USING GIN (tags)`)

	// Partial index: fast lookup of a conversation's active subscriptions by resource key.
	db.Exec(`CREATE INDEX IF NOT EXISTS idx_conv_sub_by_key ON conversation_subscriptions (org_id, resource_key) WHERE status = 'active'`)
	// Partial unique: re-subscribing to the same resource in the same conversation is a no-op.
	db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_conv_sub_idempotent ON conversation_subscriptions (conversation_id, resource_key) WHERE status = 'active'`)

	// Partial unique: prevent duplicate pending invites per (org, email).
	db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_org_invite_pending ON org_invites (org_id, email) WHERE accepted_at IS NULL AND revoked_at IS NULL`)

	// RAG schema migrations. Phase 0: empty. Phase 1 tranches populate
	// internal/rag/register.go:AutoMigrate.
	if err := rag.AutoMigrate(db); err != nil {
		return err
	}

	return nil
}
