package server

import (
	"context"
	"log/slog"
	"time"

	"github.com/ninjashadowboy/portfolio-backend-go/internal/domain"
)

// syncTimeout bounds one sync, so a hung Voyage request cannot stall the loop.
const syncTimeout = 5 * time.Minute

type knowledgeSyncer interface {
	SyncKnowledgeNow(ctx context.Context) (domain.KnowledgeSyncReport, error)
}

// startKnowledgeSync syncs the knowledge index in the background: once now,
// then every interval, until ctx ends. It returns at once; the returned
// channel closes when the loop has stopped. Errors are logged and the stored
// index stays as it was, so a Voyage outage never affects the API.
func startKnowledgeSync(ctx context.Context, svc knowledgeSyncer, interval time.Duration) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			runKnowledgeSync(ctx, svc)
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	return done
}

func runKnowledgeSync(ctx context.Context, svc knowledgeSyncer) {
	if ctx.Err() != nil {
		return
	}
	syncCtx, cancel := context.WithTimeout(ctx, syncTimeout)
	defer cancel()
	start := time.Now()
	report, err := svc.SyncKnowledgeNow(syncCtx)
	if err != nil {
		if ctx.Err() == nil {
			slog.Error("knowledge sync failed", "error", err, "duration", time.Since(start))
		}
		return
	}
	if report.Added+report.Updated > 0 || report.Deleted > 0 {
		slog.Info("knowledge index updated", "sources", report.Sources, "added", report.Added,
			"updated", report.Updated, "chunks", report.Chunks, "deleted_chunks", report.Deleted,
			"duration", time.Since(start))
	}
}

func closedChannel() <-chan struct{} {
	c := make(chan struct{})
	close(c)
	return c
}
