package github

import (
	"strconv"
	"strings"

	"github.com/usehiveloop/hiveloop/internal/rag/connectors/interfaces"
)

// docIDForPR is number-scoped (not ID-scoped) so the admin-facing
// /pull/<number> URL and the scheduler's prune diff key on the same string.
func docIDForPR(repoFullName string, pr GithubPR) string {
	return "github_pr_" + repoFullName + "_" + strconv.Itoa(pr.Number)
}

func docIDForIssue(repoFullName string, issue GithubIssue) string {
	return "github_issue_" + repoFullName + "_" + strconv.Itoa(issue.Number)
}

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

// ownerEmails falls back to login when email is absent so downstream
// consumers always see a non-empty owner list.
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

// joinLabels: Document.Metadata is string→string by contract, so the
// label list is comma-joined into a single value.
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
