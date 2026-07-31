.PHONY: fmt test build sqlc clean

fmt:
	gofmt -w cmd internal

sqlc:
	sqlc -f db/sqlc.yaml generate

test:
	go test ./...

build:
	CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o bin/portfolio-backend ./cmd/api

clean:
	rm -rf bin
