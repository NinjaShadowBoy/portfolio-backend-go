-- name: DeleteKnowledgeSource :exec
DELETE FROM knowledge_chunks
WHERE source_kind = $1 AND source_ref = $2 AND lang = $3;

-- name: InsertKnowledgeChunk :exec
INSERT INTO knowledge_chunks (source_kind, source_ref, lang, title, url, chunk_index, content, source_hash, embedding)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9);

-- name: ListKnowledgeSourceHashes :many
-- One row per source. Every chunk of a source carries the same hash.
SELECT DISTINCT source_kind, source_ref, lang, source_hash
FROM knowledge_chunks;

-- name: ListKnowledgeEmbeddings :many
SELECT id, source_kind, source_ref, lang, embedding
FROM knowledge_chunks
ORDER BY id;

-- name: SearchKnowledgeFullText :many
-- The query is parsed with the visitor's language; each chunk was indexed
-- with its own language (see the generated tsv column).
SELECT k.id
FROM knowledge_chunks AS k,
     websearch_to_tsquery(
         CASE WHEN sqlc.arg(lang)::text = 'fr' THEN 'french'::regconfig ELSE 'english'::regconfig END,
         sqlc.arg(query)::text) AS q
WHERE k.tsv @@ q
  AND (cardinality(sqlc.arg(kinds)::text[]) = 0 OR k.source_kind = ANY(sqlc.arg(kinds)::text[]))
ORDER BY ts_rank(k.tsv, q) DESC, k.id
LIMIT sqlc.arg(max_results)::int;

-- name: GetKnowledgeChunks :many
SELECT id, source_kind, source_ref, lang, title, url, chunk_index, content
FROM knowledge_chunks
WHERE id = ANY(sqlc.arg(ids)::bigint[]);

-- name: ListKnowledgeSources :many
-- One row per source (kind, ref, lang) with its chunk count and the last
-- change of one of its chunks. Title and url are per source but putting them
-- in the GROUP BY keeps the query over one table.
SELECT source_kind, source_ref, lang, title, url,
       COUNT(*)::int AS chunks,
       MAX(updated_at)::timestamp AS updated_at
FROM knowledge_chunks
GROUP BY source_kind, source_ref, lang, title, url
ORDER BY source_kind, source_ref, lang;

-- name: DeleteKnowledgeSourcesExcept :execrows
-- Removes every source whose key is not in keep. A key is
-- kind || '/' || ref || '/' || lang; kind and lang never contain '/', so the
-- key is unambiguous even when ref does.
DELETE FROM knowledge_chunks
WHERE source_kind || '/' || source_ref || '/' || lang <> ALL(sqlc.arg(keep)::text[]);
