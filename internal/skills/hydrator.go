// Package skills turns external skill sources (inline-authored, git repos)
// into immutable SkillVersion rows that bridge can consume at agent-run time.
package skills

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/model"
)

// Bundle is the JSON shape persisted in SkillVersion.Bundle. The Phase 4
// resolver adapts it into a bridge.SkillDefinition at agent-run time.
type Bundle struct {
	ID               string          `json:"id"`
	Title            string          `json:"title"`
	Description      string          `json:"description"`
	Content          string          `json:"content"`
	ParametersSchema json.RawMessage `json:"parameters_schema,omitempty"`
	Manifest         map[string]any  `json:"manifest,omitempty"`
	References       []Reference     `json:"references,omitempty"`
}

// Reference is a sibling file shipped alongside SKILL.md (scripts, templates,
// reference docs). Bridge-side support is negotiated in Phase 4.
type Reference struct {
	Path string `json:"path"`
	Body string `json:"body"`
}

// HydrateFromGit fetches the skill's repo at its tracked ref, parses
// SKILL.md + references, and writes a new SkillVersion — unless one already
// exists for that commit SHA, in which case it returns the existing row.
//
// Concurrent hydrations of the same skill are coalesced via a Postgres
// transaction-scoped advisory lock keyed on the skill ID.
func HydrateFromGit(ctx context.Context, db *gorm.DB, fetcher *GitFetcher, skillID uuid.UUID) (*model.SkillVersion, error) {
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

	// Fast path outside of any transaction — avoids taking a lock when
	// the version already exists.
	if existing, err := findVersionBySHA(ctx, db, skillID, sha); err != nil {
		return nil, err
	} else if existing != nil {
		return existing, nil
	}

	var result *model.SkillVersion
	txErr := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT pg_advisory_xact_lock(?)", advisoryLockKey(skillID)).Error; err != nil {
			return fmt.Errorf("acquire advisory lock: %w", err)
		}

		// Re-check after the lock in case another worker hydrated this SHA.
		if existing, err := findVersionBySHA(ctx, tx, skillID, sha); err != nil {
			return err
		} else if existing != nil {
			result = existing
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
			ID:          skill.Slug,
			Title:       manifestString(parsed.Manifest, "name", skill.Name),
			Description: manifestString(parsed.Manifest, "description", derefString(skill.Description)),
			Content:     parsed.SkillBody,
			Manifest:    parsed.Manifest,
			References:  parsed.References,
		}

		raw, err := json.Marshal(bundle)
		if err != nil {
			return fmt.Errorf("marshal bundle: %w", err)
		}

		now := time.Now()
		sha := sha
		sv := &model.SkillVersion{
			SkillID:    skillID,
			Version:    shortSHA(sha),
			CommitSHA:  &sha,
			Bundle:     model.RawJSON(raw),
			HydratedAt: &now,
		}
		if err := tx.Create(sv).Error; err != nil {
			return fmt.Errorf("create skill version: %w", err)
		}
		if err := tx.Model(&model.Skill{}).
			Where("id = ?", skillID).
			Update("latest_version_id", sv.ID).Error; err != nil {
			return fmt.Errorf("update latest_version_id: %w", err)
		}
		result = sv
		return nil
	})
	if txErr != nil {
		return nil, txErr
	}
	return result, nil
}

// findVersionBySHA returns the existing SkillVersion for (skillID, sha) or nil
// if none exists. Errors other than record-not-found are surfaced.
func findVersionBySHA(ctx context.Context, db *gorm.DB, skillID uuid.UUID, sha string) (*model.SkillVersion, error) {
	var sv model.SkillVersion
	err := db.WithContext(ctx).
		Where("skill_id = ? AND commit_sha = ?", skillID, sha).
		First(&sv).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("lookup version by sha: %w", err)
	}
	return &sv, nil
}

// advisoryLockKey hashes a skill UUID down to a bigint for pg_advisory_xact_lock.
// Collisions are harmless — two unrelated skills occasionally serializing is fine.
func advisoryLockKey(skillID uuid.UUID) int64 {
	return int64(binary.BigEndian.Uint64(skillID[:8]))
}

// shortSHA returns the first 7 characters of a commit SHA, or the whole string
// if shorter. Used as the human-readable Version label.
func shortSHA(sha string) string {
	if len(sha) <= 7 {
		return sha
	}
	return sha[:7]
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

func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// HydrateInline creates a new SkillVersion from a caller-supplied bundle and
// points the parent Skill at it. Used for inline-authored skills that never
// touch git.
func HydrateInline(ctx context.Context, db *gorm.DB, skillID uuid.UUID, bundle *Bundle, version string) (*model.SkillVersion, error) {
	raw, err := json.Marshal(bundle)
	if err != nil {
		return nil, fmt.Errorf("marshal bundle: %w", err)
	}

	now := time.Now()
	sv := &model.SkillVersion{
		SkillID:    skillID,
		Version:    version,
		Bundle:     model.RawJSON(raw),
		HydratedAt: &now,
	}

	err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(sv).Error; err != nil {
			return fmt.Errorf("create skill version: %w", err)
		}
		if err := tx.Model(&model.Skill{}).
			Where("id = ?", skillID).
			Update("latest_version_id", sv.ID).Error; err != nil {
			return fmt.Errorf("update latest_version_id: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return sv, nil
}
