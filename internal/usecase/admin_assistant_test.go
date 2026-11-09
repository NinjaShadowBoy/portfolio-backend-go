package usecase_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ninjashadowboy/portfolio-backend-go/internal/domain"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/dto"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/ports"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/usecase"
)

// Admin panel use cases: the chat usage report (chat.go) and the knowledge
// status / reindex endpoints (knowledge.go).

func TestChatUsageReportAggregates(t *testing.T) {
	store := newKnowledgeStore()
	day1 := time.Date(2026, 3, 12, 10, 0, 0, 0, time.UTC)
	store.setUsage(day1, "ip:a", domain.ChatUsageCounts{Requests: 2, InputTokens: 10, OutputTokens: 5})
	store.setUsage(day1, "u:7", domain.ChatUsageCounts{Requests: 2, InputTokens: 30, OutputTokens: 5})
	store.setUsage(day1, domain.ChatGlobalSubject, domain.ChatUsageCounts{Requests: 4, InputTokens: 40, OutputTokens: 10})
	store.setUsage(chatDay, domain.ChatGlobalSubject, domain.ChatUsageCounts{Requests: 1, InputTokens: 150, OutputTokens: 0})
	store.setUsage(time.Date(2026, 3, 8, 0, 0, 0, 0, time.UTC), domain.ChatGlobalSubject, domain.ChatUsageCounts{Requests: 99})

	model := &scriptedChatModel{steps: []chatStep{{deltas: []string{"x"}}}}
	svc := newChatUseCases(store, model, usecase.ChatSettings{DailyTokenBudget: 100})

	report, err := svc.ChatUsageReport(context.Background(), 3)
	if err != nil {
		t.Fatal(err)
	}
	// chatDay is 2026-03-14 12:00 UTC: the 13th had no traffic and comes back
	// as zeros, the 8th falls outside the range.
	wantDays := []dto.ChatUsageDay{
		{Day: "2026-03-12", Requests: 4, InputTokens: 40, OutputTokens: 10},
		{Day: "2026-03-13"},
		{Day: "2026-03-14", Requests: 1, InputTokens: 150},
	}
	if !slices.Equal(report.Days, wantDays) {
		t.Errorf("days = %+v, want %+v", report.Days, wantDays)
	}
	// The two subjects tied on requests: the subject string breaks the tie,
	// the global row is not a subject.
	wantSubjects := []dto.ChatUsageSubject{
		{Subject: "ip:a", Requests: 2, InputTokens: 10, OutputTokens: 5},
		{Subject: "u:7", Requests: 2, InputTokens: 30, OutputTokens: 5},
	}
	if !slices.Equal(report.TopSubjects, wantSubjects) {
		t.Errorf("top subjects = %+v, want %+v", report.TopSubjects, wantSubjects)
	}
	// Today's spent tokens overflow the budget; the remaining never goes
	// negative.
	wantToday := dto.ChatBudget{Requests: 1, Tokens: 150, TokenBudget: 100, BudgetRemaining: 0}
	if report.Today != wantToday {
		t.Errorf("today = %+v, want %+v", report.Today, wantToday)
	}
}

func TestChatUsageReportDefaultsAndCaps(t *testing.T) {
	store := newKnowledgeStore()
	for i := range 12 {
		store.setUsage(chatDay, fmt.Sprintf("u:%02d", i), domain.ChatUsageCounts{Requests: 1})
	}
	model := &scriptedChatModel{steps: []chatStep{{deltas: []string{"x"}}}}
	svc := newChatUseCases(store, model, usecase.ChatSettings{})

	report, err := svc.ChatUsageReport(context.Background(), 0) // 0 is the default
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Days) != 14 || report.Days[13].Day != "2026-03-14" {
		t.Errorf("days = %d rows ending %q, want 14 ending the current day", len(report.Days), report.Days[13].Day)
	}
	if len(report.TopSubjects) != 10 {
		t.Errorf("top subjects = %d, want 10", len(report.TopSubjects))
	}
	// No budget configured: tokenBudget and budgetRemaining stay 0.
	if report.Today.TokenBudget != 0 || report.Today.BudgetRemaining != 0 {
		t.Errorf("today = %+v, want no budget", report.Today)
	}
}

