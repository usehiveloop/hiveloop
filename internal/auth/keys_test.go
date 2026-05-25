package auth

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"testing"
)

func TestLoadRSAPrivateKey_FromBase64(t *testing.T) {
	generated, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate rsa key: %v", err)
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(generated),
	})

	b64 := base64.StdEncoding.EncodeToString(pemBytes)
	key, err := LoadRSAPrivateKey(b64)
	if err != nil {
		t.Fatalf("LoadRSAPrivateKey: %v", err)
	}
	if key.N.BitLen() < 2048 {
		t.Fatalf("expected >= 2048-bit key, got %d", key.N.BitLen())
	}
}

func TestLoadRSAPrivateKey_Empty(t *testing.T) {
	_, err := LoadRSAPrivateKey("")
	if err == nil {
		t.Fatal("expected error when key is empty")
	}
}

func TestLoadRSAPrivateKey_InvalidBase64(t *testing.T) {
	_, err := LoadRSAPrivateKey("not-valid-base64!!!")
	if err == nil {
		t.Fatal("expected error for invalid base64")
	}
}
