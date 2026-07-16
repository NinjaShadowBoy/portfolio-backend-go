package router

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/auth"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/config"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/handler"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/service"
)

// Base path segments for the HTTP API. The full mount point is
// APIPrefix + "/" + APIVersion, e.g. "/portfolio/v1". Bump APIVersion (and add a
// new group) when introducing breaking API changes so old clients keep working.
const (
	APIPrefix  = "/portfolio"
	APIVersion = "v1"
)

// BasePath is the versioned root every route is mounted under, e.g. "/portfolio/v1".
func BasePath() string {
	return APIPrefix + "/" + APIVersion
}

func New(cfg config.Config, jwt *auth.JWT, svc service.Service) http.Handler {
	binding.EnableDecoderDisallowUnknownFields = true

	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery(), requestTimeout(30*time.Second))
	r.Use(cors.New(cors.Config{
		AllowOrigins:     cfg.AllowedOrigins,
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Accept", "Authorization", "Content-Type", "X-Auth-Token"},
		ExposeHeaders:    []string{"Authorization", "Content-Type"},
		AllowCredentials: true,
		MaxAge:           time.Hour,
	}))

	basePath := BasePath()
	api := handler.New(svc, jwt, cfg.OAuthRedirectURI, basePath)
	api.Routes(r.Group(basePath))

	return r
}

func requestTimeout(timeout time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), timeout)
		defer cancel()

		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}
