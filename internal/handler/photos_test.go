package handler

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/ninjashadowboy/portfolio-backend-go/internal/domain"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/usecase"
)

// photoOrderFake records the photo reorder call the handler makes.
type photoOrderFake struct {
	usecase.Service
	called    bool
	projectID int64
	ids       []int64
	err       error
}

func (f *photoOrderFake) ReorderProjectPhotos(_ context.Context, projectID int64, ids []int64) error {
	f.called = true
	f.projectID = projectID
	f.ids = ids
	return f.err
}

func TestReorderProjectPhotosPassesProjectAndIDs(t *testing.T) {
	svc := &photoOrderFake{}
	rec := projectRequest(t, svc, http.MethodPut, "/projects/4/photos/order", `{"ids":[12,10,11]}`, adminToken(t))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("got %d, want 204 (body %s)", rec.Code, rec.Body.String())
	}
	if rec.Body.Len() != 0 {
		t.Fatalf("204 carried a body: %s", rec.Body.String())
	}
	if svc.projectID != 4 || !slices.Equal(svc.ids, []int64{12, 10, 11}) {
		t.Fatalf("service got project %d ids %v, want 4 [12 10 11]", svc.projectID, svc.ids)
	}
}

func TestReorderProjectPhotosRequiresAdmin(t *testing.T) {
	for name, tc := range map[string]struct {
		token string
		want  int
	}{
		"anonymous": {"", http.StatusUnauthorized},
		"user":      {bearer(t, domain.User{ID: 2, Role: "USER"}), http.StatusForbidden},
	} {
		t.Run(name, func(t *testing.T) {
			svc := &photoOrderFake{}
			rec := projectRequest(t, svc, http.MethodPut, "/projects/4/photos/order", `{"ids":[1]}`, tc.token)
			if rec.Code != tc.want {
				t.Fatalf("got %d, want %d", rec.Code, tc.want)
			}
			if svc.called {
				t.Fatal("service was called without admin rights")
			}
		})
	}
}

func TestReorderProjectPhotosRejectsMalformedRequests(t *testing.T) {
	for name, tc := range map[string]struct{ path, body string }{
		"empty list":     {"/projects/4/photos/order", `{"ids":[]}`},
		"missing ids":    {"/projects/4/photos/order", `{}`},
		"null ids":       {"/projects/4/photos/order", `{"ids":null}`},
		"zero id":        {"/projects/4/photos/order", `{"ids":[0,1]}`},
		"negative id":    {"/projects/4/photos/order", `{"ids":[-1]}`},
		"string id":      {"/projects/4/photos/order", `{"ids":["1"]}`},
		"unknown field":  {"/projects/4/photos/order", `{"ids":[1],"displayOrder":0}`},
		"not json":       {"/projects/4/photos/order", `ids=1`},
		"bad project id": {"/projects/abc/photos/order", `{"ids":[1]}`},
		"zero project":   {"/projects/0/photos/order", `{"ids":[1]}`},
	} {
		t.Run(name, func(t *testing.T) {
			svc := &photoOrderFake{}
			rec := projectRequest(t, svc, http.MethodPut, tc.path, tc.body, adminToken(t))
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("got %d, want 400 (body %s)", rec.Code, rec.Body.String())
			}
			if svc.called {
				t.Fatal("service was called for a malformed request")
			}
		})
	}
}

func TestReorderProjectPhotosMapsServiceErrors(t *testing.T) {
	for name, tc := range map[string]struct {
		err  error
		want int
	}{
		"bad order":       {fmt.Errorf("photo 99 is not in project 4: %w", usecase.ErrBadRequest), http.StatusBadRequest},
		"unknown project": {usecase.ErrNotFound, http.StatusNotFound},
	} {
		t.Run(name, func(t *testing.T) {
			svc := &photoOrderFake{err: tc.err}
			rec := projectRequest(t, svc, http.MethodPut, "/projects/4/photos/order", `{"ids":[99]}`, adminToken(t))
			if rec.Code != tc.want {
				t.Fatalf("got %d, want %d (body %s)", rec.Code, tc.want, rec.Body.String())
			}
			if tc.want == http.StatusBadRequest && !strings.Contains(rec.Body.String(), "photo 99 is not in project 4") {
				t.Fatalf("body %s does not carry the service message", rec.Body.String())
			}
		})
	}
}
