package handler

import (
	"context"
	"crypto/subtle"
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/dto"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/middleware"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/usecase"
)

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
