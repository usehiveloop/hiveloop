package acl_test

import (
	"testing"

	"github.com/usehiveloop/hiveloop/internal/rag/acl"
	"github.com/usehiveloop/hiveloop/internal/rag/model"
)

// Business value: every string emitted by these helpers ends up in
// LanceDB at index time and in the query filter at read time. A
// one-byte drift = 0 results (if lucky) or wrong results across users
// (if unlucky). The Onyx reference strings are pinned byte-exactly.

func TestPrefixUserEmail(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{"typical_email", "alice@example.com", "user_email:alice@example.com"},
		{"empty", "", "user_email:"},
		{"unicode_local_part", "élodie@exämple.com", "user_email:élodie@exämple.com"},
		{"plus_addressing", "alice+tag@example.com", "user_email:alice+tag@example.com"},
		{"already_prefixed_is_not_special", "user_email:x@y.z", "user_email:user_email:x@y.z"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := acl.PrefixUserEmail(tc.in)
			if got != tc.want {
				t.Fatalf("PrefixUserEmail(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestPrefixUserGroup(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{"ascii_name", "engineering", "group:engineering"},
		{"empty", "", "group:"},
		{"unicode_name", "ingeniería", "group:ingeniería"},
		{"spaces_preserved", "All Staff", "group:All Staff"},
		{"case_preserved", "AdminOps", "group:AdminOps"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := acl.PrefixUserGroup(tc.in)
			if got != tc.want {
				t.Fatalf("PrefixUserGroup(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestPrefixExternalGroup(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{"github_team", "github_backend", "external_group:github_backend"},
		{"empty", "", "external_group:"},
		{"unicode_group", "外部チーム", "external_group:外部チーム"},
		{"preserves_underscores", "google_drive_shared_x", "external_group:google_drive_shared_x"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := acl.PrefixExternalGroup(tc.in)
			if got != tc.want {
				t.Fatalf("PrefixExternalGroup(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// TestBuildExtGroupName_LowercasesAndPrefixes pins the Onyx invariant at
// backend/onyx/access/utils.py:25-26: output is always lowercased AND
// source-prefixed. Drift here cross-contaminates ACLs across sources
// (a "payments" group in GitHub vs. the same slug in Google Drive).
func TestBuildExtGroupName_LowercasesAndPrefixes(t *testing.T) {
	cases := []struct {
		name   string
		in     string
		source model.DocumentSource
		want   string
	}{
		{"github_mixed_case", "Backend", model.DocumentSourceGithub, "github_backend"},
		{"github_all_upper", "BACKEND", model.DocumentSourceGithub, "github_backend"},
		{"google_drive_space_and_case", "Eng Team", model.DocumentSourceGoogleDrive, "google_drive_eng team"},
		{"notion_lower", "writers", model.DocumentSourceNotion, "notion_writers"},
		{"confluence_with_digits", "Team42", model.DocumentSourceConfluence, "confluence_team42"},
		{"slack_empty_name", "", model.DocumentSourceSlack, "slack_"},
		{"unicode_name_lowercased", "Élodie", model.DocumentSourceGithub, "github_élodie"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := acl.BuildExtGroupName(tc.in, tc.source)
			if got != tc.want {
				t.Fatalf("BuildExtGroupName(%q, %q) = %q, want %q", tc.in, tc.source, got, tc.want)
			}
		})
	}
}

// TestBuildExtGroupName_Idempotent pins the NON-idempotent behavior:
// calling BuildExtGroupName on an already-prefixed name double-prefixes.
// This test exists so callers cannot "just run it again to be safe" —
// any code path that touches a name is expected to call this helper
// exactly once.
func TestBuildExtGroupName_Idempotent(t *testing.T) {
	once := acl.BuildExtGroupName("x", model.DocumentSourceGithub)
	twice := acl.BuildExtGroupName(once, model.DocumentSourceGithub)

	if once != "github_x" {
		t.Fatalf("first call: got %q, want %q", once, "github_x")
	}
	if twice != "github_github_x" {
		t.Fatalf("double-call: got %q, want %q (BuildExtGroupName must NOT be idempotent; callers must guard themselves)", twice, "github_github_x")
	}
}

// TestPublicDocPat pins the exact byte value. Read-path query builders
// stamp this constant into a user's ACL allow-list; write-path indexers
// stamp it into public documents' ACL arrays. Value MUST match
// backend/onyx/configs/constants.py:27 byte-for-byte.
func TestPublicDocPat(t *testing.T) {
	if acl.PublicDocPat != "PUBLIC" {
		t.Fatalf("PublicDocPat = %q, want %q (matches onyx/configs/constants.py:27)", acl.PublicDocPat, "PUBLIC")
	}
}
