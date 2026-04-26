// Port of backend/onyx/connectors/github/connector.py:442-455 (constructor
// args). The GithubConnector takes a thin shape — owner, optional repo
// allowlist, state filter, two booleans — and validates it at registration
// time. Wire-shape lives in RAGSource.Config (jsonb) and is parsed via
// LoadConfig below.
package github

import (
	"encoding/json"
	"fmt"
	"strings"
)

// GithubConfig is the per-source configuration blob persisted in
// RAGSource.Config and passed back to the connector at construction time.
//
// The schema mirrors Onyx's connector args one-to-one:
//
//   - RepoOwner: the org or user that owns the repo(s). Required.
//   - Repositories: optional repo-name allowlist (e.g. ["widget", "gadget"]).
//     When empty, the connector enumerates every repo under RepoOwner.
//   - StateFilter: GitHub PR/Issue state to fetch ("open", "closed", "all").
//     Defaults to "all" when empty.
//   - IncludePRs / IncludeIssues: feature toggles. Default true (matches
//     Onyx's include_prs/include_issues flags at connector.py:454).
type GithubConfig struct {
	RepoOwner     string   `json:"repo_owner"`
	Repositories  []string `json:"repositories,omitempty"`
	StateFilter   string   `json:"state_filter,omitempty"`
	IncludePRs    bool     `json:"include_prs"`
	IncludeIssues bool     `json:"include_issues"`
}

// validStateFilters mirrors GitHub's REST API enum for the `state` query
// param on /repos/.../pulls and /repos/.../issues.
//
// "merged" is a frequent admin mistake — it's not a real GitHub state;
// merged PRs surface as state=closed. We reject it explicitly so the
// admin gets a clean error at create time rather than silently empty
// fetches at run time.
var validStateFilters = map[string]struct{}{
	"open":   {},
	"closed": {},
	"all":    {},
}

// LoadConfig parses the raw RAGSource.Config jsonb blob into a GithubConfig
// and returns a normalized copy with defaults applied + validation enforced.
//
// Validation rules:
//
//   - RepoOwner must be non-empty after trimming whitespace. The whole
//     connector pivots on it (every URL starts /repos/{owner}/...).
//   - StateFilter, if set, must be one of the GitHub-recognised values.
//     Empty defaults to "all" (matches Onyx connector.py:611).
//   - Repositories entries are trimmed and any blank entries dropped, so
//     trailing-comma input "widget,gadget," doesn't ship a "" repo into
//     the URL builder.
func LoadConfig(raw json.RawMessage) (GithubConfig, error) {
	cfg := GithubConfig{
		IncludePRs:    true,
		IncludeIssues: true,
	}
	if len(raw) > 0 && string(raw) != "null" {
		if err := json.Unmarshal(raw, &cfg); err != nil {
			return GithubConfig{}, fmt.Errorf("github: parse config: %w", err)
		}
	}

	cfg.RepoOwner = strings.TrimSpace(cfg.RepoOwner)
	if cfg.RepoOwner == "" {
		return GithubConfig{}, fmt.Errorf("github: repo_owner is required")
	}

	cfg.StateFilter = strings.ToLower(strings.TrimSpace(cfg.StateFilter))
	if cfg.StateFilter == "" {
		cfg.StateFilter = "all"
	}
	if _, ok := validStateFilters[cfg.StateFilter]; !ok {
		return GithubConfig{}, fmt.Errorf(
			"github: state_filter %q is not one of {open, closed, all}", cfg.StateFilter,
		)
	}

	cfg.Repositories = normaliseRepoList(cfg.Repositories)
	return cfg, nil
}

// normaliseRepoList trims each entry and drops blanks. Also splits any
// single comma-joined entry — admins occasionally type "a,b,c" into a
// JSON array of one string instead of three.
func normaliseRepoList(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, 0, len(in))
	for _, entry := range in {
		// Permit one-off "a,b,c" by re-splitting on comma. Idempotent for
		// already-clean lists.
		for _, part := range strings.Split(entry, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			out = append(out, part)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
