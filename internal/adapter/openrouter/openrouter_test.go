package openrouter

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ninjashadowboy/portfolio-backend-go/internal/domain"
)

// data renders one SSE data event.
func data(payload string) string { return "data: " + payload + "\n\n" }

// capturedRequest is what the test server saw.
type capturedRequest struct {
	Body   map[string]any
	Header http.Header
}

// sseServer replays a canned SSE stream at /chat/completions.
func sseServer(t *testing.T, stream string, captured *capturedRequest) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/chat/completions" {
			http.Error(w, "unexpected "+r.Method+" "+r.URL.Path, http.StatusNotFound)
			return
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			http.Error(w, "missing api key", http.StatusUnauthorized)
			return
		}
		if captured != nil {
			raw, _ := io.ReadAll(r.Body)
			if err := json.Unmarshal(raw, &captured.Body); err != nil {
				t.Errorf("request body is not JSON: %v", err)
			}
			captured.Header = r.Header.Clone()
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, stream)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func testRequest() domain.ChatTurnRequest {
	return domain.ChatTurnRequest{
		System:        "You are the assistant.",
		SystemContext: "The visitor is on the page /projects/3.",
		MaxTokens:     800,
		Tools: []domain.ToolSpec{
			{Name: "list_projects", Description: "Lists projects.", InputSchema: map[string]any{"type": "object", "properties": map[string]any{}}},
			{Name: "get_project", Description: "One project.", InputSchema: map[string]any{
				"type": "object", "properties": map[string]any{"id": map[string]any{"type": "integer"}},
				"required": []string{"id"},
			}},
		},
		Messages: []domain.ChatMessage{
			{Role: domain.ChatRoleUser, Text: "What is this project?"},
			{Role: domain.ChatRoleAssistant, Text: "Looking.", ToolCalls: []domain.ToolCall{{ID: "toolu_0", Name: "list_projects", Input: json.RawMessage(`{}`)}}},
			{Role: domain.ChatRoleUser, ToolResults: []domain.ToolResult{{CallID: "toolu_0", Content: `{"projects":[]}`}}},
		},
	}
}

