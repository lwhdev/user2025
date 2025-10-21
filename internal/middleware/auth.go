package middleware

import (
	"net/http"
	"strings"

	"example.com/user2025/internal/auth"
	"example.com/user2025/internal/handlers"
)

type AuthMiddleware struct {
	secret string
}

func NewAuthMiddleware(secret string) *AuthMiddleware {
	return &AuthMiddleware{secret: secret}
}

func (m *AuthMiddleware) RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := r.Header.Get("Authorization")
		if header == "" {
			handlers.WriteError(w, http.StatusUnauthorized, "authorization header required")
			return
		}

		token := ""
		if strings.HasPrefix(strings.ToLower(header), "bearer ") {
			token = strings.TrimSpace(header[7:])
		} else {
			token = strings.TrimSpace(header)
		}

		if token == "" {
			handlers.WriteError(w, http.StatusUnauthorized, "authorization header required")
			return
		}

		claims, err := auth.ParseToken(m.secret, token)
		if err != nil {
			handlers.WriteError(w, http.StatusUnauthorized, "invalid token")
			return
		}

		ctx := handlers.ContextWithClaims(r.Context(), claims)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
