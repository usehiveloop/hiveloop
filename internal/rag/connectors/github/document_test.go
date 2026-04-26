package github

import (
	"reflect"
	"testing"
	"time"

	"github.com/usehiveloop/hiveloop/internal/rag/connectors/interfaces"
)

func samplePR() GithubPR {
	merged := time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC)
	return GithubPR{
		ID:        2001,
		Number:    42,
		Title:     "Fix the gizmo",
		Body:      "Closes #41. Replaces the gizmo with a sprocket.",
		State:     "closed",
		HTMLURL:   "https://github.com/acme/widget/pull/42",
		CreatedAt: time.Date(2026, 3, 30, 9, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, 4, 1, 12, 5, 0, 0, time.UTC),
		MergedAt:  &merged,
		User:      &GithubUser{Login: "alice", Email: "alice@example.com"},
		Assignees: []GithubUser{
			{Login: "bob", Email: "bob@example.com"},
			{Login: "carol"}, // hidden email — falls back to login.
		},
		Labels: []GithubLabel{{Name: "bug"}, {Name: "p1"}},
	}
}

func sampleIssue() GithubIssue {
	return GithubIssue{
		ID:        3003,
		Number:    99,
		Title:     "Sprocket spins backwards",
		Body:      "Repro: hold the gizmo at 30°. Sprocket reverses.",
		State:     "open",
		HTMLURL:   "https://github.com/acme/widget/issues/99",
		CreatedAt: time.Date(2026, 4, 5, 10, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, 4, 5, 10, 0, 0, 0, time.UTC),
		User:      &GithubUser{Login: "dave", Email: "dave@example.com"},
		Labels:    []GithubLabel{{Name: "needs-repro"}},
	}
}

func TestPRConvertedToDocument_FieldsMatch(t *testing.T) {
	pr := samplePR()
	access := &interfaces.ExternalAccess{IsPublic: true}

	doc := prToDocument("acme/widget", pr, access)

	if doc.DocID != "github_pr_acme/widget_42" {
		t.Fatalf("DocID = %q", doc.DocID)
	}
	if doc.SemanticID != "Fix the gizmo" {
		t.Fatalf("SemanticID = %q", doc.SemanticID)
	}
	if doc.Link != "https://github.com/acme/widget/pull/42" {
		t.Fatalf("Link = %q", doc.Link)
	}
	if len(doc.Sections) != 1 || doc.Sections[0].Text != pr.Body {
		t.Fatalf("Sections = %+v", doc.Sections)
	}
	if !doc.IsPublic {
		t.Fatalf("expected IsPublic from ExternalAccess")
	}
	if !reflect.DeepEqual(doc.PrimaryOwners, []string{"alice@example.com"}) {
		t.Fatalf("PrimaryOwners = %v", doc.PrimaryOwners)
	}
	want := []string{"bob@example.com", "carol"}
	if !reflect.DeepEqual(doc.SecondaryOwners, want) {
		t.Fatalf("SecondaryOwners = %v, want %v", doc.SecondaryOwners, want)
	}
	if doc.Metadata["state"] != "closed" {
		t.Fatalf("metadata.state = %q", doc.Metadata["state"])
	}
	if doc.Metadata["object_type"] != "PullRequest" {
		t.Fatalf("metadata.object_type = %q", doc.Metadata["object_type"])
	}
	if doc.Metadata["merged"] != "true" {
		t.Fatalf("metadata.merged = %q", doc.Metadata["merged"])
	}
	if doc.Metadata["labels"] != "bug,p1" {
		t.Fatalf("metadata.labels = %q", doc.Metadata["labels"])
	}
	if doc.DocUpdatedAt == nil || !doc.DocUpdatedAt.Equal(pr.UpdatedAt) {
		t.Fatalf("DocUpdatedAt = %v", doc.DocUpdatedAt)
	}
}

func TestIssueConvertedToDocument_FieldsMatch(t *testing.T) {
	issue := sampleIssue()
	access := &interfaces.ExternalAccess{
		ExternalUserGroupIDs: []string{"external_group:github_42_collaborators"},
	}

	doc := issueToDocument("acme/widget", issue, access)

	if doc.DocID != "github_issue_acme/widget_99" {
		t.Fatalf("DocID = %q", doc.DocID)
	}
	if doc.SemanticID != "Sprocket spins backwards" {
		t.Fatalf("SemanticID = %q", doc.SemanticID)
	}
	if doc.IsPublic {
		t.Fatalf("private repo issue should not be IsPublic")
	}
	if !reflect.DeepEqual(doc.ACL, []string{"external_group:github_42_collaborators"}) {
		t.Fatalf("ACL = %v", doc.ACL)
	}
	if doc.Metadata["object_type"] != "Issue" {
		t.Fatalf("metadata.object_type = %q", doc.Metadata["object_type"])
	}
	if doc.Metadata["labels"] != "needs-repro" {
		t.Fatalf("metadata.labels = %q", doc.Metadata["labels"])
	}
}

func TestPRConvertedToDocument_HiddenEmailFallsBackToLogin(t *testing.T) {
	pr := samplePR()
	pr.User = &GithubUser{Login: "alice"} // no email
	doc := prToDocument("acme/widget", pr, nil)
	if !reflect.DeepEqual(doc.PrimaryOwners, []string{"alice"}) {
		t.Fatalf("expected login fallback; got %v", doc.PrimaryOwners)
	}
}
