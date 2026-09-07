package handler

import (
	"context"
	"crypto/subtle"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/auth"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/dto"
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
	r.POST("/projects", h.CreateProject)
	r.PUT("/projects/:id", h.UpdateProject)
	r.DELETE("/projects/:id", h.DeleteProject)
	r.GET("/projects/:id/ratings", h.GetProjectRatings)
	r.GET("/projects/:id/ratings/average", h.GetAverageRating)
	r.GET("/projects/:id/ratings/count", h.GetRatingCount)
	r.GET("/projects/:id/ratings/distribution", h.GetRatingDistribution)
	r.DELETE("/projects/:id/ratings", h.DeleteProjectRatings)

	r.GET("/ratings", h.ListRatings)
	r.GET("/ratings/:id", h.GetRating)
	r.GET("/ratings/project/:projectId", h.GetProjectRatings)
	r.GET("/ratings/project/:projectId/average", h.GetAverageRating)
	r.GET("/ratings/project/:projectId/count", h.GetRatingCount)
	r.GET("/ratings/project/:projectId/distribution", h.GetRatingDistribution)
	r.DELETE("/ratings/project/:projectId", h.DeleteProjectRatings)

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
	authenticated.POST("/photos/:projectId", h.SaveProjectPhoto)
	authenticated.POST("/photos/profile", h.SaveProfilePhoto)
	authenticated.GET("/users/me/ratings", h.GetMyRatings)
	authenticated.GET("/users/me/comments", h.GetMyComments)
	authenticated.PATCH("/users/me", h.UpdateProfile)
	authenticated.PATCH("/users/me/password", h.ChangePassword)
	authenticated.DELETE("/users/me", h.DeleteAccount)

	admin := authenticated.Group("")
	admin.Use(middleware.Admin(h.jwt))
	admin.DELETE("/photos/:photoId", h.DeletePhoto)

	r.POST("/contact", h.SubmitContact)
	admin.GET("/contact", h.ListContactMessages)
	admin.GET("/contact/unread", h.ListUnreadContactMessages)
	admin.PATCH("/contact/:id/read", h.MarkContactRead)
	admin.PATCH("/contact/:id/replied", h.MarkContactReplied)
	admin.DELETE("/contact/:id", h.DeleteContactMessage)

	r.GET("/cron", h.Cron)
}

func (h *api) Login(c *gin.Context) {
	var req dto.LoginRequest
	if !bindJSON(c, &req) {
		return
	}
	resp, err := h.svc.Login(c.Request.Context(), req.Email, req.Password)
	if err != nil {
		writeError(c, err)
		return
	}
	h.claimAnonActivity(c, resp.User.ID)
	c.JSON(http.StatusOK, resp)
}

// claimAnonActivity absorbs any anonymous ratings/comments the caller made before
// authenticating into their real account, then clears the anon cookie. Best
// effort: a failure here must not block the login it rides along with.
func (h *api) claimAnonActivity(c *gin.Context, realUserID int64) {
	raw, err := c.Cookie(middleware.AnonCookieName)
	if err != nil || raw == "" {
		return
	}
	ctx := c.Request.Context()
	if anonUserID, ok, err := h.svc.ResolveAnonToken(ctx, raw); err == nil && ok {
		_ = h.svc.ClaimAnonymousActivity(ctx, anonUserID, realUserID)
	}
	middleware.ClearAnonCookie(c, h.anonCookie)
}

func (h *api) Register(c *gin.Context) {
	var req dto.RegisterRequest
	if !bindJSON(c, &req) {
		return
	}
	if err := h.svc.Register(c.Request.Context(), req); err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusCreated, dto.MessageResponse{Message: "User created successfully"})
}

func (h *api) OAuthLogin(provider usecase.OAuthProvider) gin.HandlerFunc {
	return func(c *gin.Context) {
		start, err := h.svc.StartOAuthLogin(provider)
		if err != nil {
			redirectOAuth(c, h.oauthRedirectURI, "", "OAuth2 is not configured")
			return
		}

		cookies := h.oauthCookieConfigFor(provider)
		setOAuthCookie(c, cookies.StateName, start.State, cookies.CallbackPath)
		setOAuthCookie(c, cookies.VerifierName, start.Verifier, cookies.CallbackPath)
		c.Redirect(http.StatusTemporaryRedirect, start.AuthURL)
	}
}

