package openrouter

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ninjashadowboy/portfolio-backend-go/internal/domain"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/ports"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// The knowledge index embeds through OpenRouter's /embeddings endpoint, so
// one OPENROUTER_API_KEY covers chat and search. The default models are
// Voyage 4 (served by MongoDB): the series shares one embedding space, so the
// corpus and the queries may use different sizes of it.
const (
	DefaultEmbedDocModel   = "mongodb/voyage-4-large"
	DefaultEmbedQueryModel = "mongodb/voyage-4-lite"
	DefaultDimensions      = 1024
	// DefaultBatchSize and DefaultBatchChars keep one request well under the
	// provider's limits (1,000 inputs and a model-dependent token total):
	// about 4 characters per token makes 120k characters ~30k tokens.
	DefaultBatchSize  = 64
	DefaultBatchChars = 120_000
	embedTimeout      = 30 * time.Second
)

// inputType maps an embedding purpose to OpenRouter's input_type. A live
// check (2026-11-09) showed "search_document" and "document" give the same
// vectors, and both differ from no input_type; these are the documented ones.
var inputType = map[domain.EmbedPurpose]string{
	domain.EmbedDocument: "search_document",
	domain.EmbedQuery:    "search_query",
}

type EmbedderConfig struct {
	APIKey string
	// DocModel embeds documents, QueryModel embeds search queries.
	DocModel   string
	QueryModel string
	// DataCollection ("allow" or "deny") is sent as the provider preference,
	// like the chat's CHAT_DATA_COLLECTION. Empty sends none.
	DataCollection string
	// The fields below default when zero.
	BaseURL    string
	Dimensions int
	BatchSize  int
	BatchChars int
	// MaxRetries is how often a 429 or 5xx is retried; negative disables.
	MaxRetries int
	// Backoff is the first retry delay; it doubles each retry, and a
	// Retry-After header overrides it. Capped at MaxBackoff.
	Backoff    time.Duration
	MaxBackoff time.Duration
	HTTPClient *http.Client
}

// Embedder is the knowledge index's ports.Embedder.
type Embedder struct {
	cfg  EmbedderConfig
	http *http.Client
}

var _ ports.Embedder = (*Embedder)(nil)

func NewEmbedder(cfg EmbedderConfig) *Embedder {
	if cfg.BaseURL == "" {
		cfg.BaseURL = DefaultBaseURL
	}
	cfg.BaseURL = strings.TrimRight(cfg.BaseURL, "/")
	if cfg.DocModel == "" {
		cfg.DocModel = DefaultEmbedDocModel
	}
	if cfg.QueryModel == "" {
		cfg.QueryModel = DefaultEmbedQueryModel
	}
	if cfg.Dimensions <= 0 {
		cfg.Dimensions = DefaultDimensions
	}
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = DefaultBatchSize
	}
	if cfg.BatchChars <= 0 {
		cfg.BatchChars = DefaultBatchChars
	}
	if cfg.MaxRetries == 0 {
		cfg.MaxRetries = 2
	}
	if cfg.Backoff <= 0 {
		cfg.Backoff = time.Second
	}
	if cfg.MaxBackoff <= 0 {
		cfg.MaxBackoff = 20 * time.Second
	}
	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: embedTimeout, Transport: otelhttp.NewTransport(http.DefaultTransport)}
	}
	return &Embedder{cfg: cfg, http: client}
}

type embedRequest struct {
	Input      []string            `json:"input"`
	Model      string              `json:"model"`
	InputType  string              `json:"input_type"`
	Dimensions int                 `json:"dimensions"`
	Provider   *embedProviderPrefs `json:"provider,omitempty"`
}

type embedProviderPrefs struct {
	DataCollection string `json:"data_collection"`
}

type embedResponse struct {
	Data []struct {
		Embedding []float32 `json:"embedding"`
		Index     int       `json:"index"`
	} `json:"data"`
	Error *errorBody `json:"error"`
}

