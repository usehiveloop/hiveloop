// PR/Issue → Document mapping.
//
// Onyx ports:
//   - prToDocument: connector.py:247 (_convert_pr_to_document) +
//     :283-327 (_pr_metadata).
//   - issueToDocument: connector.py:336 (_convert_issue_to_document) +
//     :364-394 (_issue_metadata).
//
// Body text is the only chunkable section — Onyx does the same. PR
// review comments and issue comments are deliberately omitted (Onyx
// defines `_fetch_issue_comments` but never calls it from the main
// loop, and PR review-comment fetching was dropped in 0.16).
package github

import (
	"strconv"
	"strings"

	"github.com/usehiveloop/hiveloop/internal/rag/connectors/interfaces"
)

// docIDForPR matches Onyx's PR document_id format ("github_pr_<number>")
// at connector.py:247 — number-scoped, not ID-scoped, because the
// admin-facing URL is /pull/<number> and the scheduler's prune diff
// keys on this same string.
func docIDForPR(repoFullName string, pr GithubPR) string {
	return "github_pr_" + repoFullName + "_" + strconv.Itoa(pr.Number)
}

func docIDForIssue(repoFullName string, issue GithubIssue) string {
	return "github_issue_" + repoFullName + "_" + strconv.Itoa(issue.Number)
}

// prToDocument maps a GithubPR + per-doc ExternalAccess into the
// neutral Document shape the scheduler ships through ragclient.IngestBatch.
//
// PrimaryOwners: the PR author (one entry). SecondaryOwners: every
// assignee. We fall back to the GitHub login when email is absent
// because every downstream consumer expects at least one entry per
// owner list.
func prToDocument(repoFullName string, pr GithubPR, access *interfaces.ExternalAccess) interfaces.Document {
	updatedAt := pr.UpdatedAt
	doc := interfaces.Document{
		DocID:         docIDForPR(repoFullName, pr),
		SemanticID:    pr.Title,
		Link:          pr.HTMLURL,
		Sections:      []interfaces.Section{{Text: pr.Body, Link: pr.HTMLURL}},
		DocUpdatedAt:  &updatedAt,
		PrimaryOwners: ownerEmails(pr.User),
		SecondaryOwners: ownerEmailsList(pr.Assignees),
		Metadata:      prMetadata(pr),
	}
	applyAccess(&doc, access)
	return doc
}

// issueToDocument maps a GithubIssue + ExternalAccess into Document.
// Same shape as PR; differs only in the doc-id prefix and the metadata
// keys (no merged_at, etc.).
func issueToDocument(repoFullName string, issue GithubIssue, access *interfaces.ExternalAccess) interfaces.Document {
	updatedAt := issue.UpdatedAt
	doc := interfaces.Document{
		DocID:         docIDForIssue(repoFullName, issue),
		SemanticID:    issue.Title,
		Link:          issue.HTMLURL,
		Sections:      []interfaces.Section{{Text: issue.Body, Link: issue.HTMLURL}},
		DocUpdatedAt:  &updatedAt,
		PrimaryOwners: ownerEmails(issue.User),
		SecondaryOwners: ownerEmailsList(issue.Assignees),
		Metadata:      issueMetadata(issue),
	}
	applyAccess(&doc, access)
	return doc
}

// applyAccess copies ExternalAccess into the Document's IsPublic + ACL
// fields. ACL is opaque to this layer — utils on perm_sync.go produced
// the prefixed strings, we just thread them through.
func applyAccess(d *interfaces.Document, a *interfaces.ExternalAccess) {
	if a == nil {
		return
	}
	d.IsPublic = a.IsPublic
	if len(a.ExternalUserGroupIDs) > 0 {
		d.ACL = append(d.ACL, a.ExternalUserGroupIDs...)
	}
	if len(a.ExternalUserEmails) > 0 {
		d.ACL = append(d.ACL, a.ExternalUserEmails...)
	}
}

// ownerEmails returns the single-element slice for PrimaryOwners. Falls
// back to login when email is nil to ensure downstream consumers always
// see a non-empty owner list.
func ownerEmails(u *GithubUser) []string {
	if u == nil {
		return nil
	}
	if u.Email != "" {
		return []string{u.Email}
	}
	if u.Login != "" {
		return []string{u.Login}
	}
	return nil
}

// ownerEmailsList is the SecondaryOwners variant — multi-element list.
func ownerEmailsList(users []GithubUser) []string {
	if len(users) == 0 {
		return nil
	}
	out := make([]string, 0, len(users))
	for _, u := range users {
		if u.Email != "" {
			out = append(out, u.Email)
		} else if u.Login != "" {
			out = append(out, u.Login)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// prMetadata builds the flat string-string map shipped to Rust.
// Mirrors connector.py:283-327 — state, number, labels, merged.
func prMetadata(pr GithubPR) map[string]string {
	m := map[string]string{
		"object_type": "PullRequest",
		"state":       pr.State,
		"number":      strconv.Itoa(pr.Number),
	}
	if pr.MergedAt != nil {
		m["merged"] = "true"
		m["merged_at"] = pr.MergedAt.UTC().Format("2006-01-02T15:04:05Z")
	}
	if labels := joinLabels(pr.Labels); labels != "" {
		m["labels"] = labels
	}
	return m
}

// issueMetadata mirrors connector.py:364-394 — same shape minus the
// PR-specific merged keys.
func issueMetadata(issue GithubIssue) map[string]string {
	m := map[string]string{
		"object_type": "Issue",
		"state":       issue.State,
		"number":      strconv.Itoa(issue.Number),
	}
	if labels := joinLabels(issue.Labels); labels != "" {
		m["labels"] = labels
	}
	return m
}

// joinLabels concatenates label names with comma separators for the
// metadata map. We don't structure as a list because Document.Metadata
// is string→string by contract (Onyx supports list[str], we restrict to
// strings — see internal/rag/connectors/interfaces/document.go).
func joinLabels(labels []GithubLabel) string {
	if len(labels) == 0 {
		return ""
	}
	parts := make([]string, 0, len(labels))
	for _, l := range labels {
		if l.Name != "" {
			parts = append(parts, l.Name)
		}
	}
	return strings.Join(parts, ",")
}
