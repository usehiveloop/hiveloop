// Package credentials is the single place that decides which Credential an
// agent's LLM calls should use.
//
// There are two kinds of credentials in the system:
//
//   - BYOK credentials: owned by a customer org, created via the user-facing
//     API, referenced by agent.credential_id.
//   - System credentials (org_id = NULL): owned by the platform, created via
//     admin-only endpoints, used by any agent whose credential_id is nil.
//
// Every credential-resolution call site routes through Resolve. That way the
// "agent.credential_id = nil means use platform keys" rule lives in exactly
// one file, and tests can swap the Picker for a fake.
package credentials
