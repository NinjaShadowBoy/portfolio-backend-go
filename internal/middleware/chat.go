package middleware

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/auth"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/domain"
	"golang.org/x/time/rate"
)

const chatSubjectKey = "chat.subject"

// AnonResolver looks up an existing anonymous identity. Unlike
// AnonProvisioner it cannot create one.
type AnonResolver interface {
	ResolveAnonToken(ctx context.Context, rawToken string) (int64, bool, error)
}

// ResolveIdentity works out who is calling, for the chat caps, without ever
// creating a user (EnsureIdentity would create a row for every bot). In
// order: a valid bearer token ('u:<id>', admin or user tier), an existing
// anon_token cookie ('u:<id>', anonymous tier), else the client IP hashed
// with ipKey ('ip:<hex>', anonymous tier). The raw IP is never stored.
func ResolveIdentity(jwt *auth.JWT, anon AnonResolver, ipKey []byte) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(chatSubjectKey, resolveSubject(c, jwt, anon, ipKey))
		c.Next()
	}
}

func resolveSubject(c *gin.Context, jwt *auth.JWT, anon AnonResolver, ipKey []byte) domain.ChatSubject {
	if token := jwt.ExtractBearerToken(c.Request); token != "" {
		if claims, err := jwt.Parse(token); err == nil && claims.UserID > 0 {
			tier := domain.ChatTierUser
			if jwt.HasRole(claims, "ADMIN") {
				tier = domain.ChatTierAdmin
			}
			return domain.ChatSubject{Key: userSubject(claims.UserID), Tier: tier}
		}
	}
	if raw, err := c.Cookie(AnonCookieName); err == nil && raw != "" && anon != nil {
		userID, ok, err := anon.ResolveAnonToken(c.Request.Context(), raw)
		switch {
		case err != nil:
			// The IP subject still applies the anonymous cap.
			slog.WarnContext(c.Request.Context(), "chat: resolve anon token failed", "error", err)
		case ok:
			return domain.ChatSubject{Key: userSubject(userID), Tier: domain.ChatTierAnonymous}
		}
	}
	return domain.ChatSubject{Key: IPSubject(ipKey, c.ClientIP()), Tier: domain.ChatTierAnonymous}
}

func userSubject(id int64) string { return "u:" + strconv.FormatInt(id, 10) }

// IPSubject is 'ip:' plus the first 16 bytes, in hex, of HMAC-SHA256(key, ip).
func IPSubject(key []byte, ip string) string {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(ip))
	return "ip:" + hex.EncodeToString(mac.Sum(nil)[:16])
}

// ChatSubjectFromContext returns the subject set by ResolveIdentity.
func ChatSubjectFromContext(c *gin.Context) (domain.ChatSubject, bool) {
	value, ok := c.Get(chatSubjectKey)
	if !ok {
		return domain.ChatSubject{}, false
	}
	subject, ok := value.(domain.ChatSubject)
	return subject, ok
}

// WriteRetryAfter sets Retry-After to d rounded up to whole seconds (at
// least 1).
func WriteRetryAfter(c *gin.Context, d time.Duration) {
	c.Header("Retry-After", strconv.Itoa(max(int(math.Ceil(d.Seconds())), 1)))
}

// RateLimiter is a token bucket per key (golang.org/x/time/rate) kept in a
// map whose idle entries are dropped after a TTL. The clock is injectable.
type RateLimiter struct {
	mu        sync.Mutex
	limit     rate.Limit
	burst     int
	ttl       time.Duration
	now       func() time.Time
	entries   map[string]*limiterEntry
	lastSweep time.Time
}

type limiterEntry struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// NewRateLimiter allows one request every `every` per key, with bursts of
// burst. A nil now means time.Now.
func NewRateLimiter(every time.Duration, burst int, now func() time.Time) *RateLimiter {
	if now == nil {
		now = time.Now
	}
	// An idle bucket refills fully after every*burst; dropping it later than
	// that loses nothing.
	ttl := max(10*time.Minute, every*time.Duration(burst))
	return &RateLimiter{limit: rate.Every(every), burst: burst, ttl: ttl, now: now, entries: map[string]*limiterEntry{}, lastSweep: now()}
}

