package interfaces

// ExternalAccess mirrors the Onyx access-model shape at
// backend/onyx/access/models.py:9-64. The three fields are opaque from
// this layer's perspective: the scheduler feeds them straight through to
// Rust-side ACL matching via IngestBatch / UpdateACL. No prefixing or
// transformation happens here.
//
// Onyx uses set[str] for the two list fields + exposes public()/empty()
// factories. In Go we use slices for JSON stability and leave
// deduplication to callers (matches the "opaque tokens" contract).
type ExternalAccess struct {
	// ExternalUserEmails is the list of external user email addresses
	// with access to the document in the source system.
	// Onyx: ExternalAccess.external_user_emails at models.py:16.
	ExternalUserEmails []string `json:"external_user_emails,omitempty"`

	// ExternalUserGroupIDs is the list of external group identifiers
	// (source-prefixed + lowercased per
	// internal/rag/acl.BuildExtGroupName) with access to the document.
	// Onyx: ExternalAccess.external_user_group_ids at models.py:18.
	ExternalUserGroupIDs []string `json:"external_user_group_ids,omitempty"`

	// IsPublic is true for source-side public docs (readable by
	// anyone in the org). Per Onyx semantics, when IsPublic is true
	// the user/group lists may be empty — that combination is
	// explicitly legal (see ExternalAccess.public() at
	// onyx/access/models.py:42-48 and TestExternalAccess_PublicDocAllowsEmptyUserScope).
	IsPublic bool `json:"is_public"`
}

// DocExternalAccess wraps a document ID alongside its current
// ExternalAccess. This is the per-document output of
// PermSyncConnector.SyncDocPermissions: one row per synced document.
//
// Onyx analog: DocExternalAccess at
// backend/onyx/access/models.py:67-104.
type DocExternalAccess struct {
	// DocID matches Document.DocID / SlimDocument.DocID one-to-one.
	DocID string `json:"doc_id"`

	// ExternalAccess is the freshly-observed ACL for this document.
	// Non-nil by construction — a doc with no permission info should
	// not be surfaced via this channel; the connector should either
	// omit it or wrap the miss in a ConnectorFailure.
	ExternalAccess *ExternalAccess `json:"external_access"`
}

// ExternalGroup is one row of the per-source group catalog produced by
// PermSyncConnector.SyncExternalGroups. The scheduler upserts these into
// RAGExternalUserGroup (Phase 1D) and junction tables.
//
// There is no direct Onyx pydantic analog — Onyx derives groups
// on-the-fly inside backend/ee/onyx/external_permissions/<provider>/group_sync.py.
// We persist them (Phase 1D rationale: admin UI + stale-sweep) so this
// shape is the contract between the connector and the scheduler's
// upsert path.
type ExternalGroup struct {
	// GroupID is the already-prefixed + lowercased group identifier
	// (produced by acl.BuildExtGroupName). Stored verbatim in
	// RAGExternalUserGroup.ExternalUserGroupID.
	GroupID string `json:"group_id"`

	// DisplayName is the human-readable label (e.g. GitHub team name).
	// Shown in the admin UI so no source-side API call is needed.
	DisplayName string `json:"display_name,omitempty"`

	// MemberEmails is the list of user email addresses belonging to
	// this group. The scheduler uses these to populate
	// RAGUserExternalUserGroup rows.
	MemberEmails []string `json:"member_emails,omitempty"`

	// GivesAnyoneAccess is true for special groups like GitHub's
	// implicit "everyone with repo read" role — documents tagged with
	// such a group are effectively public within the org.
	// Matches RAGExternalUserGroup.GivesAnyoneAccess (Phase 1D).
	GivesAnyoneAccess bool `json:"gives_anyone_access"`
}
