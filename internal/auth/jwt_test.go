package auth

import (
	"crypto/rand"
	"crypto/rsa"
	"testing"
	"time"
)

func generateTestKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generating RSA key: %v", err)
	}
	return key
}

func TestAccessToken_RoundTrip(t *testing.T) {
	key := generateTestKey(t)

	tokenStr, err := IssueAccessToken(key, "hiveloop", "https://api.usehiveloop.com",
		"user-123", "org-456", "admin", 15*time.Minute)
	if err != nil {
		t.Fatalf("IssueAccessToken: %v", err)
	}

	claims, err := ValidateAccessToken(&key.PublicKey, "hiveloop", "https://api.usehiveloop.com", tokenStr)
	if err != nil {
		t.Fatalf("ValidateAccessToken: %v", err)
	}

	if claims.UserID != "user-123" {
		t.Errorf("UserID = %q, want %q", claims.UserID, "user-123")
	}
	if claims.OrgID != "org-456" {
		t.Errorf("OrgID = %q, want %q", claims.OrgID, "org-456")
	}
	if claims.Role != "admin" {
		t.Errorf("Role = %q, want %q", claims.Role, "admin")
	}
	if claims.Subject != "user-123" {
		t.Errorf("Subject = %q, want %q", claims.Subject, "user-123")
	}
}

func TestAccessToken_WrongIssuer(t *testing.T) {
	key := generateTestKey(t)

	tokenStr, err := IssueAccessToken(key, "hiveloop", "https://api.usehiveloop.com",
		"user-123", "org-456", "admin", 15*time.Minute)
	if err != nil {
		t.Fatalf("IssueAccessToken: %v", err)
	}

	_, err = ValidateAccessToken(&key.PublicKey, "wrong-issuer", "https://api.usehiveloop.com", tokenStr)
	if err == nil {
		t.Fatal("expected error for wrong issuer")
	}
}

func TestAccessToken_WrongAudience(t *testing.T) {
	key := generateTestKey(t)

	tokenStr, err := IssueAccessToken(key, "hiveloop", "https://api.usehiveloop.com",
		"user-123", "org-456", "admin", 15*time.Minute)
	if err != nil {
		t.Fatalf("IssueAccessToken: %v", err)
	}

	_, err = ValidateAccessToken(&key.PublicKey, "hiveloop", "https://wrong.audience", tokenStr)
	if err == nil {
		t.Fatal("expected error for wrong audience")
	}
}

func TestAccessToken_Expired(t *testing.T) {
	key := generateTestKey(t)

	tokenStr, err := IssueAccessToken(key, "hiveloop", "https://api.usehiveloop.com",
		"user-123", "org-456", "admin", -1*time.Minute)
	if err != nil {
		t.Fatalf("IssueAccessToken: %v", err)
	}

	_, err = ValidateAccessToken(&key.PublicKey, "hiveloop", "https://api.usehiveloop.com", tokenStr)
	if err == nil {
		t.Fatal("expected error for expired token")
	}
}

func TestAccessToken_WrongKey(t *testing.T) {
	key1 := generateTestKey(t)
	key2 := generateTestKey(t)

	tokenStr, err := IssueAccessToken(key1, "hiveloop", "https://api.usehiveloop.com",
		"user-123", "org-456", "admin", 15*time.Minute)
	if err != nil {
		t.Fatalf("IssueAccessToken: %v", err)
	}

	_, err = ValidateAccessToken(&key2.PublicKey, "hiveloop", "https://api.usehiveloop.com", tokenStr)
	if err == nil {
		t.Fatal("expected error for wrong signing key")
	}
}

func TestRefreshToken_RoundTrip(t *testing.T) {
	hmacKey := []byte("test-secret-key-for-refresh-tokens")

	tokenStr, err := IssueRefreshToken(hmacKey, "user-123", 30*24*time.Hour)
	if err != nil {
		t.Fatalf("IssueRefreshToken: %v", err)
	}

	userID, jti, err := ValidateRefreshToken(hmacKey, tokenStr)
	if err != nil {
		t.Fatalf("ValidateRefreshToken: %v", err)
	}

	if userID != "user-123" {
		t.Errorf("UserID = %q, want %q", userID, "user-123")
	}
	if jti == "" {
		t.Error("JTI should not be empty")
	}
}

func TestRefreshToken_WrongKey(t *testing.T) {
	tokenStr, err := IssueRefreshToken([]byte("key-1"), "user-123", 30*24*time.Hour)
	if err != nil {
		t.Fatalf("IssueRefreshToken: %v", err)
	}

	_, _, err = ValidateRefreshToken([]byte("key-2"), tokenStr)
	if err == nil {
		t.Fatal("expected error for wrong HMAC key")
	}
}