func (h *api) OAuthCallback(provider usecase.OAuthProvider) gin.HandlerFunc {
	return func(c *gin.Context) {
		cookies := h.oauthCookieConfigFor(provider)
		if errMsg := c.Query("error"); errMsg != "" {
			redirectOAuth(c, h.oauthRedirectURI, "", errMsg)
			return
		}

		stateCookie, err := c.Cookie(cookies.StateName)
		if err != nil {
			redirectOAuth(c, h.oauthRedirectURI, "", "OAuth2 state is missing")
			return
		}
		verifierCookie, err := c.Cookie(cookies.VerifierName)
		if err != nil {
			redirectOAuth(c, h.oauthRedirectURI, "", "OAuth2 PKCE verifier is missing")
			return
		}

		state := c.Query("state")
		code := c.Query("code")
		if !secureEqual(stateCookie, state) {
			redirectOAuth(c, h.oauthRedirectURI, "", "OAuth2 state mismatch")
			return
		}

		resp, err := h.svc.OAuthLogin(c.Request.Context(), provider, code, stateCookie, verifierCookie)
		clearOAuthCookie(c, cookies.StateName, cookies.VerifierName, cookies.CallbackPath)
		if err != nil {
			redirectOAuth(c, h.oauthRedirectURI, "", "Authentication failed")
			return
		}

		h.claimAnonActivity(c, resp.User.ID)
		redirectOAuth(c, h.oauthRedirectURI, resp.Token, "")
	}
}

func (h *api) GitHubDeviceStart(c *gin.Context) {
	start, err := h.svc.StartGitHubDeviceLogin(c.Request.Context())
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, dto.DeviceCodeResponse{
		DeviceCode:      start.DeviceCode,
		UserCode:        start.UserCode,
		VerificationUri: start.VerificationURI,
		ExpiresIn:       start.ExpiresIn,
		Interval:        start.Interval,
	})
}

func (h *api) GitHubDevicePoll(c *gin.Context) {
	var req dto.DevicePollRequest
	if !bindJSON(c, &req) {
		return
	}
	resp, status, err := h.svc.PollGitHubDeviceLogin(c.Request.Context(), req.DeviceCode)
	if err != nil {
		writeError(c, err)
		return
	}
	if status == usecase.DeviceAuthComplete {
		c.JSON(http.StatusOK, resp)
		return
	}
	// Not authorized yet: 202 tells the client to keep polling.
	c.JSON(http.StatusAccepted, dto.DeviceStatusResponse{Status: string(status)})
}

// Me reports the current identity without provisioning one: a real user (bearer
// token), an existing anonymous user (anon_token cookie), or none. Anonymous
// users are only created when someone actually rates or comments.
func (h *api) Me(c *gin.Context) {
	ctx := c.Request.Context()
	if token := h.jwt.ExtractBearerToken(c.Request); token != "" {
		if claims, err := h.jwt.Parse(token); err == nil {
			h.writeIdentity(c, ctx, claims.UserID)
			return
		}
	}
	if raw, err := c.Cookie(middleware.AnonCookieName); err == nil && raw != "" {
		if userID, ok, err := h.svc.ResolveAnonToken(ctx, raw); err == nil && ok {
			h.writeIdentity(c, ctx, userID)
			return
		}
	}
	c.JSON(http.StatusOK, gin.H{"identity": nil})
}

func (h *api) writeIdentity(c *gin.Context, ctx context.Context, userID int64) {
	identity, err := h.svc.GetIdentity(ctx, userID)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"identity": nil})
		return
	}
	c.JSON(http.StatusOK, gin.H{"identity": identity})
}

