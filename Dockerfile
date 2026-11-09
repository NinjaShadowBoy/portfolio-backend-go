# syntax=docker/dockerfile:1.7

# Build stage: same flags as `make build`, so the image and a bare-metal
# binary are byte-for-byte the same program. VERSION is stamped into
# main.version and shows up as service.version in SigNoz.
FROM golang:1.27-alpine AS build
WORKDIR /src
ARG VERSION=dev

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=bind,source=go.mod,target=go.mod \
    --mount=type=bind,source=go.sum,target=go.sum \
    go mod download

COPY . .
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build -trimpath \
      -ldflags="-s -w -X main.version=${VERSION}" \
      -o /out/portfolio-backend ./cmd/api

# Runtime stage. Alpine rather than scratch so the compose healthcheck has
# wget and TLS roots exist for the OAuth and SMTP calls.
FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata wget \
 && addgroup -S app && adduser -S -G app app
WORKDIR /app

COPY --from=build /out/portfolio-backend /app/portfolio-backend
# Migrations and seeds ship with the binary they were built against, so a
# rollback to an older tag can never pair code with the wrong schema.
COPY --chown=app:app db /app/db
# The assistant's knowledge corpus ships the same way, next to the
# migrations, so an image always indexes the text it was built with.
COPY --chown=app:app knowledge /app/knowledge

USER app
ENV BIND_ADDR=0.0.0.0 \
    PORT=8081 \
    GIN_MODE=release \
    MIGRATIONS_PATH=db/migrations \
    SEEDS_PATH=db/seeds \
    KNOWLEDGE_DIR=knowledge
EXPOSE 8081

HEALTHCHECK --interval=15s --timeout=3s --start-period=20s --retries=3 \
  CMD wget -qO- "http://127.0.0.1:${PORT}/portfolio/v1/cron" >/dev/null || exit 1

ENTRYPOINT ["/app/portfolio-backend"]
