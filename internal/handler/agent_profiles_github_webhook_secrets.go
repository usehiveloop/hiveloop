package handler

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"

	hivecrypto "github.com/usehiveloop/hiveloop/internal/crypto"
	"github.com/usehiveloop/hiveloop/internal/model"
)

func (h *AgentProfileHandler) SetBridgeHost(host string) {
	h.bridgeHost = strings.TrimRight(strings.TrimSpace(host), "/")
}

func (h *AgentProfileHandler) githubEmployeeWebhookURL(agentID uuid.UUID) (string, error) {
	host := strings.TrimRight(strings.TrimSpace(h.bridgeHost), "/")
	if host == "" {
		return "", fmt.Errorf("bridge host is not configured")
	}
	if strings.HasPrefix(host, "http://") || strings.HasPrefix(host, "https://") {
		return fmt.Sprintf("%s/internal/webhooks/github/employees/%s", host, agentID), nil
	}
	return fmt.Sprintf("https://%s/internal/webhooks/github/employees/%s", host, agentID), nil
}

func (h *AgentProfileHandler) ensureGitHubWebhookSecret(ctx context.Context, profile *model.AgentProfile) (string, error) {
	secrets, err := decryptGitHubProfileSecrets(ctx, h.kms, *profile)
	if err != nil {
		return "", err
	}
	if secret := stringFromAny(secrets[githubWebhookSecretKey]); secret != "" {
		return secret, nil
	}
	secret, err := generateGitHubWebhookSecret()
	if err != nil {
		return "", err
	}
	secrets[githubWebhookSecretKey] = secret
	if err := encryptGitHubProfileSecrets(ctx, h.kms, profile, secrets); err != nil {
		return "", err
	}
	if err := h.db.Model(profile).Updates(map[string]any{
		"encrypted_secrets": profile.EncryptedSecrets,
		"wrapped_dek":       profile.WrappedDEK,
	}).Error; err != nil {
		return "", fmt.Errorf("save github webhook secret: %w", err)
	}
	return secret, nil
}

func decryptGitHubProfileSecrets(ctx context.Context, kms *hivecrypto.KeyWrapper, profile model.AgentProfile) (model.JSON, error) {
	if len(profile.EncryptedSecrets) == 0 {
		return model.JSON{}, nil
	}
	if kms == nil {
		return nil, fmt.Errorf("kms is not configured")
	}
	if len(profile.WrappedDEK) == 0 {
		return nil, fmt.Errorf("github profile secret key is missing")
	}
	dek, err := kms.Unwrap(ctx, profile.WrappedDEK)
	if err != nil {
		return nil, fmt.Errorf("unwrap github profile secret key: %w", err)
	}
	defer zeroBytes(dek)

	plaintext, err := hivecrypto.DecryptCredential(profile.EncryptedSecrets, dek)
	if err != nil {
		return nil, fmt.Errorf("decrypt github profile secrets: %w", err)
	}
	defer zeroBytes(plaintext)

	secrets := model.JSON{}
	if err := json.Unmarshal(plaintext, &secrets); err != nil {
		return nil, fmt.Errorf("decode github profile secrets: %w", err)
	}
	return secrets, nil
}

func encryptGitHubProfileSecrets(ctx context.Context, kms *hivecrypto.KeyWrapper, profile *model.AgentProfile, secrets model.JSON) error {
	if kms == nil {
		return fmt.Errorf("kms is not configured")
	}
	plaintext, err := json.Marshal(secrets)
	if err != nil {
		return fmt.Errorf("encode github profile secrets: %w", err)
	}
	defer zeroBytes(plaintext)

	dek, err := hivecrypto.GenerateDEK()
	if err != nil {
		return fmt.Errorf("generate github profile secret key: %w", err)
	}
	defer zeroBytes(dek)

	encrypted, err := hivecrypto.EncryptCredential(plaintext, dek)
	if err != nil {
		return fmt.Errorf("encrypt github profile secrets: %w", err)
	}
	wrappedDEK, err := kms.Wrap(ctx, dek)
	if err != nil {
		return fmt.Errorf("wrap github profile secret key: %w", err)
	}
	profile.EncryptedSecrets = encrypted
	profile.WrappedDEK = wrappedDEK
	return nil
}

func generateGitHubWebhookSecret() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate github webhook secret: %w", err)
	}
	return hex.EncodeToString(b), nil
}

func zeroBytes(b []byte) {
	for i := range b {
		b[i] = 0
	}
}
