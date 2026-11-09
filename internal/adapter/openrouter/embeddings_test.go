package openrouter

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ninjashadowboy/portfolio-backend-go/internal/domain"
)

type recordedEmbed struct {
	auth string
	body embedRequest
}

// fakeEmbeddings answers like OpenRouter's /embeddings: vector i is
// [index, len(input), 0...], returned in reverse order to check the sort.
func fakeEmbeddings(t *testing.T, dims int, status func(call int) (int, string)) (*httptest.Server, *[]recordedEmbed) {
	t.Helper()
	var mu sync.Mutex
	var calls []recordedEmbed
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/embeddings" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("content type %q", ct)
		}
		var body embedRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode: %v", err)
		}
		mu.Lock()
		calls = append(calls, recordedEmbed{auth: r.Header.Get("Authorization"), body: body})
		n := len(calls)
		mu.Unlock()
		if status != nil {
			if code, msg := status(n); code != 0 {
				if code == http.StatusTooManyRequests {
					w.Header().Set("Retry-After", "1")
				}
				w.WriteHeader(code)
				_, _ = w.Write([]byte(msg))
				return
			}
		}
		type item struct {
			Embedding []float32 `json:"embedding"`
			Index     int       `json:"index"`
		}
		resp := struct {
			Data  []item         `json:"data"`
			Usage map[string]int `json:"usage"`
		}{Usage: map[string]int{"prompt_tokens": 10, "total_tokens": 10}}
		for i := len(body.Input) - 1; i >= 0; i-- {
			v := make([]float32, dims)
			v[0], v[1] = float32(i), float32(len(body.Input[i]))
			resp.Data = append(resp.Data, item{Embedding: v, Index: i})
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(srv.Close)
	return srv, &calls
}

func newTestEmbedder(url string) *Embedder {
	return NewEmbedder(EmbedderConfig{
		APIKey: "secret", DocModel: "mongodb/voyage-4-large", QueryModel: "mongodb/voyage-4-lite", DataCollection: "deny",
		BaseURL: url + "/api/v1", Dimensions: 4, BatchSize: 2, Backoff: time.Millisecond, MaxBackoff: 5 * time.Millisecond,
	})
}

func TestEmbedRequestShapeAndPurposeModel(t *testing.T) {
	srv, calls := fakeEmbeddings(t, 4, nil)
	e := newTestEmbedder(srv.URL)

	if _, err := e.Embed(context.Background(), []string{"a doc"}, domain.EmbedDocument); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Embed(context.Background(), []string{"a query"}, domain.EmbedQuery); err != nil {
		t.Fatal(err)
	}
	got := *calls
	if len(got) != 2 {
		t.Fatalf("%d calls", len(got))
	}
	if got[0].auth != "Bearer secret" {
		t.Errorf("auth header %q", got[0].auth)
	}
	// OpenRouter's field names, checked live: dimensions (output_dimension is
	// ignored), input_type search_document/search_query, array input.
	want := embedRequest{Input: []string{"a doc"}, Model: "mongodb/voyage-4-large", InputType: "search_document", Dimensions: 4,
		Provider: &embedProviderPrefs{DataCollection: "deny"}}
	if !equalEmbedRequest(got[0].body, want) {
		t.Errorf("document request %+v, want %+v", got[0].body, want)
	}
	want = embedRequest{Input: []string{"a query"}, Model: "mongodb/voyage-4-lite", InputType: "search_query", Dimensions: 4,
		Provider: &embedProviderPrefs{DataCollection: "deny"}}
	if !equalEmbedRequest(got[1].body, want) {
		t.Errorf("query request %+v, want %+v", got[1].body, want)
	}
	if _, err := e.Embed(context.Background(), []string{"x"}, "other"); err == nil {
		t.Error("expected an error for an unknown purpose")
	}
}

func TestEmbedRequestOmitsProviderWithoutDataCollection(t *testing.T) {
	payload, err := json.Marshal(embedRequest{Input: []string{"x"}, Model: "m", InputType: "search_query", Dimensions: 8})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(payload), "provider") || !strings.Contains(string(payload), `"dimensions":8`) {
		t.Fatalf("payload %s", payload)
	}
}

func TestNewEmbedderDefaults(t *testing.T) {
	e := NewEmbedder(EmbedderConfig{APIKey: "k"})
	if e.cfg.DocModel != "mongodb/voyage-4-large" || e.cfg.QueryModel != "mongodb/voyage-4-lite" ||
		e.cfg.Dimensions != 1024 || e.cfg.BaseURL != "https://openrouter.ai/api/v1" {
		t.Fatalf("defaults %+v", e.cfg)
	}
}

func equalEmbedRequest(a, b embedRequest) bool {
	sameProvider := (a.Provider == nil) == (b.Provider == nil) && (a.Provider == nil || *a.Provider == *b.Provider)
	return a.Model == b.Model && a.InputType == b.InputType && a.Dimensions == b.Dimensions && sameProvider &&
		strings.Join(a.Input, "\x00") == strings.Join(b.Input, "\x00")
}