// Embed returns one vector per text, in order. Texts are sent in batches; a
// failed batch fails the whole call.
func (e *Embedder) Embed(ctx context.Context, texts []string, purpose domain.EmbedPurpose) ([][]float32, error) {
	kind, ok := inputType[purpose]
	if !ok {
		return nil, fmt.Errorf("openrouter: unknown embed purpose %q", purpose)
	}
	model := e.cfg.DocModel
	if purpose == domain.EmbedQuery {
		model = e.cfg.QueryModel
	}
	var provider *embedProviderPrefs
	if e.cfg.DataCollection != "" {
		provider = &embedProviderPrefs{DataCollection: e.cfg.DataCollection}
	}
	out := make([][]float32, 0, len(texts))
	for _, batch := range e.batches(texts) {
		vectors, err := e.embedBatch(ctx, embedRequest{
			Input: batch, Model: model, InputType: kind, Dimensions: e.cfg.Dimensions, Provider: provider,
		})
		if err != nil {
			return nil, err
		}
		out = append(out, vectors...)
	}
	return out, nil
}

// batches splits texts by count and by total characters, keeping order.
func (e *Embedder) batches(texts []string) [][]string {
	var out [][]string
	start, chars := 0, 0
	for i, text := range texts {
		n := len(text)
		if i > start && (i-start >= e.cfg.BatchSize || chars+n > e.cfg.BatchChars) {
			out = append(out, texts[start:i])
			start, chars = i, 0
		}
		chars += n
	}
	if start < len(texts) {
		out = append(out, texts[start:])
	}
	return out
}

func (e *Embedder) embedBatch(ctx context.Context, body embedRequest) ([][]float32, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	delay := e.cfg.Backoff
	for attempt := 0; ; attempt++ {
		vectors, err := e.post(ctx, payload, len(body.Input))
		var apiErr *APIError
		if err == nil || !errors.As(err, &apiErr) || !retryable(apiErr) || attempt >= e.cfg.MaxRetries {
			return vectors, err
		}
		wait := delay
		if apiErr.RetryAfter > 0 {
			wait = apiErr.RetryAfter
		}
		wait = min(wait, e.cfg.MaxBackoff)
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, errors.Join(ctx.Err(), err)
		case <-timer.C:
		}
		delay *= 2
	}
}

func retryable(err *APIError) bool {
	return errors.Is(err, ErrRateLimited) || errors.Is(err, ErrUnavailable)
}

func (e *Embedder) post(ctx context.Context, payload []byte, want int) ([][]float32, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.cfg.BaseURL+"/embeddings", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+e.cfg.APIKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Title", attributionTitle)
	resp, err := e.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openrouter: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return nil, fmt.Errorf("openrouter: read response: %w", err)
	}
	if resp.StatusCode/100 != 2 {
		apiErr := responseError(resp.StatusCode, data)
		apiErr.RetryAfter = retryAfter(resp.Header)
		return nil, apiErr
	}
	var parsed embedResponse
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil, fmt.Errorf("openrouter: decode embeddings: %w", err)
	}
	if parsed.Error != nil {
		// A provider failure can come back as an error object with a 200.
		apiErr := parsed.Error.err()
		if apiErr.kind == ErrStream {
			apiErr.kind = ErrUnavailable
		}
		return nil, apiErr
	}
	if len(parsed.Data) != want {
		return nil, fmt.Errorf("openrouter: got %d embeddings for %d inputs", len(parsed.Data), want)
	}
	out := make([][]float32, want)
	for _, item := range parsed.Data {
		if item.Index < 0 || item.Index >= want || out[item.Index] != nil {
			return nil, fmt.Errorf("openrouter: bad embedding index %d", item.Index)
		}
		if len(item.Embedding) != e.cfg.Dimensions {
			return nil, fmt.Errorf("openrouter: embedding has %d dimensions, want %d", len(item.Embedding), e.cfg.Dimensions)
		}
		out[item.Index] = item.Embedding
	}
	return out, nil
}

// retryAfter reads a Retry-After header in seconds; 0 when absent.
func retryAfter(h http.Header) time.Duration {
	if seconds, err := strconv.Atoi(strings.TrimSpace(h.Get("Retry-After"))); err == nil && seconds > 0 {
		return time.Duration(seconds) * time.Second
	}
	return 0
}
