package model

import (
	"time"

	"github.com/google/uuid"
)

// ConversationSubscription binds a conversation to a specific external resource
// (a GitHub PR, a Linear issue, a Slack thread, etc.) so future webhook events
// for that resource can be routed into the conversation.
//
// Subscriptions are written by the agent (via the subscribe_to_events MCP tool)
// or by trigger dispatch (when a CREATE trigger fires and seeds the
// conversation's origin resource).
//
// Routing uses ResourceKey, which is the canonical form built from the
// subscribable-resource catalog's CanonicalTemplate. ResourceID preserves the
// user-friendly form the agent typed so we can render it back in tooling and
// UI without re-parsing.
type ConversationSubscription struct {
	ID             uuid.UUID         `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	ConversationID uuid.UUID         `gorm:"type:uuid;not null;index:idx_conv_sub_by_conv"`
	Conversation   AgentConversation `gorm:"foreignKey:ConversationID;constraint:OnDelete:CASCADE"`
	OrgID          uuid.UUID         `gorm:"type:uuid;not null"`
	AgentID        uuid.UUID         `gorm:"type:uuid;not null"`
	Provider       string            `gorm:"not null"` // "github-app", "linear", etc.
	ResourceType   string            `gorm:"not null"` // "github_pull_request", etc.
	ResourceID     string            `gorm:"not null"` // user-provided, e.g. "ziraloop/ziraloop#99"
	ResourceKey    string            `gorm:"not null"` // canonical, e.g. "github/ziraloop/ziraloop/pull/99"
	Source         string            `gorm:"not null;default:'agent'"` // "agent" | "trigger" | (future slots)
	Status         string            `gorm:"not null;default:'active'"` // "active" | "closed"
	CreatedAt      time.Time
	ClosedAt       *time.Time
}

func (ConversationSubscription) TableName() string { return "conversation_subscriptions" }

// Subscription source constants.
const (
	SubscriptionSourceAgent   = "agent"
	SubscriptionSourceTrigger = "trigger"

	SubscriptionStatusActive = "active"
	SubscriptionStatusClosed = "closed"
)
