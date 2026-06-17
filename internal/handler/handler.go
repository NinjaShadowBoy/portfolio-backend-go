package handler

import (
	"crypto/subtle"
	"errors"
	"net/http"
	"net/url"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/auth"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/dto"

	"github.com/ninjashadowboy/portfolio-backend-go/internal/middleware"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/service"
)

type API interface {
	Routes(r chi.Router)
}

type api struct {
	svc              service.Service
	jwt              *auth.JWT
	oauthRedirectURI string
}

type oauthCookieConfig struct {
	StateName    string
	VerifierName string
	CallbackPath string
}

var oauthCookieConfigs = map[service.OAuthProvider]oauthCookieConfig{
	service.OAuthProviderGoogle: {
		StateName:    "oauth_google_state",
		VerifierName: "oauth_google_pkce_verifier",
		CallbackPath: "/portfolio/auth/google/callback",
	},
	service.OAuthProviderGitHub: {
		StateName:    "oauth_github_state",
		VerifierName: "oauth_github_pkce_verifier",
		CallbackPath: "/portfolio/auth/github/callback",
	},
}

const oauthCookieMaxAge = 10 * 60

func New(svc service.Service, jwt *auth.JWT, oauthRedirectURI string) API {
	return &api{svc: svc, jwt: jwt, oauthRedirectURI: oauthRedirectURI}
}

func (h *api) Routes(r chi.Router) {
	r.Post("/auth/login", h.Login)
	r.Post("/auth/register", h.Register)
	r.Get("/auth/google", h.OAuthLogin(service.OAuthProviderGoogle))
	r.Get("/auth/google/callback", h.OAuthCallback(service.OAuthProviderGoogle))
	r.Get("/auth/github", h.OAuthLogin(service.OAuthProviderGitHub))
	r.Get("/auth/github/callback", h.OAuthCallback(service.OAuthProviderGitHub))

	r.Get("/projects", h.ListProjects)
	r.Get("/projects/{id}", h.GetProject)
	r.Post("/projects", h.CreateProject)
	r.Put("/projects/{id}", h.UpdateProject)
	r.Delete("/projects/{id}", h.DeleteProject)
	r.Get("/projects/{id}/ratings", h.GetProjectRatings)
	r.Get("/projects/{id}/ratings/average", h.GetProjectAverageRating)
	r.Get("/projects/{id}/ratings/count", h.GetProjectRatingCount)
	r.Get("/projects/{id}/ratings/distribution", h.GetProjectRatingDistribution)
	r.Delete("/projects/{id}/ratings", h.ResetProjectRatings)

	r.Get("/ratings", h.ListRatings)
	r.Get("/ratings/{id}", h.GetRating)
	r.Get("/ratings/project/{projectId}", h.GetRatingsByProject)
	r.Get("/ratings/my-ratings", h.GetMyRatings)
	r.Get("/ratings/project/{projectId}/distribution", h.GetRatingDistribution)
	r.Get("/ratings/project/{projectId}/average", h.GetAverageRating)
	r.Get("/ratings/project/{projectId}/count", h.GetRatingCount)
	r.Get("/ratings/project/{projectId}/has-rated", h.HasUserRatedProject)
	r.Post("/ratings", h.CreateRating)
	r.Put("/ratings/{id}", h.UpdateRating)
	r.Delete("/ratings/{id}", h.DeleteRating)
	r.Delete("/ratings/project/{projectId}", h.DeleteAllRatingsForProject)

	r.With(middleware.Auth(h.jwt)).Post("/photos/{projectId}", h.SaveProjectPhoto)
	r.With(middleware.Auth(h.jwt)).Post("/photos/profile", h.SaveProfilePhoto)
	r.With(middleware.Auth(h.jwt), middleware.Admin(h.jwt)).Delete("/photos/{photoId}", h.DeletePhoto)

	r.Post("/contact", h.SubmitContact)
	r.With(middleware.Auth(h.jwt), middleware.Admin(h.jwt)).Get("/contact", h.ListContactMessages)
	r.With(middleware.Auth(h.jwt), middleware.Admin(h.jwt)).Get("/contact/unread", h.ListUnreadContactMessages)
	r.With(middleware.Auth(h.jwt), middleware.Admin(h.jwt)).Patch("/contact/{id}/read", h.MarkContactRead)
	r.With(middleware.Auth(h.jwt), middleware.Admin(h.jwt)).Patch("/contact/{id}/replied", h.MarkContactReplied)
	r.With(middleware.Auth(h.jwt), middleware.Admin(h.jwt)).Delete("/contact/{id}", h.DeleteContactMessage)

	r.Get("/cron", h.Cron)
}

