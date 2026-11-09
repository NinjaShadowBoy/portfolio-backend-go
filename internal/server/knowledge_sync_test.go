package server

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ninjashadowboy/portfolio-backend-go/internal/domain"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/ports"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/usecase"
)

type countingSyncer struct {
	calls atomic.Int32
	err   error
}

func (c *countingSyncer) SyncKnowledgeNow(context.Context) (domain.KnowledgeSyncReport, error) {
	c.calls.Add(1)
	return domain.KnowledgeSyncReport{}, c.err
}

func TestKnowledgeSyncRunsAtStartAndOnInterval(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	syncer := &countingSyncer{err: errors.New("voyage down")}
	done := startKnowledgeSync(ctx, syncer, 10*time.Millisecond)
	deadline := time.Now().Add(5 * time.Second)
	for syncer.calls.Load() < 3 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if syncer.calls.Load() < 3 {
		t.Fatalf("%d syncs, want the startup sync plus interval syncs even after errors", syncer.calls.Load())
	}
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the sync loop did not stop on shutdown")
	}
}

// blockingEmbedder never answers until the request context ends, like a
// Voyage API that hangs.
type blockingEmbedder struct{ entered chan struct{} }

func (b blockingEmbedder) Embed(ctx context.Context, _ []string, _ domain.EmbedPurpose) ([][]float32, error) {
	select {
	case b.entered <- struct{}{}:
	default:
	}
	<-ctx.Done()
	return nil, ctx.Err()
}

type staticCorpus []domain.KnowledgeSource

func (c staticCorpus) Load(context.Context) ([]domain.KnowledgeSource, error) { return c, nil }

type emptyKnowledgeRepos struct {
	ports.Repositories
	ports.ProjectsRepository
	ports.KnowledgeRepository
}

func (r emptyKnowledgeRepos) Projects() ports.ProjectsRepository   { return r }
func (r emptyKnowledgeRepos) Knowledge() ports.KnowledgeRepository { return r }
func (emptyKnowledgeRepos) ListProjects(context.Context) ([]domain.Project, error) {
	return nil, nil
}
func (emptyKnowledgeRepos) ListSourceHashes(context.Context) (map[domain.SourceKey]string, error) {
	return nil, nil
}
func (emptyKnowledgeRepos) DeleteSourcesExcept(context.Context, []domain.SourceKey) (int64, error) {
	return 0, nil
}
func (emptyKnowledgeRepos) ListEmbeddings(context.Context) ([]domain.ChunkEmbedding, error) {
	return nil, nil
}

func TestKnowledgeSyncDoesNotBlockStartup(t *testing.T) {
	embedder := blockingEmbedder{entered: make(chan struct{}, 1)}
	svc := usecase.New(usecase.Dependencies{
		Repositories: emptyKnowledgeRepos{}, Embedder: embedder, EmbeddingDocModel: "m",
		KnowledgeCorpus: staticCorpus{{Kind: domain.SourceCV, Ref: "cv", Lang: "en", Title: "CV", URL: "/about", Text: "text"}},
	})
	ctx, cancel := context.WithCancel(context.Background())
	start := time.Now()
	done := startKnowledgeSync(ctx, svc, time.Hour)
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("starting the sync took %v; startup must not wait for Voyage", elapsed)
	}
	select {
	case <-embedder.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("the startup sync never reached the embedder")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("a sync blocked in Voyage did not stop on shutdown")
	}
}
