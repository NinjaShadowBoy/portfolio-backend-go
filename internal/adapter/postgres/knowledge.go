package postgres

import (
	"context"
	"encoding/binary"
	"fmt"
	"math"

	"github.com/ninjashadowboy/portfolio-backend-go/internal/db"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/domain"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/ports"
)

var _ ports.KnowledgeRepository = (*repositories)(nil)

func (r *repositories) Knowledge() ports.KnowledgeRepository { return r }

func (r *repositories) ListSourceHashes(ctx context.Context) (map[domain.SourceKey]string, error) {
	rows, err := r.q.ListKnowledgeSourceHashes(ctx)
	if err != nil {
		return nil, repositoryError(err)
	}
	hashes := make(map[domain.SourceKey]string, len(rows))
	for _, row := range rows {
		hashes[domain.SourceKey{Kind: domain.SourceKind(row.SourceKind), Ref: row.SourceRef, Lang: row.Lang}] = row.SourceHash
	}
	return hashes, nil
}

func (r *repositories) ReplaceSourceChunks(ctx context.Context, key domain.SourceKey, hash string, chunks []domain.Chunk) error {
	if err := r.q.DeleteKnowledgeSource(ctx, string(key.Kind), key.Ref, key.Lang); err != nil {
		return repositoryError(err)
	}
	for _, chunk := range chunks {
		err := r.q.InsertKnowledgeChunk(ctx, string(key.Kind), key.Ref, key.Lang, chunk.Title, chunk.URL,
			int32(chunk.Index), chunk.Content, hash, encodeEmbedding(chunk.Embedding))
		if err != nil {
			return repositoryError(err)
		}
	}
	return nil
}

func (r *repositories) DeleteSourcesExcept(ctx context.Context, keep []domain.SourceKey) (int64, error) {
	keys := make([]string, len(keep))
	for i, key := range keep {
		keys[i] = key.String()
	}
	n, err := r.q.DeleteKnowledgeSourcesExcept(ctx, keys)
	return n, repositoryError(err)
}

func (r *repositories) ListEmbeddings(ctx context.Context) ([]domain.ChunkEmbedding, error) {
	rows, err := r.q.ListKnowledgeEmbeddings(ctx)
	if err != nil {
		return nil, repositoryError(err)
	}
	out := make([]domain.ChunkEmbedding, 0, len(rows))
	for _, row := range rows {
		vector, err := decodeEmbedding(row.Embedding)
		if err != nil {
			return nil, fmt.Errorf("knowledge chunk %d: %w", row.ID, err)
		}
		out = append(out, domain.ChunkEmbedding{
			ID: row.ID, Kind: domain.SourceKind(row.SourceKind), Ref: row.SourceRef, Lang: row.Lang, Embedding: vector,
		})
	}
	return out, nil
}

func (r *repositories) SearchFullText(ctx context.Context, query, lang string, kinds []domain.SourceKind, limit int) ([]int64, error) {
	names := make([]string, len(kinds))
	for i, kind := range kinds {
		names[i] = string(kind)
	}
	ids, err := r.q.SearchKnowledgeFullText(ctx, lang, query, names, int32(limit))
	return ids, repositoryError(err)
}

func (r *repositories) GetChunks(ctx context.Context, ids []int64) ([]domain.Chunk, error) {
	rows, err := r.q.GetKnowledgeChunks(ctx, ids)
	return mapRows(rows, fromKnowledgeChunkRow), repositoryError(err)
}

func (r *repositories) ListSources(ctx context.Context) ([]domain.KnowledgeSourceStatus, error) {
	rows, err := r.q.ListKnowledgeSources(ctx)
	if err != nil {
		return nil, repositoryError(err)
	}
	out := make([]domain.KnowledgeSourceStatus, len(rows))
	for i, row := range rows {
		out[i] = domain.KnowledgeSourceStatus{
			Kind: domain.SourceKind(row.SourceKind), Ref: row.SourceRef, Lang: row.Lang,
			Title: row.Title, URL: row.Url, Chunks: int(row.Chunks), UpdatedAt: row.UpdatedAt.UTC(),
		}
	}
	return out, nil
}

func fromKnowledgeChunkRow(row db.GetKnowledgeChunksRow) domain.Chunk {
	return domain.Chunk{
		ID: row.ID, Kind: domain.SourceKind(row.SourceKind), Ref: row.SourceRef, Lang: row.Lang,
		Title: row.Title, URL: row.Url, Index: int(row.ChunkIndex), Content: row.Content,
	}
}

// encodeEmbedding stores a vector as little-endian float32 values, 4 bytes each.
func encodeEmbedding(vector []float32) []byte {
	out := make([]byte, 4*len(vector))
	for i, v := range vector {
		binary.LittleEndian.PutUint32(out[4*i:], math.Float32bits(v))
	}
	return out
}

func decodeEmbedding(data []byte) ([]float32, error) {
	if len(data)%4 != 0 {
		return nil, fmt.Errorf("embedding is %d bytes, not a multiple of 4", len(data))
	}
	out := make([]float32, len(data)/4)
	for i := range out {
		out[i] = math.Float32frombits(binary.LittleEndian.Uint32(data[4*i:]))
	}
	return out, nil
}
