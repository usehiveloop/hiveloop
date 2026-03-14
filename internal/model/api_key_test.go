package model_test

import (
	"strings"
	"testing"

	"github.com/llmvault/llmvault/internal/model"
)

func TestGenerateAPIKey_Format(t *testing.T) {
	plaintext, hash, prefix, err := model.GenerateAPIKey()
	if err != nil {
		t.Fatalf("GenerateAPIKey() error: %v", err)
	}

	// Plaintext: "llmv_sk_" + 64 hex chars = 72 chars
	if len(plaintext) != 72 {
		t.Fatalf("expected plaintext length 72, got %d", len(plaintext))
	}
	if !strings.HasPrefix(plaintext, "llmv_sk_") {
		t.Fatalf("expected plaintext prefix 'llmv_sk_', got %q", plaintext[:8])
	}

	// Hash: SHA-256 hex = 64 chars
	if len(hash) != 64 {
		t.Fatalf("expected hash length 64, got %d", len(hash))
	}

	// Prefix: first 16 chars of plaintext
	if len(prefix) != 16 {
		t.Fatalf("expected prefix length 16, got %d", len(prefix))
	}
	if prefix != plaintext[:16] {
		t.Fatalf("expected prefix %q, got %q", plaintext[:16], prefix)
	}
}

func TestGenerateAPIKey_Unique(t *testing.T) {
	seen := make(map[string]bool)
	for range 100 {
		plaintext, hash, _, err := model.GenerateAPIKey()
		if err != nil {
			t.Fatalf("GenerateAPIKey() error: %v", err)
		}
		if seen[plaintext] {
			t.Fatal("duplicate plaintext generated")
		}
		if seen[hash] {
			t.Fatal("duplicate hash generated")
		}
		seen[plaintext] = true
		seen[hash] = true
	}
}

func TestHashAPIKey_Deterministic(t *testing.T) {
	key := "llmv_sk_abcdef1234567890abcdef1234567890abcdef1234567890abcdef12345678"
	h1 := model.HashAPIKey(key)
	h2 := model.HashAPIKey(key)

	if h1 != h2 {
		t.Fatalf("HashAPIKey not deterministic: %q != %q", h1, h2)
	}
	if len(h1) != 64 {
		t.Fatalf("expected hash length 64, got %d", len(h1))
	}
}

func TestHashAPIKey_MatchesGenerate(t *testing.T) {
	plaintext, expectedHash, _, err := model.GenerateAPIKey()
	if err != nil {
		t.Fatalf("GenerateAPIKey() error: %v", err)
	}

	gotHash := model.HashAPIKey(plaintext)
	if gotHash != expectedHash {
		t.Fatalf("HashAPIKey does not match GenerateAPIKey hash: %q != %q", gotHash, expectedHash)
	}
}

func TestHashAPIKey_DifferentKeys(t *testing.T) {
	h1 := model.HashAPIKey("llmv_sk_aaa")
	h2 := model.HashAPIKey("llmv_sk_bbb")

	if h1 == h2 {
		t.Fatal("different keys should produce different hashes")
	}
}

func TestValidAPIKeyScopes(t *testing.T) {
	valid := []string{"connect", "credentials", "tokens", "all"}
	for _, s := range valid {
		if !model.ValidAPIKeyScopes[s] {
			t.Fatalf("expected scope %q to be valid", s)
		}
	}

	invalid := []string{"admin", "read", "write", ""}
	for _, s := range invalid {
		if model.ValidAPIKeyScopes[s] {
			t.Fatalf("expected scope %q to be invalid", s)
		}
	}
}
