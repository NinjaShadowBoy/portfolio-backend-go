// Package openrouter implements ports.ChatModel with OpenRouter's
// OpenAI-compatible Chat Completions API (streaming). It is a plain net/http
// client: the wire format is small and stable, so no SDK is needed.
package openrouter

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ninjashadowboy/portfolio-backend-go/internal/domain"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/ports"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

const (
	DefaultBaseURL = "https://openrouter.ai/api/v1"
	// DefaultModel is OpenRouter's free models router: it picks a free model
	// among those that support the request, tool calling included. It is for
	// testing and local runs, so nothing costs money by accident.
	DefaultModel = "openrouter/free"
	// attributionTitle is the app name OpenRouter shows in its rankings.
	attributionTitle = "alexabena.me assistant"
	defaultMaxTokens = 800
	// requestTimeout bounds one stream end to end. The caller's context is
	// tighter in practice: the chat handler gives a stream 90 s.
	requestTimeout = 2 * time.Minute
	maxErrorBytes  = 8 << 10
)

// Errors callers can test with errors.Is. Every error from Stream that came
// from the API wraps one of these and an *APIError.
var (
	ErrUnauthorized = errors.New("openrouter: invalid or missing API key")
	ErrRateLimited  = errors.New("openrouter: rate limited")
	ErrUnavailable  = errors.New("openrouter: service unavailable")
	ErrBadRequest   = errors.New("openrouter: request rejected")
	ErrStream       = errors.New("openrouter: stream error")
)

// APIError is a failed request: a non-2xx response (Status set), or an error
// object inside an open stream (Status 0).
type APIError struct {
	Status  int
	Message string
	// RetryAfter is the 429 Retry-After delay, when the server sent one.
	RetryAfter time.Duration
	kind       error
}

func (e *APIError) Error() string {
	if e.Status == 0 {
		return "openrouter: stream error: " + e.Message
	}
	return fmt.Sprintf("openrouter: HTTP %d: %s", e.Status, e.Message)
}

func (e *APIError) Unwrap() error { return e.kind }

// Reasoning settings. Haiku 5.5 thinks adaptively by default and its
// thinking tokens count against max_tokens, so the assistant turns it off.
const (
	// ReasoningDefault sends no reasoning field. With require_parameters on,
	// an explicit field could exclude every free model that does not accept
	// it, so openrouter/free runs with this setting.
	ReasoningDefault = "default"
	ReasoningOff     = "off"
	ReasoningLow     = "low"
	ReasoningMedium  = "medium"
	ReasoningHigh    = "high"
)

type Config struct {
	APIKey string
	Model  string
	// ProviderSort asks OpenRouter to route by price, throughput or latency;
	// empty keeps its default routing. require_parameters is always on: the
	// assistant needs a provider that supports tools.
	ProviderSort string
	// DataCollection is "allow" or "deny": whether the chosen provider may
	// train on the conversation. Many free models require "allow".
	DataCollection string
	// Reasoning is off, low, medium or high.
	Reasoning string
	// SiteURL is the public site; its origin becomes the HTTP-Referer
	// attribution header. Optional.
	SiteURL string
	// BaseURL overrides the API root (tests). Empty means production.
	BaseURL string
	// HTTPClient overrides the client; nil means one with an otelhttp
	// transport and a timeout.
	HTTPClient *http.Client
}

type Client struct {
	apiKey         string
	model          string
	providerSort   string
	dataCollection string
	reasoning      string
	referer        string
	baseURL        string
	http           *http.Client
	tokens         metric.Int64Counter
}

var _ ports.ChatModel = (*Client)(nil)

func New(cfg Config) *Client {
	model := strings.TrimSpace(cfg.Model)
	if model == "" {
		model = DefaultModel
	}
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: requestTimeout, Transport: otelhttp.NewTransport(http.DefaultTransport)}
	}
	// The global meter provider is a no-op until telemetry.Setup installs
	// one, and instruments made before that are delegated to it.
	tokens, err := otel.Meter("github.com/ninjashadowboy/portfolio-backend-go/internal/adapter/openrouter").
		Int64Counter("chat.tokens", metric.WithUnit("{token}"), metric.WithDescription("Tokens used by the chat model, by kind"))
	if err != nil {
		otel.Handle(err)
	}
	return &Client{
		apiKey:         cfg.APIKey,
		model:          model,
		providerSort:   strings.TrimSpace(cfg.ProviderSort),
		dataCollection: strings.TrimSpace(cfg.DataCollection),
		reasoning:      strings.TrimSpace(cfg.Reasoning),
		referer:        origin(cfg.SiteURL),
		baseURL:        baseURL,
		http:           client,
		tokens:         tokens,
	}
}

// Model is the configured model id.
func (c *Client) Model() string { return c.model }

