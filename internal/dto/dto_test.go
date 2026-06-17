package dto

import (
	"database/sql"
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
