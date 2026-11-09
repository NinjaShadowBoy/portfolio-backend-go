package config

import (
	"strings"
	"testing"
	"time"
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

func TestKnowledgeDefaultsAndDisabledWithoutChat(t *testing.T) {
	setRequired(t)
	for _, key := range []string{"CHAT_ENABLED", "OPENROUTER_API_KEY", "EMBEDDING_DOC_MODEL", "EMBEDDING_QUERY_MODEL", "KNOWLEDGE_DIR", "KNOWLEDGE_SYNC_INTERVAL"} {
		t.Setenv(key, "")
	}
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	k := cfg.Knowledge
	if k.Enabled() {
		t.Fatal("knowledge index enabled with the chat off")
	}
	if k.DocModel != "mongodb/voyage-4-large" || k.QueryModel != "mongodb/voyage-4-lite" || k.Dir != "knowledge" || k.SyncInterval != 10*time.Minute {
		t.Fatalf("unexpected knowledge defaults %+v", k)
	}

	// A key alone is not enough: CHAT_ENABLED is the kill switch for both.
	t.Setenv("OPENROUTER_API_KEY", "sk-or-test")
	if cfg, _ := Load(); cfg.Knowledge.Enabled() {
		t.Fatal("knowledge index enabled by a key while the chat is off")
	}
}

func TestKnowledgeSharesTheChatKey(t *testing.T) {
	setRequired(t)
	t.Setenv("CHAT_ENABLED", "true")
	t.Setenv("OPENROUTER_API_KEY", "sk-or-test")
	t.Setenv("EMBEDDING_DOC_MODEL", "mongodb/voyage-4")
	t.Setenv("EMBEDDING_QUERY_MODEL", "")
	t.Setenv("KNOWLEDGE_DIR", "/srv/knowledge")
	t.Setenv("KNOWLEDGE_SYNC_INTERVAL", "90s")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	k := cfg.Knowledge
	if !k.Enabled() || k.APIKey != "sk-or-test" || k.DocModel != "mongodb/voyage-4" || k.QueryModel != "mongodb/voyage-4-lite" ||
		k.Dir != "/srv/knowledge" || k.SyncInterval != 90*time.Second {
		t.Fatalf("settings not applied: %+v", k)
	}
	for _, bad := range []string{"soon", "-5m", "0"} {
		t.Setenv("KNOWLEDGE_SYNC_INTERVAL", bad)
		if cfg, _ := Load(); cfg.Knowledge.SyncInterval != 10*time.Minute {
			t.Errorf("interval %q gave %v, want the default", bad, cfg.Knowledge.SyncInterval)
		}
	}
}

var chatKeys = []string{"CHAT_ENABLED", "OPENROUTER_API_KEY", "CHAT_MODEL", "CHAT_PROVIDER_SORT", "CHAT_DATA_COLLECTION",
	"CHAT_REASONING", "CHAT_DAILY_MESSAGES_ANON", "CHAT_DAILY_MESSAGES_USER",
	"CHAT_DAILY_TOKEN_BUDGET", "CHAT_MAX_CONCURRENT", "TRUSTED_PROXIES"}

func TestChatDefaultsOffWithoutKeys(t *testing.T) {
	setRequired(t)
	for _, key := range chatKeys {
		t.Setenv(key, "")
	}
	cfg, err := Load()
	if err != nil {
		t.Fatalf("chat off must need no keys: %v", err)
	}
	want := ChatConfig{
		Model: "openrouter/free", DataCollection: "allow", Reasoning: "default",
		DailyMessagesAnonymous: 30, DailyMessagesUser: 100, DailyTokenBudget: 2_000_000, MaxConcurrent: 4,
	}
	if cfg.Chat != want {
		t.Fatalf("chat defaults = %+v, want %+v", cfg.Chat, want)
	}
	if len(cfg.TrustedProxies) != 0 {
		t.Fatalf("TrustedProxies default = %v, want none", cfg.TrustedProxies)
	}
}

func TestChatEnabledRequiresOnlyOpenRouterKey(t *testing.T) {
	setRequired(t)
	for _, key := range chatKeys {
		t.Setenv(key, "")
	}
	t.Setenv("CHAT_ENABLED", "true")
	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "OPENROUTER_API_KEY") {
		t.Fatalf("expected an OPENROUTER_API_KEY error, got %v", err)
	}

	t.Setenv("OPENROUTER_API_KEY", "sk-or-test")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("chat with an OpenRouter key: %v", err)
	}
	if !cfg.Chat.Enabled || cfg.Chat.OpenRouterAPIKey != "sk-or-test" || !cfg.Knowledge.Enabled() {
		t.Fatalf("unexpected chat config %+v", cfg.Chat)
	}
}

