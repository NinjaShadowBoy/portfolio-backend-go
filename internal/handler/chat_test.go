package handler

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/auth"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/domain"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/usecase"
)

const testOrigin = "http://localhost:4200"

// chatService is a usecase.Service whose chat methods are scripted. The
// embedded nil interface panics on any other method, which the chat routes
// must never call.
type chatService struct {
	usecase.Service
	enabled  bool
	quota    domain.ChatQuota
	quotaErr error
	events   []domain.ChatEvent
	chatErr  error
	// block makes Chat wait for the request context to end, to exercise a
	// client disconnect.
	block    bool
	entered  chan struct{}
	finished chan error
	chats    int
	got      domain.ChatInput
}

func (f *chatService) ChatEnabled() bool { return f.enabled }

func (f *chatService) ChatQuota(context.Context, domain.ChatSubject) (domain.ChatQuota, error) {
	return f.quota, f.quotaErr
}

func (f *chatService) Chat(ctx context.Context, in domain.ChatInput, emit func(domain.ChatEvent) error) error {
	f.chats++
	f.got = in
	if f.entered != nil {
		close(f.entered)
	}
	if f.block {
		err := ctx.Err()
		if err == nil {
			<-ctx.Done()
			err = ctx.Err()
		}
		if f.finished != nil {
			f.finished <- err
		}
		return err
	}
	for _, event := range f.events {
		if err := emit(event); err != nil {
			return err
		}
	}
	return f.chatErr
}

// ResolveAnonToken lets ResolveIdentity fall back to the IP subject.
func (f *chatService) ResolveAnonToken(context.Context, string) (int64, bool, error) {
	return 0, false, nil
}

func newChatEngine(t *testing.T, svc usecase.Service, opts ChatOptions) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	if opts.AllowedOrigins == nil {
		opts.AllowedOrigins = []string{testOrigin}
	}
	if opts.MaxConcurrent == 0 {
		opts.MaxConcurrent = 4
	}
	if opts.IPHashKey == nil {
		opts.IPHashKey = []byte("test-ip-key")
	}
	jwt := auth.NewJWT("test-secret", time.Hour, "test-issuer")
	r := gin.New()
	New(svc, jwt, "http://localhost/redirect", "/portfolio/v1", false, opts).Routes(r.Group("/portfolio/v1"))
	return r
}

func chatPost(engine http.Handler, origin, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/portfolio/v1/chat", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	return rec
}

// chatBody is a valid one-message request.
func chatBody() string {
	return `{"locale":"en","pagePath":"/projects/1","messages":[{"role":"user","text":"Which project is this?"}]}`
}

type sseEvent struct {
	name string
	data string
}

func parseSSE(t *testing.T, body string) []sseEvent {
	t.Helper()
	var events []sseEvent
	for _, block := range strings.Split(strings.TrimSuffix(body, "\n\n"), "\n\n") {
		var event sseEvent
		for _, line := range strings.Split(block, "\n") {
			if name, ok := strings.CutPrefix(line, "event: "); ok {
				event.name = name
			}
			if data, ok := strings.CutPrefix(line, "data: "); ok {
				event.data = data
			}
		}
		if event.name != "" {
			events = append(events, event)
		}
	}
	return events
}

