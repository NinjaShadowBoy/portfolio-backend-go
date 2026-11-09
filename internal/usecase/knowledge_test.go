package usecase_test

import (
	"cmp"
	"context"
	"errors"
	"slices"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ninjashadowboy/portfolio-backend-go/internal/domain"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/ports"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/usecase"
)

// knowledgeStore is an in-memory Repositories + UnitOfWork for the knowledge
// index. It records which repositories the use cases open, so a test can
// prove that comments, users and contact messages are never read.
type knowledgeStore struct {
	mu       sync.Mutex
	accessed map[string]int
	chunks   map[domain.SourceKey][]domain.Chunk
	hashes   map[domain.SourceKey]string
	nextID   int64
	projects []domain.Project
	techs    []domain.ProjectTechnology
	// textErr makes SearchFullText fail.
	textErr error
	// chatUsage is keyed by chatUsageKey(day, subject).
	chatUsage map[string]domain.ChatUsageCounts
}

func newKnowledgeStore() *knowledgeStore {
	return &knowledgeStore{accessed: map[string]int{}, chunks: map[domain.SourceKey][]domain.Chunk{}, hashes: map[domain.SourceKey]string{}, chatUsage: map[string]domain.ChatUsageCounts{}}
}

func chatUsageKey(day time.Time, subject string) string {
	return day.UTC().Format(time.DateOnly) + "|" + subject
}

func (s *knowledgeStore) setUsage(day time.Time, subject string, counts domain.ChatUsageCounts) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.chatUsage[chatUsageKey(day, subject)] = counts
}

func (s *knowledgeStore) touch(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.accessed[name]++
}

func (s *knowledgeStore) Users() ports.UsersRepository       { s.touch("Users"); return nil }
func (s *knowledgeStore) Ratings() ports.RatingsRepository   { s.touch("Ratings"); return nil }
func (s *knowledgeStore) Comments() ports.CommentsRepository { s.touch("Comments"); return nil }
func (s *knowledgeStore) Photos() ports.PhotosRepository     { s.touch("Photos"); return nil }
func (s *knowledgeStore) Contacts() ports.ContactsRepository { s.touch("Contacts"); return nil }
func (s *knowledgeStore) AnonymousTokens() ports.AnonymousTokensRepository {
	s.touch("AnonymousTokens")
	return nil
}
func (s *knowledgeStore) Stats() ports.StatsRepository { s.touch("Stats"); return nil }
func (s *knowledgeStore) Projects() ports.ProjectsRepository {
	s.touch("Projects")
	return projectsFake{s: s}
}
func (s *knowledgeStore) Knowledge() ports.KnowledgeRepository { s.touch("Knowledge"); return s }
func (s *knowledgeStore) ChatUsage() ports.ChatUsageRepository {
	s.touch("ChatUsage")
	return s
}

func (s *knowledgeStore) AddUsage(_ context.Context, day time.Time, subject string, delta domain.ChatUsageCounts) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := chatUsageKey(day, subject)
	counts := s.chatUsage[key]
	counts.Requests += delta.Requests
	counts.InputTokens += delta.InputTokens
	counts.OutputTokens += delta.OutputTokens
	s.chatUsage[key] = counts
	return nil
}

func (s *knowledgeStore) GetUsage(_ context.Context, day time.Time, subject string) (domain.ChatUsageCounts, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.chatUsage[chatUsageKey(day, subject)], nil
}

// ListDailyUsage and ListSubjectUsage read the chatUsage map like the real
// queries read the table: '*' is the global row, subjects sum over days,
// best requests first, subject as the tiebreak.
func (s *knowledgeStore) ListDailyUsage(_ context.Context, from, to time.Time) ([]domain.ChatUsageDay, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []domain.ChatUsageDay
	for key, counts := range s.chatUsage {
		prefix, subject, _ := strings.Cut(key, "|")
		if subject != domain.ChatGlobalSubject {
			continue
		}
		day, err := time.Parse(time.DateOnly, prefix)
		if err != nil {
			return nil, err
		}
		if day.Before(from.UTC()) || day.After(to.UTC()) {
			continue
		}
		out = append(out, domain.ChatUsageDay{Day: day, ChatUsageCounts: counts})
	}
	slices.SortFunc(out, func(a, b domain.ChatUsageDay) int { return b.Day.Compare(a.Day) })
	return out, nil
}

