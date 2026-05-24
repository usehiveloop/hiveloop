package integrations

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

func (s *Seeder) disableOne(ctx context.Context, m Manifest) (string, error) {
	var existing model.Integration
	err := s.db.WithContext(ctx).
		Where("managed_by = ? AND managed_id = ?", managedBy, m.ID).
		First(&existing).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "skipped", nil
	}
	if err != nil {
		return "", fmt.Errorf("load disabled global integration %s: %w", m.ID, err)
	}
	deleted, err := s.disableIntegration(ctx, &existing)
	if err != nil {
		return "", err
	}
	if deleted {
		return "deleted", nil
	}
	return "skipped", nil
}

func (s *Seeder) disableMissing(ctx context.Context, seen map[string]bool) (int, error) {
	var integrations []model.Integration
	if err := s.db.WithContext(ctx).
		Where("managed_by = ? AND deleted_at IS NULL", managedBy).
		Find(&integrations).Error; err != nil {
		return 0, fmt.Errorf("load managed integrations: %w", err)
	}
	deleted := 0
	for i := range integrations {
		if seen[integrations[i].ManagedID] {
			continue
		}
		ok, err := s.disableIntegration(ctx, &integrations[i])
		if err != nil {
			return deleted, err
		}
		if ok {
			deleted++
		}
	}
	return deleted, nil
}

func (s *Seeder) disableIntegration(ctx context.Context, integ *model.Integration) (bool, error) {
	if integ.DeletedAt != nil {
		return false, nil
	}
	active, err := s.activeConnectionCount(ctx, integ.ID)
	if err != nil {
		return false, err
	}
	if active == 0 {
		if err := s.nango.DeleteIntegration(ctx, nangoKey(integ.UniqueKey)); err != nil && !isNotFound(err) {
			return false, fmt.Errorf("delete Nango integration %s: %w", integ.UniqueKey, err)
		}
	}
	if err := s.db.WithContext(ctx).Model(integ).Update("deleted_at", nowPtr()).Error; err != nil {
		return false, fmt.Errorf("soft-delete global integration %s: %w", integ.ManagedID, err)
	}
	return true, nil
}

func (s *Seeder) activeConnectionCount(ctx context.Context, integrationID interface{}) (int64, error) {
	var count int64
	if err := s.db.WithContext(ctx).Model(&model.Connection{}).
		Where("integration_id = ? AND revoked_at IS NULL", integrationID).
		Count(&count).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, nil
		}
		return 0, fmt.Errorf("count active integration connections: %w", err)
	}
	return count, nil
}
