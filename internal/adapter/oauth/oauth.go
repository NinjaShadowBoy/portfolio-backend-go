package oauth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/ninjashadowboy/portfolio-backend-go/internal/domain"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/ports"
	"golang.org/x/oauth2"
)

type OAuthProvider = domain.OAuthProvider
type OAuthLoginStart = domain.OAuthLoginStart
type DeviceLoginStart = domain.DeviceLoginStart
type DeviceAuthStatus = domain.DeviceAuthStatus

const (
	OAuthProviderGoogle = domain.OAuthProviderGoogle
	OAuthProviderGitHub = domain.OAuthProviderGitHub
	DeviceAuthPending   = domain.DeviceAuthPending
	DeviceAuthSlowDown  = domain.DeviceAuthSlowDown
	DeviceAuthComplete  = domain.DeviceAuthComplete
)

var ErrBadRequest = domain.ErrBadRequest

type Gateway struct {
	GoogleOAuth *oauth2.Config
	GitHubOAuth *oauth2.Config
	client      *http.Client
}

var _ ports.OAuthGateway = (*Gateway)(nil)

func New(google, github *oauth2.Config, client *http.Client) *Gateway {
	return &Gateway{GoogleOAuth: google, GitHubOAuth: github, client: client}
}

const (
	githubDeviceCodeURL   = "https://github.com/login/device/code"
	githubDeviceGrantType = "urn:ietf:params:oauth:grant-type:device_code"
)

type googleUserInfo struct {
	ID            string `json:"id"`
	Email         string `json:"email"`
	VerifiedEmail bool   `json:"verified_email"`
	Name          string `json:"name"`
	Picture       string `json:"picture"`
}

type githubUserInfo struct {
	// GitHub returns the user id as a JSON number; json.Number accepts it and
	// yields its string form for use as the opaque provider id.
	ID        json.Number `json:"id"`
	Login     string      `json:"login"`
	Name      string      `json:"name"`
	Email     string      `json:"email"`
	AvatarURL string      `json:"avatar_url"`
}

type githubEmail struct {
	Email    string `json:"email"`
	Primary  bool   `json:"primary"`
	Verified bool   `json:"verified"`
}

func NewGoogleOAuthConfig(clientID, clientSecret, baseURL string) *oauth2.Config {
	return newOAuthConfig(clientID, clientSecret, baseURL, OAuthProviderGoogle, []string{"openid", "email", "profile"}, oauth2.Endpoint{
		AuthURL:  "https://accounts.google.com/o/oauth2/auth",
		TokenURL: "https://oauth2.googleapis.com/token",
	})
}

func NewGitHubOAuthConfig(clientID, clientSecret, baseURL string) *oauth2.Config {
	return newOAuthConfig(clientID, clientSecret, baseURL, OAuthProviderGitHub, []string{"read:user", "user:email"}, oauth2.Endpoint{
		AuthURL:  "https://github.com/login/oauth/authorize",
		TokenURL: "https://github.com/login/oauth/access_token",
	})
}

func newOAuthConfig(clientID, clientSecret, baseURL string, provider OAuthProvider, scopes []string, endpoint oauth2.Endpoint) *oauth2.Config {
	clientID = strings.TrimSpace(clientID)
	clientSecret = strings.TrimSpace(clientSecret)
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if clientID == "" || clientSecret == "" || baseURL == "" {
		return nil
	}

	return &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RedirectURL:  baseURL + "/auth/" + strings.ToLower(string(provider)) + "/callback",
		Scopes:       scopes,
		Endpoint:     endpoint,
	}
}

func (s *Gateway) Start(provider OAuthProvider) (OAuthLoginStart, error) {
	config := s.oauthConfig(provider)
	if config == nil {
		return OAuthLoginStart{}, ErrBadRequest
	}

	state, err := randomURLSafeString(32)
	if err != nil {
		return OAuthLoginStart{}, err
	}

	verifier := oauth2.GenerateVerifier()
	options := []oauth2.AuthCodeOption{
		oauth2.AccessTypeOnline,
		oauth2.S256ChallengeOption(verifier),
	}
	if provider == OAuthProviderGoogle {
		options = append(options, oauth2.SetAuthURLParam("prompt", "select_account"))
	}

	authURL := config.AuthCodeURL(state, options...)
	return OAuthLoginStart{AuthURL: authURL, State: state, Verifier: verifier, Provider: provider}, nil
}

