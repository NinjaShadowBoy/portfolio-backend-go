package ports

import (
	"context"

	"github.com/ninjashadowboy/portfolio-backend-go/internal/domain"
)

// KnowledgeRepository stores the assistant's knowledge index.
type KnowledgeRepository interface {
	// ListSourceHashes returns the stored hash of every indexed source.
	ListSourceHashes(ctx context.Context) (map[domain.SourceKey]string, error)
	// ReplaceSourceChunks deletes the source's chunks and inserts chunks, all
	// tagged with hash. Call it inside a UnitOfWork so the swap is atomic.
	ReplaceSourceChunks(ctx context.Context, key domain.SourceKey, hash string, chunks []domain.Chunk) error
	// DeleteSourcesExcept removes every source not in keep and returns the
	// number of chunk rows removed.
	DeleteSourcesExcept(ctx context.Context, keep []domain.SourceKey) (int64, error)
	// ListEmbeddings returns every chunk's vector, for the in-memory index.
	ListEmbeddings(ctx context.Context) ([]domain.ChunkEmbedding, error)
	// SearchFullText returns chunk ids matching query, best first. lang picks
	// the text-search configuration of the query; an empty kinds means all.
	SearchFullText(ctx context.Context, query, lang string, kinds []domain.SourceKind, limit int) ([]int64, error)
	// GetChunks returns the chunks with these ids, without embeddings, in no
	// particular order. Unknown ids are skipped.
	GetChunks(ctx context.Context, ids []int64) ([]domain.Chunk, error)
	// ListSources returns one status row per indexed source (kind, ref, lang)
	// with its chunk count and the last time one of its chunks changed.
	ListSources(ctx context.Context) ([]domain.KnowledgeSourceStatus, error)
}