// origin turns the public site URL into the bare origin OpenRouter wants in
// HTTP-Referer, or "" when it does not parse.
func origin(siteURL string) string {
	u, err := url.Parse(strings.TrimSpace(siteURL))
	if err != nil || u.Scheme == "" || u.Host == "" {
		return ""
	}
	return u.Scheme + "://" + u.Host
}

// Stream runs one streamed Chat Completions call.
func (c *Client) Stream(ctx context.Context, req domain.ChatTurnRequest, onText func(string) error) (domain.ChatTurnResult, error) {
	payload, err := c.body(req)
	if err != nil {
		return domain.ChatTurnResult{}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return domain.ChatTurnResult{}, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	httpReq.Header.Set("X-OpenRouter-Title", attributionTitle)
	if c.referer != "" {
		httpReq.Header.Set("HTTP-Referer", c.referer)
	}

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return domain.ChatTurnResult{}, fmt.Errorf("openrouter: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBytes))
		return domain.ChatTurnResult{}, responseError(resp.StatusCode, body)
	}

	result, err := c.readStream(resp.Body, onText)
	c.recordTokens(ctx, result.Usage)
	return result, err
}

func (c *Client) readStream(body io.Reader, onText func(string) error) (domain.ChatTurnResult, error) {
	var result domain.ChatTurnResult
	var finish string
	done := false
	calls := map[int]*toolCall{}

	reader := bufio.NewReader(body)
	for {
		line, readErr := reader.ReadString('\n')
		line = strings.TrimRight(line, "\r\n")
		if payload, ok := dataLine(line); ok {
			if payload == "[DONE]" {
				done = true
				break
			}
			var chunk streamChunk
			if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
				return result, fmt.Errorf("openrouter: decode chunk: %w", err)
			}
			if chunk.Error != nil {
				return result, chunk.Error.err()
			}
			if chunk.Usage != nil {
				result.Usage = chunk.Usage.toDomain()
			}
			for _, choice := range chunk.Choices {
				// The usage chunk repeats the finish reason of the turn; it
				// is accounting, not a second terminal event.
				if choice.FinishReason != nil && *choice.FinishReason != "" {
					finish = *choice.FinishReason
				}
				if choice.Delta.Content != "" {
					result.Text += choice.Delta.Content
					if err := onText(choice.Delta.Content); err != nil {
						return result, err
					}
				}
				for _, delta := range choice.Delta.ToolCalls {
					call, ok := calls[delta.Index]
					if !ok {
						call = &toolCall{}
						calls[delta.Index] = call
					}
					if delta.ID != "" {
						call.id = delta.ID
					}
					// Name and arguments both arrive in fragments across
					// chunks; concatenating in order rebuilds them.
					call.name += delta.Function.Name
					call.arguments += delta.Function.Arguments
				}
			}
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				break
			}
			return result, fmt.Errorf("openrouter: read stream: %w", readErr)
		}
	}

	// A truncated connection with no finish reason would look like an empty
	// answer; report it instead. A provider that closes without [DONE] but
	// sent a finish reason is accepted.
	if !done && finish == "" {
		return result, fmt.Errorf("%w: ended before the turn finished", ErrStream)
	}
	result.StopReason = stopReason(finish)
	result.ToolCalls = sortedToolCalls(calls)
	return result, nil
}

// toolCall accumulates one streamed tool call.
type toolCall struct {
	id        string
	name      string
	arguments string
}

func sortedToolCalls(calls map[int]*toolCall) []domain.ToolCall {
	if len(calls) == 0 {
		return nil
	}
	indexes := make([]int, 0, len(calls))
	for index := range calls {
		indexes = append(indexes, index)
	}
	sort.Ints(indexes)
	out := make([]domain.ToolCall, 0, len(indexes))
	for _, index := range indexes {
		call := calls[index]
		id := call.id
		if id == "" {
			id = "call_" + strconv.Itoa(index)
		}
		input := strings.TrimSpace(call.arguments)
		if input == "" {
			input = "{}"
		}
		// Invalid JSON is passed through: the use case turns it into an
		// error result the model can read.
		out = append(out, domain.ToolCall{ID: id, Name: call.name, Input: json.RawMessage(input)})
	}
	return out
}

// stopReason maps an OpenAI finish_reason to the domain's provider-neutral
// stop reason.
func stopReason(finish string) string {
	switch finish {
	case "tool_calls", "function_call":
		return domain.ChatStopToolUse
	case "length":
		return domain.ChatStopMaxTokens
	case "stop":
		return domain.ChatStopEndTurn
	}
	return finish
}

