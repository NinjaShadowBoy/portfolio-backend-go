package service

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/ninjashadowboy/portfolio-backend-go/internal/auth"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/db"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/dto"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/email"
	"golang.org/x/oauth2"
)

var (
	ErrNotFound      = errors.New("resource not found")
	ErrUnauthorized  = errors.New("unauthorized")
	ErrForbidden     = errors.New("forbidden")
	ErrConflict      = errors.New("conflict")
	ErrBadRequest    = errors.New("bad request")
	ErrAlreadyExists = errors.New("already exists")
)

type OAuthProvider string

const (
	OAuthProviderGoogle OAuthProvider = "GOOGLE"
	OAuthProviderGitHub OAuthProvider = "GITHUB"
)

type Service interface {
	Login(ctx context.Context, email, password string) (dto.LoginResponse, error)
	Register(ctx context.Context, req dto.RegisterRequest) error
	StartOAuthLogin(provider OAuthProvider) (OAuthLoginStart, error)
	OAuthLogin(ctx context.Context, provider OAuthProvider, code, state, verifier string) (dto.LoginResponse, error)
	StartGitHubDeviceLogin(ctx context.Context) (DeviceLoginStart, error)
	PollGitHubDeviceLogin(ctx context.Context, deviceCode string) (dto.LoginResponse, DeviceAuthStatus, error)
	ListProjects(ctx context.Context) ([]dto.Project, error)
	GetProject(ctx context.Context, id int64) (dto.Project, error)
	CreateProject(ctx context.Context, req dto.ProjectCreate) (dto.Project, error)
	UpdateProject(ctx context.Context, id int64, req dto.ProjectUpdate) (dto.Project, error)
	DeleteProject(ctx context.Context, id int64) error
	GetProjectRatings(ctx context.Context, projectID int64) ([]dto.Rating, error)
	GetProjectAverageRating(ctx context.Context, projectID int64) (float64, error)
	GetProjectRatingCount(ctx context.Context, projectID int64) (int, error)
	GetProjectRatingDistribution(ctx context.Context, projectID int64) (map[int]int, error)
	ResetProjectRatings(ctx context.Context, projectID int64) error
	ListRatings(ctx context.Context) ([]dto.Rating, error)
	GetRating(ctx context.Context, id int64) (dto.Rating, error)
	GetRatingsByUserID(ctx context.Context, userID int64) ([]dto.Rating, error)
	CreateRating(ctx context.Context, userID int64, req dto.RatingCreate) (dto.Rating, error)
	UpdateRating(ctx context.Context, id int64, userID int64, req dto.RatingUpdate) (dto.Rating, error)
	DeleteRating(ctx context.Context, id int64, userID int64) error
	HasUserRatedProject(ctx context.Context, userID int64, projectID int64) (bool, error)
	SaveProjectPhoto(ctx context.Context, photoURL string, projectID int64) (dto.Photo, error)
	SaveProfilePhoto(ctx context.Context, photoURL string) (dto.Photo, error)
	DeletePhoto(ctx context.Context, id int64) error
	SubmitContact(ctx context.Context, req dto.Contact) (dto.ContactMessage, error)
	ListContactMessages(ctx context.Context) ([]dto.ContactMessage, error)
	ListUnreadContactMessages(ctx context.Context) ([]dto.ContactMessage, error)
	MarkContactRead(ctx context.Context, id int64) (dto.ContactMessage, error)
	MarkContactReplied(ctx context.Context, id int64) (dto.ContactMessage, error)
	DeleteContactMessage(ctx context.Context, id int64) error
	CreateComment(ctx context.Context, userID int64, req dto.CommentCreate) (dto.Comment, error)
	ListProjectComments(ctx context.Context, projectID int64) ([]dto.Comment, error)
	ListUserComments(ctx context.Context, userID int64) ([]dto.Comment, error)
	UpdateComment(ctx context.Context, id int64, userID int64, req dto.CommentUpdate) (dto.Comment, error)
	DeleteComment(ctx context.Context, id int64, userID int64) error
	CreateAnonymousUser(ctx context.Context) (int64, string, error)
	ResolveAnonToken(ctx context.Context, rawToken string) (int64, bool, error)
	GetIdentity(ctx context.Context, userID int64) (dto.Identity, error)
	ClaimAnonymousActivity(ctx context.Context, anonUserID, realUserID int64) error
}

