FROM golang:1.26-alpine AS builder

WORKDIR /src
RUN apk add --no-cache git
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/portfolio-backend ./cmd/api

FROM alpine:3.21

RUN addgroup -S app && adduser -S app -G app
WORKDIR /app
COPY --from=builder /out/portfolio-backend /app/portfolio-backend
COPY db /app/db
USER app
EXPOSE 8080
ENTRYPOINT ["/app/portfolio-backend"]
