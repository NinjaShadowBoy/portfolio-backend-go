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
	CookieSecure       bool
	Email              EmailConfig
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
		CookieSecure:       !isLocalEnvironment(),
		Email: EmailConfig{
			SMTPHost:            env("SMTP_HOST", "smtp.gmail.com"),
			SMTPPort:            envInt("SMTP_PORT", 587),
			SMTPUsername:        env("SMTP_USERNAME", ""),
			SMTPPassword:        env("SMTP_PASSWORD", ""),
			EmailRecipient:      env("EMAIL_RECIPIENT", ""),
			EmailSendingEnabled: envBool("EMAIL_SENDING_ENABLED", false),
		},
	}

	return cfg, cfg.validate()
}

// validate rejects a configuration that would run with a missing secret.
// Everything listed here has no safe default.
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
	if len(missing) == 0 {
		return nil
	}
	sort.Strings(missing)
	return fmt.Errorf("missing required configuration: %s", strings.Join(missing, ", "))
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
