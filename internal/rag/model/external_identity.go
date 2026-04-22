package model

import (
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

// RAGExternalIdentity is a Hiveloop-only addition. Onyx resolves Hiveloop-user
// → source-native-identity on-demand inside its perm-sync code; we persist
// the mapping so our sync tasks have an O(1) lookup for ACL matching.
//
// One row per (Hiveloop user, InConnection). The same GitHub user in two
// different Hiveloop orgs is two distinct rows — enforced by the org-scoped
// (provider, external_user_id, org_id) unique.
//
// References (for the caller's context, not a direct port):
//   - Onyx OAuthAccount: backend/onyx/db/models.py:299-303
//   - Onyx perm-sync identity resolution: scattered across
//     backend/onyx/external_permissions/
type RAGExternalIdentity struct {
	// ID is a bigserial surrogate key; this table is high-churn
	// (one row per user per connection) and we never look up by it
	// directly — queries are always by (user_id, in_connection_id) or
	// (provider, external_user_id, org_id).
	ID int64 `gorm:"primaryKey;autoIncrement"`

	OrgID uuid.UUID `gorm:"type:uuid;not null;index:idx_rag_external_identity_org;uniqueIndex:uq_rag_external_identity_provider_ext_id_org"`

	// UserID → users.id. CASCADE because deleting a Hiveloop user must
	// drop every cached identity they had.
	UserID uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:uq_rag_external_identity_user_conn"`

	// InConnectionID → in_connections.id. CASCADE because deleting a
	// connection invalidates any identity cached against it.
	InConnectionID uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:uq_rag_external_identity_user_conn;index:idx_rag_external_identity_conn"`

	// Provider mirrors the parent InIntegration's provider string
	// (e.g. "github", "notion"). Denormalized so lookups by
	// (provider, external_user_id, org_id) don't require a join.
	Provider string `gorm:"not null;uniqueIndex:uq_rag_external_identity_provider_ext_id_org"`

	// ExternalUserID is the source-native stable identifier
	// (GitHub numeric user id, Notion person id, etc.) — NOT a username.
	ExternalUserID string `gorm:"not null;uniqueIndex:uq_rag_external_identity_provider_ext_id_org"`

	// ExternalUserLogin is the source-native handle ("@octocat"). Nullable
	// because not every provider has one.
	ExternalUserLogin *string

	// ExternalUserEmails is every email the source associates with this
	// identity. Denormalized from the OAuthAccount.VerifiedEmails at sync
	// time so ACL matching can match on any of them without a join.
	ExternalUserEmails pq.StringArray `gorm:"type:text[]"`

	UpdatedAt time.Time
}

func (RAGExternalIdentity) TableName() string { return "rag_external_identities" }
