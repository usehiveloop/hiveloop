package model

import (
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

// RAGExternalUserGroup stores metadata for an external group (e.g. a
// GitHub team, a Google Drive shared-drive membership) that has been
// discovered inside a specific RAGSource.
//
// HIVELOOP ADDITION: Onyx has no direct analog — Onyx derives external
// group display data on-demand inside the perm-sync code path. We
// persist it because (a) the admin UI wants to render display names
// without making source-API calls, and (b) the stale-sweep pattern used
// by `RAGUserExternalUserGroup` and `RAGPublicExternalUserGroup` needs a
// parent row to sweep. See the plan's Tranche 1D section for
// justification.
//
// ExternalUserGroupID is the source-prefixed, lowercased group
// identifier produced by `acl.BuildExtGroupName`. It is ALWAYS stored in
// that normalized form so joins and ACL lookups produce identical byte
// sequences with zero ambiguity.
type RAGExternalUserGroup struct {
	ID                  uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	OrgID               uuid.UUID      `gorm:"type:uuid;not null;index"`
	// RAGSourceID — Phase 3A swap (was InConnectionID). The junction
	// table now keys off the top-level RAGSource.
	RAGSourceID         uuid.UUID      `gorm:"type:uuid;not null;uniqueIndex:uq_rag_external_user_group_source_ext,priority:1"`
	ExternalUserGroupID string         `gorm:"type:text;not null;uniqueIndex:uq_rag_external_user_group_source_ext,priority:2"`
	DisplayName         string         `gorm:"type:text;not null"`
	GivesAnyoneAccess   bool           `gorm:"not null;default:false"`
	MemberEmails        pq.StringArray `gorm:"type:text[]"`
	UpdatedAt           time.Time
}

func (RAGExternalUserGroup) TableName() string { return "rag_external_user_groups" }
