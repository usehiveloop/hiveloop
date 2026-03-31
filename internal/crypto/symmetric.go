package crypto

import (
	"encoding/base64"
	"fmt"
)

// SymmetricKey holds a 256-bit key for simple AES-256-GCM encryption.
// Used for internal secrets (e.g. Bridge API keys) that don't need
// envelope encryption with KMS.
type SymmetricKey struct {
	key []byte
}

// NewSymmetricKey creates a SymmetricKey from a base64-encoded 32-byte key.
func NewSymmetricKey(base64Key string) (*SymmetricKey, error) {
	key, err := base64.StdEncoding.DecodeString(base64Key)
	if err != nil {
		return nil, fmt.Errorf("decoding symmetric key: %w", err)
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("symmetric key must be 32 bytes, got %d", len(key))
	}
	return &SymmetricKey{key: key}, nil
}

// Encrypt encrypts plaintext using AES-256-GCM.
func (k *SymmetricKey) Encrypt(plaintext []byte) ([]byte, error) {
	return EncryptCredential(plaintext, k.key)
}

// Decrypt decrypts ciphertext using AES-256-GCM.
func (k *SymmetricKey) Decrypt(ciphertext []byte) ([]byte, error) {
	return DecryptCredential(ciphertext, k.key)
}

// EncryptString encrypts a string and returns the ciphertext bytes.
func (k *SymmetricKey) EncryptString(plaintext string) ([]byte, error) {
	return k.Encrypt([]byte(plaintext))
}

// DecryptString decrypts ciphertext and returns the plaintext string.
func (k *SymmetricKey) DecryptString(ciphertext []byte) (string, error) {
	b, err := k.Decrypt(ciphertext)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
