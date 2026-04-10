package skills_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/ziraloop/ziraloop/internal/model"
	"github.com/ziraloop/ziraloop/internal/skills"
)

const testDBURL = "postgres://ziraloop:localdev@localhost:5433/ziraloop_test?sslmode=disable"

func connectDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = testDBURL
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("cannot connect to Postgres: %v", err)
	}
	sqlDB, _ := db.DB()
	if err := sqlDB.Ping(); err != nil {
		t.Fatalf("Postgres not reachable: %v", err)
	}
	if err := model.AutoMigrate(db); err != nil {
		t.Fatalf("migration failed: %v", err)
	}
	return db
}

func createOrg(t *testing.T, db *gorm.DB) uuid.UUID {
	t.Helper()
	org := model.Org{
		ID:     uuid.New(),
		Name:   "skills-test-" + uuid.New().String()[:8],
		Active: true,
	}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	return org.ID
}

// fakeGitHubServer stands up an httptest server that mimics the minimal
// GitHub REST surface the GitFetcher uses.
func fakeGitHubServer(t *testing.T, sha string, tarballBody []byte) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/", func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/repos/")
		parts := strings.Split(path, "/")
		// parts: owner, repo, kind, ref
		if len(parts) < 4 {
			http.Error(w, "bad path", http.StatusNotFound)
			return
		}
		switch parts[2] {
		case "commits":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{"sha": sha})
		case "tarball":
			w.Header().Set("Content-Type", "application/x-gzip")
			_, _ = w.Write(tarballBody)
		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
	})
	return httptest.NewServer(mux)
}

func TestHydrateInline(t *testing.T) {
	db := connectDB(t)
	orgID := createOrg(t, db)

	skill := &model.Skill{
		OrgID:      &orgID,
		Slug:       "inline-test-" + uuid.New().String()[:8],
		Name:       "inline test",
		SourceType: model.SkillSourceInline,
		Status:     model.SkillStatusDraft,
	}
	if err := db.Create(skill).Error; err != nil {
		t.Fatalf("create skill: %v", err)
	}

	bundle := &skills.Bundle{
		ID:          skill.Slug,
		Title:       "Inline Skill",
		Description: "created in UI",
		Content:     "body of the skill",
	}

	sv, err := skills.HydrateInline(context.Background(), db, skill.ID, bundle, "v1")
	if err != nil {
		t.Fatalf("HydrateInline: %v", err)
	}
	if sv.ID == uuid.Nil {
		t.Fatal("expected non-nil version id")
	}
	if sv.CommitSHA != nil {
		t.Errorf("inline version should not have a commit sha, got %q", *sv.CommitSHA)
	}

	var reloaded model.Skill
	if err := db.First(&reloaded, "id = ?", skill.ID).Error; err != nil {
		t.Fatalf("reload skill: %v", err)
	}
	if reloaded.LatestVersionID == nil || *reloaded.LatestVersionID != sv.ID {
		t.Errorf("latest_version_id not updated: got %v, want %v", reloaded.LatestVersionID, sv.ID)
	}

	var parsedBundle skills.Bundle
	if err := json.Unmarshal(sv.Bundle, &parsedBundle); err != nil {
		t.Fatalf("unmarshal bundle: %v", err)
	}
	if parsedBundle.Content != "body of the skill" {
		t.Errorf("bundle content mismatch: %q", parsedBundle.Content)
	}
}

func TestHydrateFromGit(t *testing.T) {
	db := connectDB(t)
	orgID := createOrg(t, db)

	sha := "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef"
	skillBody := "---\nname: greet\ndescription: say hi\n---\n# How to greet\nUse hello."
	tarball := buildFakeTarball(t, map[string]string{
		"SKILL.md":       skillBody,
		"scripts/run.sh": "#!/bin/sh\necho hi",
	})

	server := fakeGitHubServer(t, sha, tarball)
	defer server.Close()

	fetcher := skills.NewGitFetcher("").WithAPIBase(server.URL)

	repoURL := "https://github.com/ziraskills/greet"
	skill := &model.Skill{
		OrgID:      nil, // public
		Slug:       "git-test-" + uuid.New().String()[:8],
		Name:       "greet",
		SourceType: model.SkillSourceGit,
		RepoURL:    &repoURL,
		RepoRef:    "main",
		Status:     model.SkillStatusPublished,
	}
	if err := db.Create(skill).Error; err != nil {
		t.Fatalf("create skill: %v", err)
	}
	_ = orgID // retained for symmetry; public skill has no org

	sv, err := skills.HydrateFromGit(context.Background(), db, fetcher, skill.ID)
	if err != nil {
		t.Fatalf("HydrateFromGit: %v", err)
	}
	if sv.CommitSHA == nil || *sv.CommitSHA != sha {
		t.Errorf("commit sha = %v, want %q", sv.CommitSHA, sha)
	}

	var parsed skills.Bundle
	if err := json.Unmarshal(sv.Bundle, &parsed); err != nil {
		t.Fatalf("unmarshal bundle: %v", err)
	}
	if parsed.Title != "greet" {
		t.Errorf("title = %q, want greet", parsed.Title)
	}
	if parsed.Description != "say hi" {
		t.Errorf("description = %q, want 'say hi'", parsed.Description)
	}
	if parsed.Content != "# How to greet\nUse hello." {
		t.Errorf("content = %q", parsed.Content)
	}
	if len(parsed.References) != 1 || parsed.References[0].Path != "scripts/run.sh" {
		t.Errorf("references = %+v", parsed.References)
	}

	// Second call must return the existing version (dedupe).
	sv2, err := skills.HydrateFromGit(context.Background(), db, fetcher, skill.ID)
	if err != nil {
		t.Fatalf("second HydrateFromGit: %v", err)
	}
	if sv2.ID != sv.ID {
		t.Errorf("expected dedupe (same id), got %v and %v", sv.ID, sv2.ID)
	}

	// Only one version should exist in the DB.
	var count int64
	if err := db.Model(&model.SkillVersion{}).Where("skill_id = ?", skill.ID).Count(&count).Error; err != nil {
		t.Fatalf("count versions: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 version, got %d", count)
	}
}

// buildFakeTarball mirrors the in-package buildTarball helper for the
// external test package. GitHub wraps everything in a top-level dir
// (<owner>-<repo>-<sha>/), which the fetcher/parser strips.
func buildFakeTarball(t *testing.T, files map[string]string) []byte {
	t.Helper()
	topDir := fmt.Sprintf("ziraskills-greet-%s", uuid.New().String()[:8])
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