func TestChatUsageReportRejectsBadDays(t *testing.T) {
	svc := newChatUseCases(newKnowledgeStore(), &scriptedChatModel{steps: []chatStep{{deltas: []string{"x"}}}}, usecase.ChatSettings{})
	for _, days := range []int{-1, 91} {
		if _, err := svc.ChatUsageReport(context.Background(), days); !errors.Is(err, usecase.ErrBadRequest) {
			t.Errorf("days = %d: error %v, want ErrBadRequest", days, err)
		}
	}
}

func newAdminKnowledgeUseCases(store *knowledgeStore, embedder ports.Embedder) usecase.Service {
	return usecase.New(usecase.Dependencies{
		Repositories: store, UnitOfWork: store, Embedder: embedder,
		KnowledgeCorpus: staticCorpus(corpus()), EmbeddingDocModel: "m",
	})
}

func TestReindexKnowledgeRecordsTheLastRun(t *testing.T) {
	store := newKnowledgeStore()
	svc := newAdminKnowledgeUseCases(store, &fakeEmbedder{})
	ctx := context.Background()

	status, err := svc.ReindexKnowledgeNow(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if status.Error != nil || status.Added != 3 || status.Unchanged != 0 {
		t.Fatalf("reindex = %+v, want a successful first run", status)
	}

	got, err := svc.KnowledgeStatus(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Enabled || got.Syncing || len(got.Sources) != 3 {
		t.Fatalf("knowledge status = %+v", got)
	}
	if got.LastSync == nil || got.LastSync.Added != 3 {
		t.Fatalf("lastSync = %+v, want the reindex run", got.LastSync)
	}

	// A second reindex embeds nothing and replaces the recorded run.
	status, err = svc.ReindexKnowledgeNow(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if status.Added != 0 || status.Updated != 0 || status.Unchanged != 3 {
		t.Fatalf("second reindex = %+v, want everything unchanged", status)
	}
}

// A finished but failed run is still the recorded last sync: the panel sees
// the error, which is why the reindex endpoint answers 200 with it set.
func TestReindexKnowledgeReportsFailedRuns(t *testing.T) {
	svc := newAdminKnowledgeUseCases(newKnowledgeStore(), &fakeEmbedder{err: errors.New("voyage down")})

	status, err := svc.ReindexKnowledgeNow(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if status.Error == nil || !strings.Contains(*status.Error, "voyage down") {
		t.Fatalf("status = %+v, want the failure recorded", status)
	}

	got, err := svc.KnowledgeStatus(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	// Nothing was embedded, so no source is listed, and the finished run
	// carries the error.
	if len(got.Sources) != 0 {
		t.Errorf("sources = %+v, want none after a failed run", got.Sources)
	}
	if got.LastSync == nil || got.LastSync.Error == nil {
		t.Errorf("lastSync = %+v, want the failed run", got.LastSync)
	}
	if got.Syncing {
		t.Fatal("status reports a sync running after the run finished")
	}
}

// One state serves the background loop and the manual reindex: while a sync
// runs, a second caller (and the endpoint) is refused, and the finished run is
// the one that panel shows.
func TestReindexKnowledgeNeverOverlaps(t *testing.T) {
	embedder := &fakeEmbedder{}
	embedder.block, embedder.entered = make(chan struct{}), make(chan struct{})
	store := newKnowledgeStore()
	svc := newAdminKnowledgeUseCases(store, embedder)
	ctx := context.Background()

	done := make(chan error, 1)
	go func() {
		// The background loop's path.
		_, err := svc.SyncKnowledgeNow(ctx)
		done <- err
	}()
	<-embedder.entered
	select {
	case err := <-done:
		t.Fatalf("background sync finished too early: %v", err)
	default:
	}

	if _, err := svc.ReindexKnowledgeNow(ctx); !errors.Is(err, domain.ErrKnowledgeSyncRunning) {
		t.Errorf("reindex during a sync: error %v, want ErrKnowledgeSyncRunning", err)
	}
	status, err := svc.KnowledgeStatus(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !status.Syncing {
		t.Error("status must report the running sync")
	}

	close(embedder.block)
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("background sync failed: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("background sync still blocked after release")
	}

	if _, err := svc.ReindexKnowledgeNow(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestReindexKnowledgeWithoutEmbedder(t *testing.T) {
	store := newKnowledgeStore()
	svc := usecase.New(usecase.Dependencies{Repositories: store, UnitOfWork: store})
	if _, err := svc.ReindexKnowledgeNow(context.Background()); !errors.Is(err, domain.ErrKnowledgeDisabled) {
		t.Errorf("error = %v, want ErrKnowledgeDisabled", err)
	}
}
