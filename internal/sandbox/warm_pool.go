package sandbox

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/usehivy/hivy/internal/config"
	"github.com/usehivy/hivy/internal/crypto"
	"github.com/usehivy/hivy/internal/model"
)

const warmSlotHealthTimeout = 5 * time.Minute

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

func (p *WarmPool) Reconcile(ctx context.Context, mode string) error {
	if p == nil {
		return nil
	}
	desired := p.DesiredCount(mode)
	if desired <= 0 {
		return nil
	}
	var available int64
	if err := p.db.WithContext(ctx).Model(&model.SandboxWarmSlot{}).
		Where("provider_id = ? AND mode = ? AND status IN ?", p.provider.ID(), mode, []string{
			model.SandboxWarmSlotStatusWarm,
			model.SandboxWarmSlotStatusWarming,
		}).
		Count(&available).Error; err != nil {
		return err
	}
	for i := available; i < int64(desired); i++ {
		if err := p.provision(ctx, mode); err != nil {
			return err
		}
	}
	return nil
}

func (p *WarmPool) provision(ctx context.Context, mode string) error {
	provider, ok := p.provider.(WarmSlotProvider)
	if !ok {
		return fmt.Errorf("provider %s does not support warm slots", p.provider.ID())
	}
	runtimeSecret, err := generateRandomHex(32)
	if err != nil {
		return fmt.Errorf("generate runtime secret: %w", err)
	}
	encrypted, err := p.encKey.EncryptString(runtimeSecret)
	if err != nil {
		return fmt.Errorf("encrypt runtime secret: %w", err)
	}
	image := p.runtimeImage(mode)
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
		return err
	}
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
		return err
	}
	if err := waitForWarmSlotHealth(ctx, info.EndpointURL); err != nil {
		_ = p.MarkError(context.WithoutCancel(ctx), slot.ID, err.Error())
		return err
	}
	return p.db.WithContext(ctx).Model(&slot).Updates(map[string]any{
		"status":        model.SandboxWarmSlotStatusWarm,
		"error_message": nil,
	}).Error
}

func (p *WarmPool) runtimeImage(mode string) string {
	if mode == model.SandboxWarmSlotModeSpecialist {
		if p.provider.ID() == ProviderRailway && strings.TrimSpace(p.cfg.RailwaySpecialistRuntimeImage) != "" {
			return strings.TrimSpace(p.cfg.RailwaySpecialistRuntimeImage)
		}
		return p.cfg.SandboxesRuntimeSpecialistImagePrefix
	}
	if p.provider.ID() == ProviderRailway && strings.TrimSpace(p.cfg.RailwayRuntimeImage) != "" {
		return strings.TrimSpace(p.cfg.RailwayRuntimeImage)
	}
	return p.cfg.SandboxesRuntimeBaseImagePrefix
}

func (p *WarmPool) slotName(mode string) string {
	return fmt.Sprintf("hivy-%s-warm-%s", mode, strings.ReplaceAll(uuid.NewString()[:8], "-", ""))
}

func waitForWarmSlotHealth(ctx context.Context, endpoint string) error {
	deadline := time.Now().Add(warmSlotHealthTimeout)
	client := &http.Client{Timeout: 5 * time.Second}
	url := strings.TrimRight(endpoint, "/") + "/healthz"
	var lastErr error
	for time.Now().Before(deadline) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return err
		}
		resp, err := client.Do(req)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
			lastErr = fmt.Errorf("healthz returned %s", resp.Status)
		} else {
			lastErr = err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(3 * time.Second):
		}
	}
	if lastErr == nil {
		return fmt.Errorf("warm slot did not become healthy")
	}
	return fmt.Errorf("warm slot did not become healthy: %w", lastErr)
}
