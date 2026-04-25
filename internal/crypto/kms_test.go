package crypto

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"testing"
)

func testWrapper(t *testing.T) *KeyWrapper {
	t.Helper()
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	b64 := base64.StdEncoding.EncodeToString(key)
	w, err := NewAEADWrapper(context.Background(), b64, "test-key")
	if err != nil {
		t.Fatal(err)
	}
	return w
}

func TestKeyWrapper_WrapUnwrapRoundtrip(t *testing.T) {
	w := testWrapper(t)
	ctx := context.Background()

	plaintext := []byte("this-is-a-32-byte-data-encrypt-k")
	wrapped, err := w.Wrap(ctx, plaintext)
	if err != nil {
		t.Fatalf("wrapping: %v", err)
	}

	if bytes.Equal(plaintext, wrapped) {
		t.Fatal("wrapped should not equal plaintext")
	}

	unwrapped, err := w.Unwrap(ctx, wrapped)
	if err != nil {
		t.Fatalf("unwrapping: %v", err)
	}

	if !bytes.Equal(plaintext, unwrapped) {
		t.Fatalf("expected %q, got %q", plaintext, unwrapped)
	}
}

func TestKeyWrapper_UnwrapCorrupted(t *testing.T) {
	w := testWrapper(t)
	ctx := context.Background()

	_, err := w.Unwrap(ctx, []byte("not-protobuf"))
	if err == nil {
		t.Fatal("expected error unwrapping corrupted ciphertext")
	}
}

func TestKeyWrapper_WrongKey(t *testing.T) {
	w1 := testWrapper(t)
	w2 := testWrapper(t)
	ctx := context.Background()

	wrapped, err := w1.Wrap(ctx, []byte("secret"))
	if err != nil {
		t.Fatal(err)
	}

	_, err = w2.Unwrap(ctx, wrapped)
	if err == nil {
		t.Fatal("expected error unwrapping with wrong key")
	}
}

func TestKeyWrapper_UniqueNonce(t *testing.T) {
	w := testWrapper(t)
	ctx := context.Background()

	c1, _ := w.Wrap(ctx, []byte("same"))
	c2, _ := w.Wrap(ctx, []byte("same"))
	if bytes.Equal(c1, c2) {
		t.Fatal("wrapping same plaintext twice should produce different output")
	}
}

func TestNewAEADWrapper_Base64(t *testing.T) {
	key := make([]byte, 32)
	rand.Read(key)
	b64 := base64.StdEncoding.EncodeToString(key)

	w, err := NewAEADWrapper(context.Background(), b64, "b64-key")
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	wrapped, _ := w.Wrap(ctx, []byte("test"))
	unwrapped, err := w.Unwrap(ctx, wrapped)
	if err != nil {
		t.Fatal(err)
	}
	if string(unwrapped) != "test" {
		t.Fatalf("expected 'test', got %q", unwrapped)
	}
}

func TestNewAEADWrapper_BadKeySize(t *testing.T) {
	b64 := base64.StdEncoding.EncodeToString([]byte("too-short"))
	_, err := NewAEADWrapper(context.Background(), b64, "bad")
	if err == nil {
		t.Fatal("expected error for wrong key size")
	}
}
