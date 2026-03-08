package crypto

import (
	"bytes"
	"testing"
)

func TestGenerateDEK(t *testing.T) {
	dek, err := GenerateDEK()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(dek) != 32 {
		t.Fatalf("expected 32 bytes, got %d", len(dek))
	}

	dek2, err := GenerateDEK()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if bytes.Equal(dek, dek2) {
		t.Fatal("two generated DEKs should not be equal")
	}
}

func TestEncryptDecryptRoundtrip(t *testing.T) {
	dek, err := GenerateDEK()
	if err != nil {
		t.Fatalf("generating DEK: %v", err)
	}

	plaintext := []byte("sk-ant-api03-secret-key-here")
	ciphertext, err := EncryptCredential(plaintext, dek)
	if err != nil {
		t.Fatalf("encrypting: %v", err)
	}

	if bytes.Equal(plaintext, ciphertext) {
		t.Fatal("ciphertext should not equal plaintext")
	}

	decrypted, err := DecryptCredential(ciphertext, dek)
	if err != nil {
		t.Fatalf("decrypting: %v", err)
	}

	if !bytes.Equal(plaintext, decrypted) {
		t.Fatalf("expected %q, got %q", plaintext, decrypted)
	}
}

func TestDecryptWithWrongDEK(t *testing.T) {
	dek1, _ := GenerateDEK()
	dek2, _ := GenerateDEK()

	plaintext := []byte("secret-api-key")
	ciphertext, err := EncryptCredential(plaintext, dek1)
	if err != nil {
		t.Fatalf("encrypting: %v", err)
	}

	_, err = DecryptCredential(ciphertext, dek2)
	if err == nil {
		t.Fatal("expected error decrypting with wrong DEK")
	}
}

func TestDecryptTruncatedCiphertext(t *testing.T) {
	dek, _ := GenerateDEK()

	_, err := DecryptCredential([]byte("short"), dek)
	if err == nil {
		t.Fatal("expected error for truncated ciphertext")
	}
}

func TestDecryptFlippedBits(t *testing.T) {
	dek, _ := GenerateDEK()

	plaintext := []byte("secret-api-key")
	ciphertext, _ := EncryptCredential(plaintext, dek)

	// Flip a bit in the ciphertext body (after nonce)
	corrupted := make([]byte, len(ciphertext))
	copy(corrupted, ciphertext)
	corrupted[len(corrupted)-1] ^= 0xFF

	_, err := DecryptCredential(corrupted, dek)
	if err == nil {
		t.Fatal("expected error for corrupted ciphertext")
	}
}

func TestNonceUniqueness(t *testing.T) {
	dek, _ := GenerateDEK()
	plaintext := []byte("same-plaintext")

	ct1, _ := EncryptCredential(plaintext, dek)
	ct2, _ := EncryptCredential(plaintext, dek)

	if bytes.Equal(ct1, ct2) {
		t.Fatal("encrypting same plaintext twice should produce different ciphertexts")
	}
}

func TestDecryptEmptyCiphertext(t *testing.T) {
	dek, _ := GenerateDEK()

	_, err := DecryptCredential(nil, dek)
	if err == nil {
		t.Fatal("expected error for nil ciphertext")
	}

	_, err = DecryptCredential([]byte{}, dek)
	if err == nil {
		t.Fatal("expected error for empty ciphertext")
	}
}
