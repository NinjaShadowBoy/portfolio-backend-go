package config

import (
	"strings"
	"testing"
)

const testSecret = "0123456789abcdef0123456789abcdef"

func setRequired(t *testing.T) {
	t.Helper()
	t.Setenv("DB_URL", "postgres://u:p@localhost:5432/db?sslmode=disable")
	t.Setenv("JWT_SECRET", testSecret)
}

func TestLoadRequiresSecrets(t *testing.T) {
	t.Setenv("DB_URL", "")
	t.Setenv("JWT_SECRET", "")

	_, err := Load()
	if err == nil {
		t.Fatal("expected an error without DB_URL and JWT_SECRET")
	}
	for _, want := range []string{"DB_URL", "JWT_SECRET"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not name %s", err, want)
		}
	}
}

func TestLoadRejectsShortJWTSecret(t *testing.T) {
	setRequired(t)
	t.Setenv("JWT_SECRET", "short")

	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "JWT_SECRET") {
		t.Fatalf("expected a JWT_SECRET error, got %v", err)
	}
}

func TestLoadRequiresSMTPWhenSendingEnabled(t *testing.T) {
	setRequired(t)
	t.Setenv("EMAIL_SENDING_ENABLED", "true")
	t.Setenv("SMTP_USERNAME", "")
	t.Setenv("SMTP_PASSWORD", "")
	t.Setenv("EMAIL_RECIPIENT", "")

	_, err := Load()
	if err == nil {
		t.Fatal("expected an error with email sending enabled and no SMTP settings")
	}
	for _, want := range []string{"SMTP_USERNAME", "SMTP_PASSWORD", "EMAIL_RECIPIENT"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not name %s", err, want)
		}
	}
}

func TestLoadWithRequiredValues(t *testing.T) {
	setRequired(t)
	t.Setenv("EMAIL_SENDING_ENABLED", "false")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.JWTSecret != testSecret {
		t.Errorf("JWTSecret = %q", cfg.JWTSecret)
	}
	if cfg.BindAddr != "127.0.0.1" || cfg.Port != "8081" {
		t.Errorf("unexpected listen defaults %s:%s", cfg.BindAddr, cfg.Port)
	}
	if cfg.Email.SMTPUsername != "" {
		t.Errorf("SMTP username should have no default, got %q", cfg.Email.SMTPUsername)
	}
}
