package domain

import (
	"errors"
	"time"
)

// ErrKnowledgeDisabled is returned by the knowledge index when no embedding
// provider is configured (the chat is off, so OpenRouter has no key).
var ErrKnowledgeDisabled = errors.New("knowledge index disabled")

// ErrKnowledgeSyncRunning is returned when a manual reindex is requested
// while another sync (the background loop or a manual one) is running.
var ErrKnowledgeSyncRunning = errors.New("knowledge index sync already running")

// SourceKind is what a knowledge source is. The values match the CHECK
// constraint on knowledge_chunks.source_kind.
type SourceKind string

const (
	SourceProject     SourceKind = "project"
	SourcePublication SourceKind = "publication"
	SourceCV          SourceKind = "cv"
	SourceAbout       SourceKind = "about"
)

// Valid reports whether k is one of the indexed kinds.
func (k SourceKind) Valid() bool {
	switch k {
	case SourceProject, SourcePublication, SourceCV, SourceAbout:
		return true
	}
	return false
}

// Languages the index stores. A source exists once per language.
const (
	LangEnglish = "en"
	LangFrench  = "fr"
)

// ValidLang reports whether lang is a language the index stores.
func ValidLang(lang string) bool { return lang == LangEnglish || lang == LangFrench }

// SourceKey identifies one source in one language.
type SourceKey struct {
	Kind SourceKind
	// Ref is the project id or the corpus slug.
	Ref  string
	Lang string
}

func (k SourceKey) String() string { return string(k.Kind) + "/" + k.Ref + "/" + k.Lang }

// KnowledgeSource is one document the assistant may quote: a corpus file or a
// project in one language. Only author-written text becomes a source;
// feedback comments and contact messages never do.
type KnowledgeSource struct {
	Kind  SourceKind
	Ref   string
	Lang  string
	Title string
	// URL is the site path without a locale prefix, e.g. /projects/12.
	URL  string
	Text string
	// Metadata holds the remaining front matter of a corpus file (pdf, venue,
	// date, summaries, reviewed). It is not indexed.
	Metadata map[string]string
}

func (s KnowledgeSource) Key() SourceKey { return SourceKey{Kind: s.Kind, Ref: s.Ref, Lang: s.Lang} }

// Chunk is one indexed piece of a source.
type Chunk struct {
	ID    int64
	Kind  SourceKind
	Ref   string
	Lang  string
	Title string
	URL   string
	Index int
	// Content is the text that was embedded, including the title prefix.
	Content string
	// Embedding is set when writing chunks; reads for search results leave it nil.
	Embedding []float32
}

func (c Chunk) Key() SourceKey { return SourceKey{Kind: c.Kind, Ref: c.Ref, Lang: c.Lang} }

// ChunkEmbedding is the part of a chunk the in-memory vector index keeps.
type ChunkEmbedding struct {
	ID        int64
	Kind      SourceKind
	Ref       string
	Lang      string
	Embedding []float32
}

// SearchHit is one search result, best first.
type SearchHit struct {
	Chunk Chunk
	// Score is the reciprocal rank fusion score; only the order is meaningful.
	Score float64
	// VectorRank and TextRank are 1-based positions in each result list, 0
	// when the chunk was not in that list.
	VectorRank int
	TextRank   int
}

// EmbedPurpose tells the embedding provider how the text will be used, so it
// can pick the document or the query model and input type.
type EmbedPurpose string

const (
	EmbedDocument EmbedPurpose = "document"
	EmbedQuery    EmbedPurpose = "query"
)

// KnowledgeSyncReport summarises one index sync.
type KnowledgeSyncReport struct {
	// Sources is the number of sources the sync saw (corpus files + projects).
	Sources int
	// Added is the number of new sources that were embedded and stored.
	Added int
	// Updated is the number of known sources whose change was embedded again.
	Updated int
	// Unchanged is the number of sources skipped because their hash matched.
	Unchanged int
	// Deleted is the number of chunk rows removed for vanished sources.
	Deleted int64
	// Chunks is the number of chunks written for the embedded sources.
	Chunks int
}

// KnowledgeSourceStatus summarises one indexed source: what the admin
// knowledge panel lists.
type KnowledgeSourceStatus struct {
	Kind  SourceKind
	Ref   string
	Lang  string
	Title string
	URL   string
	// Chunks is how many rows the source has in knowledge_chunks.
	Chunks    int
	UpdatedAt time.Time
}