func (h *api) Login(w http.ResponseWriter, r *http.Request) {
	var req dto.LoginRequest
	if err := middleware.DecodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	resp, err := h.svc.Login(r.Context(), req.Email, req.Password)
	if err != nil {
		writeError(w, err)
		return
	}
	middleware.WriteJSON(w, http.StatusOK, resp)
}

func (h *api) Register(w http.ResponseWriter, r *http.Request) {
	var req dto.RegisterRequest
	if err := middleware.DecodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	if err := h.svc.Register(r.Context(), req); err != nil {
		writeError(w, err)
		return
	}
	middleware.WriteJSON(w, http.StatusCreated, dto.MessageResponse{Message: "User created successfully"})
}

func (h *api) OAuthLogin(provider service.OAuthProvider) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start, err := h.svc.StartOAuthLogin(provider)
		if err != nil {
			redirectOAuth(w, r, h.oauthRedirectURI, "", "OAuth2 is not configured")
			return
		}

		cookies := oauthCookieConfigFor(provider)
		setOAuthCookie(w, cookies.StateName, start.State, cookies.CallbackPath)
		setOAuthCookie(w, cookies.VerifierName, start.Verifier, cookies.CallbackPath)
		http.Redirect(w, r, start.AuthURL, http.StatusTemporaryRedirect)
	}
}

func (h *api) OAuthCallback(provider service.OAuthProvider) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookies := oauthCookieConfigFor(provider)
		if errMsg := r.URL.Query().Get("error"); errMsg != "" {
			redirectOAuth(w, r, h.oauthRedirectURI, "", errMsg)
			return
		}

		stateCookie, err := r.Cookie(cookies.StateName)
		if err != nil {
			redirectOAuth(w, r, h.oauthRedirectURI, "", "OAuth2 state is missing")
			return
		}
		verifierCookie, err := r.Cookie(cookies.VerifierName)
		if err != nil {
			redirectOAuth(w, r, h.oauthRedirectURI, "", "OAuth2 PKCE verifier is missing")
			return
		}

		state := r.URL.Query().Get("state")
		code := r.URL.Query().Get("code")
		if !secureEqual(stateCookie.Value, state) {
			redirectOAuth(w, r, h.oauthRedirectURI, "", "OAuth2 state mismatch")
			return
		}

		resp, err := h.svc.OAuthLogin(r.Context(), provider, code, stateCookie.Value, verifierCookie.Value)
		clearOAuthCookie(w, cookies.StateName, cookies.VerifierName, cookies.CallbackPath)
		if err != nil {
			redirectOAuth(w, r, h.oauthRedirectURI, "", "Authentication failed")
			return
		}

		redirectOAuth(w, r, h.oauthRedirectURI, resp.Token, "")
	}
}

func oauthCookieConfigFor(provider service.OAuthProvider) oauthCookieConfig {
	if config, ok := oauthCookieConfigs[provider]; ok {
		return config
	}
	return oauthCookieConfigs[service.OAuthProviderGoogle]
}