func (h *api) oauthCookieConfigFor(provider usecase.OAuthProvider) oauthCookieConfig {
	names, ok := oauthCookieNames[provider]
	if !ok {
		provider = usecase.OAuthProviderGoogle
		names = oauthCookieNames[provider]
	}
	return oauthCookieConfig{
		StateName:    names.StateName,
		VerifierName: names.VerifierName,
		CallbackPath: h.basePath + "/auth/" + strings.ToLower(string(provider)) + "/callback",
	}
}

func (h *api) ListProjects(c *gin.Context) {
	projects, err := h.svc.ListProjects(c.Request.Context())
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, projects)
}

func (h *api) GetProject(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	project, err := h.svc.GetProject(c.Request.Context(), id)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, project)
}

func (h *api) CreateProject(c *gin.Context) {
	var req dto.ProjectCreate
	if !bindJSON(c, &req) {
		return
	}
	project, err := h.svc.CreateProject(c.Request.Context(), req)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusCreated, project)
}

func (h *api) UpdateProject(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	var req dto.ProjectUpdate
	if !bindJSON(c, &req) {
		return
	}
	project, err := h.svc.UpdateProject(c.Request.Context(), id, req)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, project)
}

func (h *api) DeleteProject(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	if err := h.svc.DeleteProject(c.Request.Context(), id); err != nil {
		writeError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// GetProjectRatings serves both /projects/:id/ratings and /ratings/project/:projectId.
func (h *api) GetProjectRatings(c *gin.Context) {
	id, ok := parseID(c, "id", "projectId")
	if !ok {
		return
	}
	ratings, err := h.svc.GetProjectRatings(c.Request.Context(), id)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, ratings)
}

func (h *api) GetAverageRating(c *gin.Context) {
	id, ok := parseID(c, "id", "projectId")
	if !ok {
		return
	}
	average, err := h.svc.GetProjectAverageRating(c.Request.Context(), id)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"averageRating": average})
}

func (h *api) GetRatingCount(c *gin.Context) {
	id, ok := parseID(c, "id", "projectId")
	if !ok {
		return
	}
	count, err := h.svc.GetProjectRatingCount(c.Request.Context(), id)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"count": count})
}

func (h *api) GetRatingDistribution(c *gin.Context) {
	id, ok := parseID(c, "id", "projectId")
	if !ok {
		return
	}
	distribution, err := h.svc.GetProjectRatingDistribution(c.Request.Context(), id)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, distribution)
}

func (h *api) DeleteProjectRatings(c *gin.Context) {
	id, ok := parseID(c, "id", "projectId")
	if !ok {
		return
	}
	if err := h.svc.ResetProjectRatings(c.Request.Context(), id); err != nil {
		writeError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *api) ListRatings(c *gin.Context) {
	ratings, err := h.svc.ListRatings(c.Request.Context())
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, ratings)
}

func (h *api) GetRating(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	rating, err := h.svc.GetRating(c.Request.Context(), id)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, rating)
}

func (h *api) GetMyRatings(c *gin.Context) {
	claims := middleware.ClaimsFromContext(c)
	ratings, err := h.svc.GetRatingsByUserID(c.Request.Context(), claims.UserID)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, ratings)
}

func (h *api) HasUserRatedProject(c *gin.Context) {
	claims := middleware.ClaimsFromContext(c)
	projectID, ok := parseID(c, "projectId")
	if !ok {
		return
	}
	hasRated, err := h.svc.HasUserRatedProject(c.Request.Context(), claims.UserID, projectID)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"hasRated": hasRated})
}

func (h *api) CreateRating(c *gin.Context) {
	identity, ok := middleware.IdentityFromContext(c)
	if !ok {
		writeError(c, usecase.ErrUnauthorized)
		return
	}
	var req dto.RatingCreate
	if !bindJSON(c, &req) {
		return
	}
	rating, err := h.svc.CreateRating(c.Request.Context(), identity.UserID, req)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusCreated, rating)
}

