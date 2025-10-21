package auth

import (
	"strings"
	"testing"
	"time"
)

func TestGenerateAndParseToken(t *testing.T) {
	claims := Claims{
		UserID:   42,
		Email:    "user@example.com",
		IssuedAt: time.Now().Unix(),
		Expires:  time.Now().Add(1 * time.Hour).Unix(),
	}

	token, err := GenerateToken("secret", claims)
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}

	parsed, err := ParseToken("secret", token)
	if err != nil {
		t.Fatalf("parse token: %v", err)
	}

	if parsed.UserID != claims.UserID || parsed.Email != claims.Email {
		t.Fatalf("unexpected claims: %#v", parsed)
	}

	if _, err := ParseToken("secret", strings.Replace(token, "a", "b", 1)); err == nil {
		t.Fatalf("expected signature mismatch")
	}
}
