package handler

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
)

const defaultAssetUploadSkillName = "asset-uploads"

func attachPublishedGlobalSkills(ctx context.Context, db *gorm.DB, agentID uuid.UUID, names []string) {
	if db == nil || len(names) == 0 {
		return
	}
	log := logging.FromContext(ctx)
	for _, name := range names {
		var skill model.Skill
		err := db.WithContext(ctx).
			Where("org_id IS NULL AND status = ? AND name = ?", model.SkillStatusPublished, name).
			Order("created_at DESC").
			First(&skill).Error
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				log.WarnContext(ctx, "default global skill not found, skipping",
					"agent_id", agentID, "skill_name", name)
			} else {
				log.ErrorContext(ctx, "lookup default global skill",
					"error", err, "agent_id", agentID, "skill_name", name)
			}
			continue
		}
		link := model.AgentSkill{AgentID: agentID, SkillID: skill.ID}
		result := db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&link)
		if result.Error != nil {
			log.ErrorContext(ctx, "attach default global skill",
				"error", result.Error, "agent_id", agentID, "skill_id", skill.ID, "skill_name", name)
			continue
		}
		if result.RowsAffected == 0 {
			continue
		}
		if err := db.WithContext(ctx).Model(&model.Skill{}).
			Where("id = ?", skill.ID).
			UpdateColumn("install_count", gorm.Expr("install_count + 1")).Error; err != nil {
			log.WarnContext(ctx, "bump install_count for default global skill",
				"error", err, "skill_id", skill.ID)
		}
	}
}
