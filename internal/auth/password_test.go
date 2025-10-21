package auth

import "testing"

func TestHashAndVerify(t *testing.T) {
	hash, err := HashPassword("super-secret")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}

	if !VerifyPassword(hash, "super-secret") {
		t.Fatalf("expected password to verify")
	}

	if VerifyPassword(hash, "bad-password") {
		t.Fatalf("unexpected verification success")
	}
}
