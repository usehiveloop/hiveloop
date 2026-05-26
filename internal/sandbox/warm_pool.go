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
	"github.com/usehivy/hivy/internal/logging"
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

type WarmSlotCheckResult struct {
	Ready   bool
	Pending bool
}

func (p *WarmPool) CheckWarmSlot(ctx context.Context, slotID uuid.UUID) (*WarmSlotCheckResult, error) {
	if p == nil {
		return &WarmSlotCheckResult{}, nil
	}
	var slot model.SandboxWarmSlot
	if err := p.db.WithContext(ctx).First(&slot, "id = ?", slotID).Error; err != nil {
		return nil, err
	}
	if slot.Status != model.SandboxWarmSlotStatusWarming {
		return &WarmSlotCheckResult{}, nil
	}
	status, err := p.provider.GetStatus(ctx, slot.ExternalID)
	if err != nil {
		return nil, err
	}
	switch status {
	case StatusRunning:
	case StatusCreating, StatusStarting:
		return &WarmSlotCheckResult{Pending: true}, nil
	case StatusStopped, StatusArchived, StatusArchiving, StatusError:
		msg := fmt.Sprintf("provider status %s", status)
		if err := p.MarkError(ctx, slot.ID, msg); err != nil {
			return nil, err
		}
		return &WarmSlotCheckResult{}, fmt.Errorf("warm slot %s entered %s", slot.ExternalID, status)
	default:
		return &WarmSlotCheckResult{Pending: true}, nil
	}
	if err := checkWarmSlotHealth(ctx, slot.EndpointURL); err != nil {
		return &WarmSlotCheckResult{Pending: true}, nil
	}
	if err := p.db.WithContext(ctx).Model(&slot).Updates(map[string]any{
		"status":        model.SandboxWarmSlotStatusWarm,
		"error_message": nil,
	}).Error; err != nil {
		return nil, err
	}
	logging.FromContext(ctx).InfoContext(ctx, "sandbox warm slot ready",
		"provider", p.provider.ID(), "mode", slot.Mode, "external_id", slot.ExternalID,
		"endpoint_url", slot.EndpointURL)
	return &WarmSlotCheckResult{Ready: true}, nil
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

func checkWarmSlotHealth(ctx context.Context, endpoint string) error {
	client := &http.Client{Timeout: 5 * time.Second}
	url := strings.TrimRight(endpoint, "/") + "/healthz"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("healthz returned %s", resp.Status)
	}
	return nil
}

func containsUUID(items []uuid.UUID, target uuid.UUID) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}
