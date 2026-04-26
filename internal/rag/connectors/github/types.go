// Plain Go types for the slice of the GitHub REST API the connector
// consumes. Field set is the strict minimum we map into Document /
// ExternalAccess; everything else GitHub returns is ignored at decode
// time.
//
// Why not go-github: the wire is Nango's proxy, not direct HTTP, and
// we'd be translating go-github structs into our Document shape anyway.
// Having a focused 80-line type file is cheaper than wiring Nango as a
// http.RoundTripper for a third-party library.
//
// Onyx analog: PyGithub's PullRequest / Issue / Repository types — we
// pin only the fields touched by connector.py:247-394 (PR/Issue conversion)
// + ee/external_permissions/github/utils.py (permission mapping).
package github

import "time"

// GithubUser maps the user shape that appears as `user`, `assignee`,
// `assignees[]`, and `members[]` across the GitHub REST API. The email
// field is null for users who hide their email — we still emit such
// users into PrimaryOwners/SecondaryOwners with the username if email is
// blank, so the chunker has a non-empty owner list.
type GithubUser struct {
	ID    int64  `json:"id"`
	Login string `json:"login"`
	Email string `json:"email,omitempty"` // optional; null for hidden-email users
	Type  string `json:"type,omitempty"`  // "User" | "Bot" | "Organization"
}

// GithubRepoOwner is the nested owner blob on a Repository response. The
// `id` field doubles as the org_id for org-owned repos — used by
// internal-visibility ACL group construction (utils.py:249-277).
type GithubRepoOwner struct {
	ID    int64  `json:"id"`
	Login string `json:"login"`
	Type  string `json:"type,omitempty"`
}

// GithubRepo carries the repo metadata the connector needs: full name
// for URL building, ID for stable identifier across renames, owner for
// org-id derivation, visibility for the public/private/internal ACL
// branch.
//
// `visibility` is the modern field name; older API responses populated
// `private` (bool) only. We read both — `visibility` wins when present.
type GithubRepo struct {
	ID         int64           `json:"id"`
	Name       string          `json:"name"`
	FullName   string          `json:"full_name"`
	Owner      GithubRepoOwner `json:"owner"`
	Private    bool            `json:"private"`
	Visibility string          `json:"visibility,omitempty"` // "public" | "private" | "internal"
	HTMLURL    string          `json:"html_url"`
}

// GithubPRRef is the `pull_request` blob attached to issue responses
// when the issue is actually a PR. Presence of the URL is the signal
// to skip the issue (matches connector.py:710 — `if issue.pull_request`).
type GithubPRRef struct {
	URL string `json:"url,omitempty"`
}

// GithubPR carries the pull-request fields we map into a Document.
// Body is documented as nullable; we treat nil/missing as empty string
// (matches connector.py:268 — `pr.body or ""`).
type GithubPR struct {
	ID        int64        `json:"id"`
	Number    int          `json:"number"`
	Title     string       `json:"title"`
	Body      string       `json:"body"`
	State     string       `json:"state"` // "open" | "closed"
	HTMLURL   string       `json:"html_url"`
	CreatedAt time.Time    `json:"created_at"`
	UpdatedAt time.Time    `json:"updated_at"`
	MergedAt  *time.Time   `json:"merged_at,omitempty"`
	User      *GithubUser  `json:"user,omitempty"`
	Assignees []GithubUser `json:"assignees,omitempty"`
	Labels    []GithubLabel `json:"labels,omitempty"`
}

// GithubIssue carries the issue fields we map into a Document. Note:
// the `pull_request` ref means this row is actually a PR — the fetch
// loop drops those before conversion (matches connector.py:710).
type GithubIssue struct {
	ID          int64         `json:"id"`
	Number      int           `json:"number"`
	Title       string        `json:"title"`
	Body        string        `json:"body"`
	State       string        `json:"state"`
	HTMLURL     string        `json:"html_url"`
	CreatedAt   time.Time     `json:"created_at"`
	UpdatedAt   time.Time     `json:"updated_at"`
	User        *GithubUser   `json:"user,omitempty"`
	Assignees   []GithubUser  `json:"assignees,omitempty"`
	Labels      []GithubLabel `json:"labels,omitempty"`
	PullRequest *GithubPRRef  `json:"pull_request,omitempty"`
}

// GithubLabel is the slim label shape — just name surfaces into the
// document metadata. Color / description are unused.
type GithubLabel struct {
	Name string `json:"name"`
}

// GithubTeam covers /repos/{owner}/{repo}/teams. Slug is what GitHub
// canonicalises (e.g. "Backend Engineering" → "backend-engineering");
// utils.py:249-277 uses the slug as the group identifier.
type GithubTeam struct {
	ID          int64  `json:"id"`
	Slug        string `json:"slug"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// GithubMembership is the slim member-of-org shape. Used by
// /orgs/{org}/members for internal-visibility repos.
type GithubMembership struct {
	ID    int64  `json:"id"`
	Login string `json:"login"`
	Email string `json:"email,omitempty"`
	Type  string `json:"type,omitempty"`
}
