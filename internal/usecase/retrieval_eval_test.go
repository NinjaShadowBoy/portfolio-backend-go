//go:build eval

// Retrieval eval against real embeddings through OpenRouter. Not part of
// `go test ./...`:
//
//	OPENROUTER_API_KEY=... go test -tags eval ./internal/usecase -run TestRetrievalEval -v
//
// It indexes knowledge/ into the in-memory store (or into Postgres when
// EVAL_DB_URL names a migrated database, which also brings its projects),
// asks every question in knowledge/eval.yaml and reports hit@3.
package usecase_test

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	_ "github.com/lib/pq"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/adapter/openrouter"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/adapter/postgres"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/knowledge"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/ports"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/usecase"
)

type evalQuestion struct {
	lang, q string
	expect  []string
}

// parseEval reads the YAML subset eval.yaml is written in.
func parseEval(t *testing.T, path string) []evalQuestion {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var out []evalQuestion
	for _, raw := range strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") || line == "questions:" {
			continue
		}
		if rest, ok := strings.CutPrefix(line, "- "); ok {
			out = append(out, evalQuestion{})
			line = rest
		}
		if len(out) == 0 {
			t.Fatalf("eval.yaml: %q before the first question", line)
		}
		key, value, _ := strings.Cut(line, ":")
		value = strings.TrimSpace(value)
		cur := &out[len(out)-1]
		switch strings.TrimSpace(key) {
		case "lang":
			cur.lang = value
		case "q":
			cur.q = value
		case "expect":
			for _, ref := range strings.Split(strings.Trim(value, "[]"), ",") {
				cur.expect = append(cur.expect, strings.TrimSpace(ref))
			}
		default:
			t.Fatalf("eval.yaml: unknown key in %q", line)
		}
	}
	return out
}

func TestRetrievalEval(t *testing.T) {
	key := os.Getenv("OPENROUTER_API_KEY")
	if key == "" {
		t.Skip("OPENROUTER_API_KEY not set")
	}
	dir := filepath.Join("..", "..", "knowledge")
	questions := parseEval(t, filepath.Join(dir, "eval.yaml"))
	embedder := openrouter.NewEmbedder(openrouter.EmbedderConfig{
		APIKey:     key,
		DocModel:   envOr("EMBEDDING_DOC_MODEL", openrouter.DefaultEmbedDocModel),
		QueryModel: envOr("EMBEDDING_QUERY_MODEL", openrouter.DefaultEmbedQueryModel),
	})

	var repos ports.Repositories
	var uow ports.UnitOfWork
	if dsn := os.Getenv("EVAL_DB_URL"); dsn != "" {
		db, err := sql.Open("postgres", dsn)
		if err != nil {
			t.Fatal(err)
		}
		defer db.Close()
		store := postgres.New(db)
		repos, uow = store.Repositories(), store
	} else {
		store := newKnowledgeStore()
		repos, uow = store, store
	}
	svc := usecase.New(usecase.Dependencies{
		Repositories: repos, UnitOfWork: uow, Embedder: embedder,
		KnowledgeCorpus: knowledge.NewDir(dir), EmbeddingDocModel: envOr("EMBEDDING_DOC_MODEL", "voyage-4-large"),
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	report, err := svc.SyncKnowledgeNow(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("indexed %d sources (%d newly embedded, %d chunks)", report.Sources, report.Added+report.Updated, report.Chunks)

	hits := map[string][2]int{}
	for _, q := range questions {
		results, err := svc.SearchKnowledge(ctx, q.q, q.lang, nil, 3)
		if err != nil {
			t.Fatalf("%q: %v", q.q, err)
		}
		var got []string
		hit := false
		for _, r := range results {
			ref := string(r.Chunk.Kind) + "/" + r.Chunk.Ref
			got = append(got, fmt.Sprintf("%s(%s)", ref, r.Chunk.Lang))
			hit = hit || slices.Contains(q.expect, ref)
		}
		counts := hits[q.lang]
		counts[1]++
		if hit {
			counts[0]++
		} else {
			t.Logf("MISS [%s] %s\n  expected %v\n  got      %v", q.lang, q.q, q.expect, got)
		}
		hits[q.lang] = counts
	}
	total, n := 0, 0
	for lang, c := range hits {
		t.Logf("hit@3 %s: %d/%d", lang, c[0], c[1])
		total += c[0]
		n += c[1]
	}
	t.Logf("hit@3 overall: %d/%d = %.2f", total, n, float64(total)/float64(n))
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
