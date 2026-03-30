package auth

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadRSAPrivateKey_FromBase64(t *testing.T) {
	keyPath := filepath.Join("..", "..", "certs", "auth.key")
	if _, err := os.Stat(keyPath); os.IsNotExist(err) {
		t.Skip("certs/auth.key not found — run 'make generate-auth-keys'")
	}

	pemBytes, err := os.ReadFile(keyPath)
	if err != nil {
		t.Fatalf("reading key file: %v", err)
	}

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