func TestChatSettings(t *testing.T) {
	setRequired(t)
	t.Setenv("CHAT_MODEL", "anthropic/claude-haiku-5.5")
	t.Setenv("CHAT_PROVIDER_SORT", "price")
	t.Setenv("CHAT_DATA_COLLECTION", "deny")
	t.Setenv("CHAT_REASONING", "off")
	t.Setenv("CHAT_DAILY_MESSAGES_ANON", "5")
	t.Setenv("CHAT_DAILY_MESSAGES_USER", "50")
	t.Setenv("CHAT_DAILY_TOKEN_BUDGET", "300000")
	t.Setenv("CHAT_MAX_CONCURRENT", "2")
	t.Setenv("TRUSTED_PROXIES", " 172.18.0.1, 10.0.0.0/8 ,")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	c := cfg.Chat
	if c.Model != "anthropic/claude-haiku-5.5" || c.ProviderSort != "price" || c.DataCollection != "deny" || c.Reasoning != "off" ||
		c.DailyMessagesAnonymous != 5 || c.DailyMessagesUser != 50 || c.DailyTokenBudget != 300000 || c.MaxConcurrent != 2 {
		t.Fatalf("settings not applied: %+v", c)
	}
	if len(cfg.TrustedProxies) != 2 || cfg.TrustedProxies[0] != "172.18.0.1" || cfg.TrustedProxies[1] != "10.0.0.0/8" {
		t.Fatalf("TrustedProxies = %q", cfg.TrustedProxies)
	}
	for _, bad := range []string{"0", "-3", "lots"} {
		t.Setenv("CHAT_MAX_CONCURRENT", bad)
		if cfg, _ := Load(); cfg.Chat.MaxConcurrent != 4 {
			t.Errorf("CHAT_MAX_CONCURRENT=%q gave %d, want the default", bad, cfg.Chat.MaxConcurrent)
		}
	}
}

func TestChatRejectsInvalidEnums(t *testing.T) {
	setRequired(t)
	for _, key := range chatKeys {
		t.Setenv(key, "")
	}
	for key, value := range map[string]string{
		"CHAT_PROVIDER_SORT":   "cheapest",
		"CHAT_DATA_COLLECTION": "sometimes",
		"CHAT_REASONING":       "lots",
	} {
		t.Setenv(key, value)
		_, err := Load()
		if err == nil || !strings.Contains(err.Error(), key) {
			t.Errorf("%s=%q gave %v, want an error naming it", key, value, err)
		}
		t.Setenv(key, "")
	}
	// The enums are case-insensitive.
	t.Setenv("CHAT_PROVIDER_SORT", "Price")
	t.Setenv("CHAT_DATA_COLLECTION", "DENY")
	t.Setenv("CHAT_REASONING", "Low")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("mixed-case enums: %v", err)
	}
	if cfg.Chat.ProviderSort != "price" || cfg.Chat.DataCollection != "deny" || cfg.Chat.Reasoning != "low" {
		t.Fatalf("enums not lower-cased: %+v", cfg.Chat)
	}
}
