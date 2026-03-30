package auth

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
)

// LoadRSAPrivateKey loads an RSA private key from a base64-encoded PEM string.
func LoadRSAPrivateKey(pemBase64 string) (*rsa.PrivateKey, error) {
	if pemBase64 == "" {
		return nil, fmt.Errorf("AUTH_RSA_PRIVATE_KEY is required")
	}

	pemBytes, err := base64.StdEncoding.DecodeString(pemBase64)
	if err != nil {
		return nil, fmt.Errorf("decoding base64 RSA key: %w", err)
	}

	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM block from RSA key")
	}

	// Try PKCS8 first, then PKCS1.
	if key, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		rsaKey, ok := key.(*rsa.PrivateKey)
		if !ok {
			return nil, fmt.Errorf("PKCS8 key is not RSA")
		}
		return rsaKey, nil
	}

	key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parsing RSA private key: %w", err)
	}
	return key, nil
}
