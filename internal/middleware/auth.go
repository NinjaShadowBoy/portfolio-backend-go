package middleware

import (
	"context"
	"net/http"

	"github.com/ninjashadowboy/portfolio-backend-go/internal/auth"
)

type contextKey string

const authClaimsKey contextKey = "auth.claims"

func Auth(jwt *auth.JWT) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := jwt.ExtractBearerToken(r)
			if token == "" {
				WriteError(w, http.StatusUnauthorized, "AUTHENTICATION_FAILED", "Authentication required")
				return
			}

			claims, err := jwt.Parse(token)
			if err != nil {
				WriteError(w, http.StatusUnauthorized, "AUTHENTICATION_FAILED", "Invalid or expired token")
				return
			}

			ctx := context.WithValue(r.Context(), authClaimsKey, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func Admin(jwt *auth.JWT) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims, ok := r.Context().Value(authClaimsKey).(*auth.Claims)
			if !ok || !jwt.HasRole(claims, "ADMIN") {
				WriteError(w, http.StatusForbidden, "ACCESS_DENIED", "Admin role required")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func ClaimsFromContext(r *http.Request) *auth.Claims {
	claims, _ := r.Context().Value(authClaimsKey).(*auth.Claims)
	return claims
}
