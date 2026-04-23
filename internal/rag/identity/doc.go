// Package identity resolves external source identities (e.g. a GitHub
// user login) to Hiveloop User records via OAuthAccount lookups.
//
// Onyx equivalent: backend/onyx/db/users.py helpers like
// fetch_user_by_email that map source principals to Onyx users. We adapt
// by extending the existing OAuthAccount table (ProviderUserEmail,
// ProviderUserLogin, VerifiedEmails, LastSyncedAt columns) rather than
// introducing a new mapping table.
package identity
