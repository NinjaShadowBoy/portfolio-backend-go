package postgres

import (
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/ninjashadowboy/portfolio-backend-go/internal/db"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/domain"
)

func TestPersistenceNullsStayOutOfDomain(t *testing.T) {
	user := fromUser(db.User{})
	if user.PasswordHash != nil || user.LastLoginAt != nil || user.ImageURL != nil {
		t.Fatalf("null values were lost: %+v", user)
	}
	now := time.Now()
	user = fromUser(db.User{Pwd: sql.NullString{String: "hash", Valid: true}, LastLoginAt: sql.NullTime{Time: now, Valid: true}})
	if user.PasswordHash == nil || *user.PasswordHash != "hash" || !user.LastLoginAt.Equal(now) {
		t.Fatalf("valid values were lost: %+v", user)
	}
	if !errors.Is(repositoryError(sql.ErrNoRows), domain.ErrNotFound) {
		t.Fatal("persistence error was not translated")
	}
}

func TestNumericAggregateMapping(t *testing.T) {
	for _, value := range []any{float64(4.5), []byte("4.5"), "4.5"} {
		if got := number(value); got != 4.5 {
			t.Fatalf("number(%v) = %v", value, got)
		}
	}
}
