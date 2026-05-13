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

	// Denormalised slug of the org's active plan ("free" when no active sub).
	// Source of truth lives in the subscriptions table; this is cached on
	// the org row so request-path checks don't need a join.
	PlanSlug string `gorm:"not null;default:'free';size:64"`

	// BYOK reports whether the org runs agents on its own LLM credentials.
	// When false, agents fall back to platform-owned system credentials.
	BYOK bool `gorm:"not null;default:false"`

	// LogoURL is a CDN-served URL to the org's square logo. Stored as the
	// asset_url returned from POST /v1/uploads/sign with asset_type=org_logo.
	// Empty string when no logo is set.
	LogoURL string `gorm:"not null;default:''"`

	Website     string `gorm:"not null;default:'';size:500"`
	Description string `gorm:"type:text;not null;default:''"`

	PromptCompany string `gorm:"type:text;not null;default:''"`

	Onboarded bool `gorm:"not null;default:false"`

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
		&ChatSession{},
		&ChatMessage{},
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
		&ConversationAsset{},
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
		&Plan{},
		&Subscription{},
		&SubscriptionChangeQuote{},
		&CreditLedgerEntry{},
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
		&AgentProfile{},
		&ConversationSubscription{},
		&FailedEvent{},
		&Team{},
		&EmployeeAsset{},
		&CloudAgentTask{},
		&EmployeeMemoryEvent{},
	); err != nil {
		return err
	}

	db.Exec(`ALTER TABLE agent_conversations DROP COLUMN IF EXISTS parent_conversation_id`)
	db.Exec(`ALTER TABLE agents DROP COLUMN IF EXISTS agent_type`)
	db.Exec(`ALTER TABLE agents DROP COLUMN IF EXISTS deleted_at`)
	installAgentHardDeleteConstraints(db)

	// Partial unique: org-scoped agents have unique (org_id, name).
	db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_agent_org_name ON agents (org_id, name) WHERE org_id IS NOT NULL`)
	// Partial unique: a team's name is unique within an org, but soft-deleted
	// rows are excluded so the name can be reused after deletion.
	db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_team_org_name ON teams (org_id, name) WHERE deleted_at IS NULL`)
	// Partial unique: system agents have globally unique names.
	db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_agent_system_name ON agents (name) WHERE org_id IS NULL`)

	// GIN indexes for JSONB metadata filtering
	db.Exec("CREATE INDEX IF NOT EXISTS idx_credentials_meta ON credentials USING GIN (meta jsonb_path_ops)")
	db.Exec("CREATE INDEX IF NOT EXISTS idx_tokens_meta ON tokens USING GIN (meta jsonb_path_ops)")

	db.Exec("CREATE INDEX IF NOT EXISTS idx_in_integrations_meta ON in_integrations USING GIN (meta jsonb_path_ops)")
	db.Exec("CREATE INDEX IF NOT EXISTS idx_in_connections_meta ON in_connections USING GIN (meta jsonb_path_ops)")

	// GIN index for generation tags array filtering
	db.Exec("CREATE INDEX IF NOT EXISTS idx_gen_tags ON generations USING GIN (tags)")

	db.Exec(`CREATE INDEX IF NOT EXISTS idx_generations_unbilled ON generations (created_at)
		WHERE billed_at IS NULL AND is_system = TRUE AND (input_tokens > 0 OR output_tokens > 0)`)

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

	// Composite index for cloud agent task lookups by employee + cloud agent.
	db.Exec(`CREATE INDEX IF NOT EXISTS idx_cloud_task_employee_agent ON cloud_agent_tasks (employee_agent_id, cloud_agent_id, created_at DESC)`)

	// Partial unique: one live profile per (agent, provider, external account).
	db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_agent_profile_account ON agent_profiles (agent_id, provider, external_id) WHERE deleted_at IS NULL AND revoked_at IS NULL`)

	// Credit ledger idempotency: when an async spend task retries after a
	// transient failure, the retry must not double-deduct. The unique index
	// on (org_id, reason, ref_type, ref_id) means the second INSERT fails
	// with a unique-violation; the task handler treats that as success.
	// The partial WHERE skips rows that intentionally have no ref_id
	// (e.g. manual adjustments) — those aren't expected to be idempotent.
	db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_credit_ledger_idempotent ON credit_ledger_entries (org_id, reason, ref_type, ref_id) WHERE ref_id != ''`)

	// RAG schema migrations live in internal/rag.AutoMigrate. Callers
	// (bootstrap/deps.go, testhelpers.ConnectTestDB) invoke it
	// immediately after model.AutoMigrate — we can't invoke it from
	// here because internal/rag/model imports internal/model (for
	// model.JSON), so an import from internal/model back into
	// internal/rag would close a cycle.

	return nil
}

func installAgentHardDeleteConstraints(db *gorm.DB) {
	db.Exec(`DELETE FROM chat_messages WHERE session_id NOT IN (SELECT id FROM chat_sessions)`)
	db.Exec(`DELETE FROM chat_sessions WHERE agent_id NOT IN (SELECT id FROM agents)`)
	db.Exec(`UPDATE sandboxes SET agent_id = NULL WHERE agent_id IS NOT NULL AND agent_id NOT IN (SELECT id FROM agents)`)
	db.Exec(`DELETE FROM router_conversations WHERE agent_id NOT IN (SELECT id FROM agents)`)
	db.Exec(`DELETE FROM employee_assets WHERE agent_id NOT IN (SELECT id FROM agents)`)
	db.Exec(`DELETE FROM hindsight_banks WHERE agent_id IS NOT NULL AND agent_id NOT IN (SELECT id FROM agents)`)
	db.Exec(`DELETE FROM conversation_subscriptions WHERE agent_id NOT IN (SELECT id FROM agents)`)

	recreateFK(db, "sandboxes", "fk_sandboxes_agent", "agent_id", "agents", "id", "CASCADE")
	recreateFK(db, "hindsight_banks", "fk_hindsight_banks_agent", "agent_id", "agents", "id", "CASCADE")
	recreateFK(db, "chat_sessions", "fk_chat_sessions_agent", "agent_id", "agents", "id", "CASCADE")
	recreateFK(db, "chat_sessions", "fk_chat_sessions_org", "org_id", "orgs", "id", "CASCADE")
	recreateFK(db, "chat_sessions", "fk_chat_sessions_user", "user_id", "users", "id", "CASCADE")
	recreateFK(db, "chat_messages", "fk_chat_messages_session", "session_id", "chat_sessions", "id", "CASCADE")
	recreateFK(db, "router_conversations", "fk_router_conversations_agent", "agent_id", "agents", "id", "CASCADE")
	recreateFK(db, "employee_assets", "fk_employee_assets_agent", "agent_id", "agents", "id", "CASCADE")
	recreateFK(db, "employee_assets", "fk_employee_assets_org", "org_id", "orgs", "id", "CASCADE")
	recreateFK(db, "employee_assets", "fk_employee_assets_sandbox", "sandbox_id", "sandboxes", "id", "CASCADE")
	recreateFK(db, "conversation_subscriptions", "fk_conversation_subscriptions_agent", "agent_id", "agents", "id", "CASCADE")
	recreateFK(db, "conversation_subscriptions", "fk_conversation_subscriptions_org", "org_id", "orgs", "id", "CASCADE")
}

func recreateFK(db *gorm.DB, table, constraint, column, refTable, refColumn, onDelete string) {
	db.Exec(`
DO $$
DECLARE constraint_name text;
BEGIN
	FOR constraint_name IN
		SELECT tc.constraint_name
		FROM information_schema.table_constraints tc
		JOIN information_schema.key_column_usage kcu
			ON tc.constraint_name = kcu.constraint_name
			AND tc.table_schema = kcu.table_schema
		WHERE tc.constraint_type = 'FOREIGN KEY'
			AND tc.table_schema = current_schema()
			AND tc.table_name = '` + table + `'
			AND kcu.column_name = '` + column + `'
	LOOP
		EXECUTE format('ALTER TABLE %I DROP CONSTRAINT %I', '` + table + `', constraint_name);
	END LOOP;
END $$;`)
	db.Exec(`ALTER TABLE ` + table + ` ADD CONSTRAINT ` + constraint +
		` FOREIGN KEY (` + column + `) REFERENCES ` + refTable + `(` + refColumn + `) ON DELETE ` + onDelete)
}
