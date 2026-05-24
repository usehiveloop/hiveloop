package skills_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/skills"
)

func TestSeedGlobalSkills_CreatesPublishedOrgNullSkillAndOverridesContent(t *testing.T) {
	db := connectDB(t)
	name := "seed-test-" + uuid.New().String()
	dir := t.TempDir()
	writeGlobalSkill(t, dir, name, "first description", "# First\n", nil)

	result, err := skills.SeedGlobalSkills(context.Background(), db, dir)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	if result.Created != 1 || result.Updated != 0 || result.Unchanged != 0 {
		t.Fatalf("first result = %+v", result)
	}

	var skill model.Skill
	if err := db.Where("org_id IS NULL AND name = ?", name).First(&skill).Error; err != nil {
		t.Fatalf("load seeded skill: %v", err)
	}
	if skill.Status != model.SkillStatusPublished {
		t.Fatalf("status = %q", skill.Status)
	}
	if skill.OrgID != nil {
		t.Fatalf("org_id should be null")
	}
	firstBundle := string(skill.Bundle)

	result, err = skills.SeedGlobalSkills(context.Background(), db, dir)
	if err != nil {
		t.Fatalf("seed unchanged: %v", err)
	}
	if result.Updated != 1 {
		t.Fatalf("second seed should refresh existing content, got %+v", result)
	}
	if err := db.Where("org_id IS NULL AND name = ?", name).First(&skill).Error; err != nil {
		t.Fatalf("reload seeded skill: %v", err)
	}
	if string(skill.Bundle) != firstBundle {
		t.Fatalf("second seed should keep same current bundle")
	}

	writeGlobalSkill(t, dir, name, "second description", "# Second\n", nil)
	result, err = skills.SeedGlobalSkills(context.Background(), db, dir)
	if err != nil {
		t.Fatalf("seed override: %v", err)
	}
	if result.Updated != 1 {
		t.Fatalf("override result = %+v", result)
	}
	if err := db.Where("org_id IS NULL AND name = ?", name).First(&skill).Error; err != nil {
		t.Fatalf("reload updated skill: %v", err)
	}
	if string(skill.Bundle) == firstBundle {
		t.Fatalf("updated seed should replace current bundle")
	}
	if !strings.Contains(string(skill.Bundle), "# Second") {
		t.Fatalf("bundle did not contain updated content: %s", string(skill.Bundle))
	}
}

func TestSeedGlobalSkills_FetchesManifestReferenceURLs(t *testing.T) {
	db := connectDB(t)
	name := "seed-ref-test-" + uuid.New().String()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("remote reference body"))
	}))
	t.Cleanup(server.Close)

	dir := t.TempDir()
	writeGlobalSkill(t, dir, name, "has refs", "# Skill\n", []map[string]string{{
		"path": "references/remote.md",
		"url":  server.URL + "/remote.md",
	}})

	if _, err := skills.SeedGlobalSkills(context.Background(), db, dir); err != nil {
		t.Fatalf("seed: %v", err)
	}
	var skill model.Skill
	if err := db.Where("org_id IS NULL AND name = ?", name).First(&skill).Error; err != nil {
		t.Fatalf("load skill: %v", err)
	}
	if !strings.Contains(string(skill.Bundle), "remote reference body") {
		t.Fatalf("bundle missing remote reference: %s", string(skill.Bundle))
	}
}