type OAuthLoginStart struct {
	AuthURL  string
	State    string
	Verifier string
	Provider OAuthProvider
}

// DeviceLoginStart is the result of initiating an OAuth device-authorization flow.
type DeviceLoginStart struct {
	DeviceCode      string
	UserCode        string
	VerificationURI string
	ExpiresIn       int
	Interval        int
}

// DeviceAuthStatus reports where a polled device-flow login stands.
type DeviceAuthStatus string

const (
	DeviceAuthPending  DeviceAuthStatus = "pending"
	DeviceAuthSlowDown DeviceAuthStatus = "slow_down"
	DeviceAuthComplete DeviceAuthStatus = "complete"
)

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
	ID        string `json:"id"`
	Login     string `json:"login"`
	Name      string `json:"name"`
	Email     string `json:"email"`
	AvatarURL string `json:"avatar_url"`
}

type githubEmail struct {
	Email    string `json:"email"`
	Primary  bool   `json:"primary"`
	Verified bool   `json:"verified"`
}

type ListRatingAggregate struct {
	Count   int
	Average float64
}

type service struct {
	Q           *db.Queries
	JWT         *auth.JWT
	Emails      email.Sender
	GoogleOAuth *oauth2.Config
	GitHubOAuth *oauth2.Config
}

func New(q *db.Queries, jwt *auth.JWT, emails email.Sender, googleOAuth, githubOAuth *oauth2.Config) Service {
	return &service{Q: q, JWT: jwt, Emails: emails, GoogleOAuth: googleOAuth, GitHubOAuth: githubOAuth}
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

func (s *service) Login(ctx context.Context, email, password string) (dto.LoginResponse, error) {
	user, err := s.Q.GetUserByEmail(ctx, email)
	if errors.Is(err, sql.ErrNoRows) {
		return dto.LoginResponse{}, ErrUnauthorized
	}
	if err != nil {
		return dto.LoginResponse{}, err
	}
	if !auth.VerifyPassword(user.Pwd, password) {
		return dto.LoginResponse{}, ErrUnauthorized
	}

	updated, err := s.Q.UpdateUserLogin(ctx, user.ID, sql.NullTime{Time: time.Now(), Valid: true})
	if err != nil {
		return dto.LoginResponse{}, err
	}

	return s.JWT.GenerateLoginResponse(updated)
}

func (s *service) Register(ctx context.Context, req dto.RegisterRequest) error {
	exists, err := s.Q.UserExistsByEmail(ctx, req.Email)
	if err != nil {
		return err
	}
	if exists {
		return ErrAlreadyExists
	}

	hashed, err := auth.HashPassword(req.Password)
	if err != nil {
		return err
	}

	_, err = s.Q.CreateUser(ctx, req.Email, sql.NullString{String: hashed, Valid: true}, req.Name, "LOCAL", sql.NullString{}, sql.NullString{}, false, sql.NullTime{})
	return err
}

func (s *service) StartOAuthLogin(provider OAuthProvider) (OAuthLoginStart, error) {
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

func (s *service) OAuthLogin(ctx context.Context, provider OAuthProvider, code, state, verifier string) (dto.LoginResponse, error) {
	config := s.oauthConfig(provider)
	if config == nil || code == "" || state == "" || verifier == "" {
		return dto.LoginResponse{}, ErrBadRequest
	}

	token, err := config.Exchange(ctx, code, oauth2.VerifierOption(verifier))
	if err != nil {
		return dto.LoginResponse{}, fmt.Errorf("%w: exchange %s OAuth code: %v", ErrBadRequest, provider, err)
	}

	email, displayName, avatarURL, providerID, emailVerified, err := s.oauthUserInfo(ctx, provider, token)
	if err != nil {
		return dto.LoginResponse{}, fmt.Errorf("%w: fetch %s user profile: %v", ErrBadRequest, provider, err)
	}
	if email == "" || providerID == "" {
		return dto.LoginResponse{}, ErrBadRequest
	}

	user, err := s.upsertOAuthUser(ctx, string(provider), providerID, email, displayName, avatarURL, emailVerified)
	if err != nil {
		return dto.LoginResponse{}, err
	}

	updated, err := s.Q.UpdateUserLogin(ctx, user.ID, sql.NullTime{Time: time.Now(), Valid: true})
	if err != nil {
		return dto.LoginResponse{}, err
	}

	return s.JWT.GenerateLoginResponse(updated)
}

// StartGitHubDeviceLogin begins a GitHub device-authorization flow. Device flow
// only needs the client id (no secret), but we require the GitHub OAuth config to
// be present so an unconfigured provider stays disabled. The GitHub OAuth App must
// have "Device Flow" enabled in its settings.
func (s *service) StartGitHubDeviceLogin(ctx context.Context) (DeviceLoginStart, error) {
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
	if err := githubPostForm(ctx, githubDeviceCodeURL, form, &body); err != nil {
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
func (s *service) PollGitHubDeviceLogin(ctx context.Context, deviceCode string) (dto.LoginResponse, DeviceAuthStatus, error) {
	if s.GitHubOAuth == nil || strings.TrimSpace(deviceCode) == "" {
		return dto.LoginResponse{}, "", ErrBadRequest
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
	if err := githubPostForm(ctx, s.GitHubOAuth.Endpoint.TokenURL, form, &body); err != nil {
		return dto.LoginResponse{}, "", err
	}

	switch body.Error {
	case "authorization_pending":
		return dto.LoginResponse{}, DeviceAuthPending, nil
	case "slow_down":
		return dto.LoginResponse{}, DeviceAuthSlowDown, nil
	case "":
		// success path continues below
	default:
		// access_denied, expired_token, incorrect_device_code, etc.
		return dto.LoginResponse{}, "", fmt.Errorf("%w: GitHub device login: %s", ErrBadRequest, oauthErrText(body.Error, body.ErrorDesc))
	}
	if body.AccessToken == "" {
		return dto.LoginResponse{}, DeviceAuthPending, nil
	}

	token := &oauth2.Token{AccessToken: body.AccessToken, TokenType: body.TokenType}
	email, displayName, avatarURL, providerID, emailVerified, err := s.oauthUserInfo(ctx, OAuthProviderGitHub, token)
	if err != nil {
		return dto.LoginResponse{}, "", fmt.Errorf("%w: fetch GitHub user profile: %v", ErrBadRequest, err)
	}
	if email == "" || providerID == "" {
		return dto.LoginResponse{}, "", ErrBadRequest
	}

	user, err := s.upsertOAuthUser(ctx, string(OAuthProviderGitHub), providerID, email, displayName, avatarURL, emailVerified)
	if err != nil {
		return dto.LoginResponse{}, "", err
	}
	updated, err := s.Q.UpdateUserLogin(ctx, user.ID, sql.NullTime{Time: time.Now(), Valid: true})
	if err != nil {
		return dto.LoginResponse{}, "", err
	}

	resp, err := s.JWT.GenerateLoginResponse(updated)
	if err != nil {
		return dto.LoginResponse{}, "", err
	}
	return resp, DeviceAuthComplete, nil
}

// githubPostForm posts a form-encoded body to a GitHub OAuth endpoint and decodes
// the JSON response into out. GitHub returns form-encoded bodies unless asked for
// JSON via the Accept header.
func githubPostForm(ctx context.Context, endpoint string, form url.Values, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
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

func (s *service) oauthConfig(provider OAuthProvider) *oauth2.Config {
	switch provider {
	case OAuthProviderGoogle:
		return s.GoogleOAuth
	case OAuthProviderGitHub:
		return s.GitHubOAuth
	default:
		return nil
	}
}

func (s *service) oauthUserInfo(ctx context.Context, provider OAuthProvider, token *oauth2.Token) (email, displayName, avatarURL, providerID string, emailVerified bool, err error) {
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
		return email, oauthDisplayName(info.Name, info.Login, info.Login), strings.TrimSpace(info.AvatarURL), strings.TrimSpace(info.ID), verified, nil
	default:
		return "", "", "", "", false, ErrBadRequest
	}
}

func (s *service) upsertOAuthUser(ctx context.Context, provider, providerID, email, displayName, avatarURL string, emailVerified bool) (db.User, error) {
	user, err := s.Q.GetUserByProvider(ctx, provider, sql.NullString{String: providerID, Valid: true})
	if err == nil {
		user, err = s.Q.UpdateUserOAuthProfile(ctx, user.ID, displayName, provider, sql.NullString{String: providerID, Valid: true}, nullableStringFrom(avatarURL), emailVerified)
		if err != nil {
			return db.User{}, err
		}
		return user, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return db.User{}, err
	}

	exists, err := s.Q.UserExistsByEmail(ctx, email)
	if err != nil {
		return db.User{}, err
	}
	if exists {
		user, err = s.Q.GetUserByEmail(ctx, email)
		if err != nil {
			return db.User{}, err
		}
		return s.Q.UpdateUserOAuthProfile(ctx, user.ID, displayName, provider, sql.NullString{String: providerID, Valid: true}, nullableStringFrom(avatarURL), emailVerified)
	}

	return s.Q.CreateUser(ctx, email, sql.NullString{}, displayName, provider, sql.NullString{String: providerID, Valid: true}, nullableStringFrom(avatarURL), emailVerified, sql.NullTime{})
}

func (s *service) getGoogleUserInfo(ctx context.Context, token *oauth2.Token) (googleUserInfo, error) {
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

func (s *service) getGitHubUserInfo(ctx context.Context, token *oauth2.Token) (githubUserInfo, error) {
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

func nullableStringFrom(value string) sql.NullString {
	value = strings.TrimSpace(value)
	if value == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: value, Valid: true}
}

func (s *service) ListProjects(ctx context.Context) ([]dto.Project, error) {
	projects, err := s.Q.ListProjects(ctx)
	if err != nil {
		return nil, err
	}
	if len(projects) == 0 {
		return []dto.Project{}, nil
	}

	ids := make([]int64, len(projects))
	for i, p := range projects {
		ids[i] = p.ID
	}

	techRows, err := s.Q.ListAllTechnologiesByProjectID(ctx, ids)
	if err != nil {
		return nil, err
	}
	technologies := make(map[int64][]string, len(ids))
	for _, row := range techRows {
		technologies[row.ProjectID] = append(technologies[row.ProjectID], row.Technology)
	}

	photoRows, err := s.Q.ListPhotosByProjectIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	photosByProject := make(map[int64][]db.Photo, len(ids))
	for _, photo := range photoRows {
		if photo.ProjectID.Valid {
			photosByProject[photo.ProjectID.Int64] = append(photosByProject[photo.ProjectID.Int64], photo)
		}
	}

	aggRows, err := s.Q.ListRatingAggregatesByProjectIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	aggregates := make(map[int64]ListRatingAggregate, len(aggRows))
	for _, row := range aggRows {
		aggregates[row.ProjectID] = ListRatingAggregate{
			Count:   int(row.Count),
			Average: toFloat64(row.Average),
		}
	}

	items := make([]dto.Project, 0, len(projects))
	for _, project := range projects {
		items = append(items, assembleProjectDTO(project, technologies[project.ID], photosByProject[project.ID], aggregates[project.ID]))
	}
	return items, nil
}

func (s *service) GetProject(ctx context.Context, id int64) (dto.Project, error) {
	project, err := s.Q.GetProject(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return dto.Project{}, ErrNotFound
	}
	if err != nil {
		return dto.Project{}, err
	}

	technologies, _ := s.Q.ListTechnologiesByProjectID(ctx, project.ID)
	photos, _ := s.Q.ListPhotosByProjectID(ctx, sql.NullInt64{Int64: project.ID, Valid: true})
	countRaw, _ := s.Q.RatingCountByProjectID(ctx, project.ID)
	averageRaw, _ := s.Q.RatingAverageByProjectID(ctx, project.ID)

	return assembleProjectDTO(project, technologies, photos, ListRatingAggregate{
		Count:   int(countRaw),
		Average: toFloat64(averageRaw),
	}), nil
}

func (s *service) CreateProject(ctx context.Context, req dto.ProjectCreate) (dto.Project, error) {
	project, err := s.Q.CreateProject(ctx, req.Name, req.Description, req.Featured,
		nullableString(req.GithubLink), nullableString(req.Challenges), nullableString(req.WhatILearned))
	if err != nil {
		return dto.Project{}, err
	}
	if err := s.replaceProjectTechnologies(ctx, project.ID, req.Technologies); err != nil {
		return dto.Project{}, err
	}
	return s.GetProject(ctx, project.ID)
}

func (s *service) UpdateProject(ctx context.Context, id int64, req dto.ProjectUpdate) (dto.Project, error) {
	existing, err := s.Q.GetProject(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return dto.Project{}, ErrNotFound
	}
	if err != nil {
		return dto.Project{}, err
	}

	name := existing.Name
	if req.Name != nil {
		name = *req.Name
	}
	description := existing.Description
	if req.Description != nil {
		description = *req.Description
	}
	featured := existing.Featured
	if req.Featured != nil {
		featured = *req.Featured
	}
	githubLink := existing.GithubLink
	if req.GithubLink != nil {
		githubLink = nullableString(req.GithubLink)
	}
	challenges := existing.Challenges
	if req.Challenges != nil {
		challenges = nullableString(req.Challenges)
	}
	whatILearned := existing.WhatILearned
	if req.WhatILearned != nil {
		whatILearned = nullableString(req.WhatILearned)
	}

	_, err = s.Q.UpdateProject(ctx, id, name, description, featured, githubLink, challenges, whatILearned)
	if err != nil {
		return dto.Project{}, err
	}
	if req.Technologies != nil {
		if err := s.replaceProjectTechnologies(ctx, id, *req.Technologies); err != nil {
			return dto.Project{}, err
		}
	}
	return s.GetProject(ctx, id)
}

func (s *service) DeleteProject(ctx context.Context, id int64) error {
	if err := s.requireProject(ctx, id); err != nil {
		return err
	}
	if err := s.Q.DeleteRatingsByProjectID(ctx, id); err != nil {
		return err
	}
	return s.Q.DeleteProject(ctx, id)
}

func (s *service) GetProjectRatings(ctx context.Context, projectID int64) ([]dto.Rating, error) {
	if err := s.requireProject(ctx, projectID); err != nil {
		return nil, err
	}
	ratings, err := s.Q.ListRatingsByProjectID(ctx, projectID)
	if err != nil {
		return nil, err
	}
	return s.ratingsDTO(ctx, ratings)
}

func (s *service) GetProjectAverageRating(ctx context.Context, projectID int64) (float64, error) {
	if err := s.requireProject(ctx, projectID); err != nil {
		return 0, err
	}
	averageRaw, err := s.Q.RatingAverageByProjectID(ctx, projectID)
	if err != nil {
		return 0, err
	}
	return toFloat64(averageRaw), nil
}

func (s *service) GetProjectRatingCount(ctx context.Context, projectID int64) (int, error) {
	if err := s.requireProject(ctx, projectID); err != nil {
		return 0, err
	}
	count, err := s.Q.RatingCountByProjectID(ctx, projectID)
	if err != nil {
		return 0, err
	}
	return int(count), nil
}

func (s *service) GetProjectRatingDistribution(ctx context.Context, projectID int64) (map[int]int, error) {
	if err := s.requireProject(ctx, projectID); err != nil {
		return nil, err
	}
	rows, err := s.Q.RatingDistributionByProjectID(ctx, projectID)
	if err != nil {
		return nil, err
	}
	distribution := map[int]int{1: 0, 2: 0, 3: 0, 4: 0, 5: 0}
	for _, row := range rows {
		distribution[int(row.Rating)] = int(row.Count)
	}
	return distribution, nil
}

func (s *service) ResetProjectRatings(ctx context.Context, projectID int64) error {
	if err := s.requireProject(ctx, projectID); err != nil {
		return err
	}
	return s.Q.DeleteRatingsByProjectID(ctx, projectID)
}

func (s *service) ListRatings(ctx context.Context) ([]dto.Rating, error) {
	ratings, err := s.Q.ListRatings(ctx)
	if err != nil {
		return nil, err
	}
	return s.ratingsDTO(ctx, ratings)
}

func (s *service) GetRating(ctx context.Context, id int64) (dto.Rating, error) {
	rating, err := s.Q.GetRating(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return dto.Rating{}, ErrNotFound
	}
	if err != nil {
		return dto.Rating{}, err
	}
	return s.ratingDTO(ctx, rating)
}

func (s *service) GetRatingsByUserID(ctx context.Context, userID int64) ([]dto.Rating, error) {
	ratings, err := s.Q.ListRatingsByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	return s.ratingsDTO(ctx, ratings)
}

func (s *service) CreateRating(ctx context.Context, userID int64, req dto.RatingCreate) (dto.Rating, error) {
	if req.Rating < 1 || req.Rating > 5 {
		return dto.Rating{}, ErrBadRequest
	}
	if _, err := s.Q.GetUserByID(ctx, userID); errors.Is(err, sql.ErrNoRows) {
		return dto.Rating{}, ErrNotFound
	} else if err != nil {
		return dto.Rating{}, err
	}
	if err := s.requireProject(ctx, req.ProjectID); err != nil {
		return dto.Rating{}, err
	}

	// A star is one-per-identity-per-project: upsert rather than reject repeats.
	rating, err := s.Q.UpsertRating(ctx, int32(req.Rating), userID, req.ProjectID)
	if err != nil {
		return dto.Rating{}, err
	}
	return s.ratingDTO(ctx, rating)
}

func (s *service) UpdateRating(ctx context.Context, id int64, userID int64, req dto.RatingUpdate) (dto.Rating, error) {
	if req.Rating != nil && (*req.Rating < 1 || *req.Rating > 5) {
		return dto.Rating{}, ErrBadRequest
	}
	rating, err := s.Q.GetRating(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return dto.Rating{}, ErrNotFound
	}
	if err != nil {
		return dto.Rating{}, err
	}
	if rating.UserID != userID {
		return dto.Rating{}, ErrForbidden
	}

	ratingValue := rating.Rating
	if req.Rating != nil {
		ratingValue = int32(*req.Rating)
	}

	updated, err := s.Q.UpdateRating(ctx, id, ratingValue)
	if err != nil {
		return dto.Rating{}, err
	}
	return s.ratingDTO(ctx, updated)
}

func (s *service) DeleteRating(ctx context.Context, id int64, userID int64) error {
	rows, err := s.Q.DeleteRating(ctx, id, userID)
	if err != nil {
		return err
	}
	if rows == 0 {
		exists, err := s.Q.GetRating(ctx, id)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err == nil && exists.UserID != userID {
			return ErrForbidden
		}
		return ErrNotFound
	}
	return nil
}

func (s *service) HasUserRatedProject(ctx context.Context, userID int64, projectID int64) (bool, error) {
	return s.Q.HasUserRatedProject(ctx, userID, projectID)
}

func (s *service) SaveProjectPhoto(ctx context.Context, photoURL string, projectID int64) (dto.Photo, error) {
	normalized, err := NormalizePhotoURL(photoURL)
	if err != nil {
		return dto.Photo{}, err
	}
	if err := s.requireProject(ctx, projectID); err != nil {
		return dto.Photo{}, err
	}
	photo, err := s.Q.SaveProjectPhoto(ctx, normalized, sql.NullInt64{Int64: projectID, Valid: true})
	if err != nil {
		return dto.Photo{}, err
	}
	return dto.Photo{ID: photo.ID, PhotoURL: photo.PhotoUrl, ProjectID: photo.ProjectID.Int64}, nil
}

func (s *service) SaveProfilePhoto(ctx context.Context, photoURL string) (dto.Photo, error) {
	normalized, err := NormalizePhotoURL(photoURL)
	if err != nil {
		return dto.Photo{}, err
	}
	if err := s.Q.DeleteOrphanProfilePhotos(ctx); err != nil {
		return dto.Photo{}, err
	}
	photo, err := s.Q.SaveProfilePhoto(ctx, normalized)
	if err != nil {
		return dto.Photo{}, err
	}
	return dto.Photo{ID: photo.ID, PhotoURL: photo.PhotoUrl, ProjectID: 0}, nil
}

func (s *service) DeletePhoto(ctx context.Context, id int64) error {
	rows, err := s.Q.DeletePhoto(ctx, id)
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *service) SubmitContact(ctx context.Context, req dto.Contact) (dto.ContactMessage, error) {
	if strings.TrimSpace(req.Name) == "" || strings.TrimSpace(req.Email) == "" || strings.TrimSpace(req.Message) == "" {
		return dto.ContactMessage{}, ErrBadRequest
	}
	message, err := s.Q.CreateContactMessage(ctx, req.Name, req.Email, req.Message)
	if err != nil {
		return dto.ContactMessage{}, err
	}

	if s.Emails != nil {
		_ = s.Emails.SendContactFormNotification(ctx, req.Name, req.Email, req.Message)
		_ = s.Emails.SendContactConfirmation(ctx, req.Name, req.Email)
	}

	return contactMessageDTO(message), nil
}

func (s *service) ListContactMessages(ctx context.Context) ([]dto.ContactMessage, error) {
	messages, err := s.Q.ListContactMessages(ctx)
	if err != nil {
		return nil, err
	}
	return contactMessagesDTO(messages), nil
}

func (s *service) ListUnreadContactMessages(ctx context.Context) ([]dto.ContactMessage, error) {
	messages, err := s.Q.ListUnreadContactMessages(ctx)
	if err != nil {
		return nil, err
	}
	return contactMessagesDTO(messages), nil
}

func (s *service) MarkContactRead(ctx context.Context, id int64) (dto.ContactMessage, error) {
	message, err := s.Q.MarkContactMessageRead(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return dto.ContactMessage{}, ErrNotFound
	}
	if err != nil {
		return dto.ContactMessage{}, err
	}
	return contactMessageDTO(message), nil
}

func (s *service) MarkContactReplied(ctx context.Context, id int64) (dto.ContactMessage, error) {
	message, err := s.Q.MarkContactMessageReplied(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return dto.ContactMessage{}, ErrNotFound
	}
	if err != nil {
		return dto.ContactMessage{}, err
	}
	return contactMessageDTO(message), nil
}

func (s *service) DeleteContactMessage(ctx context.Context, id int64) error {
	rows, err := s.Q.DeleteContactMessage(ctx, id)
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

func assembleProjectDTO(project db.Project, technologies []string, photos []db.Photo, agg ListRatingAggregate) dto.Project {
	photoURLs := make([]string, 0, len(photos))
	photoDTOs := make([]dto.Photo, 0, len(photos))
	for _, photo := range photos {
		photoURLs = append(photoURLs, photo.PhotoUrl)
		photoDTOs = append(photoDTOs, dto.Photo{ID: photo.ID, PhotoURL: photo.PhotoUrl, ProjectID: project.ID})
	}

	return dto.Project{
		ID:            project.ID,
		Name:          project.Name,
		Description:   project.Description,
		Technologies:  technologies,
		GithubLink:    dto.NullString(project.GithubLink),
		Challenges:    dto.NullString(project.Challenges),
		WhatILearned:  dto.NullString(project.WhatILearned),
		Featured:      project.Featured,
		CreatedAt:     project.CreatedAt,
		UpdatedAt:     project.UpdatedAt,
		AverageRating: agg.Average,
		TotalRatings:  agg.Count,
		PhotoUrls:     photoURLs,
		Photos:        photoDTOs,
	}
}

func toFloat64(v interface{}) float64 {
	switch v := v.(type) {
	case float64:
		return v
	case float32:
		return float64(v)
	case int64:
		return float64(v)
	case int:
		return float64(v)
	default:
		return 0
	}
}

func (s *service) ratingsDTO(ctx context.Context, ratings []db.Rating) ([]dto.Rating, error) {
	if len(ratings) == 0 {
		return []dto.Rating{}, nil
	}

	userIDs := make([]int64, 0, len(ratings))
	seen := make(map[int64]struct{}, len(ratings))
	for _, rating := range ratings {
		if _, ok := seen[rating.UserID]; ok {
			continue
		}
		seen[rating.UserID] = struct{}{}
		userIDs = append(userIDs, rating.UserID)
	}

	users, err := s.Q.GetUsersByIDs(ctx, userIDs)
	if err != nil {
		return nil, err
	}
	names := make(map[int64]string, len(users))
	for _, user := range users {
		names[user.ID] = user.Name
	}

	items := make([]dto.Rating, 0, len(ratings))
	for _, rating := range ratings {
		items = append(items, ratingToDTO(rating, names[rating.UserID]))
	}
	return items, nil
}

func (s *service) ratingDTO(ctx context.Context, rating db.Rating) (dto.Rating, error) {
	user, err := s.Q.GetUserByID(ctx, rating.UserID)
	if err != nil {
		return dto.Rating{}, err
	}
	return ratingToDTO(rating, user.Name), nil
}

func ratingToDTO(rating db.Rating, userName string) dto.Rating {
	return dto.Rating{
		ID:        rating.ID,
		UserID:    rating.UserID,
		UserName:  userName,
		ProjectID: rating.ProjectID,
		Rating:    int(rating.Rating),
		CreatedAt: rating.CreatedAt,
		UpdatedAt: rating.UpdatedAt,
	}
}

func contactMessageDTO(message db.ContactMessage) dto.ContactMessage {
	return dto.ContactMessage{
		ID:          message.ID,
		Name:        message.Name,
		Email:       message.Email,
		Message:     message.Message,
		SubmittedAt: message.SubmittedAt,
		IsRead:      message.IsRead,
		IsReplied:   message.IsReplied,
	}
}

func contactMessagesDTO(messages []db.ContactMessage) []dto.ContactMessage {
	items := make([]dto.ContactMessage, 0, len(messages))
	for _, message := range messages {
		items = append(items, contactMessageDTO(message))
	}
	return items
}

func (s *service) requireProject(ctx context.Context, projectID int64) error {
	exists, err := s.Q.ProjectExists(ctx, projectID)
	if err != nil {
		return err
	}
	if !exists {
		return ErrNotFound
	}
	return nil
}

func (s *service) replaceProjectTechnologies(ctx context.Context, projectID int64, technologies []string) error {
	if err := s.Q.DeleteProjectTechnologies(ctx, projectID); err != nil {
		return err
	}
	if len(technologies) > 0 {
		return s.Q.InsertProjectTechnologies(ctx, projectID, technologies)
	}
	return nil
}

func NormalizePhotoURL(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", fmt.Errorf("photo URL must not be blank: %w", ErrBadRequest)
	}

	parsed, err := url.Parse(trimmed)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("photo URL must include a valid scheme and host: %w", ErrBadRequest)
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", fmt.Errorf("photo URL must use HTTP or HTTPS: %w", ErrBadRequest)
	}
	return trimmed, nil
}

func nullableString(value *string) sql.NullString {
	if value == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: *value, Valid: true}
}
