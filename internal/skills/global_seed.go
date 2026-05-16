package skills

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/model"
)

const globalSkillFetchTimeout = 20 * time.Second

var obsoleteGlobalSkillNames = []string{
	"public-assets-uploads",
	"employee-public-assets-uploads",
	"employee-assets-uploads",
}

type GlobalSeedResult struct {
	Created   int
	Updated   int
	Unchanged int
}

type globalSkillManifest struct {
	Name                         string                    `json:"name"`
	Description                  string                    `json:"description"`
	Root                         string                    `json:"root"`
	Files                        []globalSkillManifestFile `json:"files"`
	Internal                     bool                      `json:"internal,omitempty"`
	RequiredEnvironmentVariables []string                  `json:"required_environment_variables,omitempty"`
}

type globalSkillManifestFile struct {
	Path string `json:"path"`
	URL  string `json:"url"`
}

type loadedGlobalSkill struct {
	manifest globalSkillManifest
	bundle   *Bundle
}

// SeedGlobalSkills loads bundled skills from global-skills/ and upserts them
// as published org-null skills. Existing skills are matched by name and get a
// fresh latest inline version so app startup always mirrors bundled content.
func SeedGlobalSkills(ctx context.Context, db *gorm.DB, skillsDir string) (*GlobalSeedResult, error) {
	if db == nil {
		return nil, fmt.Errorf("db is required")
	}
	if strings.TrimSpace(skillsDir) == "" {
		skillsDir = "global-skills"
	}
	resolvedDir, err := resolveGlobalSkillsDir(skillsDir)
	if err != nil {
		return nil, err
	}
	loaded, err := loadGlobalSkills(ctx, resolvedDir)
	if err != nil {
		return nil, err
	}

	result := &GlobalSeedResult{}
	for _, skill := range loaded {
		created, changed, err := upsertGlobalSkill(ctx, db, skill)
		if err != nil {
			return result, err
		}
		switch {
		case created:
			result.Created++
		case changed:
			result.Updated++
		default:
			result.Unchanged++
		}
	}
	if err := archiveObsoleteGlobalSkills(ctx, db); err != nil {
		return result, err
	}
	return result, nil
}

func resolveGlobalSkillsDir(skillsDir string) (string, error) {
	if stat, err := os.Stat(skillsDir); err == nil && stat.IsDir() {
		return skillsDir, nil
	}
	if filepath.IsAbs(skillsDir) {
		return "", fmt.Errorf("global skills dir %q not found", skillsDir)
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("get cwd: %w", err)
	}
	for {
		candidate := filepath.Join(cwd, skillsDir)
		if stat, err := os.Stat(candidate); err == nil && stat.IsDir() {
			return candidate, nil
		}
		parent := filepath.Dir(cwd)
		if parent == cwd {
			break
		}
		cwd = parent
	}
	return "", fmt.Errorf("global skills dir %q not found", skillsDir)
}

func loadGlobalSkills(ctx context.Context, skillsDir string) ([]loadedGlobalSkill, error) {
	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		return nil, fmt.Errorf("read global skills dir %q: %w", skillsDir, err)
	}
	out := make([]loadedGlobalSkill, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dir := filepath.Join(skillsDir, entry.Name())
		manifest, err := readGlobalSkillManifest(dir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		if manifest.Internal {
			continue
		}
		loaded, err := loadGlobalSkill(ctx, dir, manifest)
		if err != nil {
			return nil, err
		}
		out = append(out, loaded)
	}
	return out, nil
}

func readGlobalSkillManifest(dir string) (globalSkillManifest, error) {
	body, err := os.ReadFile(filepath.Join(dir, "skill.json"))
	if err != nil {
		return globalSkillManifest{}, err
	}
	var manifest globalSkillManifest
	if err := json.Unmarshal(body, &manifest); err != nil {
		return globalSkillManifest{}, fmt.Errorf("parse %s: %w", filepath.Join(dir, "skill.json"), err)
	}
	if strings.TrimSpace(manifest.Name) == "" {
		return globalSkillManifest{}, fmt.Errorf("%s: name is required", filepath.Join(dir, "skill.json"))
	}
	return manifest, nil
}

func loadGlobalSkill(ctx context.Context, dir string, manifest globalSkillManifest) (loadedGlobalSkill, error) {
	root := manifest.Root
	if strings.TrimSpace(root) == "" {
		root = "./SKILL.md"
	}
	root = filepath.Join(dir, strings.TrimPrefix(root, "./"))
	content, err := os.ReadFile(root)
	if err != nil {
		return loadedGlobalSkill{}, fmt.Errorf("read skill root %s: %w", root, err)
	}

	files := make(map[string]string, len(manifest.Files))
	references := make([]Reference, 0, len(manifest.Files))
	for _, file := range manifest.Files {
		if strings.TrimSpace(file.Path) == "" {
			return loadedGlobalSkill{}, fmt.Errorf("%s: file path is required", filepath.Join(dir, "skill.json"))
		}
		body, err := loadGlobalSkillFile(ctx, dir, file)
		if err != nil {
			return loadedGlobalSkill{}, fmt.Errorf("load %s for %s: %w", file.Path, manifest.Name, err)
		}
		files[file.Path] = body
		references = append(references, Reference{Path: file.Path, Body: body})
	}

	bundle := &Bundle{
		ID:                           model.GenerateSlug(manifest.Name),
		Title:                        manifest.Name,
		Description:                  manifest.Description,
		Content:                      string(content),
		References:                   references,
		Files:                        files,
		RequiredEnvironmentVariables: manifest.RequiredEnvironmentVariables,
	}
	return loadedGlobalSkill{manifest: manifest, bundle: bundle}, nil
}

func loadGlobalSkillFile(ctx context.Context, dir string, file globalSkillManifestFile) (string, error) {
	if strings.TrimSpace(file.URL) != "" {
		return fetchGlobalSkillURL(ctx, file.URL)
	}
	body, err := os.ReadFile(filepath.Join(dir, file.Path))
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func fetchGlobalSkillURL(ctx context.Context, rawURL string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, globalSkillFetchTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("GET %s: status %d", rawURL, resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(body), nil
}
