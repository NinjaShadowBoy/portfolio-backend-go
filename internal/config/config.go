package config

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

// API base path segments -  the single source of truth for the API version. The
// full mount point is APIPrefix + "/" + APIVersion, e.g. "/portfolio/v1". Bump
// APIVersion (and add a new route group) for breaking API changes so old clients
// keep working. The OAuthBaseURL default and the router both derive from these,
// so a bump can't leave the OAuth callback URL out of sync with the routes.
const (
	APIPrefix  = "/portfolio"
	APIVersion = "v1"
)

// BasePath is the versioned root every route is mounted under, e.g. "/portfolio/v1".
func BasePath() string {
	return APIPrefix + "/" + APIVersion
}

type EmailConfig struct {
	SMTPHost            string
	SMTPPort            int
	SMTPUsername        string
	SMTPPassword        string
	EmailRecipient      string
	EmailSendingEnabled bool
}

// KnowledgeConfig drives the assistant's knowledge index. It embeds through
// OpenRouter with the chat's key, so it runs exactly when the chat does:
// with CHAT_ENABLED off, APIKey stays empty and nothing is embedded.
type KnowledgeConfig struct {
	// APIKey is OPENROUTER_API_KEY while the chat is on, empty otherwise.
	APIKey string
	// DocModel embeds the corpus, QueryModel embeds search queries (OpenRouter
	// model ids). Voyage 4 series models share one embedding space.
	DocModel   string
	QueryModel string
	// Dir holds the corpus files, relative to the working directory like
	// MigrationsPath (the image runs from /app and ships /app/knowledge).
	Dir          string
	SyncInterval time.Duration
}

// Enabled reports whether the knowledge index runs.
func (k KnowledgeConfig) Enabled() bool { return strings.TrimSpace(k.APIKey) != "" }

// ChatConfig drives the site assistant. It is off unless Enabled (the kill
// switch, CHAT_ENABLED); nothing calls OpenRouter then.
type ChatConfig struct {
	Enabled          bool
	OpenRouterAPIKey string
	// Model is the OpenRouter model id. The default is the free models
	// router, so a local run never spends money by accident.
	Model string
	// ProviderSort is OpenRouter's provider ordering: "price", "throughput"
	// or "latency". Empty keeps its default routing.
	ProviderSort string
	// DataCollection is "allow" or "deny": whether the chosen provider may
	// train on the conversation. Many free models require "allow".
	DataCollection string
	// Reasoning is "default", "off", "low", "medium" or "high". "default"
	// (the default) sends no reasoning field, so openrouter/free can still
	// pick any free model while require_parameters is on. Haiku 5.5 thinks by
	// default and its thinking tokens count against the answer budget, so
	// production sets "off". An Anthropic effort needs max_tokens above its
	// 1024-token minimum, more than the assistant's 800-token budget.
	Reasoning string
	// DailyMessagesAnonymous and DailyMessagesUser cap requests per visitor
	// per UTC day; admins have no cap.
	DailyMessagesAnonymous int
	DailyMessagesUser      int
	// DailyTokenBudget caps input plus output tokens per UTC day across all
	// visitors. It is the owner's spend knob: when it is used up the
	// assistant rests for everyone until midnight UTC.
	DailyTokenBudget int64
	// MaxConcurrent caps simultaneous chat streams (memory in a 128 MB
	// container).
	MaxConcurrent int
}

type Config struct {
	// BindAddr is the interface the HTTP server listens on. 127.0.0.1 keeps a
	// bare-metal run private behind nginx; the container image sets 0.0.0.0
	// because Docker's port mapping is the boundary there.
	BindAddr string
	Port     string
	// AppEnv is the deployment environment (local, staging, production). It
	// gates seeding and cookie security and labels telemetry.
	AppEnv string
	// ServiceName identifies this process in traces, metrics and logs.
	ServiceName        string
	DBURL              string
	MigrationsPath     string
	SeedsPath          string
	SeedEnabled        bool
	JWTSecret          string
	JWTExpiration      time.Duration
	JWTIssuer          string
	OAuthBaseURL       string
	OAuthRedirectURI   string
	GoogleClientID     string
	GoogleClientSecret string
	GitHubClientID     string
	GitHubClientSecret string
	AllowedOrigins     []string
	// TrustedProxies are the proxy addresses or CIDRs whose X-Real-IP header
	// gives the client IP. Empty trusts none, and the client IP is the TCP
	// peer.
	TrustedProxies []string
	CookieSecure   bool
	Email          EmailConfig
	Knowledge      KnowledgeConfig
	Chat           ChatConfig
}

