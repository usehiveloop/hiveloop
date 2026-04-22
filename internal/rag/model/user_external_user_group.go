package model

import (
	"github.com/google/uuid"
)

// RAGUserExternalUserGroup maps a Hiveloop User (internal or
// external-identified) to the external groups they belong to, scoped by
// the InConnection that discovered the membership.
//
// Verbatim port of Onyx `User__ExternalUserGroupId` at
// backend/onyx/db/models.py:4320-4350. The only adaptation is
// `cc_pair_id → in_connection_id` (Hiveloop collapses Onyx's Connector
// + Credential + ConnectorCredentialPair tuple into a single
// `InConnection` row). The `stale` column and its two indexes are
// direct ports.
//
// Stale-sweep semantics (security-critical; see plan Tranche 1D):
//  1. Sync start: UPDATE ... SET stale = true WHERE in_connection_id = X
//  2. Sync body: upsert fresh rows with stale = false
//  3. Sync end:   DELETE WHERE in_connection_id = X AND stale = true
//
// If the sweep is wrong (e.g. stale rows survive), users retain
// permissions for groups they were removed from upstream. The
// `TestRAGUserExternalUserGroup_StaleSweepPattern` test pins this.
type RAGUserExternalUserGroup struct {
	UserID              uuid.UUID `gorm:"type:uuid;primaryKey"`
	ExternalUserGroupID string    `gorm:"type:text;primaryKey"`
	InConnectionID      uuid.UUID `gorm:"type:uuid;primaryKey"`

	// Stale flag for the sync pattern above. Port of
	// backend/onyx/db/models.py:4337.
	Stale bool `gorm:"not null;default:false"`
}

func (RAGUserExternalUserGroup) TableName() string { return "rag_user_external_user_groups" }
