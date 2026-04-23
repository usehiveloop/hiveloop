package interfaces

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"
)

func TestSectionEmpty_AllowedByContract(t *testing.T) {
	// The contract: Section{Text: ""} is a valid value; the chunker
	// (Phase 2E) is responsible for skipping empty sections. This test
	// pins the contract by exercising construction + round-trip without
	// any validation gate firing.
	s := Section{Text: ""}
	if s.Text != "" {
		t.Fatalf("Section.Text = %q, want empty", s.Text)
	}

	// A Document built from empty sections round-trips cleanly.
	doc := Document{
		DocID:      "d",
		SemanticID: "d",
		Sections:   []Section{s, {Text: "hello"}, {}},
	}
	raw, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var back Document
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(back.Sections) != 3 {
		t.Fatalf("round-trip lost sections: got %d, want 3", len(back.Sections))
	}
}

// ---------------------------------------------------------------------
// 8. Document JSON round-trip preserves every field
// ---------------------------------------------------------------------

func TestDocument_JSONRoundtrip(t *testing.T) {
	updated := time.Date(2026, 4, 22, 12, 30, 45, 0, time.UTC)
	orig := Document{
		DocID:      "gh-pr-42",
		SemanticID: "Fix the flaky test",
		Link:       "https://github.com/acme/foo/pull/42",
		Sections: []Section{
			{Text: "body", Link: "https://github.com/acme/foo/pull/42", Title: "PR body"},
			{Text: "comment one", Link: "https://github.com/acme/foo/pull/42#c1"},
		},
		ACL:             []string{"user_email:alice@example.com", "external_group:github_acme_backend"},
		IsPublic:        false,
		DocUpdatedAt:    &updated,
		Metadata:        map[string]string{"state": "closed", "repo": "acme/foo"},
		PrimaryOwners:   []string{"alice@example.com"},
		SecondaryOwners: []string{"bob@example.com", "carol@example.com"},
	}

	raw, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var back Document
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// DocUpdatedAt: pointer comparison is wrong; compare Time values.
	if back.DocUpdatedAt == nil || !back.DocUpdatedAt.Equal(*orig.DocUpdatedAt) {
		t.Fatalf("DocUpdatedAt lost in round-trip: %v vs %v", back.DocUpdatedAt, orig.DocUpdatedAt)
	}
	// Replace pointer fields for the structural compare.
	back.DocUpdatedAt = orig.DocUpdatedAt

	if !reflect.DeepEqual(orig, back) {
		t.Fatalf("round-trip not equal:\norig=%+v\nback=%+v\nraw=%s", orig, back, string(raw))
	}
}

// ---------------------------------------------------------------------
// 9. ExternalAccess: IsPublic with empty user scope is legal
// ---------------------------------------------------------------------

func TestExternalAccess_PublicDocAllowsEmptyUserScope(t *testing.T) {
	ea := ExternalAccess{IsPublic: true}
	if !ea.IsPublic {
		t.Fatalf("IsPublic should be true")
	}
	if len(ea.ExternalUserEmails) != 0 {
		t.Fatalf("expected empty emails, got %v", ea.ExternalUserEmails)
	}
	if len(ea.ExternalUserGroupIDs) != 0 {
		t.Fatalf("expected empty group ids, got %v", ea.ExternalUserGroupIDs)
	}

	// Round-trip: public-empty-scope must survive JSON unchanged.
	raw, err := json.Marshal(ea)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var back ExternalAccess
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !back.IsPublic {
		t.Fatalf("round-trip dropped IsPublic; raw=%s", string(raw))
	}
}

// ---------------------------------------------------------------------
// 10. DocExternalAccess round-trips
// ---------------------------------------------------------------------

func TestExternalAccess_DocExternalAccessLinksToDoc(t *testing.T) {
	dea := DocExternalAccess{
		DocID: "gh-pr-42",
		ExternalAccess: &ExternalAccess{
			ExternalUserEmails:   []string{"alice@example.com"},
			ExternalUserGroupIDs: []string{"external_group:github_acme_backend"},
			IsPublic:             false,
		},
	}

	raw, err := json.Marshal(dea)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var back DocExternalAccess
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if back.DocID != dea.DocID {
		t.Fatalf("DocID lost: %q vs %q", back.DocID, dea.DocID)
	}
	if back.ExternalAccess == nil {
		t.Fatalf("ExternalAccess pointer lost")
	}
	if !reflect.DeepEqual(back.ExternalAccess, dea.ExternalAccess) {
		t.Fatalf("ExternalAccess mismatch:\norig=%+v\nback=%+v", dea.ExternalAccess, back.ExternalAccess)
	}
}

// ---------------------------------------------------------------------
// 11. Checkpoint marker interface composes with generics
// ---------------------------------------------------------------------