func (s *Gateway) StartDevice(ctx context.Context) (DeviceLoginStart, error) {
	if s.GitHubOAuth == nil {
		return DeviceLoginStart{}, ErrBadRequest
	}

	form := url.Values{}
	form.Set("client_id", s.GitHubOAuth.ClientID)
	form.Set("scope", strings.Join(s.GitHubOAuth.Scopes, " "))

	var body struct {
		DeviceCode      string `json:"device_code"`
		UserCode        string `json:"user_code"`
		VerificationURI string `json:"verification_uri"`
		ExpiresIn       int    `json:"expires_in"`
		Interval        int    `json:"interval"`
		Error           string `json:"error"`
		ErrorDesc       string `json:"error_description"`
	}
	if err := s.githubPostForm(ctx, githubDeviceCodeURL, form, &body); err != nil {
		return DeviceLoginStart{}, err
	}
	if body.Error != "" || body.DeviceCode == "" {
		return DeviceLoginStart{}, fmt.Errorf("%w: start GitHub device login: %s", ErrBadRequest, oauthErrText(body.Error, body.ErrorDesc))
	}

	interval := body.Interval
	if interval <= 0 {
		interval = 5
	}
	return DeviceLoginStart{
		DeviceCode:      body.DeviceCode,
		UserCode:        body.UserCode,
		VerificationURI: body.VerificationURI,
		ExpiresIn:       body.ExpiresIn,
		Interval:        interval,
	}, nil
}

// PollGitHubDeviceLogin exchanges a device code for an access token. While the
// user has not yet authorized, it returns a pending/slow_down status and no error.
// On success it upserts the user and returns a signed login response.

func (s *Gateway) githubPostForm(ctx context.Context, endpoint string, form url.Values, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if err := decodeOAuthJSON(resp, "GitHub", out); err != nil {
		return fmt.Errorf("%w: %v", ErrBadRequest, err)
	}
	return nil
}

func oauthErrText(code, desc string) string {
	text := strings.Trim(strings.TrimSpace(code)+": "+strings.TrimSpace(desc), ": ")
	if text == "" {
		return "unknown error"
	}
	return text
}

func (s *Gateway) oauthConfig(provider OAuthProvider) *oauth2.Config {
	switch provider {
	case OAuthProviderGoogle:
		return s.GoogleOAuth
	case OAuthProviderGitHub:
		return s.GitHubOAuth
	default:
		return nil
	}
}

func (s *Gateway) oauthUserInfo(ctx context.Context, provider OAuthProvider, token *oauth2.Token) (email, displayName, avatarURL, providerID string, emailVerified bool, err error) {
	switch provider {
	case OAuthProviderGoogle:
		var info googleUserInfo
		info, err = s.getGoogleUserInfo(ctx, token)
		if err != nil {
			return "", "", "", "", false, err
		}
		return strings.TrimSpace(info.Email), oauthDisplayName(info.Name, info.Email, "Google User"), strings.TrimSpace(info.Picture), strings.TrimSpace(info.ID), info.VerifiedEmail, nil
	case OAuthProviderGitHub:
		var info githubUserInfo
		info, err = s.getGitHubUserInfo(ctx, token)
		if err != nil {
			return "", "", "", "", false, err
		}
		email = strings.TrimSpace(info.Email)
		verified := email != ""
		return email, oauthDisplayName(info.Name, info.Login, info.Login), strings.TrimSpace(info.AvatarURL), strings.TrimSpace(info.ID.String()), verified, nil
	default:
		return "", "", "", "", false, ErrBadRequest
	}
}

func (s *Gateway) getGoogleUserInfo(ctx context.Context, token *oauth2.Token) (googleUserInfo, error) {
	client := s.GoogleOAuth.Client(ctx, token)
	resp, err := client.Get("https://www.googleapis.com/oauth2/v2/userinfo")
	if err != nil {
		return googleUserInfo{}, err
	}
	defer resp.Body.Close()

	var info googleUserInfo
	if err := decodeOAuthJSON(resp, "Google userinfo", &info); err != nil {
		return googleUserInfo{}, err
	}

	return info, nil
}

func (s *Gateway) getGitHubUserInfo(ctx context.Context, token *oauth2.Token) (githubUserInfo, error) {
	client := s.GitHubOAuth.Client(ctx, token)
	info, err := getGitHubProfile(ctx, client, "https://api.github.com/user")
	if err != nil {
		return githubUserInfo{}, err
	}

	if strings.TrimSpace(info.Email) == "" {
		emails, err := getGitHubEmails(ctx, client)
		if err != nil {
			return githubUserInfo{}, err
		}
		info.Email = pickGitHubEmail(emails)
	}

	return info, nil
}

func getGitHubProfile(ctx context.Context, client *http.Client, url string) (githubUserInfo, error) {
	resp, err := githubGet(ctx, client, url)
	if err != nil {
		return githubUserInfo{}, err
	}
	defer resp.Body.Close()

	var info githubUserInfo
	if err := decodeOAuthJSON(resp, "GitHub API", &info); err != nil {
		return githubUserInfo{}, err
	}

	return info, nil
}