func TestStreamTextWithUsageChunk(t *testing.T) {
	var captured capturedRequest
	stream := "" +
		": OPENROUTER PROCESSING\n\n" +
		data(`{"choices":[{"delta":{"role":"assistant","content":""},"finish_reason":null}]}`) +
		data(`{"choices":[{"delta":{"content":"Hello"},"finish_reason":null}]}`) +
		data(`{"choices":[{"delta":{"content":" there."},"finish_reason":null}]}`) +
		data(`{"choices":[{"delta":{"content":""},"finish_reason":"stop"}]}`) +
		// The usage chunk repeats the finish reason: accounting, not a
		// second terminal event.
		data(`{"choices":[{"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":120,"completion_tokens":9,"total_tokens":129,"prompt_tokens_details":{"cached_tokens":100,"cache_write_tokens":5}}}`) +
		data(`[DONE]`)
	srv := sseServer(t, stream, &captured)
	c := New(Config{APIKey: "test-key", BaseURL: srv.URL, HTTPClient: srv.Client()})

	var deltas []string
	result, err := c.Stream(context.Background(), testRequest(), func(s string) error {
		deltas = append(deltas, s)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(deltas, "|") != "Hello| there." {
		t.Fatalf("text deltas = %q", deltas)
	}
	if result.Text != "Hello there." || result.StopReason != domain.ChatStopEndTurn || len(result.ToolCalls) != 0 {
		t.Fatalf("result = %+v", result)
	}
	want := domain.ChatTokenUsage{InputTokens: 15, OutputTokens: 9, CacheReadTokens: 100, CacheWriteTokens: 5}
	if result.Usage != want {
		t.Fatalf("usage = %+v, want %+v", result.Usage, want)
	}
	if got := captured.Body["model"]; got != DefaultModel {
		t.Fatalf("model = %v, want the free router default", got)
	}
	if got := captured.Header.Get("X-OpenRouter-Title"); got != attributionTitle {
		t.Fatalf("X-OpenRouter-Title = %q", got)
	}
	if got := captured.Header.Get("HTTP-Referer"); got != "" {
		t.Fatalf("HTTP-Referer without a site URL = %q, want empty", got)
	}
}

func TestStreamAccumulatesSplitToolCalls(t *testing.T) {
	var captured capturedRequest
	stream := "" +
		data(`{"choices":[{"delta":{"role":"assistant","content":""},"finish_reason":null}]}`) +
		data(`{"choices":[{"delta":{"content":"Let me look"},"finish_reason":null}]}`) +
		data(`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_abc","type":"function","function":{"name":"get_","arguments":""}}]},"finish_reason":null}]}`) +
		data(`{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"name":"project","arguments":"{\"id\":"}}]},"finish_reason":null}]}`) +
		data(`{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"3}"}}]},"finish_reason":null}]}`) +
		data(`{"choices":[{"delta":{"tool_calls":[{"index":1,"id":"call_def","type":"function","function":{"name":"list_projects","arguments":"{}"}}]},"finish_reason":null}]}`) +
		data(`{"choices":[{"delta":{},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":50,"completion_tokens":20}}`) +
		data(`[DONE]`)
	srv := sseServer(t, stream, &captured)
	c := New(Config{APIKey: "test-key", BaseURL: srv.URL, HTTPClient: srv.Client()})

	result, err := c.Stream(context.Background(), testRequest(), func(string) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if result.Text != "Let me look" || result.StopReason != domain.ChatStopToolUse {
		t.Fatalf("result = %+v", result)
	}
	if len(result.ToolCalls) != 2 {
		t.Fatalf("tool calls = %+v", result.ToolCalls)
	}
	first, second := result.ToolCalls[0], result.ToolCalls[1]
	if first.ID != "call_abc" || first.Name != "get_project" || string(first.Input) != `{"id":3}` {
		t.Fatalf("first tool call = %+v (%s)", first, first.Input)
	}
	if second.ID != "call_def" || second.Name != "list_projects" || string(second.Input) != `{}` {
		t.Fatalf("second tool call = %+v (%s)", second, second.Input)
	}
}

func TestStreamIgnoresKeepAliveComments(t *testing.T) {
	stream := ": OPENROUTER PROCESSING\n\n" +
		": another keep-alive\n\n" +
		data(`{"choices":[{"delta":{"content":"Hi"},"finish_reason":"stop"}]}`) +
		data(`[DONE]`)
	srv := sseServer(t, stream, nil)
	c := New(Config{APIKey: "test-key", BaseURL: srv.URL, HTTPClient: srv.Client()})

	result, err := c.Stream(context.Background(), testRequest(), func(string) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if result.Text != "Hi" {
		t.Fatalf("text = %q", result.Text)
	}
}

func TestStreamStopsAtDone(t *testing.T) {
	stream := data(`{"choices":[{"delta":{"content":"Hi"},"finish_reason":"stop"}]}`) +
		data(`[DONE]`) +
		// Anything after [DONE] is not part of the answer.
		data(`{"choices":[{"delta":{"content":" ignored"},"finish_reason":"stop"}]}`)
	srv := sseServer(t, stream, nil)
	c := New(Config{APIKey: "test-key", BaseURL: srv.URL, HTTPClient: srv.Client()})

	var deltas []string
	result, err := c.Stream(context.Background(), testRequest(), func(s string) error {
		deltas = append(deltas, s)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Text != "Hi" || len(deltas) != 1 {
		t.Fatalf("text = %q after deltas %q", result.Text, deltas)
	}
}

func TestStreamMapsAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"error":{"code":400,"message":"bad model"}}`)
	}))
	defer srv.Close()
	c := New(Config{APIKey: "test-key", BaseURL: srv.URL, HTTPClient: srv.Client()})

	_, err := c.Stream(context.Background(), testRequest(), func(string) error { return nil })
	if !errors.Is(err, ErrBadRequest) {
		t.Fatalf("err = %v, want ErrBadRequest", err)
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Status != http.StatusBadRequest || !strings.Contains(apiErr.Message, "bad model") {
		t.Fatalf("api error = %+v", apiErr)
	}
}

func TestStreamMapsInStreamError(t *testing.T) {
	stream := data(`{"choices":[{"delta":{"content":"Hi"},"finish_reason":null}]}`) +
		data(`{"error":{"code":502,"message":"provider down"}}`)
	srv := sseServer(t, stream, nil)
	c := New(Config{APIKey: "test-key", BaseURL: srv.URL, HTTPClient: srv.Client()})

	result, err := c.Stream(context.Background(), testRequest(), func(string) error { return nil })
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err = %v, want ErrUnavailable", err)
	}
	// The text that reached the visitor before the failure is kept.
	if result.Text != "Hi" {
		t.Fatalf("partial text = %q", result.Text)
	}
}

func TestStreamMapsRateLimitWithoutRetry(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = io.WriteString(w, `{"error":{"code":429,"message":"free-models-per-day"}}`)
	}))
	defer srv.Close()
	c := New(Config{APIKey: "test-key", BaseURL: srv.URL, HTTPClient: srv.Client()})

	_, err := c.Stream(context.Background(), testRequest(), func(string) error { return nil })
	if !errors.Is(err, ErrRateLimited) {
		t.Fatalf("err = %v, want ErrRateLimited", err)
	}
	if calls != 1 {
		t.Fatalf("server was called %d times, want no retry", calls)
	}
}

func TestStreamCancelAbortsRequest(t *testing.T) {
	released := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, data(`{"choices":[{"delta":{"content":"Hi"},"finish_reason":null}]}`))
		w.(http.Flusher).Flush()
		select {
		case <-r.Context().Done():
		case <-time.After(10 * time.Second):
		}
		close(released)
	}))
	defer srv.Close()
	c := New(Config{APIKey: "test-key", BaseURL: srv.URL, HTTPClient: srv.Client()})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := c.Stream(ctx, testRequest(), func(string) error {
			cancel()
			return nil
		})
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected an error after cancel")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Stream did not return after the context was cancelled")
	}
	select {
	case <-released:
	case <-time.After(5 * time.Second):
		t.Fatal("the upstream request was not aborted")
	}
}

func TestRequestBodyShape(t *testing.T) {
	var captured capturedRequest
	srv := sseServer(t, data(`[DONE]`), &captured)
	c := New(Config{
		APIKey:         "test-key",
		Model:          "anthropic/claude-haiku-5.5",
		ProviderSort:   "price",
		DataCollection: "deny",
		Reasoning:      ReasoningOff,
		SiteURL:        "https://alexabena.me/fr/",
		BaseURL:        srv.URL,
		HTTPClient:     srv.Client(),
	})

	if _, err := c.Stream(context.Background(), testRequest(), func(string) error { return nil }); err != nil {
		t.Fatal(err)
	}
	body := captured.Body
	if body["model"] != "anthropic/claude-haiku-5.5" || body["stream"] != true || body["max_tokens"] != float64(800) {
		t.Fatalf("model/stream/max_tokens = %v %v %v", body["model"], body["stream"], body["max_tokens"])
	}
	if cache := body["cache_control"].(map[string]any); cache["type"] != "ephemeral" {
		t.Fatalf("cache_control = %v", cache)
	}
	provider := body["provider"].(map[string]any)
	if provider["sort"] != "price" || provider["require_parameters"] != true || provider["data_collection"] != "deny" {
		t.Fatalf("provider = %v", provider)
	}
	if reasoning := body["reasoning"].(map[string]any); reasoning["enabled"] != false || reasoning["effort"] != nil {
		t.Fatalf("reasoning = %v", reasoning)
	}
	if got := captured.Header.Get("HTTP-Referer"); got != "https://alexabena.me" {
		t.Fatalf("HTTP-Referer = %q", got)
	}

	tools := body["tools"].([]any)
	if len(tools) != 2 {
		t.Fatalf("tools = %v", tools)
	}
	last := tools[1].(map[string]any)
	function := last["function"].(map[string]any)
	if last["type"] != "function" || function["name"] != "get_project" || function["description"] != "One project." {
		t.Fatalf("tool = %v", last)
	}
	if required := function["parameters"].(map[string]any)["required"].([]any); len(required) != 1 || required[0] != "id" {
		t.Fatalf("parameters = %v", function["parameters"])
	}

	messages := body["messages"].([]any)
	if len(messages) != 5 {
		t.Fatalf("messages = %v", messages)
	}
	first := messages[0].(map[string]any)
	second := messages[1].(map[string]any)
	if first["role"] != "system" || first["content"] != "You are the assistant." {
		t.Fatalf("first system message = %v", first)
	}
	if second["role"] != "system" || second["content"] != "The visitor is on the page /projects/3." {
		t.Fatalf("second system message = %v", second)
	}
	if messages[2].(map[string]any)["role"] != "user" {
		t.Fatalf("user message = %v", messages[2])
	}
	assistant := messages[3].(map[string]any)
	if assistant["role"] != "assistant" || assistant["content"] != "Looking." {
		t.Fatalf("assistant message = %v", assistant)
	}
	call := assistant["tool_calls"].([]any)[0].(map[string]any)
	if call["id"] != "toolu_0" || call["type"] != "function" || call["function"].(map[string]any)["name"] != "list_projects" {
		t.Fatalf("assistant tool call = %v", call)
	}
	toolResult := messages[4].(map[string]any)
	if toolResult["role"] != "tool" || toolResult["tool_call_id"] != "toolu_0" || toolResult["content"] != `{"projects":[]}` {
		t.Fatalf("tool result message = %v", toolResult)
	}
}

func TestRequestBodyAssistantToolCallsWithoutText(t *testing.T) {
	var captured capturedRequest
	srv := sseServer(t, data(`[DONE]`), &captured)
	c := New(Config{APIKey: "test-key", BaseURL: srv.URL, HTTPClient: srv.Client()})

	req := testRequest()
	req.Messages = []domain.ChatMessage{
		{Role: domain.ChatRoleUser, Text: "Which projects?"},
		{Role: domain.ChatRoleAssistant, ToolCalls: []domain.ToolCall{{ID: "call_1", Name: "list_projects", Input: json.RawMessage(`{}`)}}},
		{Role: domain.ChatRoleUser, ToolResults: []domain.ToolResult{{CallID: "call_1", Content: `[]`}}},
	}
	if _, err := c.Stream(context.Background(), req, func(string) error { return nil }); err != nil {
		t.Fatal(err)
	}
	// Two system messages come first, so the assistant turn is index 3.
	assistant := captured.Body["messages"].([]any)[3].(map[string]any)
	if content, ok := assistant["content"]; !ok || content != nil {
		t.Fatalf("assistant content = %v, want null", content)
	}
}

func TestRequestBodyNoToolUse(t *testing.T) {
	var captured capturedRequest
	srv := sseServer(t, data(`[DONE]`), &captured)
	c := New(Config{APIKey: "test-key", BaseURL: srv.URL, HTTPClient: srv.Client()})

	req := testRequest()
	req.NoToolUse = true
	if _, err := c.Stream(context.Background(), req, func(string) error { return nil }); err != nil {
		t.Fatal(err)
	}
	body := captured.Body
	if body["tool_choice"] != "none" {
		t.Fatalf("tool_choice = %v", body["tool_choice"])
	}
	if len(body["tools"].([]any)) != 2 {
		t.Fatalf("tools should stay defined: %v", body["tools"])
	}
}

func TestRequestBodyReasoningEffort(t *testing.T) {
	var captured capturedRequest
	srv := sseServer(t, data(`[DONE]`), &captured)
	c := New(Config{APIKey: "test-key", Reasoning: ReasoningLow, BaseURL: srv.URL, HTTPClient: srv.Client()})

	if _, err := c.Stream(context.Background(), testRequest(), func(string) error { return nil }); err != nil {
		t.Fatal(err)
	}
	reasoning := captured.Body["reasoning"].(map[string]any)
	if reasoning["effort"] != "low" || reasoning["exclude"] != true || reasoning["enabled"] != nil {
		t.Fatalf("reasoning = %v", reasoning)
	}
}

// The default setting omits the field: with require_parameters on, an
// explicit reasoning field could leave openrouter/free with no free model.
func TestRequestBodyReasoningDefaultOmitsField(t *testing.T) {
	for _, setting := range []string{ReasoningDefault, ""} {
		var captured capturedRequest
		srv := sseServer(t, data(`[DONE]`), &captured)
		c := New(Config{APIKey: "test-key", Reasoning: setting, BaseURL: srv.URL, HTTPClient: srv.Client()})

		if _, err := c.Stream(context.Background(), testRequest(), func(string) error { return nil }); err != nil {
			t.Fatal(err)
		}
		if _, ok := captured.Body["reasoning"]; ok {
			t.Fatalf("Reasoning %q sent a reasoning field: %v", setting, captured.Body["reasoning"])
		}
	}
}
