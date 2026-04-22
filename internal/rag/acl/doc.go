// Package acl builds and applies the document-level access-control strings
// that LanceDB stores alongside each chunk and filters by at query time.
//
// Ports backend/onyx/access/ — specifically utils.py (prefix_user_email,
// prefix_user_group, prefix_external_group, build_ext_group_name_for_onyx)
// and models.py (DocumentAccess.to_acl at lines 174-197).
//
// Invariant: prefix strings are applied on read and on write. Off-by-one on
// the prefix = zero search results, so this package is pure-logic + tested
// byte-exactly against the Onyx reference strings.
package acl
