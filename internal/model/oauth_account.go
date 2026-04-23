package model

import (
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

// OAuthAccount links a user to an external OAuth provider (e.g.
// GitHub, Google).
//
// The ProviderUserEmail / ProviderUserLogin / VerifiedEmails /
// LastSyncedAt columns cache source-canonical identity metadata so
// RAG permission sync can map a Hiveloop user to source-native ACL
// entries (e.g. GitHub work email vs personal) without re-hitting
// the provider on every search. Onyx resolves these on-demand inside
// perm-sync code (backend/onyx/db/models.py:299-303); we cache
// because our sync tasks need fast lookup.
type OAuthAccount struct {
	ID             uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	UserID         uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_oauth_user_provider"`
	Provider       string    `gorm:"not null;uniqueIndex:idx_oauth_provider_uid;uniqueIndex:idx_oauth_user_provider"`
	ProviderUserID string    `gorm:"not null;uniqueIndex:idx_oauth_provider_uid"`
	User           User      `gorm:"foreignKey:UserID"`

	// ProviderUserEmail is the source-canonical email for this OAuth
	// identity (e.g. a GitHub user's "work" vs "personal" email).
	// Nullable because some providers don't expose an email.
	ProviderUserEmail *string

	// ProviderUserLogin is the source-native login / username / handle
	// (e.g. GitHub "@octocat"). Nullable because some providers don't
	// have the concept.
	ProviderUserLogin *string

	// VerifiedEmails is every email address the upstream provider claims
	// is verified for this user. Used by RAG perm-sync to expand an ACL
	// match beyond the primary email.
	VerifiedEmails pq.StringArray `gorm:"type:text[]"`

	// LastSyncedAt records when the RAG identity sync task last refreshed
	// the three fields above from the provider.
	LastSyncedAt *time.Time

	CreatedAt time.Time
	UpdatedAt time.Time
}

func (OAuthAccount) TableName() string { return "oauth_accounts" }
