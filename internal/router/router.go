package router

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/auth"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/config"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/handler"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/usecase"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
)

func New(cfg config.Config, jwt *auth.JWT, svc usecase.Service) (http.Handler, error) {
	binding.EnableDecoderDisallowUnknownFields = true

	basePath := config.BasePath()
	r := gin.New()
	// Trusted proxies decide which X-Real-IP nginx wrote is the client IP.
	// Empty trusts none, so ClientIP is the TCP peer; behind a proxy every
	// visitor would then share its address.
	if err := r.SetTrustedProxies(cfg.TrustedProxies); err != nil {
		return nil, fmt.Errorf("trusted proxies: %w", err)
	}
	if len(cfg.TrustedProxies) > 0 {
		r.RemoteIPHeaders = []string{"X-Real-IP"}
	}
	// otelgin goes first so the server span wraps the logger, recovery and
	// every handler. It is a no-op until telemetry.Setup installs a provider.
	r.Use(otelgin.Middleware(cfg.ServiceName))
	r.Use(gin.Logger(), gin.Recovery())
	// Chat streams for up to 90 s and sets its own context, and a full
	// knowledge reindex may take minutes (the use case bounds it); everything
	// else keeps the 30 s bound.
	r.Use(requestTimeout(30*time.Second, basePath+"/chat", basePath+"/admin/knowledge/reindex"))
	r.Use(cors.New(cors.Config{
		AllowOrigins: cfg.AllowedOrigins,
		// Origins may use "*" wildcards (e.g. "http://localhost:*" in dev); without
		// this they are matched literally and preflights are rejected with 403.
		AllowWildcard:    true,
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Accept", "Authorization", "Content-Type", "X-Auth-Token"},
		ExposeHeaders:    []string{"Authorization", "Content-Type"},
		AllowCredentials: true,
		MaxAge:           time.Hour,
	}))

	api := handler.New(svc, jwt, cfg.OAuthRedirectURI, basePath, cfg.CookieSecure, handler.ChatOptions{
		AllowedOrigins: cfg.AllowedOrigins,
		IPHashKey:      []byte(cfg.JWTSecret),
		MaxConcurrent:  cfg.Chat.MaxConcurrent,
	})
	api.Routes(r.Group(basePath))

	return r, nil
}

// requestTimeout bounds a request's context, except for the exempt paths.
func requestTimeout(timeout time.Duration, exempt ...string) gin.HandlerFunc {
	exemptSet := make(map[string]bool, len(exempt))
	for _, path := range exempt {
		exemptSet[path] = true
	}
	return func(c *gin.Context) {
		if exemptSet[c.Request.URL.Path] {
			c.Next()
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), timeout)
		defer cancel()

		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}