// dataLine returns the payload of an SSE data line. Comment lines (":"
// keep-alives) and other fields (event:, id:, retry:) are ignored.
func dataLine(line string) (string, bool) {
	if !strings.HasPrefix(line, "data:") {
		return "", false
	}
	return strings.TrimSpace(strings.TrimPrefix(line, "data:")), true
}

func responseError(status int, body []byte) *APIError {
	e := &APIError{Status: status, Message: errorMessage(body)}
	switch {
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		e.kind = ErrUnauthorized
	case status == http.StatusTooManyRequests:
		e.kind = ErrRateLimited
	case status >= 500:
		e.kind = ErrUnavailable
	default:
		e.kind = ErrBadRequest
	}
	return e
}

// errorMessage pulls OpenRouter's {"error": {"message": ...}} message,
// falling back to the start of the body. The API key never appears in a
// response body.
func errorMessage(body []byte) string {
	var parsed struct {
		Error *errorBody `json:"error"`
	}
	if json.Unmarshal(body, &parsed) == nil && parsed.Error != nil && parsed.Error.Message != "" {
		return parsed.Error.Message
	}
	msg := strings.TrimSpace(string(body))
	if len(msg) > 200 {
		msg = msg[:200]
	}
	return msg
}

// errorBody is the error object of a non-2xx response or of an in-stream
// error chunk.
type errorBody struct {
	Code    any    `json:"code"`
	Message string `json:"message"`
}

func (e *errorBody) err() *APIError {
	apiErr := &APIError{Message: e.Message, kind: ErrStream}
	code, ok := toInt(e.Code)
	if !ok {
		return apiErr
	}
	switch {
	case code == http.StatusTooManyRequests:
		apiErr.kind = ErrRateLimited
	case code >= 500:
		apiErr.kind = ErrUnavailable
	case code >= 400:
		apiErr.kind = ErrBadRequest
	}
	return apiErr
}

func toInt(v any) (int, bool) {
	switch n := v.(type) {
	case float64:
		return int(n), true
	case string:
		parsed, err := strconv.Atoi(n)
		return parsed, err == nil
	}
	return 0, false
}

// --- request ---

type chatRequest struct {
	Model        string         `json:"model"`
	Messages     []chatMessage  `json:"messages"`
	Tools        []chatTool     `json:"tools,omitempty"`
	ToolChoice   string         `json:"tool_choice,omitempty"`
	MaxTokens    int            `json:"max_tokens"`
	Stream       bool           `json:"stream"`
	CacheControl *cacheControl  `json:"cache_control,omitempty"`
	Provider     *providerPrefs `json:"provider,omitempty"`
	Reasoning    *reasoning     `json:"reasoning,omitempty"`
}

type chatMessage struct {
	Role string `json:"role"`
	// Content is null on an assistant turn that only calls tools, as the
	// OpenAI format and OpenRouter's examples show.
	Content    *string           `json:"content"`
	ToolCalls  []messageToolCall `json:"tool_calls,omitempty"`
	ToolCallID string            `json:"tool_call_id,omitempty"`
}

type messageToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function toolFunction `json:"function"`
}

type toolFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type chatTool struct {
	Type     string         `json:"type"`
	Function toolDefinition `json:"function"`
}

type toolDefinition struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters"`
}

// cacheControl enables Anthropic's automatic prompt caching: OpenRouter puts
// the breakpoint on the last cacheable block and advances it as the
// conversation grows.
type cacheControl struct {
	Type string `json:"type"`
}

type providerPrefs struct {
	Sort              string `json:"sort,omitempty"`
	RequireParameters bool   `json:"require_parameters"`
	DataCollection    string `json:"data_collection,omitempty"`
}

type reasoning struct {
	Enabled *bool  `json:"enabled,omitempty"`
	Effort  string `json:"effort,omitempty"`
	// Exclude keeps reasoning text out of the response; we never show it.
	Exclude bool `json:"exclude,omitempty"`
}

func (c *Client) body(req domain.ChatTurnRequest) ([]byte, error) {
	messages, err := toMessages(req)
	if err != nil {
		return nil, err
	}
	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = defaultMaxTokens
	}
	body := chatRequest{
		Model:        c.model,
		Messages:     messages,
		Tools:        toTools(req.Tools),
		MaxTokens:    maxTokens,
		Stream:       true,
		CacheControl: &cacheControl{Type: "ephemeral"},
		Provider: &providerPrefs{
			Sort:              c.providerSort,
			RequireParameters: true,
			DataCollection:    c.dataCollection,
		},
		Reasoning: c.reasoningParam(),
	}
	// Tools stay defined so earlier tool turns remain valid and the cached
	// prefix is unchanged; the model is just told to answer in text.
	if req.NoToolUse && len(body.Tools) > 0 {
		body.ToolChoice = "none"
	}
	return json.Marshal(body)
}