func TestChatStreamsEvents(t *testing.T) {
	remaining := 4
	svc := &chatService{enabled: true, events: []domain.ChatEvent{
		{Kind: domain.ChatEventText, Text: "The project is "},
		{Kind: domain.ChatEventText, Text: "AcadAi."},
		{Kind: domain.ChatEventTool, Tool: "get_project"},
		{Kind: domain.ChatEventLinks, Links: []domain.ChatLink{{Title: "AcadAi", URL: "/projects/1", Kind: domain.ChatLinkProject}}},
		{Kind: domain.ChatEventDone, Remaining: &remaining},
	}}
	rec := chatPost(newChatEngine(t, svc, ChatOptions{}), testOrigin, chatBody())

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	for header, want := range map[string]string{
		"Content-Type":      "text/event-stream",
		"Cache-Control":     "no-cache",
		"X-Accel-Buffering": "no",
	} {
		if got := rec.Header().Get(header); got != want {
			t.Errorf("%s = %q, want %q", header, got, want)
		}
	}
	events := parseSSE(t, rec.Body.String())
	wantNames := []string{"text", "text", "tool", "links", "done"}
	if len(events) != len(wantNames) {
		t.Fatalf("events = %+v", events)
	}
	for i, want := range wantNames {
		if events[i].name != want {
			t.Errorf("event %d = %q, want %q", i, events[i].name, want)
		}
	}
	for _, want := range []string{
		`data: {"text":"The project is "}`,
		`data: {"name":"get_project"}`,
		`data: {"links":[{"title":"AcadAi","url":"/projects/1","kind":"project"}]}`,
		`data: {"remainingToday":4}`,
	} {
		if !strings.Contains(rec.Body.String(), want) {
			t.Errorf("stream lacks %s\n%s", want, rec.Body.String())
		}
	}
	if svc.got.Subject.Tier != domain.ChatTierAnonymous || !strings.HasPrefix(svc.got.Subject.Key, "ip:") {
		t.Errorf("subject = %+v", svc.got)
	}
	if svc.got.Locale != "en" || svc.got.PagePath != "/projects/1" {
		t.Errorf("input = %+v", svc.got)
	}
}

func TestChatRejectsBadRequestsBeforeStreaming(t *testing.T) {
	cases := []struct {
		name string
		body string
		err  error
		code string
	}{
		{"too many messages", func() string {
			var messages []string
			for i := 0; i < 13; i++ {
				messages = append(messages, `{"role":"user","text":"hi"}`)
			}
			return `{"locale":"en","messages":[` + strings.Join(messages, ",") + `]}`
		}(), nil, "VALIDATION_ERROR"},
		{"message too long", `{"locale":"en","messages":[{"role":"user","text":"` + strings.Repeat("a", 1001) + `"}]}`, nil, "VALIDATION_ERROR"},
		{"unknown role", `{"locale":"en","messages":[{"role":"system","text":"hi"}]}`, nil, "VALIDATION_ERROR"},
		{"bad locale", `{"locale":"de","messages":[{"role":"user","text":"hi"}]}`, nil, "VALIDATION_ERROR"},
		{"last message not from user", chatBody(), fmt.Errorf("%w: the last message must be from the user", usecase.ErrBadRequest), "VALIDATION_ERROR"},
		{"oversize body", `{"locale":"` + strings.Repeat("a", 17<<10) + `","messages":[{"role":"user","text":"hi"}]}`, nil, "VALIDATION_ERROR"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := &chatService{enabled: true, chatErr: tc.err}
			rec := chatPost(newChatEngine(t, svc, ChatOptions{}), testOrigin, tc.body)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
			}
			if !strings.Contains(rec.Body.String(), `"code":"`+tc.code+`"`) {
				t.Errorf("body = %s", rec.Body)
			}
			if strings.Contains(rec.Body.String(), "event:") {
				t.Error("a rejected request opened a stream")
			}
			if tc.err == nil && svc.chats != 0 {
				t.Error("the use case was called for an invalid body")
			}
		})
	}
}

func TestChatLimitedBeforeStream(t *testing.T) {
	svc := &chatService{enabled: true, chatErr: &domain.ErrChatLimited{Reason: domain.ChatLimitDaily, RetryAfter: 90 * time.Second}}
	rec := chatPost(newChatEngine(t, svc, ChatOptions{}), testOrigin, chatBody())

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	if got := rec.Header().Get("Retry-After"); got != "90" {
		t.Errorf("Retry-After = %q", got)
	}
	if !strings.Contains(rec.Body.String(), `"code":"CHAT_LIMITED"`) {
		t.Errorf("body = %s", rec.Body)
	}
	if strings.Contains(rec.Body.String(), "event:") {
		t.Error("a limited request opened a stream")
	}
}

