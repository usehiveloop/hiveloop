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

const autoMigrateAdvisoryLockKey int64 = 548713429

func AutoMigrate(db *gorm.DB) (err error) {
	if err := db.Exec("SELECT pg_advisory_lock(?)", autoMigrateAdvisoryLockKey).Error; err != nil {
		return err
	}
	defer func() {
		unlockErr := db.Exec("SELECT pg_advisory_unlock(?)", autoMigrateAdvisoryLockKey).Error
		if err == nil && unlockErr != nil {
			err = unlockErr
		}
	}()

	if err := db.Migrator().DropTable(
		"router_conversations",
		"routing_decisions",
		"routing_rules",
		"router_triggers",
		"routers",
		"conversation_subscriptions",
		"agent_skills",
		"agent_triggers",
		"agent_trigger_deliveries",
		"agent_conversations",
		"cloud_agent_tasks",
		"agent_profiles",
		"teams",
		"agent_subagents",
		"marketplace_agents",
		"chat_messages",
		"chat_sessions",
		"admin_audit_log",
	); err != nil {
		return err
	}
	if err := dropLegacyAgentStorage(db); err != nil {
		return err
	}
	if err := dropEmployeeLegacyIndexes(db); err != nil {
		return err
	}
	if err := dropEmployeeLegacyColumns(db); err != nil {
		return err
	}
	if db.Migrator().HasColumn(&AgentConversation{}, "bridge_conversation_id") &&
		!db.Migrator().HasColumn(&AgentConversation{}, "runtime_conversation_id") {
		if err := db.Migrator().RenameColumn(&AgentConversation{}, "bridge_conversation_id", "runtime_conversation_id"); err != nil {
			return err
		}
	}
	if db.Migrator().HasColumn(&ConversationEvent{}, "bridge_conversation_id") &&
		!db.Migrator().HasColumn(&ConversationEvent{}, "runtime_conversation_id") {
		if err := db.Migrator().RenameColumn(&ConversationEvent{}, "bridge_conversation_id", "runtime_conversation_id"); err != nil {
			return err
		}
	}
	if db.Migrator().HasIndex(&InIntegration{}, "idx_in_integ_provider") {
		if err := db.Migrator().DropIndex(&InIntegration{}, "idx_in_integ_provider"); err != nil {
			return err
		}
	}

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
		&Employee{},
		&Sandbox{},
		&AgentConversation{},
		&ConversationEvent{},
		&ConversationAsset{},
		&CustomDomain{},
		&HindsightBank{},
		&InIntegration{},
		&InConnection{},
		&AgentTrigger{},
		&AgentTriggerDelivery{},
		&OAuthAccount{},
		&OAuthExchangeToken{},
		&OTPCode{},
		&ToolUsage{},
		&Plan{},
		&Subscription{},
		&SubscriptionChangeQuote{},
		&CreditLedgerEntry{},
		&DriveAsset{},
		&Skill{},
		&AgentSkill{},
		&FailedEvent{},
		&EmployeeAsset{},
		&CloudAgentTask{},
		&EmployeeMemoryEvent{},
		&EmployeeSchedule{},
		&EmployeeScheduleRun{},
		&EmployeeSandboxUpgrade{},
	); err != nil {
		return err
	}

	return nil
}

func dropLegacyAgentStorage(db *gorm.DB) error {
	if !db.Migrator().HasTable("agents") {
		return nil
	}
	if db.Dialector.Name() == "postgres" {
		return db.Exec("DROP TABLE IF EXISTS agents CASCADE").Error
	}
	return db.Migrator().DropTable("agents")
}

func dropEmployeeLegacyColumns(db *gorm.DB) error {
	for _, column := range []string{
		"name",
		"team_id",
		"team",
		"category",
		"avatar_url",
		"description",
		"system_prompt",
		"identity_prompt",
		"prompt_operating_principles",
		"integrations",
		"is_employee",
		"is_system",
		"provider_group",
	} {
		if !db.Migrator().HasColumn("employees", column) {
			continue
		}
		if err := db.Migrator().DropColumn("employees", column); err != nil {
			return err
		}
	}
	return nil
}

func dropEmployeeLegacyIndexes(db *gorm.DB) error {
	for _, index := range []string{"idx_agent_org_name", "idx_employee_org_name"} {
		if !db.Migrator().HasIndex("employees", index) {
			continue
		}
		if err := db.Migrator().DropIndex("employees", index); err != nil {
			return err
		}
	}
	return nil
}