func (s *knowledgeStore) ListSubjectUsage(_ context.Context, from, to time.Time, limit int) ([]domain.ChatUsageSubject, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	totals := map[string]domain.ChatUsageCounts{}
	for key, counts := range s.chatUsage {
		dayText, subject, _ := strings.Cut(key, "|")
		if subject == domain.ChatGlobalSubject {
			continue
		}
		day, err := time.Parse(time.DateOnly, dayText)
		if err != nil {
			return nil, err
		}
		if day.Before(from.UTC()) || day.After(to.UTC()) {
			continue
		}
		total := totals[subject]
		total.Requests += counts.Requests
		total.InputTokens += counts.InputTokens
		total.OutputTokens += counts.OutputTokens
		totals[subject] = total
	}
	out := make([]domain.ChatUsageSubject, 0, len(totals))
	for subject, counts := range totals {
		out = append(out, domain.ChatUsageSubject{Subject: subject, ChatUsageCounts: counts})
	}
	slices.SortFunc(out, func(a, b domain.ChatUsageSubject) int {
		if c := cmp.Compare(b.Requests, a.Requests); c != 0 {
			return c
		}
		return cmp.Compare(a.Subject, b.Subject)
	})
	return out[:min(len(out), max(limit, 0))], nil
}

func (s *knowledgeStore) WithinTx(ctx context.Context, work func(ports.Repositories) error) error {
	return work(s)
}

type projectsFake struct {
	ports.ProjectsRepository
	s *knowledgeStore
}

func (p projectsFake) ListProjects(context.Context) ([]domain.Project, error) {
	return p.s.projects, nil
}
func (p projectsFake) ListAllTechnologiesByProjectID(context.Context, []int64) ([]domain.ProjectTechnology, error) {
	return p.s.techs, nil
}
func (p projectsFake) GetProject(_ context.Context, id int64) (domain.Project, error) {
	for _, project := range p.s.projects {
		if project.ID == id {
			return project, nil
		}
	}
	return domain.Project{}, domain.ErrNotFound
}
func (p projectsFake) ListTechnologiesByProjectID(_ context.Context, id int64) ([]string, error) {
	var out []string
	for _, tech := range p.s.techs {
		if tech.ProjectID == id {
			out = append(out, tech.Technology)
		}
	}
	return out, nil
}

func (s *knowledgeStore) ListSourceHashes(context.Context) (map[domain.SourceKey]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make(map[domain.SourceKey]string, len(s.hashes))
	for k, v := range s.hashes {
		out[k] = v
	}
	return out, nil
}

func (s *knowledgeStore) ReplaceSourceChunks(_ context.Context, key domain.SourceKey, hash string, chunks []domain.Chunk) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	stored := make([]domain.Chunk, len(chunks))
	for i, c := range chunks {
		s.nextID++
		c.ID = s.nextID
		stored[i] = c
	}
	s.chunks[key] = stored
	s.hashes[key] = hash
	return nil
}

func (s *knowledgeStore) DeleteSourcesExcept(_ context.Context, keep []domain.SourceKey) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var removed int64
	for key, chunks := range s.chunks {
		if !slices.Contains(keep, key) {
			removed += int64(len(chunks))
			delete(s.chunks, key)
			delete(s.hashes, key)
		}
	}
	return removed, nil
}

// ListSources derives the panel source list from the stored chunks. The
// fake has no per-source timestamp, so UpdatedAt stays zero.
func (s *knowledgeStore) ListSources(context.Context) ([]domain.KnowledgeSourceStatus, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]domain.KnowledgeSourceStatus, 0, len(s.chunks))
	for key, chunks := range s.chunks {
		title, url := "", ""
		if len(chunks) > 0 {
			title, url = chunks[0].Title, chunks[0].URL
		}
		out = append(out, domain.KnowledgeSourceStatus{
			Kind: key.Kind, Ref: key.Ref, Lang: key.Lang, Title: title, URL: url,
			Chunks: len(chunks), UpdatedAt: time.Time{},
		})
	}
	slices.SortFunc(out, func(a, b domain.KnowledgeSourceStatus) int {
		if c := cmp.Compare(a.Kind, b.Kind); c != 0 {
			return c
		}
		if c := cmp.Compare(a.Ref, b.Ref); c != 0 {
			return c
		}
		return cmp.Compare(a.Lang, b.Lang)
	})
	return out, nil
}