func TestChatDisabled(t *testing.T) {
	svc := &chatService{enabled: false}
	rec := chatPost(newChatEngine(t, svc, ChatOptions{}), testOrigin, chatBody())
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	if !strings.Contains(rec.Body.String(), `"code":"CHAT_DISABLED"`) {
		t.Errorf("body = %s", rec.Body)
	}
	if svc.chats != 0 {
		t.Error("the use case was called while chat is disabled")
	}

	req := httptest.NewRequest(http.MethodGet, "/portfolio/v1/chat/config", nil)
	cfgRec := httptest.NewRecorder()
	newChatEngine(t, svc, ChatOptions{}).ServeHTTP(cfgRec, req)
	if cfgRec.Code != http.StatusOK || !strings.Contains(cfgRec.Body.String(), `"enabled":false`) {
		t.Errorf("config = %d %s", cfgRec.Code, cfgRec.Body)
	}
}

func TestChatRejectsForeignOrigin(t *testing.T) {
	for _, origin := range []string{"", "https://evil.example", "null"} {
		t.Run(origin, func(t *testing.T) {
			svc := &chatService{enabled: true}
			rec := chatPost(newChatEngine(t, svc, ChatOptions{}), origin, chatBody())
			if rec.Code != http.StatusForbidden {
				t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
			}
			if !strings.Contains(rec.Body.String(), `"code":"ORIGIN_NOT_ALLOWED"`) {
				t.Errorf("body = %s", rec.Body)
			}
			if svc.chats != 0 {
				t.Error("the use case was called from a foreign origin")
			}
		})
	}
}

func TestChatStreamErrorBecomesEvent(t *testing.T) {
	svc := &chatService{enabled: true, events: []domain.ChatEvent{{Kind: domain.ChatEventText, Text: "partial"}}, chatErr: context.DeadlineExceeded}
	rec := chatPost(newChatEngine(t, svc, ChatOptions{}), testOrigin, chatBody())

	// The status line is already sent when the stream has opened.
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	events := parseSSE(t, rec.Body.String())
	if len(events) != 2 || events[1].name != "error" || events[1].data != `{"code":"timeout"}` {
		t.Fatalf("events = %+v", events)
	}
}

func TestChatCancelledClient(t *testing.T) {
	svc := &chatService{enabled: true, block: true, entered: make(chan struct{}), finished: make(chan error, 1)}
	engine := newChatEngine(t, svc, ChatOptions{})
	server := httptest.NewServer(engine)
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, server.URL+"/portfolio/v1/chat", strings.NewReader(chatBody()))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", testOrigin)

	done := make(chan struct{})
	go func() {
		defer close(done)
		resp, err := server.Client().Do(req)
		if err == nil {
			_ = resp.Body.Close()
		}
	}()
	<-svc.entered
	cancel()
	select {
	case err := <-svc.finished:
		if err == nil {
			t.Error("the use case did not see the cancellation")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the handler did not stop after the client went away")
	}
	<-done
}

func TestChatConfig(t *testing.T) {
	remaining := 7
	cases := []struct {
		name    string
		svc     *chatService
		body    string
		headers map[string]string
	}{
		{"enabled with quota", &chatService{enabled: true, quota: domain.ChatQuota{Remaining: &remaining}}, `"enabled":true,"remainingToday":7`, nil},
		{"admin uncapped", &chatService{enabled: true}, `"enabled":true,"remainingToday":null`, nil},
		{"disabled", &chatService{enabled: false}, `"enabled":false,"remainingToday":null`, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/portfolio/v1/chat/config", nil)
			for k, v := range tc.headers {
				req.Header.Set(k, v)
			}
			rec := httptest.NewRecorder()
			newChatEngine(t, tc.svc, ChatOptions{}).ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
			}
			if !strings.Contains(rec.Body.String(), tc.body) {
				t.Errorf("body = %s, want %s", rec.Body, tc.body)
			}
		})
	}
}
