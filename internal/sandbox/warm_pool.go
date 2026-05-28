package sandbox

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/usehivy/hivy/internal/config"
	"github.com/usehivy/hivy/internal/crypto"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
)

type WarmPool struct {
	db       *gorm.DB
	provider Provider
	encKey   *crypto.SymmetricKey
	cfg      *config.Config
}

type ClaimedWarmSlot struct {
	ID            uuid.UUID
	ExternalID    string
	EndpointURL   string
	RuntimeSecret string
}

func NewWarmPool(db *gorm.DB, provider Provider, encKey *crypto.SymmetricKey, cfg *config.Config) *WarmPool {
	if _, ok := provider.(WarmSlotProvider); !ok {
		return nil
	}
	return &WarmPool{db: db, provider: provider, encKey: encKey, cfg: cfg}
}

func (p *WarmPool) DesiredCount(mode string) int {
	if p == nil || p.cfg == nil {
		return 0
	}
	switch mode {
	case model.SandboxWarmSlotModeEmployee:
		return p.cfg.SandboxWarmPoolEmployeeSize
	case model.SandboxWarmSlotModeSpecialist:
		return p.cfg.SandboxWarmPoolSpecialistSize
	default:
		return 0
	}
}

func (p *WarmPool) Claim(ctx context.Context, mode string, sandboxID uuid.UUID) (*ClaimedWarmSlot, error) {
	if p == nil {
		return nil, fmt.Errorf("warm pool is not configured")
	}
	var slot model.SandboxWarmSlot
	err := p.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("provider_id = ? AND mode = ? AND status = ?", p.provider.ID(), mode, model.SandboxWarmSlotStatusWarm).
			Order("created_at ASC").
			First(&slot).Error; err != nil {
			return err
		}
		return tx.Model(&slot).Updates(map[string]any{
			"status":             model.SandboxWarmSlotStatusClaiming,
			"claimed_sandbox_id": sandboxID,
			"error_message":      nil,
		}).Error
	})
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("no warm %s sandbox slots available", mode)
		}
		return nil, err
	}
	token, err := p.encKey.DecryptString(slot.EncryptedRuntimeSecret)
	if err != nil {
		_ = p.MarkError(context.WithoutCancel(ctx), slot.ID, fmt.Sprintf("decrypt runtime secret: %v", err))
		return nil, err
	}
	return &ClaimedWarmSlot{
		ID:            slot.ID,
		ExternalID:    slot.ExternalID,
		EndpointURL:   slot.EndpointURL,
		RuntimeSecret: token,
	}, nil
}

func (p *WarmPool) MarkClaimed(ctx context.Context, slotID uuid.UUID) error {
	return p.db.WithContext(ctx).Model(&model.SandboxWarmSlot{}).
		Where("id = ?", slotID).
		Update("status", model.SandboxWarmSlotStatusClaimed).Error
}

func (p *WarmPool) MarkError(ctx context.Context, slotID uuid.UUID, message string) error {
	return p.db.WithContext(ctx).Model(&model.SandboxWarmSlot{}).
		Where("id = ?", slotID).
		Updates(map[string]any{
			"status":        model.SandboxWarmSlotStatusError,
			"error_message": message,
		}).Error
}

func (p *WarmPool) SlotMode(ctx context.Context, slotID uuid.UUID) (string, error) {
	var slot model.SandboxWarmSlot
	if err := p.db.WithContext(ctx).Select("mode").First(&slot, "id = ?", slotID).Error; err != nil {
		return "", err
	}
	return slot.Mode, nil
}

