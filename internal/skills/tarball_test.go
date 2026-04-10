package skills

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"testing"
)

func TestParseFrontmatter(t *testing.T) {
	cases := []struct {
		name           string
		input          string
		wantContent    string
		wantManifest   bool
		wantManifestOf string // optional key to check
	}{
		{
			name:           "with frontmatter",
			input:          "---\nname: greet\ndescription: hi\n---\n# Body\nhello",
			wantContent:    "# Body\nhello",
			wantManifest:   true,
			wantManifestOf: "name",
		},
		{
			name:        "no frontmatter",
			input:       "# Body\nhello",
			wantContent: "# Body\nhello",
		},
		{
			name:           "crlf after opening delimiter",
			input:          "---\r\nname: greet\r\n---\r\n# Body",
			wantContent:    "# Body",
			wantManifest:   true,
			wantManifestOf: "name",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			manifest, content, err := parseFrontmatter([]byte(tc.input))
			if err != nil {
				t.Fatalf("parseFrontmatter error: %v", err)
			}
			if content != tc.wantContent {
				t.Errorf("content mismatch\n got: %q\nwant: %q", content, tc.wantContent)
			}
			if tc.wantManifest && manifest == nil {
				t.Errorf("expected manifest, got nil")
			}
			if !tc.wantManifest && manifest != nil {
				t.Errorf("expected no manifest, got %v", manifest)
			}
			if tc.wantManifestOf != "" {
				if _, ok := manifest[tc.wantManifestOf]; !ok {
					t.Errorf("manifest missing key %q: %v", tc.wantManifestOf, manifest)
				}
			}
		})
	}
}

func TestParseGitHubURL(t *testing.T) {
	cases := []struct {
		input     string
		wantOwner string
		wantRepo  string
		wantErr   bool
	}{
		{input: "https://github.com/foo/bar", wantOwner: "foo", wantRepo: "bar"},
		{input: "https://github.com/foo/bar.git", wantOwner: "foo", wantRepo: "bar"},
		{input: "https://github.com/foo/bar/", wantOwner: "foo", wantRepo: "bar"},
		{input: "https://www.github.com/foo/bar", wantOwner: "foo", wantRepo: "bar"},
		{input: "https://gitlab.com/foo/bar", wantErr: true},
		{input: "https://github.com/foo", wantErr: true},
		{input: "not-a-url", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			owner, repo, err := parseGitHubURL(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error for %q, got owner=%q repo=%q", tc.input, owner, repo)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if owner != tc.wantOwner || repo != tc.wantRepo {
				t.Errorf("got owner=%q repo=%q, want owner=%q repo=%q", owner, repo, tc.wantOwner, tc.wantRepo)
			}
		})
	}
}

// buildTarball returns an in-memory .tar.gz whose top-level directory mimics
// a GitHub tarball ("owner-repo-abcdef/").
func buildTarball(t *testing.T, topDir string, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, body := range files {
		header := &tar.Header{
			Name:     topDir + "/" + name,
			Mode:     0o644,
			Size:     int64(len(body)),
			Typeflag: tar.TypeReg,
		}
		if err := tw.WriteHeader(header); err != nil {
			t.Fatalf("write header: %v", err)
		}
		if _, err := tw.Write([]byte(body)); err != nil {
			t.Fatalf("write body: %v", err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("close gzip: %v", err)
	}
	return buf.Bytes()
}

func TestParseSkillTarball_RootSubpath(t *testing.T) {
	skillBody := "---\nname: greet\ndescription: say hi\n---\n# How to greet\nUse `hello`."
	tarball := buildTarball(t, "foo-bar-abc123", map[string]string{
		"SKILL.md":         skillBody,
		"scripts/run.sh":   "#!/bin/sh\necho hi",
		"reference/api.md": "# API\nstuff",
	})

	parsed, err := parseSkillTarball(bytes.NewReader(tarball), "")
	if err != nil {
		t.Fatalf("parseSkillTarball error: %v", err)
	}
	if parsed.SkillBody != "# How to greet\nUse `hello`." {
		t.Errorf("skill body mismatch: %q", parsed.SkillBody)
	}
	if parsed.Manifest["name"] != "greet" {
		t.Errorf("manifest.name = %v, want greet", parsed.Manifest["name"])
	}
	if len(parsed.References) != 2 {
		t.Fatalf("expected 2 references, got %d", len(parsed.References))
	}

	found := map[string]bool{}
	for _, ref := range parsed.References {
		found[ref.Path] = true
	}
	for _, want := range []string{"scripts/run.sh", "reference/api.md"} {
		if !found[want] {
			t.Errorf("reference %q missing from parsed.References", want)
		}
	}
}

func TestParseSkillTarball_Subpath(t *testing.T) {
	tarball := buildTarball(t, "foo-bar-abc123", map[string]string{
		"skills/greet/SKILL.md":          "---\nname: greet\n---\nbody",
		"skills/greet/reference/api.md":  "refs",
		"skills/other/SKILL.md":          "other",
		"unrelated/ignore.txt":           "nope",
	})

	parsed, err := parseSkillTarball(bytes.NewReader(tarball), "skills/greet")
	if err != nil {
		t.Fatalf("parseSkillTarball error: %v", err)
	}
	if parsed.SkillBody != "body" {
		t.Errorf("unexpected skill body: %q", parsed.SkillBody)
	}
	if len(parsed.References) != 1 {
		t.Fatalf("expected 1 reference (reference/api.md), got %d: %+v", len(parsed.References), parsed.References)
	}
	if parsed.References[0].Path != "reference/api.md" {
		t.Errorf("reference path = %q, want reference/api.md", parsed.References[0].Path)
	}
}

func TestParseSkillTarball_MissingSkillMD(t *testing.T) {
	tarball := buildTarball(t, "foo-bar-abc123", map[string]string{
		"README.md": "no skill here",
	})
	if _, err := parseSkillTarball(bytes.NewReader(tarball), ""); err == nil {
		t.Fatal("expected error when SKILL.md missing, got nil")
	}
}
