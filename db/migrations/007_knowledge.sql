-- +goose Up
-- Knowledge index for the site assistant: author-written corpus files
-- (knowledge/*.md) and project text from the projects table, split into
-- chunks. Feedback comments and contact messages are never stored here.
-- Embeddings are little-endian float32 vectors in BYTEA; similarity is
-- computed in memory by the API, so no pgvector extension is needed.
CREATE TABLE knowledge_chunks (
    id BIGSERIAL PRIMARY KEY,
    source_kind TEXT NOT NULL CHECK (source_kind IN ('project', 'publication', 'cv', 'about')),
    -- Project id or corpus slug.
    source_ref TEXT NOT NULL,
    lang TEXT NOT NULL CHECK (lang IN ('en', 'fr')),
    title TEXT NOT NULL,
    -- Site path without the locale prefix, e.g. /projects/12.
    url TEXT NOT NULL,
    chunk_index INT NOT NULL,
    content TEXT NOT NULL,
    -- sha256 of the whole source text and the document embedding model; an
    -- unchanged hash means the source is not embedded again.
    source_hash TEXT NOT NULL,
    -- Little-endian float32 x dimensions (1024).
    embedding BYTEA NOT NULL,
    tsv tsvector GENERATED ALWAYS AS (to_tsvector(
        CASE lang WHEN 'fr' THEN 'french'::regconfig ELSE 'english'::regconfig END,
        title || ' ' || content)) STORED,
    updated_at TIMESTAMP NOT NULL DEFAULT now(),
    UNIQUE (source_kind, source_ref, lang, chunk_index)
);

CREATE INDEX idx_knowledge_chunks_tsv ON knowledge_chunks USING gin (tsv);

-- Daily chat usage counters (used by the chat API). No transcripts are kept.
CREATE TABLE chat_usage (
    day DATE NOT NULL,
    -- 'u:<userId>', 'ip:<hash>', or '*' for the global total.
    subject TEXT NOT NULL,
    requests INT NOT NULL DEFAULT 0,
    input_tokens BIGINT NOT NULL DEFAULT 0,
    output_tokens BIGINT NOT NULL DEFAULT 0,
    PRIMARY KEY (day, subject)
);

-- +goose Down
DROP TABLE IF EXISTS chat_usage;
DROP TABLE IF EXISTS knowledge_chunks;
