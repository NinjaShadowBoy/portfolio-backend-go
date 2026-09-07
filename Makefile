.PHONY: fmt test build sqlc clean image signoz-forge

VERSION ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo dev)
IMAGE   ?= ghcr.io/ninjashadowboy/portfolio-backend-go

fmt:
	gofmt -w cmd internal

sqlc:
	sqlc -f db/sqlc.yaml generate

test:
	go test ./...

build:
	CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w -X main.version=$(VERSION)" -o bin/portfolio-backend ./cmd/api

# Local image build, same Dockerfile CI publishes from.
image:
	docker build --build-arg VERSION=$(VERSION) -t $(IMAGE):$(VERSION) .

# Regenerate the local SigNoz compose project from its casting.yaml (after a
# SigNoz upgrade). Needs foundryctl: https://signoz.io/docs/install/docker/
signoz-forge:
	cd deploy/local/signoz && foundryctl forge -f casting.yaml -p .forge && cp -r .forge/deployment/. . && rm -rf .forge

clean:
	rm -rf bin
