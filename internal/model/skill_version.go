package model

import (
	"time"

	"github.com/google/uuid"
)

// SkillVersion is an immutable snapshot of a Skill's content. Each row holds a
// Bundle that the runtime ships directly to bridge as a SkillDefinition.
//
// Bundle shape:
//
//	{
//	  "id":                "<uuid>",
//	  "title":             "...",
//	  "description":       "...",
//	  "content":           "...",          // body of SKILL.md
//	  "parameters_schema": {...},          // optional
//	  "manifest":          {...},          // parsed SKILL.md frontmatter
//	  "references": [
//	    { "path": "scripts/foo.py", "body": "..." },
//	    { "path": "reference/api.md", "body": "..." }
//	  ]
//	}
type SkillVersion struct {
	ID      uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	SkillID uuid.UUID `gorm:"type:uuid;not null;index"`
	Skill   Skill     `gorm:"foreignKey:SkillID;constraint:OnDelete:CASCADE"`

	// Version is a human-readable label: "v1", "2026-04-10", or short commit SHA.
	Version string `gorm:"not null"`

	// CommitSHA is set for git-sourced versions; nil for inline versions.
	// A partial unique index enforces uniqueness of (skill_id, commit_sha) when non-null.
	CommitSHA *string `gorm:"type:text"`

	Bundle RawJSON `gorm:"type:jsonb;not null"`

	HydratedAt     *time.Time
	HydrationError *string `gorm:"type:text"`

	CreatedAt time.Time
}

func (SkillVersion) TableName() string { return "skill_versions" }
