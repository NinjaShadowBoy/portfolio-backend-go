.PHONY: fmt test build docker sqlc clean

fmt:
	gofmt -w cmd internal

sqlc:
	sqlc -f db/sqlc.yaml generate

test:
	go test ./...

build:
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o bin/portfolio-backend ./cmd/api

docker:
	docker build -t portfolio-backend-go .

clean:
	rm -rf bin
