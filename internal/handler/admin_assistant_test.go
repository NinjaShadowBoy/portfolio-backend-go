package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ninjashadowboy/portfolio-backend-go/internal/domain"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/dto"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/usecase"
)

var adminAssistantRoutes = []struct {
	method, path string
}{
	{"GET", "/portfolio/v1/admin/chat/usage"},
	{"GET", "/portfolio/v1/admin/knowledge"},
	{"POST", "/portfolio/v1/admin/knowledge/reindex"},
}

func TestAdminAssistantRoutesRejectNonAdmins(t *testing.T) {
	for _, route := range adminAssistantRoutes {
		req := httptest.NewRequest(route.method, route.path, nil)
		rec := httptest.NewRecorder()
		newTestRouter(t, nil).ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s anonymous: got %d, want 401", route.method, route.path, rec.Code)
		}

		req = httptest.NewRequest(route.method, route.path, nil)
		req.Header.Set("Authorization", bearer(t, domain.User{ID: 2, Role: "USER"}))
		rec = httptest.NewRecorder()
		newTestRouter(t, nil).ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Errorf("%s %s USER: got %d, want 403", route.method, route.path, rec.Code)
		}
	}
}

func TestAdminChatUsage(t *testing.T) {
	svc := &adminFakeService{usage: dto.ChatUsageReport{
		Days: []dto.ChatUsageDay{
			{Day: "2026-11-08", Requests: 2, InputTokens: 300, OutputTokens: 90},
			{Day: "2026-11-09", Requests: 1, InputTokens: 150, OutputTokens: 60},
		},
		TopSubjects: []dto.ChatUsageSubject{{Subject: "ip:1a2b", Requests: 2, InputTokens: 300, OutputTokens: 90}},
		Today:       dto.ChatBudget{Requests: 1, Tokens: 210, TokenBudget: 2000000, BudgetRemaining: 1999790},
	}}
	r := newTestRouter(t, svc)
	admin := bearer(t, domain.User{ID: 1, Role: "ADMIN"})

	req := httptest.NewRequest(http.MethodGet, "/portfolio/v1/admin/chat/usage?days=14", nil)
	req.Header.Set("Authorization", admin)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d, want 200 (body %s)", rec.Code, rec.Body.String())
	}
	var got dto.ChatUsageReport
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if svc.usageDays != 14 {
		t.Errorf("service got days = %d, want 14", svc.usageDays)
	}
	if got.Today != svc.usage.Today || len(got.Days) != 2 || got.TopSubjects[0].Subject != "ip:1a2b" {
		t.Errorf("body = %+v, want the fake's report", got)
	}

	// Absent days goes to the use case as 0, which applies its default.
	req = httptest.NewRequest(http.MethodGet, "/portfolio/v1/admin/chat/usage", nil)
	req.Header.Set("Authorization", admin)
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || svc.usageDays != 0 {
		t.Fatalf("absent days: got %d days=%d, want 200 and 0", rec.Code, svc.usageDays)
	}

	// Unparseable values are rejected before the service; out-of-range values
	// reach the use case, whose bad-request error maps to a 400.
	svc.usageErr = nil
	for _, days := range []string{"ten"} {
		req = httptest.NewRequest(http.MethodGet, "/portfolio/v1/admin/chat/usage?days="+days, nil)
		req.Header.Set("Authorization", admin)
		rec = httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("days=%s: got %d, want 400", days, rec.Code)
		}
	}
}

func TestAdminChatUsageRejectsBadRange(t *testing.T) {
	svc := &adminFakeService{}
	r := newTestRouter(t, svc)
	admin := bearer(t, domain.User{ID: 1, Role: "ADMIN"})

	for _, days := range []string{"-1", "91"} {
		// The real use case turns these into a wrapped ErrBadRequest; the fake
		// replays the same error so the mapping stays under test.
		svc.usageErr = fmt.Errorf("%w: days must be between 1 and 90", usecase.ErrBadRequest)
		req := httptest.NewRequest(http.MethodGet, "/portfolio/v1/admin/chat/usage?days="+days, nil)
		req.Header.Set("Authorization", admin)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("days=%s: got %d, want 400", days, rec.Code)
		}
	}
}

