package model

import (
	"github.com/google/uuid"
)

// RAGPublicExternalUserGroup stores external "groups" that represent
// anyone-with-the-link / anyone-in-the-domain style public shares. At
// query time the ACL allow-list for a user is extended with every
// (public_external_user_group_id) their connections have recorded,
// letting those public shares show up in search results.
//
// Verbatim port of Onyx `PublicExternalUserGroup` at
// backend/onyx/db/models.py:4352-4380. The only adaptation is
// `cc_pair_id → in_connection_id`, matching the Hiveloop connection
// model. The `stale` flag + indexes are direct ports; see the
// stale-sweep doc on `RAGUserExternalUserGroup` for the security-
// critical sync pattern.
type RAGPublicExternalUserGroup struct {
	ExternalUserGroupID string    `gorm:"type:text;primaryKey"`
	InConnectionID      uuid.UUID `gorm:"type:uuid;primaryKey"`

	// Stale flag for the sync pattern. Port of
	// backend/onyx/db/models.py:4368.
	Stale bool `gorm:"not null;default:false"`
}

func (RAGPublicExternalUserGroup) TableName() string { return "rag_public_external_user_groups" }
