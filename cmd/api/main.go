package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ninjashadowboy/portfolio-backend-go/internal/config"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/server"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/telemetry"
)

// version is stamped by the Makefile (-X main.version=...) and reported as
// service.version on every span, metric and log record.
var version = "dev"

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	shutdownTelemetry, err := telemetry.Setup(ctx, telemetry.Config{
		ServiceName:    cfg.ServiceName,
		ServiceVersion: version,
		Environment:    cfg.AppEnv,
	})
	if err != nil {
		log.Fatalf("setup telemetry: %v", err)
	}
	defer func() {
		flushCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := shutdownTelemetry(flushCtx); err != nil {
			log.Printf("shutdown telemetry: %v", err)
		}
	}()

	srv, err := server.New(cfg)
	if err != nil {
		log.Fatalf("create server: %v", err)
	}
	defer srv.Close()

	log.Printf("listening on %s:%s (version %s)", cfg.BindAddr, cfg.Port, version)
	if err := srv.Run(ctx); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
