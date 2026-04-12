package model

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

type MarketplaceAgent struct {
	ID        uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	OrgID     uuid.UUID  `gorm:"type:uuid;not null;index"`
	Org       Org        `gorm:"foreignKey:OrgID;constraint:OnDelete:CASCADE"`
	PublisherID uuid.UUID `gorm:"type:uuid;not null;index"`
	Publisher User       `gorm:"foreignKey:PublisherID;constraint:OnDelete:CASCADE"`
	SourceAgentID *uuid.UUID `gorm:"type:uuid;index"`
	SourceAgent   *Agent     `gorm:"foreignKey:SourceAgentID;constraint:OnDelete:SET NULL"`

	// Copied from Agent model
	Name         string  `gorm:"not null"`
	Description  *string `gorm:"type:text"`
	SystemPrompt string  `gorm:"type:text;not null"`
	Instructions *string `gorm:"type:text"`
	Model        string  `gorm:"not null"`
	SandboxType  string  `gorm:"not null"`
	Tools        JSON    `gorm:"type:jsonb;not null;default:'{}'"`
	McpServers   JSON    `gorm:"type:jsonb;not null;default:'{}'"`
	Skills       JSON    `gorm:"type:jsonb;not null;default:'{}'"`
	Integrations JSON    `gorm:"type:jsonb;not null;default:'{}'"`
	AgentConfig  JSON    `gorm:"type:jsonb;not null;default:'{}'"`
	Permissions  JSON    `gorm:"type:jsonb;not null;default:'{}'"`
	Team         string  `gorm:"not null;default:''"`
	SharedMemory bool    `gorm:"not null;default:false"`

	// Marketplace-specific
	Avatar               *string        `gorm:"type:text"`
	Slug                 string         `gorm:"not null;uniqueIndex"`
	Status               string         `gorm:"not null;default:'draft';index"` // draft, published, archived
	RequiredIntegrations pq.StringArray `gorm:"type:text[];default:'{}'"`
	Tags                 pq.StringArray `gorm:"type:text[];default:'{}'"`
	Featured             bool           `gorm:"not null;default:false;index"`
	Popular              bool           `gorm:"not null;default:false;index"`
	VerifiedAt           *time.Time
	Flagged              bool `gorm:"not null;default:false"`
	InstallCount         int  `gorm:"not null;default:0"`

	PublishedAt *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (MarketplaceAgent) TableName() string { return "marketplace_agents" }

var slugRe = regexp.MustCompile(`[^a-z0-9]+`)

// GenerateSlug creates a URL-friendly slug from a name with a short UUID suffix.
func GenerateSlug(name string) string {
	slug := strings.ToLower(strings.TrimSpace(name))
	slug = slugRe.ReplaceAllString(slug, "-")
	slug = strings.Trim(slug, "-")
	if slug == "" {
		slug = "agent"
	}
	return fmt.Sprintf("%s-%s", slug, uuid.New().String()[:8])
}