func Load() (Config, error) {
	loadDotEnv()

	expirationMS := envInt("JWT_EXPIRATION", 86400000)
	origins := splitCSV(env("CORS_ALLOWED_ORIGINS", "http://localhost:*"))

	// Secrets have no fallback on purpose: a missing value must fail startup
	// rather than silently run with a key that is in the git history.
	cfg := Config{
		BindAddr:           env("BIND_ADDR", "127.0.0.1"),
		Port:               env("PORT", "8081"),
		AppEnv:             env("APP_ENV", "local"),
		ServiceName:        env("OTEL_SERVICE_NAME", "portfolio-backend"),
		DBURL:              env("DB_URL", ""),
		MigrationsPath:     env("MIGRATIONS_PATH", "db/migrations"),
		SeedsPath:          env("SEEDS_PATH", "db/seeds"),
		SeedEnabled:        isLocalEnvironment() && envBool("SEED_ENABLED", true),
		JWTSecret:          env("JWT_SECRET", ""),
		JWTExpiration:      time.Duration(expirationMS) * time.Millisecond,
		JWTIssuer:          env("JWT_ISSUER", "NinjaShadowBoy-PortfolioApp"),
		OAuthBaseURL:       env("OAUTH_BASE_URL", "http://localhost:8081"+BasePath()),
		OAuthRedirectURI:   env("OAUTH2_REDIRECT_URI", "http://localhost:4200/oauth2/redirect"),
		GoogleClientID:     env("GOOGLE_CLIENT_ID", ""),
		GoogleClientSecret: env("GOOGLE_CLIENT_SECRET", ""),
		GitHubClientID:     env("GITHUB_CLIENT_ID", ""),
		GitHubClientSecret: env("GITHUB_CLIENT_SECRET", ""),
		AllowedOrigins:     origins,
		TrustedProxies:     splitCSV(env("TRUSTED_PROXIES", "")),
		CookieSecure:       !isLocalEnvironment(),
		Email: EmailConfig{
			SMTPHost:            env("SMTP_HOST", "smtp.gmail.com"),
			SMTPPort:            envInt("SMTP_PORT", 587),
			SMTPUsername:        env("SMTP_USERNAME", ""),
			SMTPPassword:        env("SMTP_PASSWORD", ""),
			EmailRecipient:      env("EMAIL_RECIPIENT", ""),
			EmailSendingEnabled: envBool("EMAIL_SENDING_ENABLED", false),
		},
		Knowledge: KnowledgeConfig{
			DocModel:     env("EMBEDDING_DOC_MODEL", "mongodb/voyage-4-large"),
			QueryModel:   env("EMBEDDING_QUERY_MODEL", "mongodb/voyage-4-lite"),
			Dir:          env("KNOWLEDGE_DIR", "knowledge"),
			SyncInterval: envDuration("KNOWLEDGE_SYNC_INTERVAL", 10*time.Minute),
		},
		Chat: ChatConfig{
			Enabled:                envBool("CHAT_ENABLED", false),
			OpenRouterAPIKey:       env("OPENROUTER_API_KEY", ""),
			Model:                  env("CHAT_MODEL", "openrouter/free"),
			ProviderSort:           strings.ToLower(strings.TrimSpace(env("CHAT_PROVIDER_SORT", ""))),
			DataCollection:         strings.ToLower(strings.TrimSpace(env("CHAT_DATA_COLLECTION", "allow"))),
			Reasoning:              strings.ToLower(strings.TrimSpace(env("CHAT_REASONING", "default"))),
			DailyMessagesAnonymous: envPositiveInt("CHAT_DAILY_MESSAGES_ANON", 30),
			DailyMessagesUser:      envPositiveInt("CHAT_DAILY_MESSAGES_USER", 100),
			DailyTokenBudget:       int64(envPositiveInt("CHAT_DAILY_TOKEN_BUDGET", 2_000_000)),
			MaxConcurrent:          envPositiveInt("CHAT_MAX_CONCURRENT", 4),
		},
	}

	// One key for chat and embeddings: the index runs whenever the chat does.
	if cfg.Chat.Enabled {
		cfg.Knowledge.APIKey = cfg.Chat.OpenRouterAPIKey
	}
	return cfg, cfg.validate()
}

