package hermes

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/logging"
	"github.com/usehiveloop/hiveloop/internal/model"

	hsdk "github.com/usehiveloop/hermes/pkg/sdk"
)

type skillBundle struct {
	ID          string                 `json:"id"`
	Title       string                 `json:"title"`
	Description string                 `json:"description"`
	Content     string                 `json:"content"`
	Files       map[string]string      `json:"files,omitempty"`
	Frontmatter map[string]any         `json:"frontmatter,omitempty"`
	Manifest    map[string]any         `json:"manifest,omitempty"`
}

func buildSkillFiles(ctx context.Context, db *gorm.DB, agentID uuid.UUID) ([]hsdk.SyncFile, error) {
	links, err := loadAgentSkillLinks(ctx, db, agentID)
	if err != nil || len(links) == 0 {
		return nil, err
	}

	skillIDs := make([]uuid.UUID, len(links))
	pinnedByID := make(map[uuid.UUID]*uuid.UUID, len(links))
	for i, l := range links {
		skillIDs[i] = l.SkillID
		pinnedByID[l.SkillID] = l.PinnedVersionID
	}

	var skills []model.Skill
	if err := db.WithContext(ctx).Where("id IN ?", skillIDs).Find(&skills).Error; err != nil {
		return nil, err
	}

	versionIDs := make([]uuid.UUID, 0, len(skills))
	for _, s := range skills {
		if pinned := pinnedByID[s.ID]; pinned != nil {
			versionIDs = append(versionIDs, *pinned)
			continue
		}
		if s.LatestVersionID != nil {
			versionIDs = append(versionIDs, *s.LatestVersionID)
		}
	}
	if len(versionIDs) == 0 {
		return nil, nil
	}

	var versions []model.SkillVersion
	if err := db.WithContext(ctx).Where("id IN ?", versionIDs).Find(&versions).Error; err != nil {
		return nil, err
	}
	versionByID := make(map[uuid.UUID]model.SkillVersion, len(versions))
	for _, v := range versions {
		versionByID[v.ID] = v
	}

	files := make([]hsdk.SyncFile, 0, len(skills)*2)
	for _, s := range skills {
		var versionID uuid.UUID
		if pinned := pinnedByID[s.ID]; pinned != nil {
			versionID = *pinned
		} else if s.LatestVersionID != nil {
			versionID = *s.LatestVersionID
		} else {
			continue
		}
		v, ok := versionByID[versionID]
		if !ok {
			continue
		}

		var bundle skillBundle
		if err := json.Unmarshal(v.Bundle, &bundle); err != nil {
			logging.FromContext(ctx).WarnContext(ctx, "skip skill: bundle unmarshal failed",
				"agent_id", agentID, "skill_id", s.ID, "version_id", versionID, "error", err)
			continue
		}

		skillDir := path.Join("skills", s.Slug)
		mainFile := composeSkillMD(s, bundle)
		files = append(files, fileEntry(path.Join(skillDir, "SKILL.md"), mainFile, "0644"))
		for relPath, body := range bundle.Files {
			rel := strings.TrimLeft(strings.TrimSpace(relPath), "/")
			if rel == "" || rel == "SKILL.md" {
				continue
			}
			files = append(files, fileEntry(path.Join(skillDir, rel), []byte(body), "0644"))
		}
	}
	return files, nil
}

func loadAgentSkillLinks(ctx context.Context, db *gorm.DB, agentID uuid.UUID) ([]model.AgentSkill, error) {
	var links []model.AgentSkill
	if err := db.WithContext(ctx).Where("agent_id = ?", agentID).Find(&links).Error; err != nil {
		return nil, err
	}
	return links, nil
}

func composeSkillMD(s model.Skill, bundle skillBundle) []byte {
	description := ""
	if s.Description != nil {
		description = *s.Description
	}
	if description == "" {
		description = bundle.Description
	}

	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString(fmt.Sprintf("name: %s\n", s.Slug))
	if description != "" {
		b.WriteString(fmt.Sprintf("description: %q\n", description))
	}
	b.WriteString("---\n\n")
	if bundle.Content != "" {
		b.WriteString(bundle.Content)
		if !strings.HasSuffix(bundle.Content, "\n") {
			b.WriteString("\n")
		}
	}
	return []byte(b.String())
}