func (h *api) UpdateRating(c *gin.Context) {
	identity, ok := middleware.IdentityFromContext(c)
	if !ok {
		writeError(c, usecase.ErrUnauthorized)
		return
	}
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	var req dto.RatingUpdate
	if !bindJSON(c, &req) {
		return
	}
	rating, err := h.svc.UpdateRating(c.Request.Context(), id, identity.UserID, req)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, rating)
}

func (h *api) DeleteRating(c *gin.Context) {
	identity, ok := middleware.IdentityFromContext(c)
	if !ok {
		writeError(c, usecase.ErrUnauthorized)
		return
	}
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	if err := h.svc.DeleteRating(c.Request.Context(), id, identity.UserID); err != nil {
		writeError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *api) ListProjectComments(c *gin.Context) {
	id, ok := parseID(c, "id", "projectId")
	if !ok {
		return
	}
	comments, err := h.svc.ListProjectComments(c.Request.Context(), id)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, comments)
}

// ListArticleComments serves both /articles/:slug/comments and
// /comments/article/:slug. The slug is a free-form string, so it is read
// directly from the route param rather than through parseID (int64-only).
func (h *api) ListArticleComments(c *gin.Context) {
	slug := c.Param("slug")
	comments, err := h.svc.ListArticleComments(c.Request.Context(), slug)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, comments)
}

func (h *api) CreateComment(c *gin.Context) {
	identity, ok := middleware.IdentityFromContext(c)
	if !ok {
		writeError(c, usecase.ErrUnauthorized)
		return
	}
	var req dto.CommentCreate
	if !bindJSON(c, &req) {
		return
	}
	comment, err := h.svc.CreateComment(c.Request.Context(), identity.UserID, req)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusCreated, comment)
}

func (h *api) UpdateComment(c *gin.Context) {
	identity, ok := middleware.IdentityFromContext(c)
	if !ok {
		writeError(c, usecase.ErrUnauthorized)
		return
	}
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	var req dto.CommentUpdate
	if !bindJSON(c, &req) {
		return
	}
	comment, err := h.svc.UpdateComment(c.Request.Context(), id, identity.UserID, req)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, comment)
}

func (h *api) DeleteComment(c *gin.Context) {
	identity, ok := middleware.IdentityFromContext(c)
	if !ok {
		writeError(c, usecase.ErrUnauthorized)
		return
	}
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	if err := h.svc.DeleteComment(c.Request.Context(), id, identity.UserID); err != nil {
		writeError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *api) GetMyComments(c *gin.Context) {
	claims := middleware.ClaimsFromContext(c)
	comments, err := h.svc.ListUserComments(c.Request.Context(), claims.UserID)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, comments)
}

func (h *api) UpdateProfile(c *gin.Context) {
	claims := middleware.ClaimsFromContext(c)
	var req dto.ProfileUpdate
	if !bindJSON(c, &req) {
		return
	}
	user, err := h.svc.UpdateProfile(c.Request.Context(), claims.UserID, req)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, user)
}

func (h *api) ChangePassword(c *gin.Context) {
	claims := middleware.ClaimsFromContext(c)
	var req dto.PasswordChange
	if !bindJSON(c, &req) {
		return
	}
	if err := h.svc.ChangePassword(c.Request.Context(), claims.UserID, req); err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, dto.MessageResponse{Message: "Password updated successfully"})
}

func (h *api) DeleteAccount(c *gin.Context) {
	claims := middleware.ClaimsFromContext(c)
	if err := h.svc.DeleteAccount(c.Request.Context(), claims.UserID); err != nil {
		writeError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *api) SaveProjectPhoto(c *gin.Context) {
	projectID, ok := parseID(c, "projectId")
	if !ok {
		return
	}
	var req dto.PhotoCreate
	if !bindJSON(c, &req) {
		return
	}
	photo, err := h.svc.SaveProjectPhoto(c.Request.Context(), req.PhotoURL, projectID)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusCreated, photo)
}

func (h *api) SaveProfilePhoto(c *gin.Context) {
	var req dto.PhotoCreate
	if !bindJSON(c, &req) {
		return
	}
	photo, err := h.svc.SaveProfilePhoto(c.Request.Context(), req.PhotoURL)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusCreated, photo)
}

