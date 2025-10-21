package handlers

import (
	"context"

	"example.com/user2025/internal/auth"
)

type userContextKey struct{}

func ContextWithClaims(ctx context.Context, claims auth.Claims) context.Context {
	return context.WithValue(ctx, userContextKey{}, claims)
}

func ClaimsFromContext(ctx context.Context) (auth.Claims, bool) {
	claims, ok := ctx.Value(userContextKey{}).(auth.Claims)
	return claims, ok
}
