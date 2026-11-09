package usecase

import (
	"sync"
	"time"

	"github.com/ninjashadowboy/portfolio-backend-go/internal/domain"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/dto"
)

// knowledgeSyncState remembers the knowledge index syncs of this process:
// whether one is running and how the last one finished. One instance lives
// on UseCases, so the background loop and the admin reindex share it and can
// never overlap. It is guarded with one mutex; the usecase copies (made by
// inTransaction) hold the same pointer.
type knowledgeSyncState struct {
	mu      sync.Mutex
	running bool
	// last is nil until the first sync of this process has finished.
	last *dto.KnowledgeSyncStatus
}

func newKnowledgeSyncState() *knowledgeSyncState { return &knowledgeSyncState{} }

// begin claims the right to sync. False means another sync is running.
func (st *knowledgeSyncState) begin() bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.running {
		return false
	}
	st.running = true
	return true
}

// finish releases the claim and records the run, success or failure.
func (st *knowledgeSyncState) finish(startedAt time.Time, report domain.KnowledgeSyncReport, err error) {
	status := &dto.KnowledgeSyncStatus{
		StartedAt:  startedAt.UTC(),
		FinishedAt: time.Now().UTC(),
		Added:      report.Added,
		Updated:    report.Updated,
		Deleted:    report.Deleted,
		Unchanged:  report.Unchanged,
	}
	if err != nil {
		text := truncate(err.Error(), 500)
		status.Error = &text
	}
	st.mu.Lock()
	st.running = false
	st.last = status
	st.mu.Unlock()
}

// status is what GET /admin/knowledge reports: the running flag and the last
// finished run, or nil before the first one.
func (st *knowledgeSyncState) status() (syncing bool, last *dto.KnowledgeSyncStatus) {
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.running, st.last
}

// finished is the last completed run, or the zero status before the first.
func (st *knowledgeSyncState) finished() dto.KnowledgeSyncStatus {
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.last == nil {
		return dto.KnowledgeSyncStatus{}
	}
	return *st.last
}

// truncate cuts s to at most max runes, keeping it one admin-readable line of
// bounded length in the lastSync.error field.
func truncate(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max])
}