func TestAdminKnowledge(t *testing.T) {
	finished := time.Date(2026, 11, 9, 9, 30, 0, 0, time.UTC)
	errText := "embed knowledge: voyage down"
	status := dto.KnowledgeStatus{
		Enabled: true,
		Sources: []dto.KnowledgeSourceStatus{{
			Kind: "publication", Ref: "cerc2026-tonal", Lang: "en", Title: "Tonal paper",
			URL: "/publications", Chunks: 4, UpdatedAt: finished,
		}},
		LastSync: &dto.KnowledgeSyncStatus{
			StartedAt: finished.Add(-time.Minute), FinishedAt: finished,
			Added: 2, Updated: 1, Deleted: 3, Unchanged: 6, Error: &errText,
		},
	}
	svc := &adminFakeService{knowledge: status}
	r := newTestRouter(t, svc)

	req := httptest.NewRequest(http.MethodGet, "/portfolio/v1/admin/knowledge", nil)
	req.Header.Set("Authorization", bearer(t, domain.User{ID: 1, Role: "ADMIN"}))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d, want 200 (body %s)", rec.Code, rec.Body.String())
	}
	var got dto.KnowledgeStatus
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Enabled != status.Enabled || got.Syncing || len(got.Sources) != 1 {
		t.Errorf("body = %+v, want the fake's status", got)
	}
	if got.LastSync == nil || got.LastSync.Updated != 1 || got.LastSync.Error == nil {
		t.Errorf("lastSync = %+v, want the fake's run", got.LastSync)
	}
}

func TestAdminReindexKnowledge(t *testing.T) {
	finished := time.Date(2026, 11, 9, 9, 30, 0, 0, time.UTC)
	run := dto.KnowledgeSyncStatus{StartedAt: finished.Add(-time.Minute), FinishedAt: finished, Added: 3, Unchanged: 5}

	t.Run("200 with the run", func(t *testing.T) {
		svc := &adminFakeService{reindex: run}
		req := httptest.NewRequest(http.MethodPost, "/portfolio/v1/admin/knowledge/reindex", nil)
		req.Header.Set("Authorization", bearer(t, domain.User{ID: 1, Role: "ADMIN"}))
		rec := httptest.NewRecorder()
		newTestRouter(t, svc).ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("got %d, want 200 (body %s)", rec.Code, rec.Body.String())
		}
		var got dto.KnowledgeSyncStatus
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		if got.Added != run.Added || got.Unchanged != run.Unchanged || got.Error != nil {
			t.Errorf("body = %+v, want the fake's run", got)
		}
	})

	t.Run("409 when a sync is running", func(t *testing.T) {
		svc := &adminFakeService{reindexErr: domain.ErrKnowledgeSyncRunning}
		req := httptest.NewRequest(http.MethodPost, "/portfolio/v1/admin/knowledge/reindex", nil)
		req.Header.Set("Authorization", bearer(t, domain.User{ID: 1, Role: "ADMIN"}))
		rec := httptest.NewRecorder()
		newTestRouter(t, svc).ServeHTTP(rec, req)
		if rec.Code != http.StatusConflict {
			t.Fatalf("got %d, want 409 (body %s)", rec.Code, rec.Body.String())
		}
		var problem dto.ErrorResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &problem); err != nil {
			t.Fatal(err)
		}
		if problem.Code != "KNOWLEDGE_SYNC_RUNNING" {
			t.Errorf("code = %q, want KNOWLEDGE_SYNC_RUNNING", problem.Code)
		}
	})

	t.Run("503 when disabled", func(t *testing.T) {
		svc := &adminFakeService{reindexErr: domain.ErrKnowledgeDisabled}
		req := httptest.NewRequest(http.MethodPost, "/portfolio/v1/admin/knowledge/reindex", nil)
		req.Header.Set("Authorization", bearer(t, domain.User{ID: 1, Role: "ADMIN"}))
		rec := httptest.NewRecorder()
		newTestRouter(t, svc).ServeHTTP(rec, req)
		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("got %d, want 503 (body %s)", rec.Code, rec.Body.String())
		}
		var problem dto.ErrorResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &problem); err != nil {
			t.Fatal(err)
		}
		if problem.Code != "KNOWLEDGE_DISABLED" {
			t.Errorf("code = %q, want KNOWLEDGE_DISABLED", problem.Code)
		}
	})

	t.Run("internal errors keep the standard shape", func(t *testing.T) {
		svc := &adminFakeService{reindexErr: errors.New("db gone")}
		req := httptest.NewRequest(http.MethodPost, "/portfolio/v1/admin/knowledge/reindex", nil)
		req.Header.Set("Authorization", bearer(t, domain.User{ID: 1, Role: "ADMIN"}))
		rec := httptest.NewRecorder()
		newTestRouter(t, svc).ServeHTTP(rec, req)
		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("got %d, want 500 (body %s)", rec.Code, rec.Body.String())
		}
		var problem dto.ErrorResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &problem); err != nil {
			t.Fatal(err)
		}
		if problem.Code != "INTERNAL_ERROR" || problem.Message == "db gone" {
			t.Errorf("body = %+v, want a generic internal error", problem)
		}
	})
}
