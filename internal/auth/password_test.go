package auth

import "testing"

func TestPassword_RoundTrip(t *testing.T) {
	hash, err := HashPassword("my-secure-password")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if !CheckPassword(hash, "my-secure-password") {
		t.Fatal("CheckPassword should return true for correct password")
	}
}

func TestPassword_WrongPassword(t *testing.T) {
	hash, err := HashPassword("correct-password")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if CheckPassword(hash, "wrong-password") {
		t.Fatal("CheckPassword should return false for wrong password")
	}
}

func TestPassword_DifferentHashesEachTime(t *testing.T) {
	h1, _ := HashPassword("same-password")
	h2, _ := HashPassword("same-password")
	if h1 == h2 {
		t.Fatal("bcrypt should produce different hashes for the same password due to random salt")
	}
}
