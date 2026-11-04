package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/auth"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/middleware"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/usecase"
)

type API interface {
	Routes(r *gin.RouterGroup)
}

type api struct {
	svc              usecase.Service
	jwt              *auth.JWT
	oauthRedirectURI string
	basePath         string
	anonCookie       middleware.AnonCookieConfig
}

type oauthCookieConfig struct {
	StateName    string
	VerifierName string
	CallbackPath string
}

// oauthCookieNames holds the per-provider cookie names. The cookie Path is
// derived from the router base path at request time (see cookieConfig) so it
// always matches the actual versioned callback route.
var oauthCookieNames = map[usecase.OAuthProvider]struct {
	StateName    string
	VerifierName string
}{
	usecase.OAuthProviderGoogle: {"oauth_google_state", "oauth_google_pkce_verifier"},
	usecase.OAuthProviderGitHub: {"oauth_github_state", "oauth_github_pkce_verifier"},
}

const oauthCookieMaxAge = 10 * 60

func New(svc usecase.Service, jwt *auth.JWT, oauthRedirectURI, basePath string, cookieSecure bool) API {
	return &api{
		svc:              svc,
		jwt:              jwt,
		oauthRedirectURI: oauthRedirectURI,
		basePath:         basePath,
		anonCookie:       middleware.AnonCookieConfig{Path: basePath, Secure: cookieSecure},
	}
}

func (h *api) Routes(r *gin.RouterGroup) {
	r.POST("/auth/login", h.Login)
	r.POST("/auth/register", h.Register)
	r.GET("/auth/google", h.OAuthLogin(usecase.OAuthProviderGoogle))
	r.GET("/auth/google/callback", h.OAuthCallback(usecase.OAuthProviderGoogle))
	r.GET("/auth/github", h.OAuthLogin(usecase.OAuthProviderGitHub))
	r.GET("/auth/github/callback", h.OAuthCallback(usecase.OAuthProviderGitHub))
	r.POST("/auth/github/device", h.GitHubDeviceStart)
	r.POST("/auth/github/device/token", h.GitHubDevicePoll)
	r.GET("/auth/me", h.Me)

	r.GET("/projects", h.ListProjects)
	r.GET("/projects/:id", h.GetProject)
	r.GET("/projects/:id/ratings", h.GetProjectRatings)
	r.GET("/projects/:id/ratings/average", h.GetAverageRating)
	r.GET("/projects/:id/ratings/count", h.GetRatingCount)
	r.GET("/projects/:id/ratings/distribution", h.GetRatingDistribution)

	r.GET("/ratings", h.ListRatings)
	r.GET("/ratings/:id", h.GetRating)
	r.GET("/ratings/project/:projectId", h.GetProjectRatings)
	r.GET("/ratings/project/:projectId/average", h.GetAverageRating)
	r.GET("/ratings/project/:projectId/count", h.GetRatingCount)
	r.GET("/ratings/project/:projectId/distribution", h.GetRatingDistribution)

	r.GET("/projects/:id/comments", h.ListProjectComments)
	r.GET("/comments/project/:projectId", h.ListProjectComments)

	// Article feedback is read by slug (a string), so these routes must not use
	// parseID. Two URLs mirror the dual-URL project comment convention above.
	r.GET("/articles/:slug/comments", h.ListArticleComments)
	r.GET("/comments/article/:slug", h.ListArticleComments)

	// Public participation: rating and commenting need no login. EnsureIdentity
	// resolves a real user from their JWT or auto-provisions an anonymous one,
	// setting the anon_token cookie so the identity persists across visits.
	identified := r.Group("")
	identified.Use(middleware.EnsureIdentity(h.jwt, h.svc, h.anonCookie))
	identified.POST("/ratings", h.CreateRating)
	identified.PUT("/ratings/:id", h.UpdateRating)
	identified.DELETE("/ratings/:id", h.DeleteRating)
	identified.POST("/comments", h.CreateComment)
	identified.PUT("/comments/:id", h.UpdateComment)
	identified.DELETE("/comments/:id", h.DeleteComment)

	// Routes that identify the caller from their JWT must run behind Auth,
	// otherwise the claims lookup returns nil and the handler panics.
	authenticated := r.Group("")
	authenticated.Use(middleware.Auth(h.jwt))
	authenticated.GET("/ratings/my-ratings", h.GetMyRatings)
	authenticated.GET("/ratings/project/:projectId/has-rated", h.HasUserRatedProject)
	authenticated.POST("/photos/profile", h.SaveProfilePhoto)
	authenticated.GET("/users/me/ratings", h.GetMyRatings)
	authenticated.GET("/users/me/comments", h.GetMyComments)
	authenticated.PATCH("/users/me", h.UpdateProfile)
	authenticated.PATCH("/users/me/password", h.ChangePassword)
	authenticated.DELETE("/users/me", h.DeleteAccount)

	admin := authenticated.Group("")
	admin.Use(middleware.Admin(h.jwt))
	admin.DELETE("/photos/:photoId", h.DeletePhoto)
	admin.POST("/photos/:projectId", h.SaveProjectPhoto)

	// Content writes. These were registered on the public group and needed no
	// login at all; every one of them is an admin action.
	admin.POST("/projects", h.CreateProject)
	admin.PUT("/projects/:id", h.UpdateProject)
	admin.DELETE("/projects/:id", h.DeleteProject)
	admin.DELETE("/projects/:id/ratings", h.DeleteProjectRatings)
	admin.DELETE("/ratings/project/:projectId", h.DeleteProjectRatings)

	admin.GET("/admin/stats", h.GetAdminStats)
	admin.GET("/admin/users", h.ListUsersForAdmin)
	admin.PATCH("/admin/users/:id/role", h.UpdateUserRole)
	admin.DELETE("/admin/users/:id", h.DeleteUserAsAdmin)
	admin.GET("/admin/comments", h.ListCommentsForAdmin)
	admin.DELETE("/admin/comments/:id", h.DeleteCommentAsAdmin)

	r.POST("/contact", h.SubmitContact)
	admin.GET("/contact", h.ListContactMessages)
	admin.GET("/contact/unread", h.ListUnreadContactMessages)
	admin.PATCH("/contact/:id/read", h.MarkContactRead)
	admin.PATCH("/contact/:id/replied", h.MarkContactReplied)
	admin.DELETE("/contact/:id", h.DeleteContactMessage)

	r.GET("/cron", h.Cron)
}

func (h *api) Cron(c *gin.Context) {
	c.String(http.StatusOK, "Portfolio backend alive")
}

// parseID reads the first of the given route params that is present and parses
// it as a positive int64, writing a 400 response and returning false on failure.
