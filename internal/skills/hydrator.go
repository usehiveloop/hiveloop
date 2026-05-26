package skills

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

// Bundle is the JSON shape persisted on Skill.Bundle.
type Bundle struct {
	ID                           string            `json:"id"`
	Title                        string            `json:"title"`
	Description                  string            `json:"description"`
	Content                      string            `json:"content"`
	ParametersSchema             json.RawMessage   `json:"parameters_schema,omitempty"`
	Manifest                     map[string]any    `json:"manifest,omitempty"`
	References                   []Reference       `json:"references,omitempty"`
	Files                        map[string]string `json:"files,omitempty"`
	RequiredEnvironmentVariables []string          `json:"required_environment_variables,omitempty"`
}

// Reference is a sibling file shipped alongside SKILL.md (scripts, templates,
// reference docs). runtime-side support is negotiated in Phase 4.
type Reference struct {
	Path string `json:"path"`
	Body string `json:"body"`
}

// HydrateFromGit fetches the skill's repo at its tracked ref, parses
// SKILL.md + references, and writes the current Skill bundle.
//
// Concurrent hydrations of the same skill are coalesced via a Postgres
// transaction-scoped advisory lock keyed on the skill ID.
func HydrateFromGit(ctx context.Context, db *gorm.DB, fetcher *GitFetcher, skillID uuid.UUID) (*model.Skill, error) {
	var skill model.Skill
	if err := db.WithContext(ctx).First(&skill, "id = ?", skillID).Error; err != nil {
		return nil, fmt.Errorf("loading skill %s: %w", skillID, err)
	}
	if skill.SourceType != model.SkillSourceGit || skill.RepoURL == nil {
		return nil, fmt.Errorf("skill %s is not git-sourced", skillID)
	}

	sha, err := fetcher.ResolveRef(ctx, *skill.RepoURL, skill.RepoRef)
	if err != nil {
		return nil, fmt.Errorf("resolve ref: %w", err)
	}
	if skill.HydratedCommitSHA != nil && *skill.HydratedCommitSHA == sha && len(skill.Bundle) > 0 {
		return &skill, nil
	}

	var result *model.Skill
	txErr := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT pg_advisory_xact_lock(?)", advisoryLockKey(skillID)).Error; err != nil {
			return fmt.Errorf("acquire advisory lock: %w", err)
		}

		var lockedSkill model.Skill
		if err := tx.First(&lockedSkill, "id = ?", skillID).Error; err != nil {
			return fmt.Errorf("reload skill: %w", err)
		}
		if lockedSkill.HydratedCommitSHA != nil && *lockedSkill.HydratedCommitSHA == sha && len(lockedSkill.Bundle) > 0 {
			result = &lockedSkill
			return nil
		}

		subpath := ""
		if skill.RepoSubpath != nil {
			subpath = *skill.RepoSubpath
		}

		tarball, err := fetcher.FetchTarball(ctx, *skill.RepoURL, sha)
		if err != nil {
			return fmt.Errorf("fetch tarball: %w", err)
		}
		defer tarball.Close()

		parsed, err := parseSkillTarball(tarball, subpath)
		if err != nil {
			return fmt.Errorf("parse tarball: %w", err)
		}

		bundle := &Bundle{
			ID:                           skill.Slug,
			Title:                        manifestString(parsed.Manifest, "name", skill.Name),
			Description:                  manifestString(parsed.Manifest, "description", derefString(skill.Description)),
			Content:                      parsed.SkillBody,
			Manifest:                     parsed.Manifest,
			References:                   parsed.References,
			RequiredEnvironmentVariables: manifestStringSlice(parsed.Manifest, "required_environment_variables"),
		}

		raw, err := json.Marshal(bundle)
		if err != nil {
			return fmt.Errorf("marshal bundle: %w", err)
		}

		now := time.Now()
		updates := map[string]any{
			"bundle":              model.RawJSON(raw),
			"hydrated_commit_sha": sha,
			"hydrated_at":         &now,
			"hydration_error":     nil,
		}
		if err := tx.Model(&lockedSkill).Updates(updates).Error; err != nil {
			return fmt.Errorf("update skill bundle: %w", err)
		}
		if err := tx.First(&lockedSkill, "id = ?", skillID).Error; err != nil {
			return fmt.Errorf("reload updated skill: %w", err)
		}
		result = &lockedSkill
		return nil
	})
	if txErr != nil {
		return nil, txErr
	}
	return result, nil
}

// advisoryLockKey hashes a skill UUID down to a bigint for pg_advisory_xact_lock.
// Collisions are harmless — two unrelated skills occasionally serializing is fine.
func advisoryLockKey(skillID uuid.UUID) int64 {
	return int64(binary.BigEndian.Uint64(skillID[:8])) // #nosec G115 -- hash truncation; sign bit is part of the hash distribution
}

// manifestString reads a string field from a parsed YAML manifest, falling
// back to defaultValue when the field is missing or not a string.
func manifestString(manifest map[string]any, key, defaultValue string) string {
	if manifest == nil {
		return defaultValue
	}
	if raw, ok := manifest[key]; ok {
		if s, ok := raw.(string); ok && s != "" {
			return s
		}
	}
	return defaultValue
}

func manifestStringSlice(manifest map[string]any, key string) []string {
	if manifest == nil {
		return nil
	}
	raw, ok := manifest[key]
	if !ok {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	switch value := raw.(type) {
	case []string:
		for _, item := range value {
			item = strings.TrimSpace(item)
			if item == "" || seen[item] {
				continue
			}
			seen[item] = true
			out = append(out, item)
		}
	case []any:
		for _, item := range value {
			s, ok := item.(string)
			if !ok {
				continue
			}
			s = strings.TrimSpace(s)
			if s == "" || seen[s] {
				continue
			}
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// HydrateInline writes a caller-supplied bundle as the current Skill content.
func HydrateInline(ctx context.Context, db *gorm.DB, skillID uuid.UUID, bundle *Bundle) (*model.Skill, error) {
	raw, err := json.Marshal(bundle)
	if err != nil {
		return nil, fmt.Errorf("marshal bundle: %w", err)
	}

	now := time.Now()
	var skill model.Skill
	err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.First(&skill, "id = ?", skillID).Error; err != nil {
			return fmt.Errorf("load skill: %w", err)
		}
		updates := map[string]any{
			"bundle":              model.RawJSON(raw),
			"hydrated_commit_sha": nil,
			"hydrated_at":         &now,
			"hydration_error":     nil,
		}
		if err := tx.Model(&skill).Updates(updates).Error; err != nil {
			return fmt.Errorf("update skill bundle: %w", err)
		}
		return tx.First(&skill, "id = ?", skillID).Error
	})
	if err != nil {
		return nil, err
	}
	return &skill, nil
}
