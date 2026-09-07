package dto

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/ninjashadowboy/portfolio-backend-go/internal/domain"
)

func TestFromUserMapsLastLoginAt(t *testing.T) {
	lastLogin := time.Date(2026, 6, 16, 8, 0, 0, 0, time.UTC)
	user := domain.User{
		ID:          1,
		Email:       "user@example.com",
		Name:        "User",
		Role:        "ADMIN",
		CreatedAt:   time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		LastLoginAt: Ptr(lastLogin),
	}

	got := FromUser(user)
	if got.ID != 1 || got.Role != "ADMIN" || got.LastLoginAt == nil || !got.LastLoginAt.Equal(lastLogin) {
		t.Fatalf("unexpected user DTO: %+v", got)
	}
}

func TestPtr(t *testing.T) {
	got := Ptr(int64(5))
	if got == nil || *got != 5 {
		t.Fatalf("Ptr(5) = %v, want pointer to 5", got)
	}
}

// The FromComment tests lock the polymorphic-target contract (exactly one of
// projectId / articleSlug is populated) at the DTO boundary the frontend
// CommentService is coded against.
func TestCommentDTOArticleTarget(t *testing.T) {
	now := time.Now()
	row := domain.Comment{
		ID:          11,
		UserID:      3,
		Body:        "great write-up",
		ProjectID:   nil,                // article comments carry no project
		ArticleSlug: Ptr("hello-world"), // target is a slug
		Type:        "suggestion",
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	got := FromComment(row, "Reader", nil)

	if got.ProjectID != nil {
		t.Fatalf("ProjectID = %v, want nil for an article comment", got.ProjectID)
	}
	if got.ArticleSlug == nil || *got.ArticleSlug != "hello-world" {
		t.Fatalf("ArticleSlug = %v, want pointer to \"hello-world\"", got.ArticleSlug)
	}
	if got.Type != "suggestion" {
		t.Fatalf("Type = %q, want \"suggestion\"", got.Type)
	}

	// The JSON wire shape (projectId null, articleSlug + type present) is the
	// contract the frontend CommentService is coded against.
	raw, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	body := string(raw)
	for _, want := range []string{`"projectId":null`, `"articleSlug":"hello-world"`, `"type":"suggestion"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("marshaled comment %s missing %s", body, want)
		}
	}
}

func TestCommentDTOProjectTarget(t *testing.T) {
	now := time.Now()
	row := domain.Comment{
		ID:          12,
		UserID:      3,
		Body:        "found a bug",
		ProjectID:   Ptr(int64(7)), // target is a project
		ArticleSlug: nil,           // no slug
		Type:        "critique",
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	got := FromComment(row, "Reader", nil)

	if got.ProjectID == nil || *got.ProjectID != 7 {
		t.Fatalf("ProjectID = %v, want pointer to 7", got.ProjectID)
	}
	if got.ArticleSlug != nil {
		t.Fatalf("ArticleSlug = %v, want nil for a project comment", got.ArticleSlug)
	}

	raw, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	body := string(raw)
	for _, want := range []string{`"projectId":7`, `"articleSlug":null`, `"type":"critique"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("marshaled comment %s missing %s", body, want)
		}
	}
}
