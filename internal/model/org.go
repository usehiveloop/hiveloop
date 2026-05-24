package model

import (
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
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
