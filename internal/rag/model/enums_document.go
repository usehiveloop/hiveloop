// This file holds a MINIMAL stub of DocumentSource so that tranche 1D can
// port Onyx's ACL prefix helpers (which take a DocumentSource argument)
// without blocking on tranche 1A.
//
// Tranche 1A owns the authoritative port of Onyx's full DocumentSource
// enum (`backend/onyx/configs/constants.py:205-262`) and will expand this
// file. Tranche 1F is the merge point that resolves any drift between
// 1A's and 1D's copies.
//
// IMPORTANT: the set of constants defined here MUST be a subset of the
// 1A port, with identical string values. 1D tests reference a handful of
// sources (github, google_drive, notion, confluence); those are
// byte-for-byte copied from Onyx.
package model

// DocumentSource is a typed string enum matching Onyx's DocumentSource at
// backend/onyx/configs/constants.py:205-269. Values are lowercased snake
// case source identifiers; they appear verbatim in ACL-prefixed strings
// via BuildExtGroupName, so drift here is a security-critical bug.
//
// TODO(1F): unify with the full 1A enum file.
type DocumentSource string

// Subset of DocumentSource constants needed by the 1D ACL tests.
// 1A will add the remaining ~50 constants; 1F will reconcile.
const (
	DocumentSourceGithub      DocumentSource = "github"
	DocumentSourceGoogleDrive DocumentSource = "google_drive"
	DocumentSourceNotion      DocumentSource = "notion"
	DocumentSourceConfluence  DocumentSource = "confluence"
	DocumentSourceSlack       DocumentSource = "slack"
)