func TestSeedGlobalSkills_PreservesRequiredEnvironmentVariables(t *testing.T) {
	db := connectDB(t)
	name := "seed-env-test-" + uuid.New().String()
	dir := t.TempDir()
	writeGlobalSkill(t, dir, name, "requires upload env", "# Skill\n", nil)

	manifestPath := filepath.Join(dir, name, "skill.json")
	body, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	var manifest map[string]any
	if err := json.Unmarshal(body, &manifest); err != nil {
		t.Fatalf("decode manifest: %v", err)
	}
	manifest["required_environment_variables"] = []string{"HIVY_DRIVE_UPLOAD_URL", "UPLOAD_BEARER"}
	body, err = json.Marshal(manifest)
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	if err := os.WriteFile(manifestPath, body, 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	if _, err := skills.SeedGlobalSkills(context.Background(), db, dir); err != nil {
		t.Fatalf("seed: %v", err)
	}
	var skill model.Skill
	if err := db.Where("org_id IS NULL AND name = ?", name).First(&skill).Error; err != nil {
		t.Fatalf("load skill: %v", err)
	}
	var bundle skills.Bundle
	if err := json.Unmarshal(skill.Bundle, &bundle); err != nil {
		t.Fatalf("decode bundle: %v", err)
	}
	want := []string{"HIVY_DRIVE_UPLOAD_URL", "UPLOAD_BEARER"}
	if !reflect.DeepEqual(bundle.RequiredEnvironmentVariables, want) {
		t.Fatalf("required env vars = %#v, want %#v", bundle.RequiredEnvironmentVariables, want)
	}
}

func TestSeedGlobalSkills_PreservesTagsAndIntegrationIDs(t *testing.T) {
	db := connectDB(t)
	name := "seed-tags-test-" + uuid.New().String()
	dir := t.TempDir()
	writeGlobalSkill(t, dir, name, "has metadata", "# Skill\n", nil)

	manifestPath := filepath.Join(dir, name, "skill.json")
	body, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	var manifest map[string]any
	if err := json.Unmarshal(body, &manifest); err != nil {
		t.Fatalf("decode manifest: %v", err)
	}
	manifest["category"] = "Project management"
	manifest["tags"] = []string{"linear", "issues"}
	manifest["integration_ids"] = []string{"linear", "github-app"}
	manifest["hidden"] = true
	body, err = json.Marshal(manifest)
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	if err := os.WriteFile(manifestPath, body, 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	if _, err := skills.SeedGlobalSkills(context.Background(), db, dir); err != nil {
		t.Fatalf("seed: %v", err)
	}
	var skill model.Skill
	if err := db.Where("org_id IS NULL AND name = ?", name).First(&skill).Error; err != nil {
		t.Fatalf("load skill: %v", err)
	}
	if skill.Category != "Project management" {
		t.Fatalf("category = %q, want %q", skill.Category, "Project management")
	}
	if want := []string{"linear", "issues"}; !reflect.DeepEqual([]string(skill.Tags), want) {
		t.Fatalf("tags = %#v, want %#v", []string(skill.Tags), want)
	}
	if want := []string{"linear", "github-app"}; !reflect.DeepEqual([]string(skill.IntegrationIDs), want) {
		t.Fatalf("integration ids = %#v, want %#v", []string(skill.IntegrationIDs), want)
	}
	if !skill.Hidden {
		t.Fatalf("hidden = false, want true")
	}

	manifest["category"] = "Engineering"
	manifest["tags"] = []string{"github", "review"}
	manifest["integration_ids"] = []string{"github-app", "github-app-code-reviews"}
	manifest["hidden"] = false
	body, err = json.Marshal(manifest)
	if err != nil {
		t.Fatalf("marshal updated manifest: %v", err)
	}
	if err := os.WriteFile(manifestPath, body, 0o644); err != nil {
		t.Fatalf("write updated manifest: %v", err)
	}
	if _, err := skills.SeedGlobalSkills(context.Background(), db, dir); err != nil {
		t.Fatalf("reseed: %v", err)
	}
	if err := db.Where("id = ?", skill.ID).First(&skill).Error; err != nil {
		t.Fatalf("reload skill: %v", err)
	}
	if skill.Category != "Engineering" {
		t.Fatalf("updated category = %q, want %q", skill.Category, "Engineering")
	}
	if want := []string{"github", "review"}; !reflect.DeepEqual([]string(skill.Tags), want) {
		t.Fatalf("updated tags = %#v, want %#v", []string(skill.Tags), want)
	}
	if want := []string{"github-app", "github-app-code-reviews"}; !reflect.DeepEqual([]string(skill.IntegrationIDs), want) {
		t.Fatalf("updated integration ids = %#v, want %#v", []string(skill.IntegrationIDs), want)
	}
	if skill.Hidden {
		t.Fatalf("updated hidden = true, want false")
	}
}

func TestBundledAssetUploadsSkillDeclaresUploadEnv(t *testing.T) {
	dir, err := skillsForRepoTest()
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := os.ReadFile(filepath.Join(dir, "global-skills", "asset-uploads", "skill.json"))
	if err != nil {
		t.Fatalf("read bundled asset uploads manifest: %v", err)
	}
	var parsed struct {
		RequiredEnvironmentVariables []string `json:"required_environment_variables"`
	}
	if err := json.Unmarshal(manifest, &parsed); err != nil {
		t.Fatalf("decode manifest: %v", err)
	}
	want := []string{"HIVY_DRIVE_UPLOAD_URL", "UPLOAD_BEARER"}
	if !reflect.DeepEqual(parsed.RequiredEnvironmentVariables, want) {
		t.Fatalf("asset upload env vars = %#v, want %#v", parsed.RequiredEnvironmentVariables, want)
	}
}

func TestSeedGlobalSkills_ArchivesObsoleteUploadSkillNames(t *testing.T) {
	db := connectDB(t)
	dir := t.TempDir()
	writeGlobalSkill(t, dir, "asset-uploads", "new uploads", "# Asset uploads\n", nil)

	for _, name := range []string{"public-assets-uploads", "employee-public-assets-uploads", "employee-assets-uploads"} {
		skill := model.Skill{
			OrgID:      nil,
			Slug:       model.GenerateSlug(name) + "-" + uuid.New().String()[:8],
			Name:       name,
			SourceType: model.SkillSourceInline,
			RepoRef:    "main",
			Status:     model.SkillStatusPublished,
		}
		if err := db.Create(&skill).Error; err != nil {
			t.Fatalf("create obsolete skill %s: %v", name, err)
		}
	}

	if _, err := skills.SeedGlobalSkills(context.Background(), db, dir); err != nil {
		t.Fatalf("seed: %v", err)
	}

	var publishedOld int64
	if err := db.Model(&model.Skill{}).
		Where("org_id IS NULL AND name IN ? AND status = ?", []string{"public-assets-uploads", "employee-public-assets-uploads", "employee-assets-uploads"}, model.SkillStatusPublished).
		Count(&publishedOld).Error; err != nil {
		t.Fatalf("count obsolete skills: %v", err)
	}
	if publishedOld != 0 {
		t.Fatalf("expected obsolete upload skills to be archived, got %d still published", publishedOld)
	}
}

func TestSeedGlobalSkills_RealBundledDirectory(t *testing.T) {
	if os.Getenv("RUN_REAL_GLOBAL_SKILLS_SEED") != "1" {
		t.Skip("set RUN_REAL_GLOBAL_SKILLS_SEED=1 to seed the real global-skills directory")
	}
	db := connectDB(t)

	result, err := skills.SeedGlobalSkills(context.Background(), db, "global-skills")
	if err != nil {
		t.Fatalf("seed real global skills: %v", err)
	}
	if result.Created+result.Updated+result.Unchanged == 0 {
		t.Fatalf("expected at least one bundled skill, got %+v", result)
	}
}

func writeGlobalSkill(t *testing.T, root, name, description, content string, files []map[string]string) {
	t.Helper()
	skillDir := filepath.Join(root, name)
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	manifest := map[string]any{
		"name":        name,
		"description": description,
		"root":        "./SKILL.md",
		"files":       files,
	}
	body, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "skill.json"), body, 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("write skill: %v", err)
	}
}

func skillsForRepoTest() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(cwd, "go.mod")); err == nil {
			return cwd, nil
		}
		parent := filepath.Dir(cwd)
		if parent == cwd {
			return "", os.ErrNotExist
		}
		cwd = parent
	}
}
