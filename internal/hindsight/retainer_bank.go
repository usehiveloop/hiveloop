package hindsight

import (
	"context"
	"fmt"
	"strings"

	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
)

// ensureOrgBankConfigured creates and configures the org-scoped Hindsight bank
// if it doesn't exist yet, or re-applies config if it has changed.
// Per-agent observation scoping is set on each RetainItem in retainConversation.
func (r *Retainer) ensureOrgBankConfigured(ctx context.Context, agent *model.Employee) error {
	bankID := OrgBankID(*agent.OrgID)
	memCfg := DefaultMemoryConfig()

	configHash := fmt.Sprintf("%x", memCfg.Hash()+"|org-"+agent.OrgID.String())

	var bank model.HindsightBank
	err := r.db.Where("bank_id = ?", bankID).First(&bank).Error

	if err == gorm.ErrRecordNotFound {
		if err := r.client.ConfigureBank(ctx, bankID, memCfg.ToBankConfigUpdate()); err != nil {
			return fmt.Errorf("configuring org bank: %w", err)
		}

		_ = r.client.CreateMentalModel(ctx, bankID, &CreateMentalModelRequest{
			Name:        "Organization Memory",
			SourceQuery: "Summarize everything known across all agents in this organization.",
			Trigger:     &MentalModelTrigger{RefreshAfterConsolidation: true},
		})

		bank = model.HindsightBank{
			BankID:     bankID,
			ConfigHash: configHash,
		}
		if err := r.db.Create(&bank).Error; err != nil {
			if !isDuplicateKey(err) {
				return fmt.Errorf("recording org bank: %w", err)
			}
		}
		logging.FromContext(ctx).InfoContext(ctx, "hindsight retainer: org bank created",
			"bank_id", bankID, "org_id", agent.OrgID)
		return nil
	}

	if err != nil {
		return fmt.Errorf("checking org bank: %w", err)
	}

	if bank.ConfigHash != configHash {
		if err := r.client.ConfigureBank(ctx, bankID, memCfg.ToBankConfigUpdate()); err != nil {
			return fmt.Errorf("updating org bank config: %w", err)
		}
		r.db.Model(&bank).Update("config_hash", configHash)
	}

	return nil
}

// isDuplicateKey checks if an error is a Postgres unique constraint violation.
func isDuplicateKey(err error) bool {
	return err != nil && strings.Contains(err.Error(), "duplicate key")
}
