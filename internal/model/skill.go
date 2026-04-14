package model

import (
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

// Skill is a reusable prompt + references bundle that agents can invoke.
// A skill with OrgID=nil is public; otherwise it is visible only to that org.
type Skill struct {
	ID          uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	OrgID       *uuid.UUID `gorm:"type:uuid;index"`
	Org         *Org       `gorm:"foreignKey:OrgID;constraint:OnDelete:CASCADE"`
	PublisherID *uuid.UUID `gorm:"type:uuid;index"`
	Publisher   *User      `gorm:"foreignKey:PublisherID;constraint:OnDelete:SET NULL"`

	Slug        string  `gorm:"not null;uniqueIndex"`
	Name        string  `gorm:"not null"`
	Description *string `gorm:"type:text"`

	// SourceType is "inline" (content authored in the UI) or "git" (hydrated from a repo).
	SourceType  string  `gorm:"not null"`
	RepoURL     *string `gorm:"type:text"`
	RepoSubpath *string `gorm:"type:text"`
	RepoRef     string  `gorm:"not null;default:'main'"`

	// LatestVersionID points at the newest SkillVersion for this skill. It is
	// intentionally not a foreign-key association — SkillVersion has a FK back
	// to Skill, and declaring both sides creates a cyclical migration that
	// Postgres rejects at AutoMigrate time.
	LatestVersionID *uuid.UUID `gorm:"type:uuid"`

	Tags         pq.StringArray `gorm:"type:text[];default:'{}'"`
	InstallCount int            `gorm:"not null;default:0"`
	Featured     bool           `gorm:"not null;default:false;index"`
	VerifiedAt   *time.Time

	// Status is draft, published, or archived.
	Status string `gorm:"not null;default:'draft';index"`

	// PublicSkillID references the cloned public skill when this org skill has
	// been published to the marketplace.
	PublicSkillID *uuid.UUID `gorm:"type:uuid;index"`

	// OriginSkillID references the original org-scoped skill that was cloned
	// to create this public skill. Only set on public (OrgID=nil) skills.
	OriginSkillID *uuid.UUID `gorm:"type:uuid;index"`
	// OriginOrgID is the org that published this public skill.
	OriginOrgID *uuid.UUID `gorm:"type:uuid"`
	// PublishedAt is when this skill was made public.
	PublishedAt *time.Time

	CreatedAt time.Time
	UpdatedAt time.Time
}

func (Skill) TableName() string { return "skills" }

const (
	SkillSourceInline = "inline"
	SkillSourceGit    = "git"

	SkillStatusDraft     = "draft"
	SkillStatusPublished = "published"
	SkillStatusArchived  = "archived"
)
