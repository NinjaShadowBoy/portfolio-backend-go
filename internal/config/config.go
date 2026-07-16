package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// API base path segments — the single source of truth for the API version. The
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
	Port               string
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
	Email              EmailConfig
}

func Load() (Config, error) {
	loadDotEnv()

	expirationMS := envInt("JWT_EXPIRATION", 86400000)
	origins := splitCSV(env("CORS_ALLOWED_ORIGINS", "http://localhost:*"))

	cfg := Config{
		Port:               env("PORT", "8080"),
		DBURL:              env("DB_URL", "postgres://postgres:123456789@localhost:5432/portfolio?sslmode=disable"),
		MigrationsPath:     env("MIGRATIONS_PATH", "db/migrations"),
		SeedsPath:          env("SEEDS_PATH", "db/seeds"),
		SeedEnabled:        isLocalEnvironment() && envBool("SEED_ENABLED", true),
		JWTSecret:          env("JWT_SECRET", "404E635266556A586E3072357538782F413F4428472B4B6250645367566B5970"),
		JWTExpiration:      time.Duration(expirationMS) * time.Millisecond,
		JWTIssuer:          env("JWT_ISSUER", "NinjaShadowBoy-PortfolioApp"),
		OAuthBaseURL:       env("OAUTH_BASE_URL", "http://localhost:8080"+BasePath()),
		OAuthRedirectURI:   env("OAUTH2_REDIRECT_URI", "http://localhost:4200/oauth2/redirect"),
		GoogleClientID:     env("GOOGLE_CLIENT_ID", ""),
		GoogleClientSecret: env("GOOGLE_CLIENT_SECRET", ""),
		GitHubClientID:     env("GITHUB_CLIENT_ID", ""),
		GitHubClientSecret: env("GITHUB_CLIENT_SECRET", ""),
		AllowedOrigins:     origins,
		Email: EmailConfig{
			SMTPHost:            env("SMTP_HOST", "smtp.gmail.com"),
			SMTPPort:            envInt("SMTP_PORT", 587),
			SMTPUsername:        env("SMTP_USERNAME", "traintype@gmail.com"),
			SMTPPassword:        env("SMTP_PASSWORD", "yourAppPassword"),
			EmailRecipient:      env("EMAIL_RECIPIENT", "alex.nelson.bryan@gmail.com"),
			EmailSendingEnabled: envBool("EMAIL_SENDING_ENABLED", false),
		},
	}

	if strings.TrimSpace(cfg.JWTSecret) == "" {
		return Config{}, fmt.Errorf("JWT_SECRET must not be empty")
	}

	return cfg, nil
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