// Allow takes one token for key. When none is left it returns false and how
// long until one is.
func (l *RateLimiter) Allow(key string) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	if now.Sub(l.lastSweep) >= l.ttl {
		for k, e := range l.entries {
			if now.Sub(e.lastSeen) >= l.ttl {
				delete(l.entries, k)
			}
		}
		l.lastSweep = now
	}
	e := l.entries[key]
	if e == nil {
		e = &limiterEntry{limiter: rate.NewLimiter(l.limit, l.burst)}
		l.entries[key] = e
	}
	e.lastSeen = now
	r := e.limiter.ReserveN(now, 1)
	if !r.OK() {
		return false, l.ttl
	}
	if delay := r.DelayFrom(now); delay > 0 {
		r.CancelAt(now)
		return false, delay
	}
	return true, 0
}

// Len is the number of keys tracked (for tests).
func (l *RateLimiter) Len() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.entries)
}

// RateLimit rejects a request with 429 and Retry-After when its key is out
// of tokens. key reads the subject; an empty key is not limited here.
func RateLimit(l *RateLimiter, key func(*gin.Context) string) gin.HandlerFunc {
	return func(c *gin.Context) {
		k := key(c)
		if k != "" {
			if ok, wait := l.Allow(k); !ok {
				WriteRetryAfter(c, wait)
				WriteError(c, http.StatusTooManyRequests, "RATE_LIMITED", "Too many requests, slow down")
				c.Abort()
				return
			}
		}
		c.Next()
	}
}

// busyRetryAfter is the Retry-After sent when every stream slot is taken.
const busyRetryAfter = 5 * time.Second

// Concurrency lets at most n requests run the rest of the chain at once and
// answers 429 with Retry-After to the others, without queueing them.
func Concurrency(n int) gin.HandlerFunc {
	slots := make(chan struct{}, max(n, 1))
	return func(c *gin.Context) {
		select {
		case slots <- struct{}{}:
		default:
			WriteRetryAfter(c, busyRetryAfter)
			WriteError(c, http.StatusTooManyRequests, "CHAT_BUSY", "The assistant is busy, try again in a few seconds")
			c.Abort()
			return
		}
		defer func() { <-slots }()
		c.Next()
	}
}

// RequireOrigin rejects with 403 a request whose Origin header is missing or
// not in allowed (the CORS_ALLOWED_ORIGINS patterns). CORS only stops
// browsers from reading a response; this stops scripts that send no or a
// foreign Origin from using the endpoint at all.
func RequireOrigin(allowed []string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !OriginAllowed(c.GetHeader("Origin"), allowed) {
			WriteError(c, http.StatusForbidden, "ORIGIN_NOT_ALLOWED", "Origin not allowed")
			c.Abort()
			return
		}
		c.Next()
	}
}

// OriginAllowed matches origin against patterns the way the CORS middleware
// does with wildcards on: "*" allows any origin, a pattern with one "*"
// matches its prefix and suffix, anything else must be equal (ignoring
// case). An empty origin never matches.
func OriginAllowed(origin string, patterns []string) bool {
	origin = strings.ToLower(strings.TrimSpace(origin))
	if origin == "" || origin == "null" {
		return false
	}
	for _, p := range patterns {
		p = strings.ToLower(strings.TrimSpace(p))
		if p == "*" {
			return true
		}
		if prefix, suffix, found := strings.Cut(p, "*"); found {
			if len(origin) >= len(prefix)+len(suffix) && strings.HasPrefix(origin, prefix) && strings.HasSuffix(origin, suffix) {
				return true
			}
			continue
		}
		if origin == p {
			return true
		}
	}
	return false
}
