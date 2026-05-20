package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/model"
	skillpkg "github.com/usehiveloop/hiveloop/internal/skills"
)

type employeeSkillSyncPayload struct {
	Action                       string            `json:"action"`
	Name                         string            `json:"name"`
	Description                  string            `json:"description"`
	Category                     string            `json:"category"`
	Tags                         []string          `json:"tags"`
	RelatedSkills                []string          `json:"related_skills"`
	RequiredEnvironmentVariables []string          `json:"required_environment_variables"`
	Content                      string            `json:"content"`
	Files                        map[string]string `json:"files"`
	Deleted                      bool              `json:"deleted"`
	AbsorbedInto                 string            `json:"absorbed_into"`
}

func (h *EmployeeOutboundWebhookHandler) syncSkillEvent(ctx context.Context, sb *model.Sandbox, raw map[string]any) error {
	if sb.OrgID == nil || sb.AgentID == nil {
		return nil
	}
	var payload employeeSkillSyncPayload
	body, err := json.Marshal(raw)
	if err != nil {
		return fmt.Errorf("marshal skill payload: %w", err)
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return fmt.Errorf("decode skill payload: %w", err)
	}
	payload.Name = strings.TrimSpace(payload.Name)
	if payload.Name == "" {
		return errors.New("skill name is required")
	}
	slug := strings.ToLower(payload.Name)

	if payload.Action == "delete" || payload.Deleted {
		return h.detachSyncedSkill(ctx, *sb.OrgID, *sb.AgentID, slug)
	}
	if strings.TrimSpace(payload.Content) == "" {
		return errors.New("skill content is required")
	}
	return h.upsertSyncedSkill(ctx, *sb.OrgID, *sb.AgentID, slug, payload)
}

func (h *EmployeeOutboundWebhookHandler) detachSyncedSkill(ctx context.Context, orgID, agentID uuid.UUID, slug string) error {
	return h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var skill model.Skill
		err := tx.
			Where("org_id = ? AND slug = ?", orgID, slug).
			First(&skill).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("load skill for detach: %w", err)
		}
		if err := tx.
			Where("agent_id = ? AND skill_id = ?", agentID, skill.ID).
			Delete(&model.AgentSkill{}).Error; err != nil {
			return fmt.Errorf("detach skill from agent: %w", err)
		}
		if err := tx.Model(&model.Skill{}).
			Where("id = ?", skill.ID).
			UpdateColumn("install_count", gorm.Expr("(SELECT COUNT(*) FROM employee_skills WHERE skill_id = ?)", skill.ID)).Error; err != nil {
			return fmt.Errorf("update install count: %w", err)
		}
		return nil
	})
}

func (h *EmployeeOutboundWebhookHandler) upsertSyncedSkill(ctx context.Context, orgID, agentID uuid.UUID, slug string, payload employeeSkillSyncPayload) error {
	description := strings.TrimSpace(payload.Description)
	title := strings.TrimSpace(payload.Name)
	contentBody, manifest := splitSkillMarkdown(payload.Content)
	if description == "" {
		description = stringFromManifest(manifest, "description")
	}
	if title == "" {
		title = slug
	}
	now := time.Now()
	bundle := &skillpkg.Bundle{
		ID:                           slug,
		Title:                        title,
		Description:                  description,
		Content:                      contentBody,
		Manifest:                     manifest,
		References:                   referencesFromFiles(payload.Files),
		Files:                        payload.Files,
		RequiredEnvironmentVariables: payload.RequiredEnvironmentVariables,
	}

	return h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var skill model.Skill
		err := tx.Where("org_id = ? AND slug = ?", orgID, slug).First(&skill).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("load skill: %w", err)
		}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			skill = model.Skill{
				OrgID:       &orgID,
				Slug:        slug,
				Name:        title,
				Description: stringPtr(description),
				SourceType:  model.SkillSourceInline,
				RepoRef:     "main",
				Tags:        pq.StringArray(payload.Tags),
				Status:      model.SkillStatusPublished,
				PublishedAt: &now,
			}
			if err := tx.Create(&skill).Error; err != nil {
				return fmt.Errorf("create skill: %w", err)
			}
		} else {
			updates := map[string]any{
				"name":         title,
				"description":  stringPtr(description),
				"source_type":  model.SkillSourceInline,
				"repo_url":     nil,
				"repo_subpath": nil,
				"repo_ref":     "main",
				"tags":         pq.StringArray(payload.Tags),
				"status":       model.SkillStatusPublished,
				"published_at": coalesceTime(skill.PublishedAt, now),
			}
			if err := tx.Model(&skill).Updates(updates).Error; err != nil {
				return fmt.Errorf("update skill: %w", err)
			}
		}

		var versionCount int64
		if err := tx.Model(&model.SkillVersion{}).Where("skill_id = ?", skill.ID).Count(&versionCount).Error; err != nil {
			return fmt.Errorf("count versions: %w", err)
		}
		if _, err := skillpkg.HydrateInline(ctx, tx, skill.ID, bundle, fmt.Sprintf("v%d", versionCount+1)); err != nil {
			return fmt.Errorf("hydrate inline skill: %w", err)
		}
		link := model.AgentSkill{AgentID: agentID, SkillID: skill.ID}
		if err := tx.Where("agent_id = ? AND skill_id = ?", agentID, skill.ID).
			FirstOrCreate(&link).Error; err != nil {
			return fmt.Errorf("attach skill to agent: %w", err)
		}
		if err := tx.Model(&model.Skill{}).
			Where("id = ?", skill.ID).
			UpdateColumn("install_count", gorm.Expr("(SELECT COUNT(*) FROM employee_skills WHERE skill_id = ?)", skill.ID)).Error; err != nil {
			return fmt.Errorf("update install count: %w", err)
		}
		return nil
	})
}

func splitSkillMarkdown(content string) (string, map[string]any) {
	manifest := map[string]any{}
	lines := strings.Split(content, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return content, manifest
	}
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			return strings.Join(lines[i+1:], "\n"), manifest
		}
		if key, value, ok := strings.Cut(lines[i], ":"); ok {
			manifest[strings.TrimSpace(key)] = strings.Trim(strings.TrimSpace(value), `"`)
		}
	}
	return content, manifest
}

func stringFromManifest(manifest map[string]any, key string) string {
	value, _ := manifest[key].(string)
	return strings.TrimSpace(value)
}

func referencesFromFiles(files map[string]string) []skillpkg.Reference {
	if len(files) == 0 {
		return nil
	}
	refs := make([]skillpkg.Reference, 0, len(files))
	for path, body := range files {
		refs = append(refs, skillpkg.Reference{Path: path, Body: body})
	}
	return refs
}

func stringPtr(value string) *string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return &value
}

func coalesceTime(existing *time.Time, fallback time.Time) *time.Time {
	if existing != nil {
		return existing
	}
	return &fallback
}
