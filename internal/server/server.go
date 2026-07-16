package server

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"time"

	_ "github.com/lib/pq"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/auth"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/config"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/db"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/email"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/router"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/service"
	"github.com/pressly/goose/v3"
)

type Server interface {
	Run(ctx context.Context) error
	Close() error
}

type server struct {
	cfg     config.Config
	db      *sql.DB
	queries *db.Queries
	jwt     *auth.JWT
	emails  email.Sender
	svc     service.Service
	http    *http.Server
}

func New(cfg config.Config) (Server, error) {
	sqlDB, err := sql.Open("postgres", cfg.DBURL)
	if err != nil {
		return nil, err
	}
	sqlDB.SetMaxOpenConns(10)
	sqlDB.SetMaxIdleConns(5)
	sqlDB.SetConnMaxLifetime(30 * time.Minute)

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

	queries := db.New(sqlDB)
	jwt := auth.NewJWT(cfg.JWTSecret, cfg.JWTExpiration, cfg.JWTIssuer)
	emails := email.NewSender(cfg.Email)
	googleOAuth := service.NewGoogleOAuthConfig(cfg.GoogleClientID, cfg.GoogleClientSecret, cfg.OAuthBaseURL)
	githubOAuth := service.NewGitHubOAuthConfig(cfg.GitHubClientID, cfg.GitHubClientSecret, cfg.OAuthBaseURL)
	svc := service.New(queries, jwt, emails, googleOAuth, githubOAuth)
	handler := router.New(cfg, jwt, svc)

	return &server{
		cfg:     cfg,
		db:      sqlDB,
		queries: queries,
		jwt:     jwt,
		emails:  emails,
		svc:     svc,
		http: &http.Server{
			Addr:              ":" + cfg.Port,
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
