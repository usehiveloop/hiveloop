package paystack

import (
	"crypto/hmac"
	"crypto/sha512"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestVerifyWebhook_ValidSignature(t *testing.T) {
	secret := "sk_test_shh"
	body := []byte(`{"event":"charge.success","data":{}}`)

	mac := hmac.New(sha512.New, []byte(secret))
	mac.Write(body)
	sig := hex.EncodeToString(mac.Sum(nil))

	p := New(Config{SecretKey: secret})
	r := httptest.NewRequest(http.MethodPost, "/internal/webhooks/billing/paystack", nil)
	r.Header.Set(signatureHeader, sig)

	if err := p.VerifyWebhook(r, body); err != nil {
		t.Fatalf("expected valid signature to pass, got %v", err)
	}
}

func TestVerifyWebhook_MissingHeader(t *testing.T) {
	p := New(Config{SecretKey: "sk"})
	r := httptest.NewRequest(http.MethodPost, "/", nil)
	err := p.VerifyWebhook(r, []byte("body"))
	if !errors.Is(err, errInvalidSignature) {
		t.Fatalf("want errInvalidSignature, got %v", err)
	}
}

func TestVerifyWebhook_WrongSignature(t *testing.T) {
	p := New(Config{SecretKey: "sk"})
	r := httptest.NewRequest(http.MethodPost, "/", nil)
	r.Header.Set(signatureHeader, "deadbeef")
	err := p.VerifyWebhook(r, []byte("body"))
	if !errors.Is(err, errInvalidSignature) {
		t.Fatalf("want errInvalidSignature, got %v", err)
	}
}

func TestVerifyWebhook_BodyTamperedAfterSigning(t *testing.T) {
	secret := "sk_test"
	original := []byte(`{"event":"charge.success"}`)
	mac := hmac.New(sha512.New, []byte(secret))
	mac.Write(original)
	sig := hex.EncodeToString(mac.Sum(nil))

	p := New(Config{SecretKey: secret})
	r := httptest.NewRequest(http.MethodPost, "/", nil)
	r.Header.Set(signatureHeader, sig)

	// Replay with a modified body — must be rejected.
	tampered := []byte(`{"event":"charge.success","extra":"evil"}`)
	err := p.VerifyWebhook(r, tampered)
	if !errors.Is(err, errInvalidSignature) {
		t.Fatalf("tampered body should fail verification, got %v", err)
	}
}