// reasoningParam maps CHAT_REASONING to OpenRouter's unified reasoning
// field: default (or empty) omits it, off sends {"enabled": false}
// (Anthropic's thinking disabled), the other levels send an effort.
func (c *Client) reasoningParam() *reasoning {
	switch c.reasoning {
	case ReasoningLow, ReasoningMedium, ReasoningHigh:
		return &reasoning{Effort: c.reasoning, Exclude: true}
	case ReasoningOff:
		off := false
		return &reasoning{Enabled: &off}
	default:
		return nil
	}
}

func toTools(specs []domain.ToolSpec) []chatTool {
	tools := make([]chatTool, 0, len(specs))
	for _, spec := range specs {
		schema := spec.InputSchema
		if schema == nil {
			schema = map[string]any{"type": "object", "properties": map[string]any{}}
		}
		tools = append(tools, chatTool{
			Type: "function",
			Function: toolDefinition{
				Name:        spec.Name,
				Description: spec.Description,
				Parameters:  schema,
			},
		})
	}
	return tools
}

func toMessages(req domain.ChatTurnRequest) ([]chatMessage, error) {
	out := make([]chatMessage, 0, len(req.Messages)+2)
	if req.System != "" {
		out = append(out, chatMessage{Role: "system", Content: text(req.System)})
	}
	if req.SystemContext != "" {
		out = append(out, chatMessage{Role: "system", Content: text(req.SystemContext)})
	}
	for _, m := range req.Messages {
		switch m.Role {
		case domain.ChatRoleUser:
			for _, result := range m.ToolResults {
				out = append(out, chatMessage{Role: "tool", ToolCallID: result.CallID, Content: text(result.Content)})
			}
			if m.Text != "" {
				out = append(out, chatMessage{Role: "user", Content: text(m.Text)})
			}
			if m.Text == "" && len(m.ToolResults) == 0 {
				return nil, errors.New("openrouter: empty user message")
			}
		case domain.ChatRoleAssistant:
			calls := make([]messageToolCall, 0, len(m.ToolCalls))
			for _, call := range m.ToolCalls {
				arguments := strings.TrimSpace(string(call.Input))
				if arguments == "" || !json.Valid([]byte(arguments)) {
					arguments = "{}"
				}
				calls = append(calls, messageToolCall{
					ID:       call.ID,
					Type:     "function",
					Function: toolFunction{Name: call.Name, Arguments: arguments},
				})
			}
			if m.Text == "" && len(calls) == 0 {
				return nil, errors.New("openrouter: empty assistant message")
			}
			message := chatMessage{Role: "assistant", ToolCalls: calls}
			if m.Text != "" {
				message.Content = text(m.Text)
			}
			out = append(out, message)
		default:
			return nil, fmt.Errorf("openrouter: unknown role %q", m.Role)
		}
	}
	return out, nil
}

func text(s string) *string { return &s }

// --- response ---

type streamChunk struct {
	Choices []struct {
		Delta struct {
			Content   string `json:"content"`
			ToolCalls []struct {
				Index    int    `json:"index"`
				ID       string `json:"id"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"delta"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
	Usage *usage     `json:"usage"`
	Error *errorBody `json:"error"`
}

type usage struct {
	PromptTokens        int64 `json:"prompt_tokens"`
	CompletionTokens    int64 `json:"completion_tokens"`
	PromptTokensDetails struct {
		CachedTokens     int64 `json:"cached_tokens"`
		CacheWriteTokens int64 `json:"cache_write_tokens"`
	} `json:"prompt_tokens_details"`
}

// toDomain maps OpenRouter's usage to the domain type. prompt_tokens already
// includes the cached part, and the domain counts cached tokens separately in
// TotalInput, so subtract them to avoid counting twice.
func (u *usage) toDomain() domain.ChatTokenUsage {
	cached := u.PromptTokensDetails.CachedTokens
	written := u.PromptTokensDetails.CacheWriteTokens
	return domain.ChatTokenUsage{
		InputTokens:      max(u.PromptTokens-cached-written, 0),
		OutputTokens:     u.CompletionTokens,
		CacheReadTokens:  cached,
		CacheWriteTokens: written,
	}
}

func (c *Client) recordTokens(ctx context.Context, u domain.ChatTokenUsage) {
	if c.tokens == nil {
		return
	}
	// Recorded even when the request was cancelled: the tokens were spent.
	ctx = context.WithoutCancel(ctx)
	for kind, n := range map[string]int64{
		"input": u.InputTokens, "output": u.OutputTokens, "cache_read": u.CacheReadTokens, "cache_write": u.CacheWriteTokens,
	} {
		if n > 0 {
			c.tokens.Add(ctx, n, metric.WithAttributes(attribute.String("model", c.model), attribute.String("kind", kind)))
		}
	}
}
