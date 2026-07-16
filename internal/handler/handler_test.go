package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/auth"
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
	New(nil, jwt, "http://localhost/redirect", "/portfolio/v1").Routes(r.Group("/portfolio/v1"))
	return r
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
		{"create rating requires auth", http.MethodPost, "/portfolio/v1/ratings", http.StatusUnauthorized},
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
