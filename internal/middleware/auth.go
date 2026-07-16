package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/auth"
)

const authClaimsKey = "auth.claims"

func Auth(jwt *auth.JWT) gin.HandlerFunc {
	return func(c *gin.Context) {
		token := jwt.ExtractBearerToken(c.Request)
		if token == "" {
			WriteError(c, http.StatusUnauthorized, "AUTHENTICATION_FAILED", "Authentication required")
			c.Abort()
			return
		}

		claims, err := jwt.Parse(token)
		if err != nil {
			WriteError(c, http.StatusUnauthorized, "AUTHENTICATION_FAILED", "Invalid or expired token")
			c.Abort()
			return
		}

		c.Set(authClaimsKey, claims)
		c.Next()
	}
}

func Admin(jwt *auth.JWT) gin.HandlerFunc {
	return func(c *gin.Context) {
		claims := ClaimsFromContext(c)
		if claims == nil || !jwt.HasRole(claims, "ADMIN") {
			WriteError(c, http.StatusForbidden, "ACCESS_DENIED", "Admin role required")
			c.Abort()
			return
		}
		c.Next()
	}
}

func ClaimsFromContext(c *gin.Context) *auth.Claims {
	value, ok := c.Get(authClaimsKey)
	if !ok {
		return nil
	}
	claims, _ := value.(*auth.Claims)
	return claims
}
