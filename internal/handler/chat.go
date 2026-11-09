package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/domain"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/dto"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/middleware"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/usecase"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

const (
	// chatBodyBytes caps the POST /chat body: twelve 1000-character messages
	// plus their JSON overhead fit comfortably.
	chatBodyBytes = 16 << 10
	// chatTimeout bounds one stream. The router exempts /chat from the global
	// 30 s request timeout, and the server's WriteTimeout is extended to match.
	chatTimeout = 90 * time.Second
	// chatPingInterval keeps an idle stream alive through proxies.
	chatPingInterval = 15 * time.Second
	// chatRateEvery and chatRateBurst are the per-subject burst limiter: one
	// message every 4 s, three in a row.
	chatRateEvery = 4 * time.Second
	chatRateBurst = 3
)

// chatSubjectKey keys the burst limiter by chat subject.
func chatSubjectKey(c *gin.Context) string {
	subject, _ := middleware.ChatSubjectFromContext(c)
	return subject.Key
}

// ChatOptions configures the chat routes' middleware.
type ChatOptions struct {
	// AllowedOrigins are the CORS_ALLOWED_ORIGINS patterns a POST /chat
	// Origin must match. CORS alone does not stop non-browser clients.
	AllowedOrigins []string
	// IPHashKey keys the HMAC that turns a client IP into a subject.
	IPHashKey []byte
	// MaxConcurrent caps simultaneous streams.
	MaxConcurrent int
}

// Chat answers POST /chat with an SSE stream. Errors found before the first
// event (disabled chat, caps, invalid messages) are ordinary JSON responses
// with their status code; once the stream has opened they arrive as an error
// event, because the status line is already sent.
func (h *api) Chat(c *gin.Context) {
	if !h.svc.ChatEnabled() {
		middleware.WriteError(c, http.StatusNotFound, "CHAT_DISABLED", "The chat assistant is disabled")
		return
	}
	subject, ok := middleware.ChatSubjectFromContext(c)
	if !ok {
		middleware.WriteError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Internal server error")
		return
	}
	var req dto.ChatRequest
	if !bindJSONLimit(c, &req, chatBodyBytes) {
		return
	}
	messages := make([]domain.ChatMessage, len(req.Messages))
	for i, message := range req.Messages {
		messages[i] = domain.ChatMessage{Role: domain.ChatRole(message.Role), Text: message.Text}
	}

	// A client disconnect cancels the request context, which aborts the model
	// stream as well.
	ctx, cancel := context.WithTimeout(c.Request.Context(), chatTimeout)
	defer cancel()
	// The server's WriteTimeout is 30 s, too short for an answer plus tools.
	_ = http.NewResponseController(c.Writer).SetWriteDeadline(time.Now().Add(chatTimeout))

	stream := &chatStream{c: c}
	emit := func(event domain.ChatEvent) error {
		if !stream.started {
			stream.start()
		}
		return stream.send(event)
	}
	err := h.svc.Chat(ctx, domain.ChatInput{
		Subject:  subject,
		Locale:   req.Locale,
		PagePath: req.PagePath,
		Messages: messages,
	}, emit)
	h.recordChatRequest(ctx, err)
	if err != nil {
		if stream.started {
			// The status line is already sent; the failure can only be an
			// event. Write it before stop closes the stream.
			_ = stream.write("error", chatErrorEvent{Code: chatErrorCode(err)})
		}
	}
	stream.stop()
	if err == nil {
		return
	}
	if !stream.started {
		writeChatError(c, err)
	}
}

// ChatConfig answers GET /chat/config. The frontend hides the mascot when
// enabled is false.
func (h *api) ChatConfig(c *gin.Context) {
	if !h.svc.ChatEnabled() {
		c.JSON(http.StatusOK, dto.ChatConfigResponse{Enabled: false})
		return
	}
	subject, ok := middleware.ChatSubjectFromContext(c)
	if !ok {
		middleware.WriteError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Internal server error")
		return
	}
	quota, err := h.svc.ChatQuota(c.Request.Context(), subject)
	if errors.Is(err, usecase.ErrChatDisabled) {
		c.JSON(http.StatusOK, dto.ChatConfigResponse{Enabled: false})
		return
	}
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, dto.ChatConfigResponse{Enabled: true, RemainingToday: quota.Remaining})
}

