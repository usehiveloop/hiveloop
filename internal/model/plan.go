package model

import (
	"time"

	"github.com/google/uuid"
)

// Plan is a billing plan. Plans are seeded and managed administratively — the
// catalog is small and change-controlled. A subscription references a plan
// by PlanID; the plan.Slug is the stable identifier used in UIs and webhook
// metadata.
type Plan struct {
	ID             uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	Slug           string    `gorm:"not null;uniqueIndex;size:64"`
	Name           string    `gorm:"not null;size:128"`
	Provider       string    `gorm:"not null;default:'';size:32;index"`  // empty = provider-agnostic; otherwise the billing provider this row is for (e.g. "paystack")
	ProviderPlanID string    `gorm:"not null;default:'';size:128;index"` // upstream id this plan resolves to in Provider's system (e.g. Paystack "PLN_xxx"); empty for free / unsynced plans
	Features       RawJSON   `gorm:"type:jsonb"`                         // optional; typically an array of bullet strings, but the column accepts any JSON shape
	MonthlyCredits int64     `gorm:"not null;default:0"`
	WelcomeCredits int64     `gorm:"not null;default:0"` // one-time, granted to a user's default org on signup; non-expiring
	PriceCents     int64     `gorm:"not null;default:0"`
	Currency       string    `gorm:"not null;default:'USD';size:8"`
	Active         bool      `gorm:"not null;default:true"`
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// TableName returns the plans table name.
func (Plan) TableName() string { return "plans" }
