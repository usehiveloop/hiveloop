package crypto

import (
	"bytes"
	"os"
	"testing"
)

func getVaultClient(t *testing.T) *VaultTransit {
	t.Helper()

	addr := os.Getenv("VAULT_ADDR")
	if addr == "" {
		addr = "http://localhost:8200"
	}
	token := os.Getenv("VAULT_TOKEN")
	if token == "" {
		token = "dev-token"
	}

	vt, err := NewVaultTransit(addr, token, "proxy-bridge-master")
	if err != nil {
		t.Fatalf("creating vault transit client: %v", err)
	}
	return vt
}

func TestVaultTransit_WrapUnwrapRoundtrip(t *testing.T) {
	vt := getVaultClient(t)

	plaintext := []byte("this-is-a-32-byte-data-encrypt-k")
	wrapped, err := vt.Wrap(plaintext)
	if err != nil {
		t.Fatalf("wrapping: %v", err)
	}

	if bytes.Equal(plaintext, wrapped) {
		t.Fatal("wrapped should not equal plaintext")
	}

	unwrapped, err := vt.Unwrap(wrapped)
	if err != nil {
		t.Fatalf("unwrapping: %v", err)
	}

	if !bytes.Equal(plaintext, unwrapped) {
		t.Fatalf("expected %q, got %q", plaintext, unwrapped)
	}
}

func TestVaultTransit_UnwrapCorrupted(t *testing.T) {
	vt := getVaultClient(t)

	_, err := vt.Unwrap([]byte("vault:v1:corrupted-ciphertext"))
	if err == nil {
		t.Fatal("expected error unwrapping corrupted ciphertext")
	}
}
