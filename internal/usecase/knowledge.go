package usecase

import (
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ninjashadowboy/portfolio-backend-go/internal/domain"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/dto"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/ports"
)

// Knowledge index: the corpus files plus the projects table, chunked,
// embedded and stored in knowledge_chunks. Search is hybrid: cosine over an
// in-memory copy of the vectors plus Postgres full-text search, merged by
// reciprocal rank fusion.
const (
	chunkSize    = 1200 // characters of body per chunk, title prefix excluded
	chunkOverlap = 150  // characters repeated from the end of the previous piece
	// chunkerVersion is part of every source hash; bump it when chunking or
	// the project text layout changes so every source is embedded again.
	chunkerVersion = "1"
	// rrfK is the reciprocal rank fusion constant from Cormack et al. (2009).
	rrfK = 60

	defaultSearchResults = 5
	maxSearchResults     = 20
)

// knowledgeIndex is shared by every copy of UseCases (inTransaction copies
// the struct, not the index).
type knowledgeIndex struct {
	embedder ports.Embedder
	corpus   ports.KnowledgeCorpus
	docModel string

	// syncMu serialises syncs (startup, the periodic loop, an admin reindex).
	syncMu sync.Mutex
	// loadMu guards the first lazy load of the vector cache.
	loadMu sync.Mutex
	// snapshot is swapped whole after each sync, so searches running during a
	// sync keep reading the previous, consistent snapshot.
	snapshot atomic.Pointer[vectorSnapshot]
}

func newKnowledgeIndex(deps Dependencies) *knowledgeIndex {
	if deps.Embedder == nil {
		return nil
	}
	return &knowledgeIndex{embedder: deps.Embedder, corpus: deps.KnowledgeCorpus, docModel: deps.EmbeddingDocModel}
}

// KnowledgeEnabled reports whether an embedding provider is configured.
func (s *UseCases) KnowledgeEnabled() bool { return s.knowledge != nil }

// SyncKnowledgeNow loads the corpus files and syncs the index. The
// background loop and the admin reindex endpoint call it: their runs share
// one knowledgeSyncState, so they never overlap (the second caller gets
// domain.ErrKnowledgeSyncRunning) and both are visible to the admin panel.
func (s *UseCases) SyncKnowledgeNow(ctx context.Context) (domain.KnowledgeSyncReport, error) {
	if s.knowledge == nil {
		return domain.KnowledgeSyncReport{}, domain.ErrKnowledgeDisabled
	}
	if s.knowledge.corpus == nil {
		return domain.KnowledgeSyncReport{}, errors.New("knowledge corpus not configured")
	}
	if !s.syncs.begin() {
		return domain.KnowledgeSyncReport{}, domain.ErrKnowledgeSyncRunning
	}
	startedAt := time.Now()
	files, err := s.knowledge.corpus.Load(ctx)
	var report domain.KnowledgeSyncReport
	if err == nil {
		report, err = s.SyncKnowledge(ctx, files)
	} else {
		err = fmt.Errorf("load knowledge corpus: %w", err)
	}
	s.syncs.finish(startedAt, report, err)
	return report, err
}

