package model

import (
	"time"

	"github.com/google/uuid"
)

// ToolUsage records a single tool API call (e.g., Spider crawl, search, screenshot)
// with full observability for per-org and per-agent billing and overage tracking.
type ToolUsage struct {
	ID       string    `gorm:"primaryKey" json:"id"`                                        // "tu_" + ULID
	OrgID    uuid.UUID `gorm:"type:uuid;not null;index:idx_tu_org_created" json:"org_id"`
	AgentID  string    `gorm:"not null;index:idx_tu_org_agent" json:"agent_id"`
	TokenJTI string    `gorm:"column:token_jti;not null" json:"token_jti"`

	// Tool metadata
	ToolName string `gorm:"not null" json:"tool_name"` // "crawl", "search", "links", "screenshot", "transform"
	Input    string `gorm:"type:text" json:"input"`    // URL or search query

	// Response metadata
	PagesReturned int    `gorm:"default:0" json:"pages_returned"`
	Status        string `gorm:"not null" json:"status"`                       // "success" or "error"
	ErrorMessage  string `gorm:"type:text" json:"error_message,omitempty"`

	// Timing
	TotalMs int `gorm:"column:total_ms" json:"total_ms"`

	// Billing
	CreditsUsed int `gorm:"default:0" json:"credits_used"`

	IPAddress *string   `gorm:"type:inet" json:"ip_address,omitempty"`
	CreatedAt time.Time `gorm:"not null;index:idx_tu_org_created" json:"created_at"`
}

func (ToolUsage) TableName() string { return "tool_usages" }
