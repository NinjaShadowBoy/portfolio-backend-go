package postgres

import (
	"context"
	"database/sql"
	"math"
	"os"
	"reflect"
	"slices"
	"testing"

	_ "github.com/lib/pq"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/domain"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/ports"
	"github.com/pressly/goose/v3"
)

func TestEmbeddingRoundTrip(t *testing.T) {
	in := []float32{0, 1, -1, 0.5, -0.125, math.MaxFloat32, math.SmallestNonzeroFloat32, float32(math.Inf(-1))}
	data := encodeEmbedding(in)
	if len(data) != 4*len(in) {
		t.Fatalf("encoded %d bytes, want %d", len(data), 4*len(in))
	}
	// Little-endian: 1.0 is 0x3f800000, stored low byte first.
	if !slices.Equal(data[4:8], []byte{0x00, 0x00, 0x80, 0x3f}) {
		t.Fatalf("1.0 encoded as % x", data[4:8])
	}
	out, err := decodeEmbedding(data)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(in, out) {
		t.Fatalf("round trip changed the vector: %v -> %v", in, out)
	}
	if _, err := decodeEmbedding([]byte{1, 2, 3}); err == nil {
		t.Fatal("expected an error for a length that is not a multiple of 4")
	}
	if out, err := decodeEmbedding(nil); err != nil || len(out) != 0 {
		t.Fatalf("empty embedding: %v %v", out, err)
	}
}

// TestKnowledgeQueries runs the knowledge queries against a real, throwaway
// Postgres. It is skipped unless KNOWLEDGE_TEST_DB_URL is set, e.g.
// postgres://app:pw@127.0.0.1:55499/app?sslmode=disable. It migrates that
// database and deletes every knowledge chunk.
func TestKnowledgeQueries(t *testing.T) {
	dsn := os.Getenv("KNOWLEDGE_TEST_DB_URL")
	if dsn == "" {
		t.Skip("KNOWLEDGE_TEST_DB_URL not set")
	}
	sqlDB, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	if err := goose.Up(sqlDB, "../../../db/migrations"); err != nil {
		t.Fatal(err)
	}
	if _, err := sqlDB.Exec("DELETE FROM knowledge_chunks"); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	store := New(sqlDB)
	repo := store.Repositories().Knowledge()

	en := domain.SourceKey{Kind: domain.SourceCV, Ref: "cv", Lang: "en"}
	fr := domain.SourceKey{Kind: domain.SourceCV, Ref: "cv", Lang: "fr"}
	paper := domain.SourceKey{Kind: domain.SourcePublication, Ref: "a/b", Lang: "en"}
	write := func(key domain.SourceKey, hash string, contents ...string) {
		chunks := make([]domain.Chunk, len(contents))
		for i, c := range contents {
			chunks[i] = domain.Chunk{Title: "T", URL: "/about", Index: i, Content: c, Embedding: []float32{float32(i), 1}}
		}
		err := store.WithinTx(ctx, func(r ports.Repositories) error { return r.Knowledge().ReplaceSourceChunks(ctx, key, hash, chunks) })
		if err != nil {
			t.Fatal(err)
		}
	}
	write(en, "h1", "Engineering student developing speech translation", "second chunk")
	write(en, "h2", "Engineering students developing speech translation systems")
	write(fr, "h3", "Élève-ingénieur à Yaoundé, développement de traduction vocale")
	write(paper, "h4", "Whisper and SeamlessM4T on Yoruba")

	hashes, err := repo.ListSourceHashes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if want := map[domain.SourceKey]string{en: "h2", fr: "h3", paper: "h4"}; !reflect.DeepEqual(hashes, want) {
		t.Fatalf("hashes = %v, want %v", hashes, want)
	}
	vectors, err := repo.ListEmbeddings(ctx)
	if err != nil || len(vectors) != 3 || !slices.Equal(vectors[0].Embedding, []float32{0, 1}) {
		t.Fatalf("embeddings = %+v, %v", vectors, err)
	}

	ids, err := repo.SearchFullText(ctx, "ingénieurs développements", "fr", nil, 5)
	if err != nil || len(ids) != 1 {
		t.Fatalf("french search = %v, %v", ids, err)
	}
	chunks, err := repo.GetChunks(ctx, ids)
	if err != nil || len(chunks) != 1 || chunks[0].Lang != "fr" || chunks[0].URL != "/about" {
		t.Fatalf("chunks = %+v, %v", chunks, err)
	}
	if ids, err := repo.SearchFullText(ctx, "yoruba", "en", []domain.SourceKind{domain.SourceCV}, 5); err != nil || len(ids) != 0 {
		t.Fatalf("kind filter: %v, %v", ids, err)
	}
	if ids, err := repo.SearchFullText(ctx, "yoruba", "en", []domain.SourceKind{domain.SourcePublication}, 5); err != nil || len(ids) != 1 {
		t.Fatalf("kind filter: %v, %v", ids, err)
	}

	removed, err := repo.DeleteSourcesExcept(ctx, []domain.SourceKey{en, paper})
	if err != nil || removed != 1 {
		t.Fatalf("removed %d, %v", removed, err)
	}
	hashes, _ = repo.ListSourceHashes(ctx)
	if _, ok := hashes[fr]; ok || len(hashes) != 2 {
		t.Fatalf("after delete: %v", hashes)
	}
}