func getGitHubEmails(ctx context.Context, client *http.Client) ([]githubEmail, error) {
	resp, err := githubGet(ctx, client, "https://api.github.com/user/emails")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var emails []githubEmail
	if err := decodeOAuthJSON(resp, "GitHub emails API", &emails); err != nil {
		return nil, err
	}

	return emails, nil
}

// decodeOAuthJSON checks resp for a 2xx status and decodes its JSON body into out.
// label names the upstream source for error messages. It does not close resp.Body.

func decodeOAuthJSON(resp *http.Response, label string, out any) error {
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("%s returned %s: %s", label, resp.Status, strings.TrimSpace(string(body)))
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(out)
}

func githubGet(ctx context.Context, client *http.Client, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	return client.Do(req)
}

func pickGitHubEmail(emails []githubEmail) string {
	var fallback string
	for _, email := range emails {
		trimmed := strings.TrimSpace(email.Email)
		if trimmed == "" {
			continue
		}
		if fallback == "" {
			fallback = trimmed
		}
		if email.Verified {
			return trimmed
		}
		if email.Primary {
			fallback = trimmed
		}
	}
	return fallback
}

func oauthDisplayName(name, fallback, emptyFallback string) string {
	name = strings.TrimSpace(name)
	if name != "" {
		return name
	}
	if fallback != "" {
		return fallback
	}
	return emptyFallback
}

func randomURLSafeString(size int) (string, error) {
	bytes := make([]byte, size)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(bytes), nil
}
func (s *Gateway) Exchange(ctx context.Context, provider OAuthProvider, code, state, verifier string) (domain.OAuthProfile, error) {
	ctx = context.WithValue(ctx, oauth2.HTTPClient, s.client)
	config := s.oauthConfig(provider)
	if config == nil || code == "" || state == "" || verifier == "" {
		return domain.OAuthProfile{}, ErrBadRequest
	}

	token, err := config.Exchange(ctx, code, oauth2.VerifierOption(verifier))
	if err != nil {
		return domain.OAuthProfile{}, fmt.Errorf("%w: exchange %s OAuth code: %v", ErrBadRequest, provider, err)
	}

	email, displayName, avatarURL, providerID, emailVerified, err := s.oauthUserInfo(ctx, provider, token)
	if err != nil {
		return domain.OAuthProfile{}, fmt.Errorf("%w: fetch %s user profile: %v", ErrBadRequest, provider, err)
	}
	if email == "" || providerID == "" {
		return domain.OAuthProfile{}, ErrBadRequest
	}

	return domain.OAuthProfile{Provider: provider, ProviderID: providerID, Email: email, DisplayName: displayName, AvatarURL: avatarURL, EmailVerified: emailVerified}, nil
}

func (s *Gateway) PollDevice(ctx context.Context, deviceCode string) (domain.OAuthProfile, DeviceAuthStatus, error) {
	ctx = context.WithValue(ctx, oauth2.HTTPClient, s.client)
	if s.GitHubOAuth == nil || strings.TrimSpace(deviceCode) == "" {
		return domain.OAuthProfile{}, "", ErrBadRequest
	}

	form := url.Values{}
	form.Set("client_id", s.GitHubOAuth.ClientID)
	form.Set("device_code", deviceCode)
	form.Set("grant_type", githubDeviceGrantType)

	var body struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		Scope       string `json:"scope"`
		Error       string `json:"error"`
		ErrorDesc   string `json:"error_description"`
	}
	if err := s.githubPostForm(ctx, s.GitHubOAuth.Endpoint.TokenURL, form, &body); err != nil {
		return domain.OAuthProfile{}, "", err
	}

	switch body.Error {
	case "authorization_pending":
		return domain.OAuthProfile{}, DeviceAuthPending, nil
	case "slow_down":
		return domain.OAuthProfile{}, DeviceAuthSlowDown, nil
	case "":
		// success path continues below
	default:
		// access_denied, expired_token, incorrect_device_code, etc.
		return domain.OAuthProfile{}, "", fmt.Errorf("%w: GitHub device login: %s", ErrBadRequest, oauthErrText(body.Error, body.ErrorDesc))
	}
	if body.AccessToken == "" {
		return domain.OAuthProfile{}, DeviceAuthPending, nil
	}

	token := &oauth2.Token{AccessToken: body.AccessToken, TokenType: body.TokenType}
	email, displayName, avatarURL, providerID, emailVerified, err := s.oauthUserInfo(ctx, OAuthProviderGitHub, token)
	if err != nil {
		return domain.OAuthProfile{}, "", fmt.Errorf("%w: fetch GitHub user profile: %v", ErrBadRequest, err)
	}
	if email == "" || providerID == "" {
		return domain.OAuthProfile{}, "", ErrBadRequest
	}

	return domain.OAuthProfile{Provider: OAuthProviderGitHub, ProviderID: providerID, Email: email, DisplayName: displayName, AvatarURL: avatarURL, EmailVerified: emailVerified}, DeviceAuthComplete, nil
}
