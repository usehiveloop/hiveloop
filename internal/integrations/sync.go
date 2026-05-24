package integrations

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/nango"
)

func (s *Seeder) syncOne(ctx context.Context, m Manifest) (string, error) {
	if !enabled(m) {
		return s.disableOne(ctx, m)
	}
	if !m.AllowNoCatalog {
		if _, ok := s.catalog.GetProvider(m.Provider); !ok {
			return "", fmt.Errorf("%s: provider %q has no MCP catalog entry", m.SourcePath, m.Provider)
		}
	}
	provider, ok := s.nango.GetProvider(nangoProvider(m))
	if !ok {
		if m.Required {
			return "", fmt.Errorf("%s: Nango provider %q not found", m.SourcePath, nangoProvider(m))
		}
		logSkip(ctx, m, "nango provider not found")
		return "skipped", nil
	}
	creds, err := credentialsFromManifest(m, provider)
	if err != nil {
		var skipped skippedIntegration
		if errors.As(err, &skipped) {
			logSkip(ctx, m, skipped.reason)
			return "skipped", nil
		}
		return "", err
	}
	hash, err := manifestHash(m)
	if err != nil {
		return "", fmt.Errorf("hash %s: %w", m.SourcePath, err)
	}
	if err := s.upsertNango(ctx, m, creds); err != nil {
		if m.Required {
			return "", err
		}
		logSkip(ctx, m, err.Error())
		return "skipped", nil
	}
	cfg, err := s.fetchConfig(ctx, m)
	if err != nil {
		if m.Required {
			return "", err
		}
		logSkip(ctx, m, err.Error())
		return "skipped", nil
	}
	return s.upsertDB(ctx, m, cfg, hash)
}

func (s *Seeder) upsertNango(ctx context.Context, m Manifest, creds *nango.Credentials) error {
	key := nangoKey(m.UniqueKey)
	req := nango.UpdateIntegrationRequest{
		DisplayName: m.DisplayName,
		Credentials: creds,
	}
	_, err := s.nango.GetIntegration(ctx, key)
	if err == nil {
		if err := s.nango.UpdateIntegration(ctx, key, req); err != nil {
			return fmt.Errorf("update Nango integration %s: %w", key, err)
		}
		return nil
	}
	if !isNotFound(err) {
		return fmt.Errorf("get Nango integration %s: %w", key, err)
	}
	createReq := nango.CreateIntegrationRequest{
		UniqueKey:   key,
		Provider:    nangoProvider(m),
		DisplayName: m.DisplayName,
		Credentials: creds,
	}
	if err := s.nango.CreateIntegration(ctx, createReq); err != nil {
		return fmt.Errorf("create Nango integration %s: %w", key, err)
	}
	return nil
}

func (s *Seeder) fetchConfig(ctx context.Context, m Manifest) (model.JSON, error) {
	key := nangoKey(m.UniqueKey)
	resp, err := s.nango.GetIntegration(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("fetch Nango integration %s: %w", key, err)
	}
	template, _ := s.nango.GetProviderTemplate(nangoProvider(m))
	return model.JSON(nango.BuildConfig(resp, template, s.nango.CallbackURL())), nil
}

func (s *Seeder) upsertDB(ctx context.Context, m Manifest, cfg model.JSON, hash string) (string, error) {
	var existing model.Integration
	err := s.db.WithContext(ctx).
		Where("managed_by = ? AND managed_id = ?", managedBy, m.ID).
		First(&existing).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		err = s.db.WithContext(ctx).
			Where("unique_key = ?", m.UniqueKey).
			First(&existing).Error
	}
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return "", fmt.Errorf("load global integration %s: %w", m.ID, err)
	}
	created := errors.Is(err, gorm.ErrRecordNotFound)
	if !created && existing.UniqueKey != m.UniqueKey {
		if active, err := s.activeConnectionCount(ctx, existing.ID); err != nil {
			return "", err
		} else if active > 0 {
			return "", fmt.Errorf("global integration %s changes unique_key from %q to %q with %d active connection(s)",
				m.ID, existing.UniqueKey, m.UniqueKey, active)
		}
		if err := s.nango.DeleteIntegration(ctx, nangoKey(existing.UniqueKey)); err != nil && !isNotFound(err) {
			return "", fmt.Errorf("delete old Nango integration %s: %w", existing.UniqueKey, err)
		}
	}
	if created {
		integ := model.Integration{
			UniqueKey:         m.UniqueKey,
			Provider:          m.Provider,
			DisplayName:       m.DisplayName,
			Meta:              m.Meta,
			NangoConfig:       cfg,
			SupportsRAGSource: m.SupportsRAGSource,
			ManagedBy:         managedBy,
			ManagedID:         m.ID,
			ManagedHash:       hash,
			Required:          m.Required,
		}
		if err := s.db.WithContext(ctx).Create(&integ).Error; err != nil {
			return "", fmt.Errorf("create global integration %s: %w", m.ID, err)
		}
		return "created", nil
	}
	updates := map[string]any{
		"unique_key":          m.UniqueKey,
		"provider":            m.Provider,
		"display_name":        m.DisplayName,
		"meta":                m.Meta,
		"nango_config":        cfg,
		"supports_rag_source": m.SupportsRAGSource,
		"managed_by":          managedBy,
		"managed_id":          m.ID,
		"managed_hash":        hash,
		"required":            m.Required,
		"deleted_at":          nil,
	}
	if existing.ManagedHash == hash && existing.DeletedAt == nil && integrationMatches(existing, m, cfg) {
		return "unchanged", nil
	}
	if err := s.db.WithContext(ctx).Model(&existing).Updates(updates).Error; err != nil {
		return "", fmt.Errorf("update global integration %s: %w", m.ID, err)
	}
	return "updated", nil
}

func integrationMatches(existing model.Integration, m Manifest, cfg model.JSON) bool {
	return existing.UniqueKey == m.UniqueKey &&
		existing.Provider == m.Provider &&
		existing.DisplayName == m.DisplayName &&
		existing.SupportsRAGSource == m.SupportsRAGSource &&
		existing.ManagedBy == managedBy &&
		existing.ManagedID == m.ID &&
		existing.Required == m.Required &&
		jsonEqual(existing.Meta, m.Meta) &&
		jsonEqual(existing.NangoConfig, cfg)
}

func jsonEqual(a, b model.JSON) bool {
	ab, err := a.Value()
	if err != nil {
		return false
	}
	bb, err := b.Value()
	if err != nil {
		return false
	}
	return ab == bb
}