func TestEmbedBatchesAndKeepsOrder(t *testing.T) {
	srv, calls := fakeEmbeddings(t, 4, nil)
	texts := []string{"a", "bb", "ccc", "dddd", "eeeee"}
	vectors, err := newTestEmbedder(srv.URL).Embed(context.Background(), texts, domain.EmbedDocument)
	if err != nil {
		t.Fatal(err)
	}
	if len(*calls) != 3 {
		t.Fatalf("%d requests for 5 texts with batch size 2, want 3", len(*calls))
	}
	for i, v := range vectors {
		if int(v[1]) != len(texts[i]) {
			t.Fatalf("vector %d belongs to a text of length %v, want %d", i, v[1], len(texts[i]))
		}
	}
}

func TestBatchesRespectCharacterBudget(t *testing.T) {
	e := NewEmbedder(EmbedderConfig{BatchSize: 10, BatchChars: 5})
	got := e.batches([]string{"abc", "de", "f", "ghijklmn", "o"})
	want := [][]string{{"abc", "de"}, {"f"}, {"ghijklmn"}, {"o"}}
	if len(got) != len(want) {
		t.Fatalf("batches = %q, want %q", got, want)
	}
	for i := range want {
		if strings.Join(got[i], ",") != strings.Join(want[i], ",") {
			t.Fatalf("batches = %q, want %q", got, want)
		}
	}
	if len(e.batches(nil)) != 0 {
		t.Fatal("no texts should make no batches")
	}
}

func TestEmbedErrorMapping(t *testing.T) {
	for _, tc := range []struct {
		status int
		want   error
		calls  int
	}{
		{http.StatusUnauthorized, ErrUnauthorized, 1},
		{http.StatusBadRequest, ErrBadRequest, 1},
		{http.StatusTooManyRequests, ErrRateLimited, 3},
		{http.StatusBadGateway, ErrUnavailable, 3},
	} {
		srv, calls := fakeEmbeddings(t, 4, func(int) (int, string) { return tc.status, `{"error":{"code":400,"message":"nope"}}` })
		_, err := newTestEmbedder(srv.URL).Embed(context.Background(), []string{"x"}, domain.EmbedDocument)
		if !errors.Is(err, tc.want) {
			t.Errorf("status %d: error %v, want %v", tc.status, err, tc.want)
		}
		var apiErr *APIError
		if !errors.As(err, &apiErr) || apiErr.Status != tc.status || apiErr.Message != "nope" {
			t.Errorf("status %d: api error %+v", tc.status, apiErr)
		}
		if strings.Contains(err.Error(), "secret") {
			t.Errorf("error leaks the API key: %v", err)
		}
		if len(*calls) != tc.calls {
			t.Errorf("status %d: %d calls, want %d (retries only for 429/5xx)", tc.status, len(*calls), tc.calls)
		}
	}
}

func TestEmbedErrorObjectWithStatus200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"error":{"code":502,"message":"provider down"}}`))
	}))
	t.Cleanup(srv.Close)
	e := NewEmbedder(EmbedderConfig{APIKey: "k", BaseURL: srv.URL, Dimensions: 4, MaxRetries: -1})
	_, err := e.Embed(context.Background(), []string{"x"}, domain.EmbedQuery)
	if !errors.Is(err, ErrUnavailable) || !strings.Contains(err.Error(), "provider down") {
		t.Fatalf("err %v, want an unavailable error with the message", err)
	}
}

func TestEmbedRetriesThenSucceeds(t *testing.T) {
	srv, calls := fakeEmbeddings(t, 4, func(call int) (int, string) {
		if call == 1 {
			return http.StatusServiceUnavailable, "busy"
		}
		return 0, ""
	})
	vectors, err := newTestEmbedder(srv.URL).Embed(context.Background(), []string{"x"}, domain.EmbedQuery)
	if err != nil || len(vectors) != 1 || len(*calls) != 2 {
		t.Fatalf("vectors %v, err %v, calls %d", vectors, err, len(*calls))
	}
}

func TestRetryAfterIsParsed(t *testing.T) {
	if got := retryAfter(http.Header{"Retry-After": {"7"}}); got != 7*time.Second {
		t.Fatalf("got %v", got)
	}
	if got := retryAfter(http.Header{}); got != 0 {
		t.Fatalf("got %v without the header", got)
	}
}

func TestEmbedRejectsWrongDimensions(t *testing.T) {
	srv, _ := fakeEmbeddings(t, 3, nil)
	if _, err := newTestEmbedder(srv.URL).Embed(context.Background(), []string{"x"}, domain.EmbedDocument); err == nil {
		t.Fatal("expected a dimension mismatch error")
	}
}

func TestEmbedStopsOnCancelledContext(t *testing.T) {
	srv, _ := fakeEmbeddings(t, 4, func(int) (int, string) { return http.StatusTooManyRequests, "slow down" })
	e := NewEmbedder(EmbedderConfig{APIKey: "k", BaseURL: srv.URL + "/api/v1", Dimensions: 4, Backoff: time.Hour, MaxBackoff: time.Hour})
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := e.Embed(ctx, []string{"x"}, domain.EmbedDocument)
	if !errors.Is(err, context.DeadlineExceeded) || time.Since(start) > 5*time.Second {
		t.Fatalf("err %v after %v", err, time.Since(start))
	}
}