func (s *knowledgeStore) all() []domain.Chunk {
	var out []domain.Chunk
	for _, chunks := range s.chunks {
		out = append(out, chunks...)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (s *knowledgeStore) ListEmbeddings(context.Context) ([]domain.ChunkEmbedding, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []domain.ChunkEmbedding
	for _, c := range s.all() {
		out = append(out, domain.ChunkEmbedding{ID: c.ID, Kind: c.Kind, Ref: c.Ref, Lang: c.Lang, Embedding: c.Embedding})
	}
	return out, nil
}

// SearchFullText matches chunks containing every query word, by id.
func (s *knowledgeStore) SearchFullText(_ context.Context, query, _ string, kinds []domain.SourceKind, limit int) ([]int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.textErr != nil {
		return nil, s.textErr
	}
	var ids []int64
	for _, c := range s.all() {
		if len(kinds) > 0 && !slices.Contains(kinds, c.Kind) {
			continue
		}
		match := true
		for _, w := range strings.Fields(strings.ToLower(query)) {
			match = match && strings.Contains(strings.ToLower(c.Content), w)
		}
		if match {
			ids = append(ids, c.ID)
		}
	}
	return ids[:min(limit, len(ids))], nil
}

func (s *knowledgeStore) GetChunks(_ context.Context, ids []int64) ([]domain.Chunk, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []domain.Chunk
	for _, c := range s.all() {
		if slices.Contains(ids, c.ID) {
			c.Embedding = nil
			out = append(out, c)
		}
	}
	return out, nil
}

func (s *knowledgeStore) sourceKeys() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var keys []string
	for k := range s.chunks {
		keys = append(keys, k.String())
	}
	sort.Strings(keys)
	return keys
}

// fakeEmbedder maps text to a 3-d vector by keywords: speech, cv, project.
type fakeEmbedder struct {
	mu       sync.Mutex
	calls    []domain.EmbedPurpose
	texts    int
	err      error
	queryErr error
	// block, when set, holds document embedding until it is closed.
	block chan struct{}
	// entered is closed when a blocked document call starts.
	entered chan struct{}
}

func (e *fakeEmbedder) Embed(ctx context.Context, texts []string, purpose domain.EmbedPurpose) ([][]float32, error) {
	e.mu.Lock()
	e.calls = append(e.calls, purpose)
	e.texts += len(texts)
	block, entered, err := e.block, e.entered, e.err
	if purpose == domain.EmbedQuery {
		block, err = nil, e.queryErr
	}
	e.mu.Unlock()
	if block != nil {
		if entered != nil {
			close(entered)
		}
		select {
		case <-block:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if err != nil {
		return nil, err
	}
	out := make([][]float32, len(texts))
	for i, t := range texts {
		t = strings.ToLower(t)
		v := []float32{0.01, 0.01, 0.01}
		for j, word := range []string{"speech", "curriculum", "project"} {
			if strings.Contains(t, word) {
				v[j] = 1
			}
		}
		out[i] = v
	}
	return out, nil
}

func (e *fakeEmbedder) documentCalls() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	n := 0
	for _, p := range e.calls {
		if p == domain.EmbedDocument {
			n++
		}
	}
	return n
}

func corpus() []domain.KnowledgeSource {
	return []domain.KnowledgeSource{
		{Kind: domain.SourcePublication, Ref: "cerc2026-tonal", Lang: "en", Title: "Tonal paper", URL: "/publications", Text: "Speech translation for Igbo and Yoruba."},
		{Kind: domain.SourceCV, Ref: "cv", Lang: "en", Title: "CV", URL: "/about", Text: "Curriculum: engineering student in Grenoble."},
		{Kind: domain.SourceCV, Ref: "cv", Lang: "fr", Title: "CV", URL: "/about", Text: "Curriculum : élève-ingénieur à Grenoble."},
	}
}

func newKnowledgeUseCases(store *knowledgeStore, embedder ports.Embedder, model string) usecase.Service {
	return usecase.New(usecase.Dependencies{Repositories: store, UnitOfWork: store, Embedder: embedder, EmbeddingDocModel: model})
}

func strPtr(s string) *string { return &s }

func TestSyncKnowledgeSkipsUnchangedSources(t *testing.T) {
	store, embedder := newKnowledgeStore(), &fakeEmbedder{}
	svc := newKnowledgeUseCases(store, embedder, "voyage-4-large")
	ctx := context.Background()

	report, err := svc.SyncKnowledge(ctx, corpus())
	if err != nil {
		t.Fatal(err)
	}
	if report.Sources != 3 || report.Added != 3 || report.Unchanged != 0 || report.Chunks != 3 {
		t.Fatalf("first sync report = %+v", report)
	}
	if embedder.documentCalls() != 1 {
		t.Fatalf("first sync made %d embed calls, want 1 batched call", embedder.documentCalls())
	}

	report, err = svc.SyncKnowledge(ctx, corpus())
	if err != nil {
		t.Fatal(err)
	}
	if embedder.documentCalls() != 1 {
		t.Fatalf("unchanged sources were embedded again (%d calls)", embedder.documentCalls())
	}
	if report.Added != 0 || report.Updated != 0 || report.Unchanged != 3 {
		t.Fatalf("second sync report = %+v", report)
	}

	// Changing one source re-embeds only that source.
	files := corpus()
	files[0].Text += " Hausa too."
	embedder.texts = 0
	if report, err = svc.SyncKnowledge(ctx, files); err != nil || report.Updated != 1 || report.Unchanged != 2 {
		t.Fatalf("after one change: %+v, %v", report, err)
	}
	if embedder.texts != 1 {
		t.Fatalf("embedded %d texts, want the 1 changed chunk", embedder.texts)
	}
}

func TestSyncKnowledgeReembedsWhenDocModelChanges(t *testing.T) {
	store := newKnowledgeStore()
	if _, err := newKnowledgeUseCases(store, &fakeEmbedder{}, "voyage-4-large").SyncKnowledge(context.Background(), corpus()); err != nil {
		t.Fatal(err)
	}
	embedder := &fakeEmbedder{}
	report, err := newKnowledgeUseCases(store, embedder, "voyage-4").SyncKnowledge(context.Background(), corpus())
	if err != nil {
		t.Fatal(err)
	}
	if report.Updated != 3 || embedder.documentCalls() != 1 {
		t.Fatalf("model change: report %+v, %d calls", report, embedder.documentCalls())
	}
}

func TestSyncKnowledgeDeletesVanishedSources(t *testing.T) {
	store := newKnowledgeStore()
	svc := newKnowledgeUseCases(store, &fakeEmbedder{}, "m")
	ctx := context.Background()
	if _, err := svc.SyncKnowledge(ctx, corpus()); err != nil {
		t.Fatal(err)
	}
	report, err := svc.SyncKnowledge(ctx, corpus()[:1])
	if err != nil {
		t.Fatal(err)
	}
	if report.Deleted != 2 {
		t.Fatalf("deleted %d chunks, want 2", report.Deleted)
	}
	if got := store.sourceKeys(); !slices.Equal(got, []string{"publication/cerc2026-tonal/en"}) {
		t.Fatalf("sources left = %v", got)
	}
	// The vector cache follows the deletion: the CV can no longer be found.
	hits, err := svc.SearchKnowledge(ctx, "curriculum", "en", nil, 5)
	if err != nil {
		t.Fatal(err)
	}
	for _, h := range hits {
		if h.Chunk.Kind == domain.SourceCV {
			t.Fatalf("deleted CV still found: %+v", h)
		}
	}
}

func TestSyncKnowledgeReadsOnlyProjectsAndCorpus(t *testing.T) {
	store := newKnowledgeStore()
	store.projects = []domain.Project{{ID: 1, Name: "Lora", Description: "Offline-first platform."}}
	svc := newKnowledgeUseCases(store, &fakeEmbedder{}, "m")
	if _, err := svc.SyncKnowledge(context.Background(), corpus()); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SearchKnowledge(context.Background(), "speech", "en", nil, 3); err != nil {
		t.Fatal(err)
	}
	for name := range store.accessed {
		if name != "Projects" && name != "Knowledge" {
			t.Errorf("the knowledge index opened the %s repository; only projects and the corpus may be indexed", name)
		}
	}
	if store.accessed["Projects"] == 0 {
		t.Error("projects were not read")
	}
}

func TestSyncKnowledgeIndexesProjectsInBothLanguages(t *testing.T) {
	store := newKnowledgeStore()
	store.projects = []domain.Project{{
		ID: 12, Name: "AcadAi", Featured: true,
		Description:   "A RAG assistant.\nPeriod: 2025 - 2026.",
		DescriptionFr: strPtr("Un assistant RAG."),
		Challenges:    strPtr("Bilingual answers."),
		WhatILearned:  strPtr("Evaluation."), WhatILearnedFr: strPtr("  "),
		GithubLink: strPtr("https://github.com/x/acadai"),
	}}
	store.techs = []domain.ProjectTechnology{{ProjectID: 12, Technology: "FastAPI"}, {ProjectID: 12, Technology: "ChromaDB"}}
	if _, err := newKnowledgeUseCases(store, &fakeEmbedder{}, "m").SyncKnowledge(context.Background(), corpus()); err != nil {
		t.Fatal(err)
	}
	en := store.chunks[domain.SourceKey{Kind: domain.SourceProject, Ref: "12", Lang: "en"}]
	fr := store.chunks[domain.SourceKey{Kind: domain.SourceProject, Ref: "12", Lang: "fr"}]
	if len(en) != 1 || len(fr) != 1 {
		t.Fatalf("project chunks: en %d, fr %d", len(en), len(fr))
	}
	for _, want := range []string{"AcadAi\n\nProject: AcadAi", "Technologies: FastAPI, ChromaDB", "Period: 2025 - 2026", "Source code: https://github.com/x/acadai", "## Description\n\nA RAG assistant.", "## Challenges\n\nBilingual answers.", "## What I learned\n\nEvaluation."} {
		if !strings.Contains(en[0].Content, want) {
			t.Errorf("English project text lacks %q:\n%s", want, en[0].Content)
		}
	}
	if strings.Contains(en[0].Content, "Period: 2025 - 2026.\n\n## Description\n\nA RAG assistant.\nPeriod") {
		t.Error("the period line was not taken out of the description")
	}
	// French: translated description, English fallback for the rest.
	for _, want := range []string{"Projet : AcadAi", "Période : 2025 - 2026", "## Description\n\nUn assistant RAG.", "## Défis\n\nBilingual answers.", "## Ce que j'ai appris\n\nEvaluation."} {
		if !strings.Contains(fr[0].Content, want) {
			t.Errorf("French project text lacks %q:\n%s", want, fr[0].Content)
		}
	}
	if en[0].URL != "/projects/12" || fr[0].URL != "/projects/12" {
		t.Errorf("urls %q %q", en[0].URL, fr[0].URL)
	}
}

func TestSyncKnowledgeRejectsBadCorpus(t *testing.T) {
	svc := newKnowledgeUseCases(newKnowledgeStore(), &fakeEmbedder{}, "m")
	if _, err := svc.SyncKnowledge(context.Background(), nil); !errors.Is(err, usecase.ErrBadRequest) {
		t.Fatalf("empty corpus: %v", err)
	}
	for _, bad := range []domain.KnowledgeSource{
		{Kind: domain.SourceProject, Ref: "1", Lang: "en", Title: "x", URL: "/x", Text: "x"},
		{Kind: "blog", Ref: "a", Lang: "en", Title: "x", URL: "/x", Text: "x"},
		{Kind: domain.SourceCV, Ref: "cv", Lang: "de", Title: "x", URL: "/x", Text: "x"},
		{Kind: domain.SourceCV, Ref: "cv", Lang: "en", Title: "x", URL: "https://evil.example", Text: "x"},
		{Kind: domain.SourceCV, Ref: "cv", Lang: "en", Title: "x", URL: "/x", Text: "  "},
	} {
		if _, err := svc.SyncKnowledge(context.Background(), []domain.KnowledgeSource{bad}); !errors.Is(err, usecase.ErrBadRequest) {
			t.Errorf("%+v accepted: %v", bad, err)
		}
	}
	dup := append(corpus(), corpus()[0])
	if _, err := svc.SyncKnowledge(context.Background(), dup); !errors.Is(err, usecase.ErrBadRequest) {
		t.Errorf("duplicate accepted: %v", err)
	}
}

func TestSyncKnowledgeKeepsChunksWhenEmbeddingFails(t *testing.T) {
	store := newKnowledgeStore()
	ctx := context.Background()
	if _, err := newKnowledgeUseCases(store, &fakeEmbedder{}, "m").SyncKnowledge(ctx, corpus()); err != nil {
		t.Fatal(err)
	}
	files := corpus()
	files[1].Text = "Curriculum: changed."
	failing := &fakeEmbedder{err: errors.New("voyage down")}
	svc := newKnowledgeUseCases(store, failing, "m")
	if _, err := svc.SyncKnowledge(ctx, files); err == nil {
		t.Fatal("expected the embed error")
	}
	cv := store.chunks[domain.SourceKey{Kind: domain.SourceCV, Ref: "cv", Lang: "en"}]
	if len(cv) != 1 || !strings.Contains(cv[0].Content, "engineering student") {
		t.Fatalf("old chunks were not kept: %+v", cv)
	}
	// The cache was still loaded, so search works from the stored chunks.
	failing.err = nil
	if hits, err := svc.SearchKnowledge(ctx, "speech", "en", nil, 3); err != nil || len(hits) == 0 {
		t.Fatalf("search after failed sync: %v, %v", hits, err)
	}
}

func TestSearchKnowledgeMergesAndFilters(t *testing.T) {
	store := newKnowledgeStore()
	store.projects = []domain.Project{{ID: 3, Name: "Speech demo", Description: "A speech project."}}
	svc := newKnowledgeUseCases(store, &fakeEmbedder{}, "m")
	ctx := context.Background()
	if _, err := svc.SyncKnowledge(ctx, corpus()); err != nil {
		t.Fatal(err)
	}

	hits, err := svc.SearchKnowledge(ctx, "speech", "en", nil, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) < 2 {
		t.Fatalf("hits = %+v", hits)
	}
	// Both the paper and the project mention speech in vectors and text; they
	// rank above everything that only one list returned.
	top := []domain.SourceKind{hits[0].Chunk.Kind, hits[1].Chunk.Kind}
	if !slices.Contains(top, domain.SourcePublication) || !slices.Contains(top, domain.SourceProject) {
		t.Fatalf("top hits = %v", top)
	}
	for i := 1; i < len(hits); i++ {
		if hits[i].Score > hits[i-1].Score {
			t.Fatalf("hits not sorted by score: %+v", hits)
		}
	}
	if hits[0].VectorRank == 0 || hits[0].TextRank == 0 || hits[0].Chunk.Embedding != nil {
		t.Fatalf("top hit = %+v", hits[0])
	}

	// Kinds filter.
	hits, err = svc.SearchKnowledge(ctx, "speech", "en", []domain.SourceKind{domain.SourcePublication}, 5)
	if err != nil || len(hits) != 1 || hits[0].Chunk.Ref != "cerc2026-tonal" {
		t.Fatalf("kind filter: %+v, %v", hits, err)
	}

	// Language preference: the French CV replaces the English one, and the
	// English-only paper is still returned to a French visitor.
	hits, err = svc.SearchKnowledge(ctx, "curriculum", "fr", []domain.SourceKind{domain.SourceCV}, 5)
	if err != nil || len(hits) != 1 || hits[0].Chunk.Lang != "fr" {
		t.Fatalf("fr cv: %+v, %v", hits, err)
	}
	hits, err = svc.SearchKnowledge(ctx, "speech", "fr", []domain.SourceKind{domain.SourcePublication}, 5)
	if err != nil || len(hits) != 1 || hits[0].Chunk.Lang != "en" {
		t.Fatalf("english-only paper for fr: %+v, %v", hits, err)
	}
	hits, err = svc.SearchKnowledge(ctx, "speech", "fr", []domain.SourceKind{domain.SourceProject}, 5)
	if err != nil || len(hits) != 1 || hits[0].Chunk.Lang != "fr" {
		t.Fatalf("fr project: %+v, %v", hits, err)
	}

	// k limits the results.
	if hits, _ := svc.SearchKnowledge(ctx, "speech", "", nil, 1); len(hits) != 1 {
		t.Fatalf("k=1 gave %d hits", len(hits))
	}
	for _, bad := range []struct {
		q, lang string
		kinds   []domain.SourceKind
	}{{"  ", "en", nil}, {"x", "de", nil}, {"x", "en", []domain.SourceKind{"blog"}}} {
		if _, err := svc.SearchKnowledge(ctx, bad.q, bad.lang, bad.kinds, 3); !errors.Is(err, usecase.ErrBadRequest) {
			t.Errorf("%+v: %v", bad, err)
		}
	}
}

func TestSearchKnowledgeFallsBackToFullText(t *testing.T) {
	store, embedder := newKnowledgeStore(), &fakeEmbedder{}
	svc := newKnowledgeUseCases(store, embedder, "m")
	ctx := context.Background()
	if _, err := svc.SyncKnowledge(ctx, corpus()); err != nil {
		t.Fatal(err)
	}
	embedder.queryErr = errors.New("voyage down")
	hits, err := svc.SearchKnowledge(ctx, "yoruba", "en", nil, 3)
	if err != nil || len(hits) != 1 || hits[0].VectorRank != 0 || hits[0].TextRank != 1 {
		t.Fatalf("full-text fallback: %+v, %v", hits, err)
	}
	store.textErr = errors.New("db down")
	if _, err := svc.SearchKnowledge(ctx, "yoruba", "en", nil, 3); err == nil {
		t.Fatal("expected an error when both searches fail")
	}
}

func TestSearchKnowledgeLoadsStoredVectorsLazily(t *testing.T) {
	store := newKnowledgeStore()
	ctx := context.Background()
	if _, err := newKnowledgeUseCases(store, &fakeEmbedder{}, "m").SyncKnowledge(ctx, corpus()); err != nil {
		t.Fatal(err)
	}
	// A fresh process has no cache until its first sync; search loads it.
	fresh := newKnowledgeUseCases(store, &fakeEmbedder{}, "m")
	hits, err := fresh.SearchKnowledge(ctx, "anything about speech", "en", nil, 3)
	if err != nil || len(hits) == 0 || hits[0].Chunk.Ref != "cerc2026-tonal" || hits[0].VectorRank != 1 {
		t.Fatalf("hits = %+v, %v", hits, err)
	}
}

func TestSearchKnowledgeDuringSyncReadsPreviousSnapshot(t *testing.T) {
	store := newKnowledgeStore()
	ctx := context.Background()
	embedder := &fakeEmbedder{}
	svc := newKnowledgeUseCases(store, embedder, "m")
	if _, err := svc.SyncKnowledge(ctx, corpus()); err != nil {
		t.Fatal(err)
	}
	embedder.mu.Lock()
	embedder.block, embedder.entered = make(chan struct{}), make(chan struct{})
	entered, release := embedder.entered, embedder.block
	embedder.mu.Unlock()

	files := corpus()
	files[0].Text = "Speech translation, revised."
	done := make(chan error, 1)
	go func() { _, err := svc.SyncKnowledge(ctx, files); done <- err }()
	<-entered

	searched := make(chan error, 1)
	go func() {
		hits, err := svc.SearchKnowledge(ctx, "speech", "en", nil, 3)
		if err == nil && len(hits) == 0 {
			err = errors.New("no hits")
		}
		searched <- err
	}()
	select {
	case err := <-searched:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("search blocked behind a running sync")
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestKnowledgeDisabledWithoutEmbedder(t *testing.T) {
	svc := usecase.New(usecase.Dependencies{Repositories: newKnowledgeStore()})
	if svc.KnowledgeEnabled() {
		t.Fatal("enabled without an embedder")
	}
	ctx := context.Background()
	if _, err := svc.SyncKnowledge(ctx, corpus()); !errors.Is(err, domain.ErrKnowledgeDisabled) {
		t.Fatalf("sync: %v", err)
	}
	if _, err := svc.SyncKnowledgeNow(ctx); !errors.Is(err, domain.ErrKnowledgeDisabled) {
		t.Fatalf("sync now: %v", err)
	}
	if _, err := svc.SearchKnowledge(ctx, "x", "en", nil, 3); !errors.Is(err, domain.ErrKnowledgeDisabled) {
		t.Fatalf("search: %v", err)
	}
}

type staticCorpus []domain.KnowledgeSource

func (c staticCorpus) Load(context.Context) ([]domain.KnowledgeSource, error) { return c, nil }

func TestSyncKnowledgeNowUsesCorpus(t *testing.T) {
	store := newKnowledgeStore()
	svc := usecase.New(usecase.Dependencies{
		Repositories: store, UnitOfWork: store, Embedder: &fakeEmbedder{}, KnowledgeCorpus: staticCorpus(corpus()), EmbeddingDocModel: "m",
	})
	report, err := svc.SyncKnowledgeNow(context.Background())
	if err != nil || report.Added != 3 {
		t.Fatalf("%+v, %v", report, err)
	}
}