func (h *api) DeletePhoto(c *gin.Context) {
	id, ok := parseID(c, "photoId")
	if !ok {
		return
	}
	if err := h.svc.DeletePhoto(c.Request.Context(), id); err != nil {
		writeError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *api) SubmitContact(c *gin.Context) {
	var req dto.Contact
	if !bindJSON(c, &req) {
		return
	}
	message, err := h.svc.SubmitContact(c.Request.Context(), req)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusCreated, dto.ContactResponse{Success: true, Message: "Thank you for your message! I'll get back to you soon.", Data: &message})
}

func (h *api) ListContactMessages(c *gin.Context) {
	messages, err := h.svc.ListContactMessages(c.Request.Context())
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, messages)
}

func (h *api) ListUnreadContactMessages(c *gin.Context) {
	messages, err := h.svc.ListUnreadContactMessages(c.Request.Context())
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, messages)
}

func (h *api) MarkContactRead(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	message, err := h.svc.MarkContactRead(c.Request.Context(), id)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, message)
}

func (h *api) MarkContactReplied(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	message, err := h.svc.MarkContactReplied(c.Request.Context(), id)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, message)
}

func (h *api) DeleteContactMessage(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	if err := h.svc.DeleteContactMessage(c.Request.Context(), id); err != nil {
		writeError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *api) Cron(c *gin.Context) {
	c.String(http.StatusOK, "Portfolio backend alive")
}

// parseID reads the first of the given route params that is present and parses
// it as a positive int64, writing a 400 response and returning false on failure.
func parseID(c *gin.Context, names ...string) (int64, bool) {
	raw := ""
	for _, name := range names {
		if value := c.Param(name); value != "" {
			raw = value
			break
		}
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		middleware.WriteError(c, http.StatusBadRequest, "TYPE_MISMATCH", "Invalid ID")
		return 0, false
	}
	return id, true
}

func bindJSON(c *gin.Context, dst any) bool {
	if err := c.ShouldBindJSON(dst); err != nil {
		middleware.WriteError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid request body")
		return false
	}
	return true
}

func setOAuthCookie(c *gin.Context, name, value, path string) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     path,
		MaxAge:   oauthCookieMaxAge,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

func clearOAuthCookie(c *gin.Context, stateName, verifierName, path string) {
	for _, name := range []string{stateName, verifierName} {
		http.SetCookie(c.Writer, &http.Cookie{
			Name:     name,
			Value:    "",
			Path:     path,
			MaxAge:   -1,
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
		})
	}
}

func secureEqual(a, b string) bool {
	return len(a) == len(b) && subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

func redirectOAuth(c *gin.Context, baseURL, token, errMsg string) {
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		c.String(http.StatusInternalServerError, "Invalid OAuth redirect URI")
		return
	}

	query := parsed.Query()
	if errMsg != "" {
		query.Set("error", errMsg)
		query.Del("token")
	} else {
		query.Set("token", token)
		query.Del("error")
	}
	parsed.RawQuery = query.Encode()

	c.Redirect(http.StatusTemporaryRedirect, parsed.String())
}

func writeError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, usecase.ErrNotFound):
		middleware.WriteError(c, http.StatusNotFound, "RESOURCE_NOT_FOUND", "Resource not found")
	case errors.Is(err, usecase.ErrUnauthorized):
		middleware.WriteError(c, http.StatusUnauthorized, "AUTHENTICATION_FAILED", "Authentication failed")
	case errors.Is(err, usecase.ErrForbidden):
		middleware.WriteError(c, http.StatusForbidden, "ACCESS_DENIED", "Access denied")
	case errors.Is(err, usecase.ErrConflict) || errors.Is(err, usecase.ErrAlreadyExists):
		middleware.WriteError(c, http.StatusConflict, "RESOURCE_ALREADY_EXISTS", "Resource already exists")
	case errors.Is(err, usecase.ErrBadRequest):
		middleware.WriteError(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
	default:
		middleware.WriteError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Internal server error")
	}
}
