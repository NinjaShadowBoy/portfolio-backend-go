package auth

import (
	"testing"
	"time"

	"github.com/ninjashadowboy/portfolio-backend-go/internal/domain"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/dto"
)

func TestJWTGenerateAndParse(t *testing.T) {
	jwt := NewJWT("test-secret", time.Hour, "test-issuer")
	user := domain.User{
		ID:           7,
		Email:        "user@example.com",
		Name:         "User",
		Role:         "USER",
		PasswordHash: dto.Ptr("password"),
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
	if !VerifyPassword(dto.Ptr(hashed), "password") {
		t.Fatal("expected hashed password to match")
	}
	if VerifyPassword(dto.Ptr(hashed), "wrong") {
		t.Fatal("expected wrong password to fail")
	}
	if VerifyPassword(nil, "password") {
		t.Fatal("expected missing password to fail")
	}
	// Backward compatibility: legacy plaintext values still verify.
	if !VerifyPassword(dto.Ptr("legacy"), "legacy") {
		t.Fatal("expected legacy plaintext password to match")
	}
}
