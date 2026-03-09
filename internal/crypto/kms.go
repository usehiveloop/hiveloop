package crypto

import (
	"context"
	"fmt"

	wrapping "github.com/hashicorp/go-kms-wrapping/v2"
	"github.com/hashicorp/go-kms-wrapping/wrappers/aead/v2"
	awskms "github.com/hashicorp/go-kms-wrapping/wrappers/awskms/v2"
	"google.golang.org/protobuf/proto"
)

// KeyWrapper wraps and unwraps data encryption keys using a KMS backend.
// It adapts go-kms-wrapping's Wrapper interface to a simple []byte in/out API
// so callers don't need to know about BlobInfo serialization.
type KeyWrapper struct {
	wrapper wrapping.Wrapper
}

// Wrap encrypts plaintext (typically a DEK) and returns the serialized blob
// suitable for database storage.
func (kw *KeyWrapper) Wrap(ctx context.Context, plaintext []byte) ([]byte, error) {
	blob, err := kw.wrapper.Encrypt(ctx, plaintext)
	if err != nil {
		return nil, fmt.Errorf("kms encrypt: %w", err)
	}
	data, err := proto.Marshal(blob)
	if err != nil {
		return nil, fmt.Errorf("marshal blob: %w", err)
	}
	return data, nil
}

// Unwrap decrypts a serialized blob back to plaintext.
func (kw *KeyWrapper) Unwrap(ctx context.Context, ciphertext []byte) ([]byte, error) {
	var blob wrapping.BlobInfo
	if err := proto.Unmarshal(ciphertext, &blob); err != nil {
		return nil, fmt.Errorf("unmarshal blob: %w", err)
	}
	plaintext, err := kw.wrapper.Decrypt(ctx, &blob)
	if err != nil {
		return nil, fmt.Errorf("kms decrypt: %w", err)
	}
	return plaintext, nil
}

// NewAEADWrapper creates a KeyWrapper using local AES-256-GCM encryption.
// keyBase64 is a base64-encoded 32-byte key. Suitable for dev/test and
// single-node deployments.
func NewAEADWrapper(keyBase64, keyID string) (*KeyWrapper, error) {
	w := aead.NewWrapper()
	_, err := w.SetConfig(context.Background(), wrapping.WithConfigMap(map[string]string{
		"aead_type": "aes-gcm",
		"key":       keyBase64,
		"key_id":    keyID,
	}))
	if err != nil {
		return nil, fmt.Errorf("configuring AEAD wrapper: %w", err)
	}
	return &KeyWrapper{wrapper: w}, nil
}

// NewAWSKMSWrapper creates a KeyWrapper backed by AWS KMS.
// The KMS key ID can be a key ID, key ARN, alias name, or alias ARN.
// AWS credentials are resolved from the standard chain (env vars, instance
// profile, shared credentials file, etc.).
func NewAWSKMSWrapper(kmsKeyID, region string) (*KeyWrapper, error) {
	w := awskms.NewWrapper()
	cfg := map[string]string{
		"kms_key_id": kmsKeyID,
	}
	if region != "" {
		cfg["region"] = region
	}
	_, err := w.SetConfig(context.Background(), wrapping.WithConfigMap(cfg))
	if err != nil {
		return nil, fmt.Errorf("configuring AWS KMS wrapper: %w", err)
	}
	return &KeyWrapper{wrapper: w}, nil
}