func (h *api) ListProjects(w http.ResponseWriter, r *http.Request) {
	projects, err := h.svc.ListProjects(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	middleware.WriteJSON(w, http.StatusOK, projects)
}

func (h *api) GetProject(w http.ResponseWriter, r *http.Request) {
	id, ok := writeID(w, r, "id")
	if !ok {
		return
	}
	project, err := h.svc.GetProject(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	middleware.WriteJSON(w, http.StatusOK, project)
}

func (h *api) CreateProject(w http.ResponseWriter, r *http.Request) {
	var req dto.ProjectCreate
	if err := middleware.DecodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	project, err := h.svc.CreateProject(r.Context(), req)
	if err != nil {
		writeError(w, err)
		return
	}
	middleware.WriteJSON(w, http.StatusCreated, project)
}

func (h *api) UpdateProject(w http.ResponseWriter, r *http.Request) {
	id, ok := writeID(w, r, "id")
	if !ok {
		return
	}
	var req dto.ProjectUpdate
	if err := middleware.DecodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	project, err := h.svc.UpdateProject(r.Context(), id, req)
	if err != nil {
		writeError(w, err)
		return
	}
	middleware.WriteJSON(w, http.StatusOK, project)
}

func (h *api) DeleteProject(w http.ResponseWriter, r *http.Request) {
	id, ok := writeID(w, r, "id")
	if !ok {
		return
	}
	if err := h.svc.DeleteProject(r.Context(), id); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *api) GetProjectRatings(w http.ResponseWriter, r *http.Request) {
	id, ok := writeID(w, r, "id")
	if !ok {
		return
	}
	ratings, err := h.svc.GetProjectRatings(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	middleware.WriteJSON(w, http.StatusOK, ratings)
}

func (h *api) GetProjectAverageRating(w http.ResponseWriter, r *http.Request) {
	id, ok := writeID(w, r, "id")
	if !ok {
		return
	}
	average, err := h.svc.GetProjectAverageRating(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	middleware.WriteJSON(w, http.StatusOK, map[string]float64{"averageRating": average})
}

func (h *api) GetProjectRatingCount(w http.ResponseWriter, r *http.Request) {
	id, ok := writeID(w, r, "id")
	if !ok {
		return
	}
	count, err := h.svc.GetProjectRatingCount(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	middleware.WriteJSON(w, http.StatusOK, map[string]int{"count": count})
}

func (h *api) GetProjectRatingDistribution(w http.ResponseWriter, r *http.Request) {
	id, ok := writeID(w, r, "id")
	if !ok {
		return
	}
	distribution, err := h.svc.GetProjectRatingDistribution(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	middleware.WriteJSON(w, http.StatusOK, distribution)
}

func (h *api) ResetProjectRatings(w http.ResponseWriter, r *http.Request) {
	id, ok := writeID(w, r, "id")
	if !ok {
		return
	}
	if err := h.svc.ResetProjectRatings(r.Context(), id); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *api) ListRatings(w http.ResponseWriter, r *http.Request) {
	ratings, err := h.svc.ListRatings(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	middleware.WriteJSON(w, http.StatusOK, ratings)
}

func (h *api) GetRating(w http.ResponseWriter, r *http.Request) {
	id, ok := writeID(w, r, "id")
	if !ok {
		return
	}
	rating, err := h.svc.GetRating(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	middleware.WriteJSON(w, http.StatusOK, rating)
}

func (h *api) GetRatingsByProject(w http.ResponseWriter, r *http.Request) {
	projectID, ok := writeID(w, r, "projectId")
	if !ok {
		return
	}
	ratings, err := h.svc.GetRatingsByProjectID(r.Context(), projectID)
	if err != nil {
		writeError(w, err)
		return
	}
	middleware.WriteJSON(w, http.StatusOK, ratings)
}

func (h *api) GetMyRatings(w http.ResponseWriter, r *http.Request) {
	claims := middleware.ClaimsFromContext(r)
	ratings, err := h.svc.GetRatingsByUserID(r.Context(), claims.UserID)
	if err != nil {
		writeError(w, err)
		return
	}
	middleware.WriteJSON(w, http.StatusOK, ratings)
}

func (h *api) GetRatingDistribution(w http.ResponseWriter, r *http.Request) {
	projectID, ok := writeID(w, r, "projectId")
	if !ok {
		return
	}
	distribution, err := h.svc.GetProjectRatingDistribution(r.Context(), projectID)
	if err != nil {
		writeError(w, err)
		return
	}
	middleware.WriteJSON(w, http.StatusOK, distribution)
}

func (h *api) GetAverageRating(w http.ResponseWriter, r *http.Request) {
	projectID, ok := writeID(w, r, "projectId")
	if !ok {
		return
	}
	average, err := h.svc.GetProjectAverageRating(r.Context(), projectID)
	if err != nil {
		writeError(w, err)
		return
	}
	middleware.WriteJSON(w, http.StatusOK, map[string]float64{"averageRating": average})
}

func (h *api) GetRatingCount(w http.ResponseWriter, r *http.Request) {
	projectID, ok := writeID(w, r, "projectId")
	if !ok {
		return
	}
	count, err := h.svc.GetProjectRatingCount(r.Context(), projectID)
	if err != nil {
		writeError(w, err)
		return
	}
	middleware.WriteJSON(w, http.StatusOK, map[string]int{"count": count})
}

func (h *api) HasUserRatedProject(w http.ResponseWriter, r *http.Request) {
	claims := middleware.ClaimsFromContext(r)
	projectID, ok := writeID(w, r, "projectId")
	if !ok {
		return
	}
	hasRated, err := h.svc.HasUserRatedProject(r.Context(), claims.UserID, projectID)
	if err != nil {
		writeError(w, err)
		return
	}
	middleware.WriteJSON(w, http.StatusOK, map[string]bool{"hasRated": hasRated})
}

func (h *api) CreateRating(w http.ResponseWriter, r *http.Request) {
	claims := middleware.ClaimsFromContext(r)
	var req dto.RatingCreate
	if err := middleware.DecodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	rating, err := h.svc.CreateRating(r.Context(), claims.UserID, req)
	if err != nil {
		writeError(w, err)
		return
	}
	middleware.WriteJSON(w, http.StatusCreated, rating)
}

func (h *api) UpdateRating(w http.ResponseWriter, r *http.Request) {
	claims := middleware.ClaimsFromContext(r)
	id, ok := writeID(w, r, "id")
	if !ok {
		return
	}
	var req dto.RatingUpdate
	if err := middleware.DecodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	rating, err := h.svc.UpdateRating(r.Context(), id, claims.UserID, req)
	if err != nil {
		writeError(w, err)
		return
	}
	middleware.WriteJSON(w, http.StatusOK, rating)
}

func (h *api) DeleteRating(w http.ResponseWriter, r *http.Request) {
	claims := middleware.ClaimsFromContext(r)
	id, ok := writeID(w, r, "id")
	if !ok {
		return
	}
	if err := h.svc.DeleteRating(r.Context(), id, claims.UserID); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *api) DeleteAllRatingsForProject(w http.ResponseWriter, r *http.Request) {
	projectID, ok := writeID(w, r, "projectId")
	if !ok {
		return
	}
	if err := h.svc.DeleteAllRatingsByProjectID(r.Context(), projectID); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *api) SaveProjectPhoto(w http.ResponseWriter, r *http.Request) {
	projectID, ok := writeID(w, r, "projectId")
	if !ok {
		return
	}
	var req dto.PhotoCreate
	if err := middleware.DecodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	photo, err := h.svc.SaveProjectPhoto(r.Context(), req.PhotoURL, projectID)
	if err != nil {
		writeError(w, err)
		return
	}
	middleware.WriteJSON(w, http.StatusCreated, photo)
}

func (h *api) SaveProfilePhoto(w http.ResponseWriter, r *http.Request) {
	var req dto.PhotoCreate
	if err := middleware.DecodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	photo, err := h.svc.SaveProfilePhoto(r.Context(), req.PhotoURL)
	if err != nil {
		writeError(w, err)
		return
	}
	middleware.WriteJSON(w, http.StatusCreated, photo)
}

func (h *api) DeletePhoto(w http.ResponseWriter, r *http.Request) {
	id, ok := writeID(w, r, "photoId")
	if !ok {
		return
	}
	if err := h.svc.DeletePhoto(r.Context(), id); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *api) SubmitContact(w http.ResponseWriter, r *http.Request) {
	var req dto.Contact
	if err := middleware.DecodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	message, err := h.svc.SubmitContact(r.Context(), req)
	if err != nil {
		writeError(w, err)
		return
	}
	middleware.WriteJSON(w, http.StatusCreated, dto.ContactResponse{Success: true, Message: "Thank you for your message! I'll get back to you soon.", Data: &message})
}

func (h *api) ListContactMessages(w http.ResponseWriter, r *http.Request) {
	messages, err := h.svc.ListContactMessages(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	middleware.WriteJSON(w, http.StatusOK, messages)
}

func (h *api) ListUnreadContactMessages(w http.ResponseWriter, r *http.Request) {
	messages, err := h.svc.ListUnreadContactMessages(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	middleware.WriteJSON(w, http.StatusOK, messages)
}

func (h *api) MarkContactRead(w http.ResponseWriter, r *http.Request) {
	id, ok := writeID(w, r, "id")
	if !ok {
		return
	}
	message, err := h.svc.MarkContactRead(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	middleware.WriteJSON(w, http.StatusOK, message)
}

func (h *api) MarkContactReplied(w http.ResponseWriter, r *http.Request) {
	id, ok := writeID(w, r, "id")
	if !ok {
		return
	}
	message, err := h.svc.MarkContactReplied(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	middleware.WriteJSON(w, http.StatusOK, message)
}

func (h *api) DeleteContactMessage(w http.ResponseWriter, r *http.Request) {
	id, ok := writeID(w, r, "id")
	if !ok {
		return
	}
	if err := h.svc.DeleteContactMessage(r.Context(), id); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *api) Cron(w http.ResponseWriter, r *http.Request) {
	_, _ = w.Write([]byte("Portfolio backend alive"))
}

func writeID(w http.ResponseWriter, r *http.Request, name string) (int64, bool) {
	raw := chi.URLParam(r, name)
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		middleware.WriteError(w, http.StatusBadRequest, "TYPE_MISMATCH", "Invalid ID")
		return 0, false
	}
	return id, true
}

func setOAuthCookie(w http.ResponseWriter, name, value, path string) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     path,
		MaxAge:   oauthCookieMaxAge,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

func clearOAuthCookie(w http.ResponseWriter, stateName, verifierName, path string) {
	http.SetCookie(w, &http.Cookie{
		Name:     stateName,
		Value:    "",
		Path:     path,
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	http.SetCookie(w, &http.Cookie{
		Name:     verifierName,
		Value:    "",
		Path:     path,
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

func secureEqual(a, b string) bool {
	return len(a) == len(b) && subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

func redirectOAuth(w http.ResponseWriter, r *http.Request, baseURL, token, errMsg string) {
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		http.Error(w, "Invalid OAuth redirect URI", http.StatusInternalServerError)
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

	http.Redirect(w, r, parsed.String(), http.StatusTemporaryRedirect)
}

func writeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, service.ErrNotFound):
		middleware.WriteError(w, http.StatusNotFound, "RESOURCE_NOT_FOUND", "Resource not found")
	case errors.Is(err, service.ErrUnauthorized):
		middleware.WriteError(w, http.StatusUnauthorized, "AUTHENTICATION_FAILED", "Authentication failed")
	case errors.Is(err, service.ErrForbidden):
		middleware.WriteError(w, http.StatusForbidden, "ACCESS_DENIED", "Access denied")
	case errors.Is(err, service.ErrConflict) || errors.Is(err, service.ErrAlreadyExists):
		middleware.WriteError(w, http.StatusConflict, "RESOURCE_ALREADY_EXISTS", "Resource already exists")
	case errors.Is(err, service.ErrBadRequest):
		middleware.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
	default:
		middleware.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Internal server error")
	}
}