// SyncKnowledge indexes files plus every project in both languages. A source
// whose hash is unchanged is not embedded again; a source that no longer
// exists is deleted. When embedding fails the stored chunks stay as they are.
func (s *UseCases) SyncKnowledge(ctx context.Context, files []domain.KnowledgeSource) (domain.KnowledgeSyncReport, error) {
	idx := s.knowledge
	var report domain.KnowledgeSyncReport
	if idx == nil {
		return report, domain.ErrKnowledgeDisabled
	}
	// An empty corpus means a missing or broken knowledge directory; syncing
	// it would delete every indexed paper and CV.
	if len(files) == 0 {
		return report, fmt.Errorf("%w: no knowledge corpus files", ErrBadRequest)
	}
	idx.syncMu.Lock()
	defer idx.syncMu.Unlock()

	sources, err := s.knowledgeSources(ctx, files)
	if err != nil {
		return report, err
	}
	report.Sources = len(sources)
	repo := s.repos.Knowledge()
	stored, err := repo.ListSourceHashes(ctx)
	if err != nil {
		return report, err
	}

	keys := make([]domain.SourceKey, len(sources))
	for i, src := range sources {
		keys[i] = src.Key()
	}
	if report.Deleted, err = repo.DeleteSourcesExcept(ctx, keys); err != nil {
		return report, err
	}

	type pendingSource struct {
		source   domain.KnowledgeSource
		hash     string
		contents []string
	}
	var pending []pendingSource
	var texts []string
	for _, src := range sources {
		hash := sourceHash(src, idx.docModel)
		key := src.Key()
		if stored[key] == hash {
			report.Unchanged++
			continue
		}
		if _, known := stored[key]; known {
			report.Updated++
		} else {
			report.Added++
		}
		contents := chunkText(src.Title, src.Text, chunkSize, chunkOverlap)
		pending = append(pending, pendingSource{source: src, hash: hash, contents: contents})
		texts = append(texts, contents...)
	}

	defer func() {
		if report.Deleted > 0 || report.Added+report.Updated > 0 || idx.snapshot.Load() == nil {
			if loadErr := idx.reload(ctx, repo); loadErr != nil {
				err = errors.Join(err, fmt.Errorf("reload knowledge vectors: %w", loadErr))
			}
		}
	}()
	if len(texts) == 0 {
		return report, nil
	}

	vectors, err := idx.embedder.Embed(ctx, texts, domain.EmbedDocument)
	if err != nil {
		return report, fmt.Errorf("embed knowledge: %w", err)
	}
	if len(vectors) != len(texts) {
		return report, fmt.Errorf("embed knowledge: got %d vectors for %d chunks", len(vectors), len(texts))
	}
	next := 0
	for _, p := range pending {
		chunks := make([]domain.Chunk, len(p.contents))
		for i, content := range p.contents {
			chunks[i] = domain.Chunk{
				Kind: p.source.Kind, Ref: p.source.Ref, Lang: p.source.Lang, Title: p.source.Title, URL: p.source.URL,
				Index: i, Content: content, Embedding: vectors[next],
			}
			next++
		}
		_, err = inTransaction(ctx, s, func(tx *UseCases) (struct{}, error) {
			return struct{}{}, tx.repos.Knowledge().ReplaceSourceChunks(ctx, p.source.Key(), p.hash, chunks)
		})
		if err != nil {
			return report, fmt.Errorf("store %s: %w", p.source.Key(), err)
		}
		report.Chunks += len(chunks)
	}
	return report, nil
}

// knowledgeSources validates the corpus files and adds the projects. Only the
// projects repository is read: comments, ratings, users and contact messages
// are never indexed.
func (s *UseCases) knowledgeSources(ctx context.Context, files []domain.KnowledgeSource) ([]domain.KnowledgeSource, error) {
	seen := make(map[domain.SourceKey]bool, len(files))
	out := make([]domain.KnowledgeSource, 0, len(files)+20)
	for _, f := range files {
		if err := validateCorpusSource(f); err != nil {
			return nil, err
		}
		if seen[f.Key()] {
			return nil, fmt.Errorf("%w: duplicate knowledge source %s", ErrBadRequest, f.Key())
		}
		seen[f.Key()] = true
		out = append(out, f)
	}

	projects, err := s.repos.Projects().ListProjects(ctx)
	if err != nil {
		return nil, err
	}
	if len(projects) == 0 {
		return out, nil
	}
	ids := make([]int64, len(projects))
	for i, p := range projects {
		ids[i] = p.ID
	}
	rows, err := s.repos.Projects().ListAllTechnologiesByProjectID(ctx, ids)
	if err != nil {
		return nil, err
	}
	technologies := make(map[int64][]string, len(projects))
	for _, row := range rows {
		technologies[row.ProjectID] = append(technologies[row.ProjectID], row.Technology)
	}
	for _, p := range projects {
		out = append(out, projectSource(p, technologies[p.ID], domain.LangEnglish), projectSource(p, technologies[p.ID], domain.LangFrench))
	}
	return out, nil
}

