package router

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/auth"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/config"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/domain"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/usecase"
)

const (
	testOrigin     = "http://localhost:4200"
	testClientIP   = "203.0.113.7"
	testProxyPeer  = "127.0.0.1:1234"
	testChatBody   = `{"locale":"en","messages":[{"role":"user","text":"hello"}]}`
	testJWTSecret  = "ssssssssssssssssssssssssssssssss"
	testServiceTag = "test"
)

// fakeChatService implements only the methods the chat routes touch; the
// embedded interface satisfies the rest of usecase.Service and panics if any
// un-stubbed method is unexpectedly called.
type fakeChatService struct {
	usecase.Service
	deadlineOK        bool
	deadlineRemaining time.Duration
}

func (f *fakeChatService) ChatEnabled() bool { return true }

func (f *fakeChatService) Chat(ctx context.Context, _ domain.ChatInput, emit func(domain.ChatEvent) error) error {
	deadline, ok := ctx.Deadline()
	f.deadlineOK = ok
	if ok {
		f.deadlineRemaining = time.Until(deadline)
	}
	if err := emit(domain.ChatEvent{Kind: domain.ChatEventText, Text: "hello"}); err != nil {
		return err
	}
	return emit(domain.ChatEvent{Kind: domain.ChatEventDone})
}

func (f *fakeChatService) ChatQuota(context.Context, domain.ChatSubject) (domain.ChatQuota, error) {
	return domain.ChatQuota{}, nil
}

func (f *fakeChatService) ResolveAnonToken(context.Context, string) (int64, bool, error) {
	return 0, false, nil
}

func testConfig(trustedProxies []string) config.Config {
	return config.Config{
		ServiceName:    testServiceTag,
		JWTSecret:      testJWTSecret,
		AllowedOrigins: []string{testOrigin},
		TrustedProxies: trustedProxies,
		Chat:           config.ChatConfig{Enabled: true, MaxConcurrent: 2},
	}
}

func newTestHandler(t *testing.T, svc usecase.Service, trustedProxies []string) http.Handler {
	t.Helper()
	gin.SetMode(gin.TestMode)
	cfg := testConfig(trustedProxies)
	h, err := New(cfg, auth.NewJWT(cfg.JWTSecret, time.Hour, testServiceTag), svc)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return h
}

func chatRequest(method, path, body, remoteAddr string) *http.Request {
	var req *http.Request
	if body == "" {
		req = httptest.NewRequest(method, path, nil)
	} else {
		req = httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Origin", testOrigin)
	req.Header.Set("X-Real-IP", testClientIP)
	req.RemoteAddr = remoteAddr
	return req
}

// TestChatExemptFromRequestTimeout proves the /chat route is not bounded by
// the router's 30 s timeout: the handler sees the 90 s chat deadline instead.
func TestChatExemptFromRequestTimeout(t *testing.T) {
	svc := &fakeChatService{}
	h := newTestHandler(t, svc, []string{"127.0.0.1"})

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, chatRequest(http.MethodPost, config.BasePath()+"/chat", testChatBody, testProxyPeer))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/event-stream") {
		t.Errorf("Content-Type = %q, want text/event-stream", got)
	}
	if body := rec.Body.String(); !strings.Contains(body, "event: done") {
		t.Errorf("body = %q, want an event: done line", body)
	}
	if !svc.deadlineOK {
		t.Fatal("Chat context had no deadline")
	}
	// The handler's own 90 s chat timeout, not the router's 30 s bound.
	if svc.deadlineRemaining <= 60*time.Second {
		t.Errorf("Chat deadline remaining = %v, want > 60s (30s timeout must be skipped)", svc.deadlineRemaining)
	}
}

func TestChatConfig(t *testing.T) {
	svc := &fakeChatService{}
	h := newTestHandler(t, svc, []string{"127.0.0.1"})

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, chatRequest(http.MethodGet, config.BasePath()+"/chat/config", "", testProxyPeer))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", got)
	}
	if body := rec.Body.String(); !strings.Contains(body, `"enabled":true`) {
		t.Errorf("body = %q, want enabled true", body)
	}
}

// TestRequestTimeout exercises the middleware directly, since a route handler
// registered through New has no hook to read its context deadline from outside.
func TestRequestTimeout(t *testing.T) {
	gin.SetMode(gin.TestMode)

	type observation struct {
		ok        bool
		deadline  time.Time
		remaining time.Duration
	}

	run := func(t *testing.T, path string, parentDeadline time.Time) observation {
		t.Helper()
		r := gin.New()
		r.Use(requestTimeout(30*time.Second, config.BasePath()+"/chat", config.BasePath()+"/admin/knowledge/reindex"))
		var got observation
		r.GET(path, func(c *gin.Context) {
			got.deadline, got.ok = c.Request.Context().Deadline()
			got.remaining = time.Until(got.deadline)
			c.Status(http.StatusOK)
		})

		ctx, cancel := context.WithDeadline(context.Background(), parentDeadline)
		defer cancel()
		req := httptest.NewRequest(http.MethodGet, path, nil).WithContext(ctx)
		r.ServeHTTP(httptest.NewRecorder(), req)
		return got
	}

	t.Run("other routes get the 30 second bound", func(t *testing.T) {
		got := run(t, config.BasePath()+"/cron", time.Now().Add(5*time.Minute))
		if !got.ok {
			t.Fatal("handler saw no deadline")
		}
		if got.remaining <= 0 || got.remaining > 30*time.Second {
			t.Errorf("remaining = %v, want (0, 30s]", got.remaining)
		}
	})

	for _, path := range []string{config.BasePath() + "/chat", config.BasePath() + "/admin/knowledge/reindex"} {
		t.Run(path+" keeps the parent deadline", func(t *testing.T) {
			parentDeadline := time.Now().Add(5 * time.Minute)
			got := run(t, path, parentDeadline)
			if !got.ok {
				t.Fatal("handler saw no deadline")
			}
			if !got.deadline.Equal(parentDeadline) {
				t.Errorf("deadline = %v, want the parent's %v", got.deadline, parentDeadline)
			}
			if got.remaining <= 30*time.Second {
				t.Errorf("remaining = %v, want the parent's > 30s deadline", got.remaining)
			}
		})
	}
}

func TestNewRejectsMalformedTrustedProxy(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := testConfig([]string{"not-an-ip"})

	h, err := New(cfg, auth.NewJWT(cfg.JWTSecret, time.Hour, testServiceTag), &fakeChatService{})
	if err == nil {
		t.Fatalf("New() error = nil with handler %T, want a trusted-proxies error", h)
	}
	if !strings.Contains(err.Error(), "trusted proxies") {
		t.Errorf("error = %v, want it to mention trusted proxies", err)
	}
}
