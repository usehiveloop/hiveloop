package credentials

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/model"
)

// Resolve returns the Credential an agent's next LLM call should use.
//
//   - When agent.CredentialID is set (BYOK), the referenced credential is
//     loaded directly.
//   - When it's nil (platform keys), the agent's ProviderGroup drives a
//     lookup via Picker.
//
// This is the only place in the codebase that knows agents can omit
// credential_id to opt into platform keys. Call sites (currently
// sandbox.Pusher at initial push and token rotation) used to do a direct
// DB load; they now call Resolve and get the same return shape.
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

	if agent.ProviderGroup == "" {
		return nil, fmt.Errorf("agent %s has no credential_id and no provider_group; cannot resolve a system credential", agent.ID)
	}
	if picker == nil {
		return nil, fmt.Errorf("agent %s opted into platform keys but no Picker is configured", agent.ID)
	}
	return picker.Pick(ctx, agent.ProviderGroup)
}