func (p *WarmPool) Reconcile(ctx context.Context, mode string, onCreated func(context.Context, uuid.UUID) error) ([]uuid.UUID, error) {
	if p == nil {
		return nil, nil
	}
	desired := p.DesiredCount(mode)
	if desired <= 0 {
		return nil, nil
	}
	logger := logging.FromContext(ctx)
	var available int64
	if err := p.db.WithContext(ctx).Model(&model.SandboxWarmSlot{}).
		Where("provider_id = ? AND mode = ? AND status IN ?", p.provider.ID(), mode, []string{
			model.SandboxWarmSlotStatusWarm,
			model.SandboxWarmSlotStatusWarming,
		}).
		Count(&available).Error; err != nil {
		return nil, err
	}
	logger.InfoContext(ctx, "sandbox warm pool reconcile",
		"provider", p.provider.ID(), "mode", mode, "desired", desired, "available", available)
	createCount := int64(desired) - available
	if createCount < 0 {
		createCount = 0
	}
	created := make([]uuid.UUID, 0, int(createCount))
	for i := available; i < int64(desired); i++ {
		slotID, err := p.provision(ctx, mode)
		if err != nil {
			return created, err
		}
		created = append(created, slotID)
		if onCreated != nil {
			if err := onCreated(ctx, slotID); err != nil {
				return created, err
			}
		}
	}
	var warming []model.SandboxWarmSlot
	if err := p.db.WithContext(ctx).
		Where("provider_id = ? AND mode = ? AND status = ?", p.provider.ID(), mode, model.SandboxWarmSlotStatusWarming).
		Find(&warming).Error; err != nil {
		return created, err
	}
	for _, slot := range warming {
		if !containsUUID(created, slot.ID) {
			created = append(created, slot.ID)
			if onCreated != nil {
				if err := onCreated(ctx, slot.ID); err != nil {
					return created, err
				}
			}
		}
	}
	return created, nil
}

func (p *WarmPool) provision(ctx context.Context, mode string) (uuid.UUID, error) {
	provider, ok := p.provider.(WarmSlotProvider)
	if !ok {
		return uuid.Nil, fmt.Errorf("provider %s does not support warm slots", p.provider.ID())
	}
	runtimeSecret, err := generateRandomHex(32)
	if err != nil {
		return uuid.Nil, fmt.Errorf("generate runtime secret: %w", err)
	}
	encrypted, err := p.encKey.EncryptString(runtimeSecret)
	if err != nil {
		return uuid.Nil, fmt.Errorf("encrypt runtime secret: %w", err)
	}
	image := p.runtimeImage(mode)
	logger := logging.FromContext(ctx)
	logger.InfoContext(ctx, "sandbox warm slot provisioning",
		"provider", p.provider.ID(), "mode", mode, "image", image, "port", p.cfg.RailwayRuntimePort)
	info, err := provider.CreateWarmSlot(ctx, WarmSlotCreateOpts{
		Name:          p.slotName(mode),
		Mode:          mode,
		RuntimeImage:  image,
		RuntimePort:   p.cfg.RailwayRuntimePort,
		RuntimeSecret: runtimeSecret,
		Labels: map[string]string{
			"mode":     mode,
			"provider": p.provider.ID(),
		},
	})
	if err != nil {
		return uuid.Nil, err
	}
	logger.InfoContext(ctx, "sandbox warm slot provider resource created",
		"provider", p.provider.ID(), "mode", mode, "external_id", info.ExternalID,
		"endpoint_url", info.EndpointURL, "port", info.RuntimePort)
	slot := model.SandboxWarmSlot{
		ProviderID:             p.provider.ID(),
		Mode:                   mode,
		Status:                 model.SandboxWarmSlotStatusWarming,
		ExternalID:             info.ExternalID,
		EndpointURL:            info.EndpointURL,
		RuntimeImage:           image,
		RuntimePort:            info.RuntimePort,
		Region:                 p.cfg.RailwayRegion,
		EncryptedRuntimeSecret: encrypted,
	}
	if err := p.db.WithContext(ctx).Create(&slot).Error; err != nil {
		_ = p.provider.DeleteSandbox(context.WithoutCancel(ctx), info.ExternalID)
		return uuid.Nil, err
	}
	return slot.ID, nil
}

func (p *WarmPool) runtimeImage(mode string) string {
	if mode == model.SandboxWarmSlotModeSpecialist {
		if _, usesWarmPool := p.provider.(WarmPoolCapable); usesWarmPool && strings.TrimSpace(p.cfg.RailwaySpecialistRuntimeImage) != "" {
			return strings.TrimSpace(p.cfg.RailwaySpecialistRuntimeImage)
		}
		return p.cfg.SandboxesRuntimeSpecialistImagePrefix
	}
	if _, usesWarmPool := p.provider.(WarmPoolCapable); usesWarmPool && strings.TrimSpace(p.cfg.RailwayRuntimeImage) != "" {
		return strings.TrimSpace(p.cfg.RailwayRuntimeImage)
	}
	return p.cfg.SandboxesRuntimeBaseImagePrefix
}

func (p *WarmPool) slotName(mode string) string {
	return fmt.Sprintf("hivy-%s-warm-%s", mode, strings.ReplaceAll(uuid.NewString()[:8], "-", ""))
}

func containsUUID(items []uuid.UUID, target uuid.UUID) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}
