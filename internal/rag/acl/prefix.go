// Verbatim port of backend/onyx/access/utils.py (27 lines) plus the
// PUBLIC_DOC_PAT constant from backend/onyx/configs/constants.py:27.
//
// These four prefix helpers and the public-doc sentinel string form the
// security boundary of the RAG subsystem: at write time the indexer
// stamps the same prefixed strings into each chunk's ACL list; at read
// time the query filter reconstructs the same prefixed strings to match
// them. Any byte-level drift between the two sides = 0 results or, worse,
// wrong results (cross-user / cross-group leakage).
//
// Tests in prefix_test.go pin every output string against its Onyx
// reference, so changes to this file MUST be made in lockstep with the
// Python source and the test fixtures.
package acl

import (
	"strings"

	"github.com/usehiveloop/hiveloop/internal/rag/model"
)

// PublicDocPat is the sentinel ACL string stamped on documents that are
// readable by every member of an org. Port of PUBLIC_DOC_PAT at
// backend/onyx/configs/constants.py:27.
//
// Deliberately uppercase. Queries that run on behalf of an authenticated
// org member add this constant to their ACL allow-list; queries run as
// a non-member (e.g. an external share) do not.
const PublicDocPat = "PUBLIC"

// PrefixUserEmail mirrors prefix_user_email at
// backend/onyx/access/utils.py:4-8. The "user_email:" prefix prevents
// collision with group names inside the same ACL list and lets the
// vector-store filter short-circuit on prefix at query time.
func PrefixUserEmail(email string) string {
	return "user_email:" + email
}

// PrefixUserGroup mirrors prefix_user_group at
// backend/onyx/access/utils.py:11-14. The "group:" prefix namespaces
// Onyx-internal user groups against user emails and external groups.
//
// NOTE: Hiveloop does not port Onyx's EE UserGroup table. This helper
// is still ported verbatim because indexing code paths that compute
// ACLs may encounter legacy group rows during migration and because
// the helper is cheap to keep in sync.
func PrefixUserGroup(name string) string {
	return "group:" + name
}

// PrefixExternalGroup mirrors prefix_external_group at
// backend/onyx/access/utils.py:17-19. The "external_group:" prefix
// namespaces source-synced groups (e.g. GitHub teams, Google Drive
// shared-drive members) against Onyx user emails and Onyx user groups.
func PrefixExternalGroup(name string) string {
	return "external_group:" + name
}

// BuildExtGroupName mirrors build_ext_group_name_for_onyx at
// backend/onyx/access/utils.py:22-27. The source value prefixes the
// raw group name so two sources emitting the same group slug (e.g.
// "engineering" in GitHub and in Google Drive) do not collide inside
// the ACL list.
//
// The result is lowercased per the Onyx comment at utils.py:25-26 —
// case-preserving would make ACL matching case-sensitive, which is
// inconsistent with how most sources return group names. This lowercase
// is a load-bearing invariant: the indexer and the query path both call
// this function, so both paths produce the same byte sequence.
//
// NOTE: this function is NOT idempotent. Calling it twice with the same
// source double-prefixes: `BuildExtGroupName(BuildExtGroupName("x", github), github)`
// → "github_github_x". The test `TestBuildExtGroupName_Idempotent` pins
// that behavior so callers do not accidentally layer prefixes.
func BuildExtGroupName(extGroupName string, source model.DocumentSource) string {
	return strings.ToLower(string(source) + "_" + extGroupName)
}
