package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/auth"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/domain"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/dto"
)

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	os.Exit(m.Run())
}

// fakeAnonResolver deliberately has no create/provision method. That it
// satisfies AnonResolver at compile time is the proof that ResolveIdentity
// cannot provision a user.
type fakeAnonResolver struct {
	id     int64
	ok     bool
	err    error
	calls  int
	tokens []string
}

var _ AnonResolver = (*fakeAnonResolver)(nil)

func (f *fakeAnonResolver) ResolveAnonToken(_ context.Context, rawToken string) (int64, bool, error) {
	f.calls++
	f.tokens = append(f.tokens, rawToken)
	return f.id, f.ok, f.err
}

func newChatEngine(handlers ...gin.HandlerFunc) *gin.Engine {
	r := gin.New()
	r.POST("/chat", handlers...)
	return r
}

func doRequest(r http.Handler, req *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func okHandler(c *gin.Context) { c.Status(http.StatusOK) }

func subjectHandler(c *gin.Context) {
	subject, ok := ChatSubjectFromContext(c)
	if !ok {
		c.String(http.StatusInternalServerError, "missing subject")
		return
	}
	c.JSON(http.StatusOK, subject)
}

func tokenFor(t *testing.T, user domain.User) string {
	t.Helper()
	token, err := auth.NewJWT("test-secret", time.Hour, "test-issuer").GenerateToken(user)
	if err != nil {
		t.Fatalf("GenerateToken() error = %v", err)
	}
	return token
}

// newIdentityEngine disables trusted proxies so ClientIP is the request's
// RemoteAddr, not an X-Real-IP header.
func newIdentityEngine(t *testing.T, jwtSvc *auth.JWT, anon AnonResolver, ipKey []byte) *gin.Engine {
	t.Helper()
	r := newChatEngine(ResolveIdentity(jwtSvc, anon, ipKey), subjectHandler)
	if err := r.SetTrustedProxies(nil); err != nil {
		t.Fatalf("SetTrustedProxies() error = %v", err)
	}
	return r
}

func decodeChatSubject(t *testing.T, rec *httptest.ResponseRecorder) domain.ChatSubject {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var subject domain.ChatSubject
	if err := json.NewDecoder(rec.Body).Decode(&subject); err != nil {
		t.Fatalf("decode chat subject: %v", err)
	}
	return subject
}

func TestResolveIdentityTiers(t *testing.T) {
	jwtSvc := auth.NewJWT("test-secret", time.Hour, "test-issuer")
	ipKey := []byte("ip-hmac-key")
	const clientIP = "203.0.113.9"
	wantIP := IPSubject(ipKey, clientIP)

	request := func(bearer string, withCookie bool) *http.Request {
		req := httptest.NewRequest(http.MethodPost, "/chat", nil)
		req.RemoteAddr = clientIP + ":4567"
		if bearer != "" {
			req.Header.Set("Authorization", "Bearer "+bearer)
		}
		if withCookie {
			req.AddCookie(&http.Cookie{Name: AnonCookieName, Value: "raw-cookie-value"})
		}
		return req
	}

	t.Run("user bearer token", func(t *testing.T) {
		anon := &fakeAnonResolver{}
		token := tokenFor(t, domain.User{ID: 7, Email: "user@example.com", Role: "USER"})
		got := decodeChatSubject(t, doRequest(newIdentityEngine(t, jwtSvc, anon, ipKey), request(token, false)))
		if got.Key != "u:7" || got.Tier != domain.ChatTierUser {
			t.Fatalf("subject = %+v, want u:7 user", got)
		}
		if anon.calls != 0 {
			t.Fatalf("resolver called %d times for a bearer request", anon.calls)
		}
	})

	t.Run("admin bearer token", func(t *testing.T) {
		anon := &fakeAnonResolver{}
		token := tokenFor(t, domain.User{ID: 1, Email: "admin@example.com", Role: "ADMIN"})
		got := decodeChatSubject(t, doRequest(newIdentityEngine(t, jwtSvc, anon, ipKey), request(token, false)))
		if got.Key != "u:1" || got.Tier != domain.ChatTierAdmin {
			t.Fatalf("subject = %+v, want u:1 admin", got)
		}
	})

	t.Run("anon cookie resolves", func(t *testing.T) {
		anon := &fakeAnonResolver{id: 42, ok: true}
		got := decodeChatSubject(t, doRequest(newIdentityEngine(t, jwtSvc, anon, ipKey), request("", true)))
		if got.Key != "u:42" || got.Tier != domain.ChatTierAnonymous {
			t.Fatalf("subject = %+v, want u:42 anonymous", got)
		}
		if anon.calls != 1 || len(anon.tokens) != 1 || anon.tokens[0] != "raw-cookie-value" {
			t.Fatalf("resolver calls = %d tokens = %v, want one call with the raw cookie value", anon.calls, anon.tokens)
		}
	})

	t.Run("unknown anon cookie falls back to IP", func(t *testing.T) {
		anon := &fakeAnonResolver{ok: false}
		got := decodeChatSubject(t, doRequest(newIdentityEngine(t, jwtSvc, anon, ipKey), request("", true)))
		if got.Key != wantIP || got.Tier != domain.ChatTierAnonymous {
			t.Fatalf("subject = %+v, want %s anonymous", got, wantIP)
		}
		if anon.calls != 1 {
			t.Fatalf("resolver calls = %d, want 1", anon.calls)
		}
	})

	t.Run("no credentials falls back to IP", func(t *testing.T) {
		anon := &fakeAnonResolver{}
		got := decodeChatSubject(t, doRequest(newIdentityEngine(t, jwtSvc, anon, ipKey), request("", false)))
		if got.Key != wantIP || got.Tier != domain.ChatTierAnonymous {
			t.Fatalf("subject = %+v, want %s anonymous", got, wantIP)
		}
		if anon.calls != 0 {
			t.Fatalf("resolver calls = %d, want 0", anon.calls)
		}
	})

	t.Run("garbage bearer token falls back", func(t *testing.T) {
		anon := &fakeAnonResolver{id: 42, ok: true}
		got := decodeChatSubject(t, doRequest(newIdentityEngine(t, jwtSvc, anon, ipKey), request("not.a.jwt", true)))
		if got.Key != "u:42" || got.Tier != domain.ChatTierAnonymous {
			t.Fatalf("subject = %+v, want u:42 anonymous from the cookie", got)
		}

		anon = &fakeAnonResolver{}
		got = decodeChatSubject(t, doRequest(newIdentityEngine(t, jwtSvc, anon, ipKey), request("garbage", false)))
		if got.Key != wantIP || got.Tier != domain.ChatTierAnonymous {
			t.Fatalf("subject = %+v, want %s anonymous from the IP", got, wantIP)
		}
	})
}

func TestIPSubject(t *testing.T) {
	key := []byte("first-hmac-key")
	otherKey := []byte("second-hmac-key")
	const ip = "203.0.113.9"

	base := IPSubject(key, ip)
	if !strings.HasPrefix(base, "ip:") {
		t.Fatalf("IPSubject() = %q, want ip: prefix", base)
	}
	if strings.Contains(base, ip) {
		t.Fatalf("IPSubject() = %q, must not contain the raw IP", base)
	}
	if again := IPSubject(key, ip); again != base {
		t.Fatalf("IPSubject() is not deterministic: %q != %q", again, base)
	}
	if other := IPSubject(key, "203.0.113.10"); other == base {
		t.Fatalf("different IPs produced the same subject %q", other)
	}
	if other := IPSubject(otherKey, ip); other == base {
		t.Fatalf("different HMAC keys produced the same subject %q", other)
	}

	// Request-based case: the key is built from gin's ClientIP.
	r := gin.New()
	if err := r.SetTrustedProxies(nil); err != nil {
		t.Fatalf("SetTrustedProxies() error = %v", err)
	}
	var got string
	r.GET("/ip", func(c *gin.Context) { got = IPSubject(key, c.ClientIP()) })
	req := httptest.NewRequest(http.MethodGet, "/ip", nil)
	req.RemoteAddr = ip + ":4567"
	if rec := doRequest(r, req); rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if got != base {
		t.Fatalf("request IPSubject() = %q, want %q", got, base)
	}
}

func TestRateLimiterAllow(t *testing.T) {
	now := time.Date(2026, time.November, 9, 12, 0, 0, 0, time.UTC)
	limiter := NewRateLimiter(time.Second, 2, func() time.Time { return now })

	for i := 0; i < 2; i++ {
		if ok, wait := limiter.Allow("k"); !ok {
			t.Fatalf("request %d denied, wait = %v; want allowed", i+1, wait)
		}
	}
	ok, wait := limiter.Allow("k")
	if ok || wait <= 0 {
		t.Fatalf("third request = (%v, %v), want denied with a positive wait", ok, wait)
	}

	now = now.Add(time.Second)
	if ok, wait := limiter.Allow("k"); !ok {
		t.Fatalf("request after refill denied, wait = %v; want allowed", wait)
	}
}

func TestRateLimiterLenAndSweep(t *testing.T) {
	now := time.Date(2026, time.November, 9, 12, 0, 0, 0, time.UTC)
	limiter := NewRateLimiter(time.Second, 2, func() time.Time { return now })

	limiter.Allow("a")
	limiter.Allow("b")
	limiter.Allow("a")
	if got := limiter.Len(); got != 2 {
		t.Fatalf("Len() = %d, want 2 distinct keys", got)
	}

	// The idle TTL is max(10m, every*burst) = 10m here.
	now = now.Add(11 * time.Minute)
	limiter.Allow("c")
	if got := limiter.Len(); got != 1 {
		t.Fatalf("Len() after sweep = %d, want 1 (idle keys dropped)", got)
	}
}

func TestRateLimitMiddleware(t *testing.T) {
	limiter := NewRateLimiter(time.Hour, 1, nil)
	r := newChatEngine(RateLimit(limiter, func(*gin.Context) string { return "u:7" }), okHandler)

	if rec := doRequest(r, httptest.NewRequest(http.MethodPost, "/chat", nil)); rec.Code != http.StatusOK {
		t.Fatalf("first status = %d, want 200", rec.Code)
	}

	rec := doRequest(r, httptest.NewRequest(http.MethodPost, "/chat", nil))
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("second status = %d, want 429", rec.Code)
	}
	retryAfter, err := strconv.Atoi(rec.Header().Get("Retry-After"))
	if err != nil || retryAfter < 1 {
		t.Fatalf("Retry-After = %q, want a whole number >= 1", rec.Header().Get("Retry-After"))
	}
	var body dto.ErrorResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode error body: %v", err)
	}
	if body.Code != "RATE_LIMITED" {
		t.Fatalf("code = %q, want RATE_LIMITED", body.Code)
	}
}

