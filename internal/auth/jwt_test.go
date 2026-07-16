package auth

import (
	"database/sql"
	"testing"
	"time"

	"github.com/ninjashadowboy/portfolio-backend-go/internal/db"
)

func TestJWTGenerateAndParse(t *testing.T) {
	jwt := NewJWT("test-secret", time.Hour, "test-issuer")
	user := db.User{
		ID:    7,
		Email: "user@example.com",
		Name:  "User",
		Role:  "USER",
		Pwd:   sql.NullString{String: "password", Valid: true},
	}

	resp, err := jwt.GenerateLoginResponse(user)
	if err != nil {
		t.Fatalf("GenerateLoginResponse() error = %v", err)
	}
	if resp.Token == "" || resp.ExpiresIn != int64(time.Hour/time.Millisecond) {
		t.Fatalf("unexpected login response: %+v", resp)
	}

	claims, err := jwt.Parse(resp.Token)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if claims.UserID != 7 || claims.Subject != "user@example.com" || claims.Issuer != "test-issuer" {
		t.Fatalf("unexpected claims: %+v", claims)
	}
}

func TestVerifyPassword(t *testing.T) {
	hashed, err := HashPassword("password")
	if err != nil {
		t.Fatalf("HashPassword() error = %v", err)
	}
	if !VerifyPassword(sql.NullString{String: hashed, Valid: true}, "password") {
		t.Fatal("expected hashed password to match")
	}
	if VerifyPassword(sql.NullString{String: hashed, Valid: true}, "wrong") {
		t.Fatal("expected wrong password to fail")
	}
	if VerifyPassword(sql.NullString{}, "password") {
		t.Fatal("expected missing password to fail")
	}
	// Backward compatibility: legacy plaintext values still verify.
	if !VerifyPassword(sql.NullString{String: "legacy", Valid: true}, "legacy") {
		t.Fatal("expected legacy plaintext password to match")
	}
}
