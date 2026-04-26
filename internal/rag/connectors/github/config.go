package github

import (
	"encoding/json"
	"fmt"
	"strings"
)

type GithubConfig struct {
	RepoOwner     string   `json:"repo_owner"`
	Repositories  []string `json:"repositories,omitempty"`
	StateFilter   string   `json:"state_filter,omitempty"`
	IncludePRs    bool     `json:"include_prs"`
	IncludeIssues bool     `json:"include_issues"`
}

// validStateFilters: "merged" is a frequent admin mistake — it's not a
// real GitHub state (merged PRs surface as closed), so we reject it
// explicitly to fail fast at create time rather than silently return
// empty fetches at run time.
var validStateFilters = map[string]struct{}{
	"open":   {},
	"closed": {},
	"all":    {},
}

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

// normaliseRepoList re-splits comma-joined entries — admins occasionally
// type "a,b,c" into a JSON array of one string instead of three.
func normaliseRepoList(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, 0, len(in))
	for _, entry := range in {
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
