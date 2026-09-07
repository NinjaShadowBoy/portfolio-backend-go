package server

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/XSAM/otelsql"
	_ "github.com/lib/pq"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/adapter/oauth"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/adapter/postgres"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/auth"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/config"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/email"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/router"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/usecase"
	"github.com/pressly/goose/v3"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"
)

type Server interface {
	Run(ctx context.Context) error
	Close() error
}

type server struct {
	cfg    config.Config
	db     *sql.DB
	jwt    *auth.JWT
	emails email.Sender
	svc    usecase.Service
	http   *http.Server
}

func New(cfg config.Config) (Server, error) {
	// otelsql wraps the pq driver so every query becomes a child span of the
	// request that issued it. Without a global tracer provider it is inert.
	sqlDB, err := otelsql.Open("postgres", cfg.DBURL,
		otelsql.WithAttributes(semconv.DBSystemNamePostgreSQL),
		otelsql.WithSpanOptions(otelsql.SpanOptions{OmitConnResetSession: true, OmitConnPrepare: true}),
	)
	if err != nil {
		return nil, err
	}
	sqlDB.SetMaxOpenConns(10)
	sqlDB.SetMaxIdleConns(5)
	sqlDB.SetConnMaxLifetime(30 * time.Minute)
	if _, err := otelsql.RegisterDBStatsMetrics(sqlDB, otelsql.WithAttributes(semconv.DBSystemNamePostgreSQL)); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("register db metrics: %w", err)
	}

	if err := goose.Up(sqlDB, cfg.MigrationsPath); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("run migrations: %w", err)
	}

	if cfg.SeedEnabled {
		if err := runSeeds(context.Background(), sqlDB, cfg.SeedsPath); err != nil {
			_ = sqlDB.Close()
			return nil, fmt.Errorf("run local seeds: %w", err)
		}
	}

	store := postgres.New(sqlDB)
	jwt := auth.NewJWT(cfg.JWTSecret, cfg.JWTExpiration, cfg.JWTIssuer)
	emails := email.NewSender(cfg.Email)
	googleOAuth := oauth.NewGoogleOAuthConfig(cfg.GoogleClientID, cfg.GoogleClientSecret, cfg.OAuthBaseURL)
	githubOAuth := oauth.NewGitHubOAuthConfig(cfg.GitHubClientID, cfg.GitHubClientSecret, cfg.OAuthBaseURL)
	svc := usecase.New(usecase.Dependencies{
		Repositories: store.Repositories(),
		UnitOfWork:   store,
		Tokens:       jwt,
		Passwords:    auth.Passwords{},
		OAuth: oauth.New(googleOAuth, githubOAuth, &http.Client{
			Timeout:   15 * time.Second,
			Transport: otelhttp.NewTransport(http.DefaultTransport),
		}),
		Emails: emails,
	})
	handler := router.New(cfg, jwt, svc)

	return &server{
		cfg:    cfg,
		db:     sqlDB,
		jwt:    jwt,
		emails: emails,
		svc:    svc,
		http: &http.Server{
			Addr:              net.JoinHostPort(cfg.BindAddr, cfg.Port),
			Handler:           handler,
			ReadHeaderTimeout: 10 * time.Second,
			ReadTimeout:       30 * time.Second,
			WriteTimeout:      30 * time.Second,
			IdleTimeout:       120 * time.Second,
		},
	}, nil
}

func (s *server) Run(ctx context.Context) error {
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = s.http.Shutdown(shutdownCtx)
	}()

	err := s.http.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func (s *server) Close() error {
	return s.db.Close()
}
