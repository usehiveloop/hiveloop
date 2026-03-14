package token

import (
	"testing"
	"time"
)

var testKey = []byte("test-signing-key-at-least-32-bytes!")

func TestMintAndValidateRoundtrip(t *testing.T) {
	tokenStr, jti, err := Mint(testKey, "org-123", "cred-456", time.Hour)
	if err != nil {
		t.Fatalf("minting: %v", err)
	}
	if tokenStr == "" {
		t.Fatal("token string should not be empty")
	}
	if jti == "" {
		t.Fatal("jti should not be empty")
	}

	claims, err := Validate(testKey, tokenStr)
	if err != nil {
		t.Fatalf("validating: %v", err)
	}

	if claims.OrgID != "org-123" {
		t.Errorf("expected org_id 'org-123', got %q", claims.OrgID)
	}
	if claims.CredentialID != "cred-456" {
		t.Errorf("expected cred_id 'cred-456', got %q", claims.CredentialID)
	}
	if claims.ID != jti {
		t.Errorf("expected jti %q, got %q", jti, claims.ID)
	}
}

func TestValidateExpiredToken(t *testing.T) {
	tokenStr, _, err := Mint(testKey, "org-123", "cred-456", -time.Hour)
	if err != nil {
		t.Fatalf("minting: %v", err)
	}

	_, err = Validate(testKey, tokenStr)
	if err == nil {
		t.Fatal("expected error for expired token")
	}
}

func TestValidateWrongSigningKey(t *testing.T) {
	tokenStr, _, err := Mint(testKey, "org-123", "cred-456", time.Hour)
	if err != nil {
		t.Fatalf("minting: %v", err)
	}

	wrongKey := []byte("wrong-signing-key-at-least-32-bytes!")
	_, err = Validate(wrongKey, tokenStr)
	if err == nil {
		t.Fatal("expected error for wrong signing key")
	}
}

func TestValidateTamperedToken(t *testing.T) {
	tokenStr, _, err := Mint(testKey, "org-123", "cred-456", time.Hour)
	if err != nil {
		t.Fatalf("minting: %v", err)
	}

	// Flip a character in the middle of the signature to avoid base64 padding bit ambiguity
	b := []byte(tokenStr)
	mid := len(b) - len(b)/4
	if b[mid] == 'A' {
		b[mid] = 'B'
	} else {
		b[mid] = 'A'
	}
	_, err = Validate(testKey, string(b))
	if err == nil {
		t.Fatal("expected error for tampered token")
	}
}

func TestValidateGarbageToken(t *testing.T) {
	_, err := Validate(testKey, "not-a-jwt")
	if err == nil {
		t.Fatal("expected error for garbage token")
	}
}

func TestValidateEmptyToken(t *testing.T) {
	_, err := Validate(testKey, "")
	if err == nil {
		t.Fatal("expected error for empty token")
	}
}

func TestMintUniqueJTI(t *testing.T) {
	_, jti1, _ := Mint(testKey, "org-123", "cred-456", time.Hour)
	_, jti2, _ := Mint(testKey, "org-123", "cred-456", time.Hour)

	if jti1 == jti2 {
		t.Fatal("two minted tokens should have different JTIs")
	}
}
