package model

import (
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

// Generation records a single LLM proxy request with full observability data:
// token usage, cost, latency, attribution, and error tracking.
type Generation struct {
	ID             string         `gorm:"primaryKey" json:"id"`
	OrgID          uuid.UUID      `gorm:"type:uuid;not null;index:idx_gen_org_created" json:"org_id"`
	CredentialID   uuid.UUID      `gorm:"type:uuid;not null;index:idx_gen_org_credential" json:"credential_id"`
	IdentityID     *uuid.UUID     `gorm:"type:uuid" json:"identity_id,omitempty"`
	TokenJTI       string         `gorm:"column:token_jti;not null" json:"token_jti"`

	// Request metadata
	ProviderID  string `gorm:"not null;index:idx_gen_org_provider" json:"provider_id"`
	Model       string `gorm:"index:idx_gen_org_model" json:"model"`
	RequestPath string `json:"request_path"`
	IsStreaming bool   `gorm:"default:false" json:"is_streaming"`

	// Token usage
	InputTokens     int `gorm:"default:0" json:"input_tokens"`
	OutputTokens    int `gorm:"default:0" json:"output_tokens"`
	CachedTokens    int `gorm:"default:0" json:"cached_tokens"`
	ReasoningTokens int `gorm:"default:0" json:"reasoning_tokens"`

	// Cost in USD
	Cost float64 `gorm:"type:numeric(12,8);default:0" json:"cost"`

	// Timing
	TTFBMs         *int `gorm:"column:ttfb_ms" json:"ttfb_ms,omitempty"`
	TotalMs        int  `gorm:"column:total_ms" json:"total_ms"`
	UpstreamStatus int  `gorm:"column:upstream_status" json:"upstream_status"`

	// Attribution (from token.meta)
	UserID string         `gorm:"index:idx_gen_org_user" json:"user_id,omitempty"`
	Tags   pq.StringArray `gorm:"type:text[]" json:"tags,omitempty"`

	// Error tracking
	ErrorType    string `json:"error_type,omitempty"`
	ErrorMessage string `gorm:"type:text" json:"error_message,omitempty"`

	IPAddress *string   `gorm:"type:inet" json:"ip_address,omitempty"`
	CreatedAt time.Time `gorm:"not null;index:idx_gen_org_created" json:"created_at"`
}

func (Generation) TableName() string { return "generations" }
