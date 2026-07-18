package dto

import (
	"database/sql"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/ninjashadowboy/portfolio-backend-go/internal/db"
)

func TestFromUserMapsLastLoginAt(t *testing.T) {
	lastLogin := time.Date(2026, 6, 16, 8, 0, 0, 0, time.UTC)
	user := db.User{
		ID:          1,
		Email:       "user@example.com",
		Name:        "User",
		Role:        "ADMIN",
		CreatedAt:   time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		LastLoginAt: sql.NullTime{Time: lastLogin, Valid: true},
	}

	got := FromUser(user)
	if got.ID != 1 || got.Role != "ADMIN" || got.LastLoginAt == nil || !got.LastLoginAt.Equal(lastLogin) {
		t.Fatalf("unexpected user DTO: %+v", got)
	}
}

func TestNullString(t *testing.T) {
	if got := NullString(sql.NullString{}); got != nil {
		t.Fatalf("NullString(invalid) = %v, want nil", got)
	}
	got := NullString(sql.NullString{String: "hello-world", Valid: true})
	if got == nil || *got != "hello-world" {
		t.Fatalf("NullString(valid) = %v, want pointer to \"hello-world\"", got)
	}
}

func TestPtr(t *testing.T) {
	got := Ptr(int64(5))
	if got == nil || *got != 5 {
		t.Fatalf("Ptr(5) = %v, want pointer to 5", got)
	}
}

// commentFromDB mirrors the field/pointer mapping the service layer performs
// when turning a persisted db.Comment into the wire DTO: dto.NullString for the
// nullable slug and a nil-aware pointer for the now-nullable project id. Keeping
// a copy here locks the polymorphic-target contract (exactly one of projectId /
// articleSlug is populated) at the DTO boundary the frontend is coded against.
func commentFromDB(c db.Comment) Comment {
	var projectID *int64
	if c.ProjectID.Valid {
		v := c.ProjectID.Int64
		projectID = &v
	}
	return Comment{
		ID:          c.ID,
		UserID:      c.UserID,
		ProjectID:   projectID,
		ArticleSlug: NullString(c.ArticleSlug),
		Type:        c.Type,
		Body:        c.Body,
		CreatedAt:   c.CreatedAt,
		UpdatedAt:   c.UpdatedAt,
	}
}

func TestCommentDTOArticleTarget(t *testing.T) {
	now := time.Now()
	row := db.Comment{
		ID:          11,
		UserID:      3,
		Body:        "great write-up",
		ProjectID:   sql.NullInt64{},                                    // article comments carry no project
		ArticleSlug: sql.NullString{String: "hello-world", Valid: true}, // target is a slug
		Type:        "suggestion",
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	got := commentFromDB(row)

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
	row := db.Comment{
		ID:          12,
		UserID:      3,
		Body:        "found a bug",
		ProjectID:   sql.NullInt64{Int64: 7, Valid: true}, // target is a project
		ArticleSlug: sql.NullString{},                     // no slug
		Type:        "critique",
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	got := commentFromDB(row)

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
