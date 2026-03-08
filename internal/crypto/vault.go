package crypto

import (
	"encoding/base64"
	"fmt"

	vault "github.com/hashicorp/vault/api"
)

// VaultTransit wraps Vault's Transit secrets engine for key wrapping operations.
type VaultTransit struct {
	client  *vault.Client
	keyName string
}

// NewVaultTransit creates a new Vault Transit client.
func NewVaultTransit(addr, token, keyName string) (*VaultTransit, error) {
	config := vault.DefaultConfig()
	config.Address = addr

	client, err := vault.NewClient(config)
	if err != nil {
		return nil, fmt.Errorf("creating vault client: %w", err)
	}

	client.SetToken(token)

	return &VaultTransit{
		client:  client,
		keyName: keyName,
	}, nil
}

// Wrap encrypts plaintext using Vault Transit (wraps a DEK).
func (v *VaultTransit) Wrap(plaintext []byte) ([]byte, error) {
	secret, err := v.client.Logical().Write(
		fmt.Sprintf("transit/encrypt/%s", v.keyName),
		map[string]any{
			"plaintext": base64.StdEncoding.EncodeToString(plaintext),
		},
	)
	if err != nil {
		return nil, fmt.Errorf("vault transit encrypt: %w", err)
	}

	ciphertext, ok := secret.Data["ciphertext"].(string)
	if !ok {
		return nil, fmt.Errorf("vault transit: unexpected ciphertext type")
	}

	return []byte(ciphertext), nil
}

// Unwrap decrypts ciphertext using Vault Transit (unwraps a DEK).
func (v *VaultTransit) Unwrap(ciphertext []byte) ([]byte, error) {
	secret, err := v.client.Logical().Write(
		fmt.Sprintf("transit/decrypt/%s", v.keyName),
		map[string]any{
			"ciphertext": string(ciphertext),
		},
	)
	if err != nil {
		return nil, fmt.Errorf("vault transit decrypt: %w", err)
	}

	plaintextB64, ok := secret.Data["plaintext"].(string)
	if !ok {
		return nil, fmt.Errorf("vault transit: unexpected plaintext type")
	}

	plaintext, err := base64.StdEncoding.DecodeString(plaintextB64)
	if err != nil {
		return nil, fmt.Errorf("vault transit: decoding base64: %w", err)
	}

	return plaintext, nil
}
