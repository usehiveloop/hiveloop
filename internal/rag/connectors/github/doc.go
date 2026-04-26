// Package github implements the GitHub PR + Issue connector.
//
// The connector talks to GitHub's REST API exclusively through Nango's
// proxy boundary — the connector code never sees, stores, or logs a
// GitHub token. Auth, refresh, and rotation live in Nango.
//
// Surfaces: CheckpointedConnector (PR + Issue ingest with resumable
// pagination), PermSyncConnector (visibility-based ACL + team/org group
// enumeration), SlimConnector (cheap doc-ID listing for prune diffing).
//
// Onyx ports — see plans/onyx-port-phase3d.md for the full citation
// index. Top of the list:
//
//   - backend/onyx/connectors/github/connector.py:437-1026 → connector.go
//   - backend/onyx/connectors/github/rate_limit_utils.py    → rate_limit.go
//   - backend/ee/onyx/external_permissions/github/*.py     → perm_sync.go
package github
