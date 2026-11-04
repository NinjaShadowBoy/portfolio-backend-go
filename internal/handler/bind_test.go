package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin/binding"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/dto"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/usecase"
)

type bindFake struct {
	usecase.Service
	registered dto.RegisterRequest
	contact    dto.Contact
}

func (f *bindFake) Register(_ context.Context, req dto.RegisterRequest) error {
	f.registered = req
	return nil
}

func (f *bindFake) SubmitContact(_ context.Context, req dto.Contact) (dto.ContactMessage, error) {
	f.contact = req
	return dto.ContactMessage{}, nil
}

func init() { binding.EnableDecoderDisallowUnknownFields = true }

func post(t *testing.T, svc usecase.Service, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/portfolio/v1"+path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	newTestRouter(t, svc).ServeHTTP(rec, req)
	return rec
}

func TestRegisterSanitizesFields(t *testing.T) {
	svc := &bindFake{}
	rec := post(t, svc, "/auth/register", `{"email":" Alex@Example.COM ","password":" pass word 1 ","name":"<b>Alex</b>"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("got %d, want 201 (body %s)", rec.Code, rec.Body.String())
	}
	got := svc.registered
	if got.Email != "alex@example.com" || got.Name != "Alex" {
		t.Errorf("email/name not cleaned: %+v", got)
	}
	if got.Password != " pass word 1 " {
		t.Errorf("password changed: %q", got.Password)
	}
}

func TestBindRejectsInvalidInput(t *testing.T) {
	long := strings.Repeat("a", 73)
	for name, tc := range map[string]struct{ path, body string }{
		"bad email":            {"/auth/register", `{"email":"nope","password":"password1","name":"A"}`},
		"short password":       {"/auth/register", `{"email":"a@b.co","password":"short","name":"A"}`},
		"password over 72 B":   {"/auth/register", `{"email":"a@b.co","password":"` + long + `","name":"A"}`},
		"name only markup":     {"/auth/register", `{"email":"a@b.co","password":"password1","name":"<i></i>"}`},
		"unknown field":        {"/auth/register", `{"email":"a@b.co","password":"password1","name":"A","role":"ADMIN"}`},
		"contact blank msg":    {"/contact", `{"name":"A","email":"a@b.co","message":"  <br>  "}`},
		"body over size limit": {"/contact", `{"name":"A","email":"a@b.co","message":"` + strings.Repeat("a", maxBodyBytes) + `"}`},
	} {
		t.Run(name, func(t *testing.T) {
			if rec := post(t, &bindFake{}, tc.path, tc.body); rec.Code != http.StatusBadRequest {
				t.Fatalf("got %d, want 400 (body %.200s)", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestContactStripsHTMLAndHeaderInjection(t *testing.T) {
	svc := &bindFake{}
	rec := post(t, svc, "/contact", `{"name":"Eve\r\nBcc: x@y.z","email":"e@x.io","message":"hi <script>alert(1)</script>there\nline 2"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("got %d, want 201 (body %s)", rec.Code, rec.Body.String())
	}
	if svc.contact.Name != "EveBcc: x@y.z" {
		t.Errorf("name = %q", svc.contact.Name)
	}
	if svc.contact.Message != "hi there\nline 2" {
		t.Errorf("message = %q", svc.contact.Message)
	}
}