// validate rejects a configuration that would run with a missing secret or
// an invalid setting. Everything listed here has no safe default.
func (c Config) validate() error {
	var missing []string
	if strings.TrimSpace(c.DBURL) == "" {
		missing = append(missing, "DB_URL")
	}
	if len(strings.TrimSpace(c.JWTSecret)) < 32 {
		missing = append(missing, "JWT_SECRET (at least 32 characters)")
	}
	if c.Email.EmailSendingEnabled {
		for key, value := range map[string]string{
			"SMTP_USERNAME":   c.Email.SMTPUsername,
			"SMTP_PASSWORD":   c.Email.SMTPPassword,
			"EMAIL_RECIPIENT": c.Email.EmailRecipient,
		} {
			if strings.TrimSpace(value) == "" {
				missing = append(missing, key+" (required when EMAIL_SENDING_ENABLED=true)")
			}
		}
	}
	if c.Chat.Enabled && strings.TrimSpace(c.Chat.OpenRouterAPIKey) == "" {
		missing = append(missing, "OPENROUTER_API_KEY (required when CHAT_ENABLED=true)")
	}
	var invalid []string
	switch c.Chat.ProviderSort {
	case "", "price", "throughput", "latency":
	default:
		invalid = append(invalid, fmt.Sprintf("CHAT_PROVIDER_SORT=%q (want price, throughput or latency)", c.Chat.ProviderSort))
	}
	switch c.Chat.DataCollection {
	case "allow", "deny":
	default:
		invalid = append(invalid, fmt.Sprintf("CHAT_DATA_COLLECTION=%q (want allow or deny)", c.Chat.DataCollection))
	}
	switch c.Chat.Reasoning {
	case "default", "off", "low", "medium", "high":
	default:
		invalid = append(invalid, fmt.Sprintf("CHAT_REASONING=%q (want default, off, low, medium or high)", c.Chat.Reasoning))
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return fmt.Errorf("missing required configuration: %s", strings.Join(missing, ", "))
	}
	if len(invalid) > 0 {
		return fmt.Errorf("invalid configuration: %s", strings.Join(invalid, ", "))
	}
	return nil
}

func loadDotEnv() {
	contents, err := os.ReadFile(".env")
	if err != nil {
		return
	}

	for _, line := range strings.Split(string(contents), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		parts := strings.SplitN(trimmed, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		value = strings.Trim(value, `"'`)
		if key != "" && !envExists(key) {
			os.Setenv(key, value)
		}
	}
}

func envExists(key string) bool {
	_, exists := os.LookupEnv(key)
	return exists
}

func isLocalEnvironment() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("APP_ENV"))) {
	case "local", "dev", "development":
		return true
	default:
		return false
	}
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func envInt(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}

	return parsed
}

// envPositiveInt is envInt where zero or a negative value also gives the
// fallback, for caps that must never be zero.
func envPositiveInt(key string, fallback int) int {
	if v := envInt(key, fallback); v > 0 {
		return v
	}
	return fallback
}

// envDuration parses a Go duration such as "10m". A missing, invalid or
// non-positive value gives the fallback.
func envDuration(key string, fallback time.Duration) time.Duration {
	parsed, err := time.ParseDuration(strings.TrimSpace(os.Getenv(key)))
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func envBool(key string, fallback bool) bool {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "t", "true", "y", "yes", "on":
		return true
	case "0", "f", "false", "n", "no", "off":
		return false
	default:
		return fallback
	}
}

func splitCSV(value string) []string {
	parts := strings.Split(value, ",")
	items := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			items = append(items, trimmed)
		}
	}
	return items
}
