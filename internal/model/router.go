package model

import (
	"time"

	"github.com/google/uuid"
)

// Router is the per-org Zira routing identity. One router per org.
// All inbound webhook events route through the router, which triages
// (via LLM or deterministic rules) and dispatches to specialist agents.
type Router struct {
	ID             uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	OrgID          uuid.UUID  `gorm:"type:uuid;not null;uniqueIndex"`
	Org            Org        `gorm:"foreignKey:OrgID;constraint:OnDelete:CASCADE"`
	Name           string     `gorm:"not null;default:'Zira'"`
	Persona        string     `gorm:"type:text;not null;default:''"` // shared voice injected into every specialist's instructions
	DefaultAgentID *uuid.UUID `gorm:"type:uuid"`
	DefaultAgent   *Agent     `gorm:"foreignKey:DefaultAgentID;constraint:OnDelete:SET NULL"`
	MemoryTeam     string     `gorm:"not null;default:''"` // Hindsight namespace all specialists share
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func (Router) TableName() string { return "routers" }
