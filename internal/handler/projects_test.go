package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/ninjashadowboy/portfolio-backend-go/internal/domain"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/dto"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/usecase"
)

// projectFake records the project calls the handlers make.
type projectFake struct {
	usecase.Service
	reorderCalled bool
	reorderIDs    []int64
	reorderErr    error
	created       dto.ProjectCreate
	updated       dto.ProjectUpdate
	listed        []dto.Project
}

func (f *projectFake) ReorderProjects(_ context.Context, ids []int64) error {
	f.reorderCalled = true
	f.reorderIDs = ids
	return f.reorderErr
}

func (f *projectFake) CreateProject(_ context.Context, req dto.ProjectCreate) (dto.Project, error) {
	f.created = req
	return dto.Project{ID: 9}, nil
}

func (f *projectFake) UpdateProject(_ context.Context, id int64, req dto.ProjectUpdate) (dto.Project, error) {
	f.updated = req
	return dto.Project{ID: id}, nil
}

func (f *projectFake) ListProjects(context.Context) ([]dto.Project, error) {
	return f.listed, nil
}

func adminToken(t *testing.T) string {
	t.Helper()
	return bearer(t, domain.User{ID: 1, Role: "ADMIN"})
}

// projectRequest sends a JSON request. An empty token sends no Authorization header.
func projectRequest(t *testing.T, svc usecase.Service, method, path, body, token string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, "/portfolio/v1"+path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", token)
	}
	rec := httptest.NewRecorder()
	newTestRouter(t, svc).ServeHTTP(rec, req)
	return rec
}

func jsonBody(t *testing.T, v any) string {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestReorderProjectsPassesIDsInOrder(t *testing.T) {
	svc := &projectFake{}
	rec := projectRequest(t, svc, http.MethodPut, "/projects/order", `{"ids":[3,1,2]}`, adminToken(t))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("got %d, want 204 (body %s)", rec.Code, rec.Body.String())
	}
	if !slices.Equal(svc.reorderIDs, []int64{3, 1, 2}) {
		t.Fatalf("service got ids %v, want [3 1 2]", svc.reorderIDs)
	}
}

func TestReorderProjectsRejectsMalformedBodies(t *testing.T) {
	for name, body := range map[string]string{
		"empty list":    `{"ids":[]}`,
		"missing ids":   `{}`,
		"null ids":      `{"ids":null}`,
		"zero id":       `{"ids":[0,1]}`,
		"negative id":   `{"ids":[-1]}`,
		"string id":     `{"ids":["1"]}`,
		"unknown field": `{"ids":[1],"displayOrder":0}`,
	} {
		t.Run(name, func(t *testing.T) {
			svc := &projectFake{}
			rec := projectRequest(t, svc, http.MethodPut, "/projects/order", body, adminToken(t))
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("got %d, want 400 (body %s)", rec.Code, rec.Body.String())
			}
			if svc.reorderCalled {
				t.Fatal("service was called for a malformed body")
			}
		})
	}
}

func TestReorderProjectsShowsServiceBadRequest(t *testing.T) {
	svc := &projectFake{reorderErr: fmt.Errorf("project 99 does not exist: %w", usecase.ErrBadRequest)}
	rec := projectRequest(t, svc, http.MethodPut, "/projects/order", `{"ids":[99]}`, adminToken(t))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("got %d, want 400 (body %s)", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "project 99 does not exist") {
		t.Fatalf("body %s does not carry the service message", rec.Body.String())
	}
}

func TestReorderProjectsRequiresAdmin(t *testing.T) {
	for name, tc := range map[string]struct {
		token string
		want  int
	}{
		"anonymous": {"", http.StatusUnauthorized},
		"user":      {bearer(t, domain.User{ID: 2, Role: "USER"}), http.StatusForbidden},
	} {
		t.Run(name, func(t *testing.T) {
			svc := &projectFake{}
			rec := projectRequest(t, svc, http.MethodPut, "/projects/order", `{"ids":[1]}`, tc.token)
			if rec.Code != tc.want {
				t.Fatalf("got %d, want %d", rec.Code, tc.want)
			}
			if svc.reorderCalled {
				t.Fatal("service was called without admin rights")
			}
		})
	}
}

func TestProjectDemoLinkAcceptsAndTrimsHTTPURL(t *testing.T) {
	svc := &projectFake{}
	body := `{"name":"Train Type","description":"d","demoLink":" https://traintype.alexabena.me/ "}`
	rec := projectRequest(t, svc, http.MethodPost, "/projects", body, adminToken(t))
	if rec.Code != http.StatusCreated {
		t.Fatalf("got %d, want 201 (body %s)", rec.Code, rec.Body.String())
	}
	if svc.created.DemoLink == nil || *svc.created.DemoLink != "https://traintype.alexabena.me/" {
		t.Fatalf("demoLink = %v, want trimmed URL", svc.created.DemoLink)
	}
}

func TestProjectDemoLinkRejectsNonHTTPURLs(t *testing.T) {
	for name, link := range map[string]string{
		"ftp scheme":   "ftp://example.com/file",
		"javascript":   "javascript:alert(1)",
		"no scheme":    "example.com",
		"no host":      "https://",
		"plain string": "not a url",
	} {
		t.Run(name, func(t *testing.T) {
			body := jsonBody(t, map[string]string{"name": "A", "description": "d", "demoLink": link})
			svc := &projectFake{}
			if rec := projectRequest(t, svc, http.MethodPost, "/projects", body, adminToken(t)); rec.Code != http.StatusBadRequest {
				t.Fatalf("create: got %d, want 400 (body %s)", rec.Code, rec.Body.String())
			}
			if rec := projectRequest(t, svc, http.MethodPut, "/projects/1", body, adminToken(t)); rec.Code != http.StatusBadRequest {
				t.Fatalf("update: got %d, want 400 (body %s)", rec.Code, rec.Body.String())
			}
			if svc.created.Name != "" || svc.updated.DemoLink != nil {
				t.Fatal("service was called with an invalid demoLink")
			}
		})
	}
}

// An empty demoLink is passed through as a non-nil pointer. The use case reads
// that as "clear the link".
func TestProjectUpdateEmptyDemoLinkReachesService(t *testing.T) {
	svc := &projectFake{}
	rec := projectRequest(t, svc, http.MethodPut, "/projects/1", `{"demoLink":""}`, adminToken(t))
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d, want 200 (body %s)", rec.Code, rec.Body.String())
	}
	if svc.updated.DemoLink == nil || *svc.updated.DemoLink != "" {
		t.Fatalf("demoLink = %v, want pointer to empty string", svc.updated.DemoLink)
	}
}

func TestProjectJSONCarriesDemoLinkAndDisplayOrder(t *testing.T) {
	svc := &projectFake{listed: []dto.Project{
		{ID: 1, DemoLink: dto.Ptr("https://demo.example.com"), DisplayOrder: 0},
		{ID: 2, DisplayOrder: 1},
	}}
	rec := projectRequest(t, svc, http.MethodGet, "/projects", "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d, want 200 (body %s)", rec.Code, rec.Body.String())
	}
	var items []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &items); err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("got %d projects, want 2", len(items))
	}
	if items[0]["demoLink"] != "https://demo.example.com" || items[0]["displayOrder"] != float64(0) {
		t.Fatalf("first project = %v", items[0])
	}
	if value, ok := items[1]["demoLink"]; !ok || value != nil {
		t.Fatalf("demoLink for a project without one = %v (present %v), want null", value, ok)
	}
	if items[1]["displayOrder"] != float64(1) {
		t.Fatalf("second project displayOrder = %v, want 1", items[1]["displayOrder"])
	}
}
