package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

type Claims struct {
	UserID   int64  `json:"uid"`
	Email    string `json:"email"`
	IssuedAt int64  `json:"iat"`
	Expires  int64  `json:"exp"`
}

func GenerateToken(secret string, claims Claims) (string, error) {
	if secret == "" {
		return "", errors.New("jwt secret cannot be empty")
	}

	header := map[string]string{
		"alg": "HS256",
		"typ": "JWT",
	}

	headerJSON, err := json.Marshal(header)
	if err != nil {
		return "", fmt.Errorf("marshal header: %w", err)
	}

	payloadJSON, err := json.Marshal(claims)
	if err != nil {
		return "", fmt.Errorf("marshal claims: %w", err)
	}

	encodedHeader := base64.RawURLEncoding.EncodeToString(headerJSON)
	encodedPayload := base64.RawURLEncoding.EncodeToString(payloadJSON)
	signingInput := encodedHeader + "." + encodedPayload

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(signingInput))
	signature := mac.Sum(nil)

	encodedSignature := base64.RawURLEncoding.EncodeToString(signature)

	return signingInput + "." + encodedSignature, nil
}

func ParseToken(secret, token string) (Claims, error) {
	var claims Claims
	if secret == "" {
		return claims, errors.New("jwt secret cannot be empty")
	}

	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return claims, errors.New("invalid token format")
	}

	signingInput := strings.Join(parts[:2], ".")

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(signingInput))
	expectedSignature := mac.Sum(nil)

	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return claims, errors.New("invalid signature encoding")
	}

	if !hmac.Equal(signature, expectedSignature) {
		return claims, errors.New("signature mismatch")
	}

	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return claims, errors.New("invalid payload encoding")
	}

	if err := json.Unmarshal(payloadBytes, &claims); err != nil {
		return claims, fmt.Errorf("unmarshal claims: %w", err)
	}

	now := time.Now().Unix()
	if claims.Expires > 0 && now > claims.Expires {
		return claims, errors.New("token expired")
	}

	return claims, nil
}
