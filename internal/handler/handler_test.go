package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/auth"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/dto"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/service"
)

// newTestRouter builds the full route table on a gin engine. A nil service is
// safe here because every case below is rejected (bad param / missing auth)
// before any service method is reached. Registering the routes also exercises
// gin's tree for wildcard/static conflicts, which only surface at build time.
func newTestRouter(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	jwt := auth.NewJWT("test-secret", time.Hour, "test-issuer")
	r := gin.New()
	New(nil, jwt, "http://localhost/redirect", "/portfolio/v1", false).Routes(r.Group("/portfolio/v1"))
	return r
}

// routerWithService is newTestRouter but backed by a concrete service so cases
// that actually reach a handler body (article listing, comment creation) can
// assert on the service interaction.
func routerWithService(t *testing.T, svc service.Service) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	jwt := auth.NewJWT("test-secret", time.Hour, "test-issuer")
	r := gin.New()
	New(svc, jwt, "http://localhost/redirect", "/portfolio/v1", false).Routes(r.Group("/portfolio/v1"))
	return r
}

// fakeService implements only the methods the feedback routes touch; the
// embedded interface satisfies the rest of service.Service and panics if any
// un-stubbed method is unexpectedly called.
type fakeService struct {
	service.Service
	gotSlug         string
	articleComments []dto.Comment
	articleErr      error
	createResult    dto.Comment
	createErr       error
	createCalled    bool
	anonProvisioned bool
}

func (f *fakeService) ListArticleComments(_ context.Context, slug string) ([]dto.Comment, error) {
	f.gotSlug = slug
	return f.articleComments, f.articleErr
}

func (f *fakeService) CreateComment(_ context.Context, _ int64, _ dto.CommentCreate) (dto.Comment, error) {
	f.createCalled = true
	return f.createResult, f.createErr
}

// CreateAnonymousUser / ResolveAnonToken feed EnsureIdentity so anonymous
// participation keeps working without a real DB.
func (f *fakeService) CreateAnonymousUser(_ context.Context) (int64, string, error) {
	f.anonProvisioned = true
	return 99, "anon-token-value", nil
}

func (f *fakeService) ResolveAnonToken(_ context.Context, _ string) (int64, bool, error) {
	return 0, false, nil
}

func TestRoutesRegisterWithoutConflict(t *testing.T) {
	// If gin rejected the wildcard/static route combinations this would panic.
	newTestRouter(t)
}

func TestRouting(t *testing.T) {
	r := newTestRouter(t)

	cases := []struct {
		name   string
		method string
		path   string
		want   int
	}{
		{"cron ok", http.MethodGet, "/portfolio/v1/cron", http.StatusOK},
		{"project invalid id", http.MethodGet, "/portfolio/v1/projects/abc", http.StatusBadRequest},
		{"rating dual-param invalid id", http.MethodGet, "/portfolio/v1/ratings/project/xyz/average", http.StatusBadRequest},
		{"comments invalid project id", http.MethodGet, "/portfolio/v1/comments/project/xyz", http.StatusBadRequest},
		{"my-ratings requires auth", http.MethodGet, "/portfolio/v1/ratings/my-ratings", http.StatusUnauthorized},
		{"delete photo requires auth", http.MethodDelete, "/portfolio/v1/photos/5", http.StatusUnauthorized},
		{"admin contact requires auth", http.MethodGet, "/portfolio/v1/contact", http.StatusUnauthorized},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, nil)
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)
			if rec.Code != tc.want {
				t.Fatalf("%s %s: got status %d, want %d (body: %s)", tc.method, tc.path, rec.Code, tc.want, rec.Body.String())
			}
		})
	}
}

// TestArticleCommentRouteResolves covers both slug-based GET URLs: the route
// resolves (no parseID on the string slug), the slug reaches the service
// verbatim, and the typed comment is serialized back.
func TestArticleCommentRouteResolves(t *testing.T) {
	for _, path := range []string{
		"/portfolio/v1/articles/hello-world/comments",
		"/portfolio/v1/comments/article/hello-world",
	} {
		t.Run(path, func(t *testing.T) {
			svc := &fakeService{articleComments: []dto.Comment{{
				ID:          1,
				Type:        "suggestion",
				ArticleSlug: dto.Ptr("hello-world"),
				Body:        "nice article",
			}}}
			r := routerWithService(t, svc)

			req := httptest.NewRequest(http.MethodGet, path, nil)
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("GET %s: got %d, want 200 (body %s)", path, rec.Code, rec.Body.String())
			}
			if svc.gotSlug != "hello-world" {
				t.Fatalf("service received slug %q, want \"hello-world\"", svc.gotSlug)
			}
			if !strings.Contains(rec.Body.String(), `"suggestion"`) {
				t.Fatalf("response body %s missing comment type", rec.Body.String())
			}
		})
	}
}

// TestCreateCommentUnauthorizedWithoutIdentity exercises the handler's own
// guard: with no identity on the context it must return 401 rather than
// dereferencing a nil identity.
func TestCreateCommentUnauthorizedWithoutIdentity(t *testing.T) {
	gin.SetMode(gin.TestMode)
	jwt := auth.NewJWT("test-secret", time.Hour, "test-issuer")
	h := New(&fakeService{}, jwt, "http://localhost/redirect", "/portfolio/v1", false).(*api)

	r := gin.New()
	// Registered WITHOUT EnsureIdentity, so IdentityFromContext returns !ok.
	r.POST("/comments", h.CreateComment)

	req := httptest.NewRequest(http.MethodPost, "/comments", strings.NewReader(`{"type":"critique","projectId":1,"body":"hi"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("POST /comments without identity: got %d, want 401 (body %s)", rec.Code, rec.Body.String())
	}
}

// TestCreateCommentBadRequestUnderIdentity drives the real /comments POST route:
// EnsureIdentity provisions an anonymous user (no cookie, no bearer token), the
// handler reaches the service, and a validation failure maps to 400.
func TestCreateCommentBadRequestUnderIdentity(t *testing.T) {
	svc := &fakeService{createErr: service.ErrBadRequest}
	r := routerWithService(t, svc)

	req := httptest.NewRequest(http.MethodPost, "/portfolio/v1/comments", strings.NewReader(`{"type":"critique","body":"missing a target"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("POST /comments with invalid target: got %d, want 400 (body %s)", rec.Code, rec.Body.String())
	}
	if !svc.anonProvisioned {
		t.Fatal("expected EnsureIdentity to provision an anonymous user")
	}
	if !svc.createCalled {
		t.Fatal("expected CreateComment to be reached under identity")
	}
}
