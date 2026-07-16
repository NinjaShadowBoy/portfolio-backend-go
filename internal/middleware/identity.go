package middleware

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/auth"
)

const identityKey = "identity"

// AnonCookieName holds the opaque anonymous-identity token. The value is a raw
// token; the server stores only its SHA-256 hash.
const AnonCookieName = "anon_token"

// One year: anonymous contributions should survive well beyond a single visit.
const anonCookieMaxAge = 365 * 24 * 60 * 60

// Identity is the caller resolved by EnsureIdentity: either a real logged-in
// account or an auto-provisioned anonymous user.
type Identity struct {
	UserID    int64
	Anonymous bool
}

// AnonProvisioner is the slice of the service EnsureIdentity needs to resolve or
// mint anonymous identities (kept narrow so middleware doesn't depend on service).
type AnonProvisioner interface {
	ResolveAnonToken(ctx context.Context, rawToken string) (int64, bool, error)
	CreateAnonymousUser(ctx context.Context) (int64, string, error)
}

// AnonCookieConfig configures the anon_token cookie's scope and security.
type AnonCookieConfig struct {
	Path   string
	Secure bool
}

// EnsureIdentity resolves the caller in order: a valid bearer token (real user),
// then an existing anon_token cookie, otherwise it provisions a fresh anonymous
// user and sets the cookie. The resolved Identity is stored on the context.
func EnsureIdentity(jwt *auth.JWT, anon AnonProvisioner, cfg AnonCookieConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		if token := jwt.ExtractBearerToken(c.Request); token != "" {
			if claims, err := jwt.Parse(token); err == nil {
				c.Set(identityKey, Identity{UserID: claims.UserID, Anonymous: false})
				c.Next()
				return
			}
		}

		ctx := c.Request.Context()
		if raw, err := c.Cookie(AnonCookieName); err == nil && raw != "" {
			userID, ok, err := anon.ResolveAnonToken(ctx, raw)
			if err != nil {
				WriteError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Internal server error")
				c.Abort()
				return
			}
			if ok {
				c.Set(identityKey, Identity{UserID: userID, Anonymous: true})
				c.Next()
				return
			}
		}

		userID, rawToken, err := anon.CreateAnonymousUser(ctx)
		if err != nil {
			WriteError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Internal server error")
			c.Abort()
			return
		}
		SetAnonCookie(c, rawToken, cfg)
		c.Set(identityKey, Identity{UserID: userID, Anonymous: true})
		c.Next()
	}
}

// IdentityFromContext returns the Identity set by EnsureIdentity.
func IdentityFromContext(c *gin.Context) (Identity, bool) {
	value, ok := c.Get(identityKey)
	if !ok {
		return Identity{}, false
	}
	identity, ok := value.(Identity)
	return identity, ok
}

func SetAnonCookie(c *gin.Context, rawToken string, cfg AnonCookieConfig) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     AnonCookieName,
		Value:    rawToken,
		Path:     cfg.Path,
		MaxAge:   anonCookieMaxAge,
		HttpOnly: true,
		Secure:   cfg.Secure,
		SameSite: http.SameSiteLaxMode,
	})
}

func ClearAnonCookie(c *gin.Context, cfg AnonCookieConfig) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     AnonCookieName,
		Value:    "",
		Path:     cfg.Path,
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   cfg.Secure,
		SameSite: http.SameSiteLaxMode,
	})
}