func validateCorpusSource(f domain.KnowledgeSource) error {
	switch {
	case !f.Kind.Valid() || f.Kind == domain.SourceProject:
		return fmt.Errorf("%w: knowledge file %s: kind must be publication, cv or about", ErrBadRequest, f.Key())
	case !domain.ValidLang(f.Lang):
		return fmt.Errorf("%w: knowledge file %s: lang must be en or fr", ErrBadRequest, f.Key())
	case strings.TrimSpace(f.Ref) == "" || strings.Contains(f.Ref, "/"):
		return fmt.Errorf("%w: knowledge file %s: ref must be set and contain no '/'", ErrBadRequest, f.Key())
	case strings.TrimSpace(f.Title) == "":
		return fmt.Errorf("%w: knowledge file %s: title is required", ErrBadRequest, f.Key())
	case !strings.HasPrefix(f.URL, "/"):
		return fmt.Errorf("%w: knowledge file %s: url must be a site path starting with /", ErrBadRequest, f.Key())
	case strings.TrimSpace(f.Text) == "":
		return fmt.Errorf("%w: knowledge file %s: body is empty", ErrBadRequest, f.Key())
	}
	return nil
}

// periodLine matches the "Period: 2023 - 2024." line projects keep in their
// English description, like splitPeriod in the frontend.
var periodLine = regexp.MustCompile(`(?im)^[ \t]*Period:[ \t]*(.+?)\.?[ \t]*$`)

func splitPeriod(description string) (period, body string) {
	loc := periodLine.FindStringSubmatchIndex(description)
	if loc == nil {
		return "", strings.TrimSpace(description)
	}
	period = strings.TrimSpace(description[loc[2]:loc[3]])
	body = description[:loc[0]] + description[loc[1]:]
	for strings.Contains(body, "\n\n\n") {
		body = strings.ReplaceAll(body, "\n\n\n", "\n\n")
	}
	return period, strings.TrimSpace(body)
}

