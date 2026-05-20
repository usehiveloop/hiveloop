package credentials

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

func Resolve(
	ctx context.Context,
	db *gorm.DB,
	picker Picker,
	agent *model.Agent,
) (*model.Credential, error) {
	if agent == nil {
		return nil, fmt.Errorf("credentials: Resolve called with nil agent")
	}

	if agent.CredentialID != nil {
		var cred model.Credential
		if err := db.WithContext(ctx).
			Where("id = ?", *agent.CredentialID).
			First(&cred).Error; err != nil {
			return nil, fmt.Errorf("loading agent credential %s: %w", *agent.CredentialID, err)
		}
		return &cred, nil
	}

	if agent.Model == "" {
		return nil, fmt.Errorf("agent %s has no credential_id and no model; cannot resolve a system credential", agent.ID)
	}
	if picker == nil {
		return nil, fmt.Errorf("agent %s opted into platform keys but no Picker is configured", agent.ID)
	}
	return picker.PickByModel(ctx, agent.Model)
}