// writeChatError maps a pre-stream failure to its HTTP status. A missing
// Retry-After here would leave the client guessing when the cap resets.
func writeChatError(c *gin.Context, err error) {
	var limited *domain.ErrChatLimited
	switch {
	case errors.As(err, &limited):
		middleware.WriteRetryAfter(c, limited.RetryAfter)
		middleware.WriteError(c, http.StatusTooManyRequests, "CHAT_LIMITED", "The assistant has reached its limit for today")
	case errors.Is(err, usecase.ErrBadRequest):
		middleware.WriteError(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
	case errors.Is(err, usecase.ErrChatDisabled):
		middleware.WriteError(c, http.StatusNotFound, "CHAT_DISABLED", "The chat assistant is disabled")
	case errors.Is(err, context.DeadlineExceeded):
		middleware.WriteError(c, http.StatusGatewayTimeout, "CHAT_TIMEOUT", "The assistant took too long")
	case errors.Is(err, context.Canceled):
		// The client went away: there is nobody to answer.
		c.Abort()
	default:
		middleware.WriteError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Internal server error")
	}
}

// chatErrorCode is the code of an error event sent after the stream opened.
func chatErrorCode(err error) string {
	var limited *domain.ErrChatLimited
	switch {
	case errors.As(err, &limited):
		return "limited"
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case errors.Is(err, context.Canceled):
		return "cancelled"
	}
	return "internal"
}

// recordChatRequest counts finished streams by outcome. It is a no-op until
// telemetry.Setup installs a meter provider.
func (h *api) recordChatRequest(ctx context.Context, err error) {
	if h.chatRequests == nil {
		return
	}
	var limited *domain.ErrChatLimited
	outcome := "ok"
	switch {
	case err == nil:
	case errors.As(err, &limited):
		outcome = "limited"
	case errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded):
		outcome = "cancelled"
	default:
		outcome = "error"
	}
	h.chatRequests.Add(context.WithoutCancel(ctx), 1, metric.WithAttributes(attribute.String("outcome", outcome)))
}

// SSE event data shapes. The names and JSON keys are the frontend contract.
type (
	chatTextEvent struct {
		Text string `json:"text"`
	}
	chatToolEvent struct {
		Name string `json:"name"`
	}
	chatLinksEvent struct {
		Links []domain.ChatLink `json:"links"`
	}
	chatDoneEvent struct {
		// RemainingToday is null for a subject with no daily cap (an admin).
		RemainingToday *int `json:"remainingToday"`
	}
	chatErrorEvent struct {
		Code string `json:"code"`
	}
)

// chatStream writes the SSE framing. Every write and the keep-alive pings
// share a mutex, because gin's writer is not safe for concurrent use.
type chatStream struct {
	c       *gin.Context
	started bool

	mu     sync.Mutex
	closed bool
	done   chan struct{}
}

func (s *chatStream) start() {
	s.started = true
	header := s.c.Writer.Header()
	header.Set("Content-Type", "text/event-stream")
	header.Set("Cache-Control", "no-cache")
	header.Set("X-Accel-Buffering", "no")
	s.c.Writer.WriteHeader(http.StatusOK)
	s.c.Writer.Flush()
	s.done = make(chan struct{})
	go s.ping()
}

// ping writes a comment line every chatPingInterval until stop closes the
// stream.
func (s *chatStream) ping() {
	ticker := time.NewTicker(chatPingInterval)
	defer ticker.Stop()
	for {
		select {
		case <-s.done:
			return
		case <-ticker.C:
			s.mu.Lock()
			if !s.closed {
				_, _ = s.c.Writer.WriteString(": ping\n\n")
				s.c.Writer.Flush()
			}
			s.mu.Unlock()
		}
	}
}

func (s *chatStream) stop() {
	if !s.started {
		return
	}
	close(s.done)
	s.mu.Lock()
	s.closed = true
	s.mu.Unlock()
}

func (s *chatStream) send(event domain.ChatEvent) error {
	switch event.Kind {
	case domain.ChatEventText:
		return s.write("text", chatTextEvent{Text: event.Text})
	case domain.ChatEventTool:
		return s.write("tool", chatToolEvent{Name: event.Tool})
	case domain.ChatEventLinks:
		return s.write("links", chatLinksEvent{Links: event.Links})
	case domain.ChatEventDone:
		return s.write("done", chatDoneEvent{RemainingToday: event.Remaining})
	case domain.ChatEventMove:
		return s.write("move", chatToolEvent{Name: event.Move})
	}
	return fmt.Errorf("chat: unknown event kind %q", event.Kind)
}

func (s *chatStream) write(name string, data any) error {
	payload, err := json.Marshal(data)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return context.Canceled
	}
	if _, err := fmt.Fprintf(s.c.Writer, "event: %s\ndata: %s\n\n", name, payload); err != nil {
		return err
	}
	s.c.Writer.Flush()
	return nil
}

// chatRequestCounter is created once so every chat route reports the same
// instrument.
func chatRequestCounter() metric.Int64Counter {
	counter, err := otel.Meter("github.com/ninjashadowboy/portfolio-backend-go/internal/handler").
		Int64Counter("chat.requests", metric.WithUnit("{request}"), metric.WithDescription("Chat requests, by outcome"))
	if err != nil {
		otel.Handle(err)
		return nil
	}
	return counter
}