// projectSource renders a project as text in one language. French fields
// fall back to English one by one, as projectText does on the site.
func projectSource(p domain.Project, technologies []string, lang string) domain.KnowledgeSource {
	period, description := splitPeriod(p.Description)
	challenges, learned := deref(p.Challenges), deref(p.WhatILearned)
	labels := projectLabelsEN
	if lang == domain.LangFrench {
		labels = projectLabelsFR
		if fr := strings.TrimSpace(deref(p.DescriptionFr)); fr != "" {
			_, description = splitPeriod(fr)
		}
		challenges = pickText(challenges, deref(p.ChallengesFr))
		learned = pickText(learned, deref(p.WhatILearnedFr))
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%s%s\n", labels.project, p.Name)
	if len(technologies) > 0 {
		fmt.Fprintf(&b, "%s%s\n", labels.technologies, strings.Join(technologies, ", "))
	}
	if period != "" {
		fmt.Fprintf(&b, "%s%s\n", labels.period, period)
	}
	if p.Featured {
		b.WriteString(labels.featured + "\n")
	}
	if link := strings.TrimSpace(deref(p.GithubLink)); link != "" {
		fmt.Fprintf(&b, "%s%s\n", labels.source, link)
	}
	if link := strings.TrimSpace(deref(p.DemoLink)); link != "" {
		fmt.Fprintf(&b, "%s%s\n", labels.demo, link)
	}
	for _, section := range []struct{ heading, text string }{
		{labels.description, description},
		{labels.challenges, strings.TrimSpace(challenges)},
		{labels.learned, strings.TrimSpace(learned)},
	} {
		if section.text != "" {
			fmt.Fprintf(&b, "\n## %s\n\n%s\n", section.heading, section.text)
		}
	}
	return domain.KnowledgeSource{
		Kind: domain.SourceProject, Ref: strconv.FormatInt(p.ID, 10), Lang: lang, Title: p.Name,
		URL: "/projects/" + strconv.FormatInt(p.ID, 10), Text: b.String(),
	}
}

type projectLabels struct {
	project, technologies, period, featured, source, demo, description, challenges, learned string
}

var (
	projectLabelsEN = projectLabels{
		project: "Project: ", technologies: "Technologies: ", period: "Period: ", featured: "Featured project.",
		source: "Source code: ", demo: "Live demo: ", description: "Description", challenges: "Challenges", learned: "What I learned",
	}
	projectLabelsFR = projectLabels{
		project: "Projet : ", technologies: "Technologies : ", period: "Période : ", featured: "Projet mis en avant.",
		source: "Code source : ", demo: "Démo : ", description: "Description", challenges: "Défis", learned: "Ce que j'ai appris",
	}
)

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func pickText(english, french string) string {
	if fr := strings.TrimSpace(french); fr != "" {
		return fr
	}
	return english
}

// sourceHash covers everything that ends up in the stored chunks, plus the
// document model: changing the model re-embeds every source.
func sourceHash(src domain.KnowledgeSource, docModel string) string {
	h := sha256.New()
	for _, part := range []string{chunkerVersion, docModel, src.Key().String(), src.Title, src.URL, src.Text} {
		h.Write([]byte(part))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

// chunkText splits a markdown source into chunks of at most size characters
// of body. Sections start at heading lines; small neighbouring sections are
// packed together, and a section longer than size is cut at paragraph,
// sentence or word boundaries with overlap characters repeated. Each chunk
// starts with the source title (and the section heading when a long section
// continues), so a chunk read alone still says what it is about.
func chunkText(title, text string, size, overlap int) []string {
	title = strings.TrimSpace(title)
	prefix := title + "\n\n"
	var chunks []string
	var current string
	flush := func() {
		if strings.TrimSpace(current) != "" {
			chunks = append(chunks, prefix+strings.TrimSpace(current))
		}
		current = ""
	}
	for _, sec := range splitSections(text) {
		if runeLen(sec.text) > size {
			flush()
			for i, piece := range splitWindow(sec.text, size, overlap) {
				if i > 0 && sec.heading != "" {
					piece = "## " + sec.heading + "\n\n" + piece
				}
				chunks = append(chunks, prefix+piece)
			}
			continue
		}
		joined := sec.text
		if current != "" {
			joined = current + "\n\n" + sec.text
		}
		if runeLen(joined) > size {
			flush()
			joined = sec.text
		}
		current = joined
	}
	flush()
	return chunks
}

type section struct {
	heading string
	// text includes the heading line itself.
	text string
}

func splitSections(text string) []section {
	var sections []section
	var heading string
	var lines []string
	emit := func() {
		if body := strings.TrimSpace(strings.Join(lines, "\n")); body != "" {
			sections = append(sections, section{heading: heading, text: body})
		}
		lines = nil
	}
	for _, line := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		if trimmed := strings.TrimSpace(line); strings.HasPrefix(trimmed, "#") {
			emit()
			heading = strings.TrimSpace(strings.TrimLeft(trimmed, "#"))
		}
		lines = append(lines, line)
	}
	emit()
	return sections
}

// splitWindow cuts text into pieces of at most size runes. Each piece after
// the first starts about overlap runes before the previous piece ended, at a
// word boundary.
func splitWindow(text string, size, overlap int) []string {
	runes := []rune(strings.TrimSpace(text))
	var pieces []string
	for start := 0; start < len(runes); {
		end := start + size
		if end >= len(runes) {
			pieces = append(pieces, strings.TrimSpace(string(runes[start:])))
			break
		}
		cut := breakPoint(runes, start, end)
		pieces = append(pieces, strings.TrimSpace(string(runes[start:cut])))
		next := cut - overlap
		if next <= start {
			next = cut
		}
		for next < cut && !isSpace(runes[next-1]) {
			next++
		}
		for next < len(runes) && isSpace(runes[next]) {
			next++
		}
		start = next
	}
	return pieces
}

// breakPoint picks where a piece ending near end should stop: the last
// paragraph break, else sentence end, else space in the second half of the
// window; a hard cut at end when there is none.
func breakPoint(runes []rune, start, end int) int {
	floor := start + (end-start)/2
	for _, isBreak := range []func(i int) bool{
		func(i int) bool { return runes[i] == '\n' && runes[i-1] == '\n' },
		func(i int) bool { return isSpace(runes[i]) && strings.ContainsRune(".!?;:", runes[i-1]) },
		func(i int) bool { return isSpace(runes[i]) },
	} {
		for i := end; i > floor; i-- {
			if isBreak(i) {
				return i
			}
		}
	}
	return end
}

func isSpace(r rune) bool { return r == ' ' || r == '\n' || r == '\t' || r == '\r' }

func runeLen(s string) int { return len([]rune(s)) }

// vectorSnapshot is the in-memory copy of every chunk vector. Vectors are
// normalised on load, so cosine similarity is a dot product.
type vectorSnapshot struct {
	entries []vectorEntry
	byID    map[int64]int
	// langs lists the languages each (kind, ref) exists in.
	langs map[string]map[string]bool
}

type vectorEntry struct {
	id   int64
	kind domain.SourceKind
	ref  string
	lang string
	vec  []float32
}

func (idx *knowledgeIndex) reload(ctx context.Context, repo ports.KnowledgeRepository) error {
	rows, err := repo.ListEmbeddings(ctx)
	if err != nil {
		return err
	}
	snap := &vectorSnapshot{
		entries: make([]vectorEntry, 0, len(rows)),
		byID:    make(map[int64]int, len(rows)),
		langs:   map[string]map[string]bool{},
	}
	for _, row := range rows {
		snap.byID[row.ID] = len(snap.entries)
		snap.entries = append(snap.entries, vectorEntry{id: row.ID, kind: row.Kind, ref: row.Ref, lang: row.Lang, vec: normalize(row.Embedding)})
		doc := sourceDoc(row.Kind, row.Ref)
		if snap.langs[doc] == nil {
			snap.langs[doc] = map[string]bool{}
		}
		snap.langs[doc][row.Lang] = true
	}
	idx.snapshot.Store(snap)
	return nil
}

func (idx *knowledgeIndex) vectors(ctx context.Context, repo ports.KnowledgeRepository) (*vectorSnapshot, error) {
	if snap := idx.snapshot.Load(); snap != nil {
		return snap, nil
	}
	idx.loadMu.Lock()
	defer idx.loadMu.Unlock()
	if snap := idx.snapshot.Load(); snap != nil {
		return snap, nil
	}
	if err := idx.reload(ctx, repo); err != nil {
		return nil, err
	}
	return idx.snapshot.Load(), nil
}

func sourceDoc(kind domain.SourceKind, ref string) string { return string(kind) + "/" + ref }

// allowed applies the kinds filter and the language preference: a chunk in
// another language is kept only when its source has no version in lang, so a
// French visitor gets the French CV but still the English-only paper text.
func (snap *vectorSnapshot) allowed(e vectorEntry, lang string, kinds []domain.SourceKind) bool {
	if len(kinds) > 0 && !slices.Contains(kinds, e.kind) {
		return false
	}
	return lang == "" || e.lang == lang || !snap.langs[sourceDoc(e.kind, e.ref)][lang]
}

func (snap *vectorSnapshot) nearest(query []float32, limit int, lang string, kinds []domain.SourceKind) []int64 {
	query = normalize(query)
	type scored struct {
		id    int64
		score float64
	}
	var all []scored
	for _, e := range snap.entries {
		if len(e.vec) != len(query) || !snap.allowed(e, lang, kinds) {
			continue
		}
		all = append(all, scored{e.id, dot(e.vec, query)})
	}
	slices.SortFunc(all, func(a, b scored) int {
		if c := cmp.Compare(b.score, a.score); c != 0 {
			return c
		}
		return cmp.Compare(a.id, b.id)
	})
	ids := make([]int64, 0, min(limit, len(all)))
	for _, s := range all[:min(limit, len(all))] {
		ids = append(ids, s.id)
	}
	return ids
}

func (snap *vectorSnapshot) filterIDs(ids []int64, lang string, kinds []domain.SourceKind) []int64 {
	out := ids[:0:0]
	for _, id := range ids {
		if i, ok := snap.byID[id]; ok && snap.allowed(snap.entries[i], lang, kinds) {
			out = append(out, id)
		}
	}
	return out
}

func normalize(v []float32) []float32 {
	var norm float64
	for _, x := range v {
		norm += float64(x) * float64(x)
	}
	out := make([]float32, len(v))
	if norm == 0 {
		return out
	}
	norm = math.Sqrt(norm)
	for i, x := range v {
		out[i] = float32(float64(x) / norm)
	}
	return out
}

func dot(a, b []float32) float64 {
	var sum float64
	for i := range a {
		sum += float64(a[i]) * float64(b[i])
	}
	return sum
}

// cosine is the cosine similarity of a and b; 0 when either is all zeros or
// the lengths differ.
func cosine(a, b []float32) float64 {
	if len(a) != len(b) {
		return 0
	}
	return dot(normalize(a), normalize(b))
}

type fusedHit struct {
	id         int64
	score      float64
	vectorRank int
	textRank   int
}

// fuseRanks merges two best-first id lists by reciprocal rank fusion:
// score = sum over lists of 1 / (k + rank), rank starting at 1. Ties keep the
// better single rank first, then the lower id.
func fuseRanks(vector, text []int64, k int) []fusedHit {
	byID := map[int64]*fusedHit{}
	var order []*fusedHit
	add := func(ids []int64, rank func(*fusedHit) *int) {
		for i, id := range ids {
			h := byID[id]
			if h == nil {
				h = &fusedHit{id: id}
				byID[id] = h
				order = append(order, h)
			}
			if r := rank(h); *r != 0 {
				continue // a duplicate in the same list counts once
			} else {
				*r = i + 1
			}
			h.score += 1 / float64(k+i+1)
		}
	}
	add(vector, func(h *fusedHit) *int { return &h.vectorRank })
	add(text, func(h *fusedHit) *int { return &h.textRank })
	best := func(h *fusedHit) int {
		switch {
		case h.vectorRank == 0:
			return h.textRank
		case h.textRank == 0:
			return h.vectorRank
		}
		return min(h.vectorRank, h.textRank)
	}
	slices.SortFunc(order, func(a, b *fusedHit) int {
		if c := cmp.Compare(b.score, a.score); c != 0 {
			return c
		}
		if c := cmp.Compare(best(a), best(b)); c != 0 {
			return c
		}
		return cmp.Compare(a.id, b.id)
	})
	out := make([]fusedHit, len(order))
	for i, h := range order {
		out[i] = *h
	}
	return out
}

// ReindexKnowledgeNow runs the sync for POST /admin/knowledge/reindex. The
// caller's context is detached and bounded by reindexTimeout: an admin
// closing the tab must not cancel a reindex half-way. The response carries
// the same KnowledgeSyncStatus the next GET /admin/knowledge will show. Two
// syncs (any mix of background and manual) never overlap: the second caller
// gets domain.ErrKnowledgeSyncRunning, and no run is recorded for it.
const reindexTimeout = 5 * time.Minute

func (s *UseCases) ReindexKnowledgeNow(ctx context.Context) (dto.KnowledgeSyncStatus, error) {
	if !s.KnowledgeEnabled() {
		return dto.KnowledgeSyncStatus{}, domain.ErrKnowledgeDisabled
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), reindexTimeout)
	defer cancel()
	_, err := s.SyncKnowledgeNow(ctx)
	if errors.Is(err, domain.ErrKnowledgeSyncRunning) {
		return dto.KnowledgeSyncStatus{}, err
	}
	// A failed run is still a completed attempt: its status (with the error)
	// is what the panel should show, so it arrives as 200.
	return s.syncs.finished(), nil
}

// KnowledgeStatus is GET /admin/knowledge: what the database has indexed,
// how the last sync of this process finished and whether one is running.
// It works with the index disabled, too: the chunk rows may still exist.
func (s *UseCases) KnowledgeStatus(ctx context.Context) (dto.KnowledgeStatus, error) {
	sources, err := s.repos.Knowledge().ListSources(ctx)
	if err != nil {
		return dto.KnowledgeStatus{}, err
	}
	running, last := s.syncs.status()
	out := dto.KnowledgeStatus{
		Enabled:  s.KnowledgeEnabled(),
		Sources:  make([]dto.KnowledgeSourceStatus, len(sources)),
		Syncing:  running,
		LastSync: last,
	}
	for i, src := range sources {
		out.Sources[i] = dto.KnowledgeSourceStatus{
			Kind: string(src.Kind), Ref: src.Ref, Lang: src.Lang, Title: src.Title, URL: src.URL,
			Chunks: src.Chunks, UpdatedAt: src.UpdatedAt,
		}
	}
	return out, nil
}

func (s *UseCases) SearchKnowledge(ctx context.Context, query, lang string, kinds []domain.SourceKind, k int) ([]domain.SearchHit, error) {
	idx := s.knowledge
	if idx == nil {
		return nil, domain.ErrKnowledgeDisabled
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("%w: empty search query", ErrBadRequest)
	}
	if lang != "" && !domain.ValidLang(lang) {
		return nil, fmt.Errorf("%w: unknown language %q", ErrBadRequest, lang)
	}
	for _, kind := range kinds {
		if !kind.Valid() {
			return nil, fmt.Errorf("%w: unknown source kind %q", ErrBadRequest, kind)
		}
	}
	if k <= 0 {
		k = defaultSearchResults
	}
	k = min(k, maxSearchResults)
	pool := max(4*k, 20)

	repo := s.repos.Knowledge()
	snap, err := idx.vectors(ctx, repo)
	if err != nil {
		return nil, fmt.Errorf("load knowledge vectors: %w", err)
	}

	var vectorIDs []int64
	embedded, vectorErr := idx.embedder.Embed(ctx, []string{query}, domain.EmbedQuery)
	if vectorErr == nil && len(embedded) != 1 {
		vectorErr = fmt.Errorf("got %d query vectors", len(embedded))
	}
	if vectorErr == nil {
		vectorIDs = snap.nearest(embedded[0], pool, lang, kinds)
	}
	textIDs, textErr := repo.SearchFullText(ctx, query, lang, kinds, pool)
	if vectorErr != nil && textErr != nil {
		return nil, errors.Join(fmt.Errorf("embed query: %w", vectorErr), fmt.Errorf("full-text search: %w", textErr))
	}
	textIDs = snap.filterIDs(textIDs, lang, kinds)

	fused := fuseRanks(vectorIDs, textIDs, rrfK)
	fused = fused[:min(k, len(fused))]
	if len(fused) == 0 {
		return []domain.SearchHit{}, nil
	}
	ids := make([]int64, len(fused))
	for i, h := range fused {
		ids[i] = h.id
	}
	chunks, err := repo.GetChunks(ctx, ids)
	if err != nil {
		return nil, err
	}
	byID := make(map[int64]domain.Chunk, len(chunks))
	for _, c := range chunks {
		byID[c.ID] = c
	}
	hits := make([]domain.SearchHit, 0, len(fused))
	for _, h := range fused {
		if c, ok := byID[h.id]; ok {
			hits = append(hits, domain.SearchHit{Chunk: c, Score: h.score, VectorRank: h.vectorRank, TextRank: h.textRank})
		}
	}
	return hits, nil
}