func TestRateLimitMiddlewareEmptyKeyBypasses(t *testing.T) {
	limiter := NewRateLimiter(time.Hour, 1, nil)
	r := newChatEngine(RateLimit(limiter, func(*gin.Context) string { return "" }), okHandler)

	for i := 0; i < 3; i++ {
		if rec := doRequest(r, httptest.NewRequest(http.MethodPost, "/chat", nil)); rec.Code != http.StatusOK {
			t.Fatalf("request %d status = %d, want 200", i+1, rec.Code)
		}
	}
	if got := limiter.Len(); got != 0 {
		t.Fatalf("Len() = %d, want 0 keys for an empty key func", got)
	}
}

func TestConcurrency(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }

	blocking := func(c *gin.Context) {
		close(entered)
		<-release
		c.Status(http.StatusOK)
	}
	r := newChatEngine(Concurrency(1), blocking)

	var first sync.WaitGroup
	first.Add(1)
	var firstRec *httptest.ResponseRecorder
	go func() {
		defer first.Done()
		firstRec = doRequest(r, httptest.NewRequest(http.MethodPost, "/chat", nil))
	}()
	defer func() {
		unblock()
		first.Wait()
	}()

	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("first request never reached the handler")
	}

	second := doRequest(r, httptest.NewRequest(http.MethodPost, "/chat", nil))
	if second.Code != http.StatusTooManyRequests {
		t.Fatalf("second status = %d, want 429", second.Code)
	}
	if retryAfter, err := strconv.Atoi(second.Header().Get("Retry-After")); err != nil || retryAfter < 1 {
		t.Fatalf("Retry-After = %q, want a whole number >= 1", second.Header().Get("Retry-After"))
	}
	var body dto.ErrorResponse
	if err := json.NewDecoder(second.Body).Decode(&body); err != nil {
		t.Fatalf("decode error body: %v", err)
	}
	if body.Code != "CHAT_BUSY" {
		t.Fatalf("code = %q, want CHAT_BUSY", body.Code)
	}

	unblock()
	first.Wait()
	if firstRec.Code != http.StatusOK {
		t.Fatalf("first status = %d, want 200 after release", firstRec.Code)
	}
}

