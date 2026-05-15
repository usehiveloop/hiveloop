package handler

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/model"
)

func parseUniqueUUIDStrings(rawIDs []string, field string) ([]uuid.UUID, error) {
	if len(rawIDs) == 0 {
		return nil, nil
	}
	ids := make([]uuid.UUID, 0, len(rawIDs))
	seen := make(map[uuid.UUID]struct{}, len(rawIDs))
	for _, rawID := range rawIDs {
		id, err := uuid.Parse(rawID)
		if err != nil {
			return nil, fmt.Errorf("invalid %s %q", field, rawID)
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	return ids, nil
}

func loadVisibleSkillsForOrg(ctx context.Context, db *gorm.DB, orgID uuid.UUID, ids []uuid.UUID) ([]model.Skill, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	var skills []model.Skill
	if err := db.WithContext(ctx).
		Select("id").
		Where("id IN ? AND (org_id = ? OR (org_id IS NULL AND status = ?))", ids, orgID, model.SkillStatusPublished).
		Find(&skills).Error; err != nil {
		return nil, fmt.Errorf("validate skill_ids: %w", err)
	}
	if len(skills) != len(ids) {
		return nil, fmt.Errorf("one or more skill_ids are not visible to this org")
	}
	return skills, nil
}
