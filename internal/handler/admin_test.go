package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ninjashadowboy/portfolio-backend-go/internal/auth"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/domain"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/dto"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/usecase"
)

// adminFakeService records what the admin handlers pass to the service.
type adminFakeService struct {
	usecase.Service
	userQuery  dto.AdminUserQuery
	roleActor  int64
	roleTarget int64
	role       string
}

func (f *adminFakeService) ListUsersForAdmin(_ context.Context, query dto.AdminUserQuery) (dto.Page[dto.AdminUser], error) {
	f.userQuery = query
	return dto.Page[dto.AdminUser]{Items: []dto.AdminUser{}}, nil
}

func (f *adminFakeService) UpdateUserRole(_ context.Context, actorID, userID int64, role string) (dto.User, error) {
	f.roleActor, f.roleTarget, f.role = actorID, userID, role
	return dto.User{ID: userID, Role: role}, nil
}

func bearer(t *testing.T, user domain.User) string {
	t.Helper()
	token, err := auth.NewJWT("test-secret", time.Hour, "test-issuer").GenerateToken(user)
	if err != nil {
		t.Fatal(err)
	}
	return "Bearer " + token
}

func TestAdminRoutesRejectNonAdmins(t *testing.T) {
	r := newTestRouter(t, nil)
	req := httptest.NewRequest(http.MethodGet, "/portfolio/v1/admin/users", nil)
	req.Header.Set("Authorization", bearer(t, domain.User{ID: 2, Role: "USER"}))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("USER token on /admin/users: got %d, want 403", rec.Code)
	}
}

func TestAdminListUsersParsesQuery(t *testing.T) {
	svc := &adminFakeService{}
	r := newTestRouter(t, svc)
	admin := bearer(t, domain.User{ID: 1, Role: "ADMIN"})

	req := httptest.NewRequest(http.MethodGet, "/portfolio/v1/admin/users?search=ann&includeAnonymous=true&limit=20&offset=40", nil)
	req.Header.Set("Authorization", admin)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d, want 200 (body %s)", rec.Code, rec.Body.String())
	}
	want := dto.AdminUserQuery{Search: "ann", IncludeAnonymous: true, Limit: 20, Offset: 40}
	if svc.userQuery != want {
		t.Fatalf("service got %+v, want %+v", svc.userQuery, want)
	}

	for _, query := range []string{"limit=ten", "offset=2147483648", "offset=-1"} {
		req = httptest.NewRequest(http.MethodGet, "/portfolio/v1/admin/users?"+query, nil)
		req.Header.Set("Authorization", admin)
		rec = httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("%s: got %d, want 400", query, rec.Code)
		}
	}
}

func TestAdminUpdateRolePassesActor(t *testing.T) {
	svc := &adminFakeService{}
	r := newTestRouter(t, svc)

	req := httptest.NewRequest(http.MethodPatch, "/portfolio/v1/admin/users/8/role", strings.NewReader(`{"role":"ADMIN"}`))
	req.Header.Set("Authorization", bearer(t, domain.User{ID: 1, Role: "ADMIN"}))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d, want 200 (body %s)", rec.Code, rec.Body.String())
	}
	if svc.roleActor != 1 || svc.roleTarget != 8 || svc.role != "ADMIN" {
		t.Fatalf("service got actor=%d target=%d role=%q", svc.roleActor, svc.roleTarget, svc.role)
	}

	req = httptest.NewRequest(http.MethodPatch, "/portfolio/v1/admin/users/8/role", strings.NewReader(`{"role":"OWNER"}`))
	req.Header.Set("Authorization", bearer(t, domain.User{ID: 1, Role: "ADMIN"}))
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("unknown role: got %d, want 400", rec.Code)
	}
}