func TestOriginAllowed(t *testing.T) {
	cases := []struct {
		name     string
		origin   string
		patterns []string
		want     bool
	}{
		{"exact", "https://alexabena.me", []string{"https://alexabena.me"}, true},
		{"exact case insensitive", "https://ALEXABENA.ME", []string{"https://alexabena.me"}, true},
		{"wildcard port", "http://localhost:4200", []string{"http://localhost:*"}, true},
		{"star allows any", "https://anything.example", []string{"*"}, true},
		{"wildcard host", "https://api.example.com", []string{"https://*.example.com"}, true},
		{"empty origin", "", []string{"*"}, false},
		{"null origin", "null", []string{"*"}, false},
		{"null pattern", "null", []string{"null"}, false},
		{"no match", "https://evil.example", []string{"https://alexabena.me", "http://localhost:*"}, false},
		{"no patterns", "https://alexabena.me", nil, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := OriginAllowed(tc.origin, tc.patterns); got != tc.want {
				t.Fatalf("OriginAllowed(%q, %v) = %v, want %v", tc.origin, tc.patterns, got, tc.want)
			}
		})
	}
}

func TestRequireOriginMiddleware(t *testing.T) {
	allowed := []string{"https://alexabena.me", "http://localhost:*"}
	r := newChatEngine(RequireOrigin(allowed), okHandler)

	rec := doRequest(r, httptest.NewRequest(http.MethodPost, "/chat", nil))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("missing Origin status = %d, want 403", rec.Code)
	}
	var body dto.ErrorResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode error body: %v", err)
	}
	if body.Code != "ORIGIN_NOT_ALLOWED" {
		t.Fatalf("code = %q, want ORIGIN_NOT_ALLOWED", body.Code)
	}

	for _, origin := range []string{"https://alexabena.me", "https://ALEXABENA.ME", "http://localhost:4200"} {
		req := httptest.NewRequest(http.MethodPost, "/chat", nil)
		req.Header.Set("Origin", origin)
		if rec := doRequest(r, req); rec.Code != http.StatusOK {
			t.Fatalf("Origin %q status = %d, want 200", origin, rec.Code)
		}
	}
}

func TestWriteRetryAfter(t *testing.T) {
	cases := []struct {
		in   time.Duration
		want string
	}{
		{0, "1"},
		{time.Millisecond, "1"},
		{999 * time.Millisecond, "1"},
		{time.Second, "1"},
		{1001 * time.Millisecond, "2"},
		{2 * time.Second, "2"},
		{2500 * time.Millisecond, "3"},
		{-time.Second, "1"},
	}

	for _, tc := range cases {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		WriteRetryAfter(c, tc.in)
		if got := rec.Header().Get("Retry-After"); got != tc.want {
			t.Errorf("WriteRetryAfter(%v) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
