package server

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/XSAM/otelsql"
	_ "github.com/lib/pq"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/adapter/oauth"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/adapter/openrouter"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/adapter/postgres"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/auth"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/config"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/email"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/knowledge"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/ports"
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
	// The knowledge index embeds through OpenRouter with the chat's key, so
	// it runs when the chat does; otherwise no embedder is built and the use
	// cases report it disabled. The corpus is always built: the chat's page,
	// publication and contact tools read it without an embedder.
	var embedder ports.Embedder
	corpus := knowledge.NewDir(cfg.Knowledge.Dir)
	if cfg.Knowledge.Enabled() {
		embedder = openrouter.NewEmbedder(openrouter.EmbedderConfig{
			APIKey:         cfg.Knowledge.APIKey,
			DocModel:       cfg.Knowledge.DocModel,
			QueryModel:     cfg.Knowledge.QueryModel,
			DataCollection: cfg.Chat.DataCollection,
		})
	}
	// A nil model disables chat; without CHAT_ENABLED nothing calls
	// OpenRouter.
	var chatModel ports.ChatModel
	if cfg.Chat.Enabled {
		chatModel = openrouter.New(openrouter.Config{
			APIKey:         cfg.Chat.OpenRouterAPIKey,
			Model:          cfg.Chat.Model,
			ProviderSort:   cfg.Chat.ProviderSort,
			DataCollection: cfg.Chat.DataCollection,
			Reasoning:      cfg.Chat.Reasoning,
			// The OAuth redirect URI is the frontend URL; its origin is
			// the site OpenRouter shows in the HTTP-Referer header.
			SiteURL: cfg.OAuthRedirectURI,
		})
	}
	svc := usecase.New(usecase.Dependencies{
		Repositories: store.Repositories(),
		UnitOfWork:   store,
		Tokens:       jwt,
		Passwords:    auth.Passwords{},
		OAuth: oauth.New(googleOAuth, githubOAuth, &http.Client{
			Timeout:   15 * time.Second,
			Transport: otelhttp.NewTransport(http.DefaultTransport),
		}),
		Emails:            emails,
		Embedder:          embedder,
		KnowledgeCorpus:   corpus,
		EmbeddingDocModel: cfg.Knowledge.DocModel,
		Chat: usecase.ChatSettings{
			Model:                  chatModel,
			DailyMessagesAnonymous: cfg.Chat.DailyMessagesAnonymous,
			DailyMessagesUser:      cfg.Chat.DailyMessagesUser,
			DailyTokenBudget:       cfg.Chat.DailyTokenBudget,
		},
	})
	handler, err := router.New(cfg, jwt, svc)
	if err != nil {
		_ = sqlDB.Close()
		return nil, err
	}

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
	// The sync runs beside the HTTP server and never delays it. It stops with
	// Run, and Run waits for it so Close never pulls the database from under
	// a running sync.
	syncCtx, stopSync := context.WithCancel(ctx)
	syncDone := closedChannel()
	if s.svc.KnowledgeEnabled() {
		syncDone = startKnowledgeSync(syncCtx, s.svc, s.cfg.Knowledge.SyncInterval)
	} else {
		slog.Info("knowledge index disabled: needs CHAT_ENABLED=true and OPENROUTER_API_KEY")
	}
	defer func() {
		stopSync()
		<-syncDone
	}()

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
