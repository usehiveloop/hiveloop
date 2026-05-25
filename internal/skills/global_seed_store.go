package skills

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

func upsertGlobalSkill(ctx context.Context, db *gorm.DB, loaded loadedGlobalSkill) (created bool, changed bool, err error) {
	raw, err := json.Marshal(loaded.bundle)
	if err != nil {
		return false, false, fmt.Errorf("marshal global skill %s: %w", loaded.manifest.Name, err)
	}
	description := loaded.manifest.Description
	slug := model.GenerateSlug(loaded.manifest.Name)
	now := time.Now()

	err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var skill model.Skill
		err := tx.
			Where("org_id IS NULL AND lower(name) = lower(?)", loaded.manifest.Name).
			Order("created_at DESC").
			First(&skill).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("load global skill %s: %w", loaded.manifest.Name, err)
		}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			if err := createGlobalSkill(tx, &skill, loaded.manifest, slug, description, now); err != nil {
				return err
			}
			created = true
			if err := updateGlobalSkillBundle(tx, skill.ID, loaded.manifest.Name, raw, now); err != nil {
				return err
			}
		} else {
			changed = globalSkillChanged(skill, loaded.manifest, slug, description, raw)
			if changed {
				if err := updateGlobalSkill(tx, &skill, loaded.manifest, slug, description, now); err != nil {
					return err
				}
				if err := updateGlobalSkillBundle(tx, skill.ID, loaded.manifest.Name, raw, now); err != nil {
					return err
				}
			}
		}

		return archiveDuplicateGlobalSkills(tx, loaded.manifest.Name, skill.ID)
	})
	return created, changed, err
}

func globalSkillChanged(skill model.Skill, manifest globalSkillManifest, slug string, description string, raw []byte) bool {
	if skill.Slug != slug || skill.Name != manifest.Name || derefSkillDescription(skill.Description) != description {
		return true
	}
	if skill.Category != manifest.Category || skill.SourceType != model.SkillSourceInline || skill.RepoURL != nil || skill.RepoSubpath != nil || skill.RepoRef != "main" {
		return true
	}
	if !sameStringList([]string(skill.Tags), manifest.Tags) || !sameStringList([]string(skill.IntegrationIDs), manifest.IntegrationIDs) {
		return true
	}
	if skill.Hidden != manifest.Hidden || skill.Status != model.SkillStatusPublished || skill.PublishedAt == nil {
		return true
	}
	if skill.HydratedCommitSHA != nil || skill.HydrationError != nil {
		return true
	}
	return !rawJSONEqual(skill.Bundle, raw)
}

func sameStringList(left []string, right []string) bool {
	if len(left) == 0 && len(right) == 0 {
		return true
	}
	return slices.Equal(left, right)
}

func rawJSONEqual(left model.RawJSON, right []byte) bool {
	var leftValue any
	var rightValue any
	if err := json.Unmarshal(left, &leftValue); err != nil {
		return false
	}
	if err := json.Unmarshal(right, &rightValue); err != nil {
		return false
	}
	return reflect.DeepEqual(leftValue, rightValue)
}

func derefSkillDescription(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func createGlobalSkill(tx *gorm.DB, skill *model.Skill, manifest globalSkillManifest, slug, description string, now time.Time) error {
	*skill = model.Skill{
		OrgID:          nil,
		Slug:           slug,
		Name:           manifest.Name,
		Description:    &description,
		Category:       manifest.Category,
		SourceType:     model.SkillSourceInline,
		RepoRef:        "main",
		Tags:           manifest.Tags,
		IntegrationIDs: manifest.IntegrationIDs,
		Hidden:         manifest.Hidden,
		Status:         model.SkillStatusPublished,
		PublishedAt:    &now,
	}
	if err := tx.Create(skill).Error; err != nil {
		return fmt.Errorf("create global skill %s: %w", manifest.Name, err)
	}
	return nil
}

func updateGlobalSkill(tx *gorm.DB, skill *model.Skill, manifest globalSkillManifest, slug, description string, now time.Time) error {
	updates := map[string]any{
		"slug":            slug,
		"name":            manifest.Name,
		"description":     &description,
		"category":        manifest.Category,
		"source_type":     model.SkillSourceInline,
		"repo_url":        nil,
		"repo_subpath":    nil,
		"repo_ref":        "main",
		"tags":            pq.StringArray(manifest.Tags),
		"integration_ids": pq.StringArray(manifest.IntegrationIDs),
		"hidden":          manifest.Hidden,
		"status":          model.SkillStatusPublished,
		"published_at":    coalesceTime(skill.PublishedAt, now),
	}
	if err := tx.Model(skill).Updates(updates).Error; err != nil {
		return fmt.Errorf("update global skill %s: %w", manifest.Name, err)
	}
	return nil
}

func updateGlobalSkillBundle(tx *gorm.DB, skillID uuid.UUID, name string, raw []byte, now time.Time) error {
	updates := map[string]any{
		"bundle":              model.RawJSON(raw),
		"hydrated_commit_sha": nil,
		"hydrated_at":         &now,
		"hydration_error":     nil,
	}
	if err := tx.Model(&model.Skill{}).Where("id = ?", skillID).Updates(updates).Error; err != nil {
		return fmt.Errorf("update global skill bundle %s: %w", name, err)
	}
	return nil
}

func archiveDuplicateGlobalSkills(tx *gorm.DB, name string, keepID uuid.UUID) error {
	err := tx.Model(&model.Skill{}).
		Where("org_id IS NULL AND lower(name) = lower(?) AND id <> ?", name, keepID).
		Update("status", model.SkillStatusArchived).Error
	if err != nil {
		return fmt.Errorf("archive duplicate global skill %s: %w", name, err)
	}
	return nil
}

func archiveObsoleteGlobalSkills(ctx context.Context, db *gorm.DB) error {
	if len(obsoleteGlobalSkillNames) == 0 {
		return nil
	}
	err := db.WithContext(ctx).
		Model(&model.Skill{}).
		Where("org_id IS NULL AND lower(name) IN ?", obsoleteGlobalSkillNames).
		Update("status", model.SkillStatusArchived).Error
	if err != nil {
		return fmt.Errorf("archive obsolete global skills: %w", err)
	}
	return nil
}

func coalesceTime(existing *time.Time, fallback time.Time) *time.Time {
	if existing != nil {
		return existing
	}
	return &fallback
}
