package crypto

import (
	"encoding/base64"
	"testing"
)

func TestSymmetricKey_RoundTrip(t *testing.T) {
	// Generate a valid 32-byte key
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}
	b64Key := base64.StdEncoding.EncodeToString(key)

	sk, err := NewSymmetricKey(b64Key)
	if err != nil {
		t.Fatalf("NewSymmetricKey: %v", err)
	}

	plaintext := "bridge-api-key-secret-value-12345"
	encrypted, err := sk.EncryptString(plaintext)
	if err != nil {
		t.Fatalf("EncryptString: %v", err)
	}

	if string(encrypted) == plaintext {
		t.Fatal("encrypted should not equal plaintext")
	}

	decrypted, err := sk.DecryptString(encrypted)
	if err != nil {
		t.Fatalf("DecryptString: %v", err)
	}

	if decrypted != plaintext {
		t.Errorf("round-trip failed: got %q, want %q", decrypted, plaintext)
	}
}

func TestSymmetricKey_DifferentCiphertexts(t *testing.T) {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i + 10)
	}
	sk, _ := NewSymmetricKey(base64.StdEncoding.EncodeToString(key))

	plaintext := "same-input"
	enc1, _ := sk.EncryptString(plaintext)
	enc2, _ := sk.EncryptString(plaintext)

	// AES-GCM uses random nonce, so encryptions of the same plaintext differ
	if string(enc1) == string(enc2) {
		t.Error("two encryptions of same plaintext should produce different ciphertexts")
	}

	// Both should decrypt to the same value
	dec1, _ := sk.DecryptString(enc1)
	dec2, _ := sk.DecryptString(enc2)
	if dec1 != dec2 || dec1 != plaintext {
		t.Errorf("both should decrypt to %q, got %q and %q", plaintext, dec1, dec2)
	}
}

func TestSymmetricKey_InvalidKeyLength(t *testing.T) {
	short := base64.StdEncoding.EncodeToString([]byte("tooshort"))
	_, err := NewSymmetricKey(short)
	if err == nil {
		t.Fatal("expected error for short key")
	}
}

func TestSymmetricKey_InvalidBase64(t *testing.T) {
	_, err := NewSymmetricKey("not-valid-base64!!!")
	if err == nil {
		t.Fatal("expected error for invalid base64")
	}
}

func TestSymmetricKey_WrongKeyDecrypt(t *testing.T) {
	key1 := make([]byte, 32)
	key2 := make([]byte, 32)
	for i := range key1 {
		key1[i] = byte(i)
		key2[i] = byte(i + 1)
	}

	sk1, _ := NewSymmetricKey(base64.StdEncoding.EncodeToString(key1))
	sk2, _ := NewSymmetricKey(base64.StdEncoding.EncodeToString(key2))

	encrypted, _ := sk1.EncryptString("secret")
	_, err := sk2.DecryptString(encrypted)
	if err == nil {
		t.Fatal("decrypting with wrong key should fail")
	}
}
