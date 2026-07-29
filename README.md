# Portfolio Backend Go

This folder contains a Go rewrite of the portfolio backend, kept separate from the existing Kotlin/Spring Boot backend.

## Stack

- Go
- `github.com/go-chi/chi/v5`
- `sqlc`
- PostgreSQL
- `golang-migrate` via Goose
- JWT auth
- Google and GitHub OAuth2 login
- SMTP email notifications for contact form submissions

## Setup

Copy the example environment file:

```bash
cp .env.example .env
```

The app loads `.env` automatically on startup. Real environment variables override values from `.env`.

Set at least these values:

```bash
APP_ENV=local
DB_URL=postgres://postgres:123456789@localhost:5432/portfolio?sslmode=disable
JWT_SECRET=change-me
SMTP_USERNAME=your-smtp-username
SMTP_PASSWORD=your-smtp-password
EMAIL_RECIPIENT=your-recipient@example.com
EMAIL_SENDING_ENABLED=true
```

For OAuth2, configure the callback base URL and provider credentials:

```bash
OAUTH_BASE_URL=http://localhost:8081/portfolio
OAUTH2_REDIRECT_URI=http://localhost:4200/oauth2/redirect
GOOGLE_CLIENT_ID=your-google-client-id
GOOGLE_CLIENT_SECRET=your-google-client-secret
GITHUB_CLIENT_ID=your-github-client-id
GITHUB_CLIENT_SECRET=your-github-client-secret
```

Register these authorized redirect URIs:

```text
http://localhost:8081/portfolio/auth/google/callback
http://localhost:8081/portfolio/auth/github/callback
```

Local database seeding is enabled when `APP_ENV` is `local`, `dev`, or `development` and `SEED_ENABLED=true`. Seeding is skipped for any other `APP_ENV`, including production.

## Run

```bash
go run ./cmd/api
```

On startup, migrations run first. If `APP_ENV` is `local`, `dev`, or `development`, `SEED_ENABLED=true`, and `SEEDS_PATH` points to SQL files, the server runs those seed files automatically before accepting requests.

The API is served under:

```text
/portfolio/api/v1
```

Health endpoint:

```text
GET /portfolio/cron
```

OAuth2 flows:

```text
GET /portfolio/auth/google
GET /portfolio/auth/google/callback
GET /portfolio/auth/github
GET /portfolio/auth/github/callback
```

Each callback redirects to `OAUTH2_REDIRECT_URI` with either `token=<jwt>` or `error=<message>`.

## Generate sqlc code

```bash
sqlc -f db/sqlc.yaml generate
```

## Test

```bash
go test ./...
```

## Build

```bash
go build -o bin/portfolio-backend ./cmd/api
```

## Docker

```bash
docker build -t portfolio-backend-go .
docker run -p 8081:8081 \
  -e DB_URL=postgres://postgres:123456789@host.docker.internal:5432/portfolio?sslmode=disable \
  -e JWT_SECRET=change-me \
  -e OAUTH_BASE_URL=https://your-domain.example/portfolio \
  -e OAUTH2_REDIRECT_URI=https://your-domain.example/oauth2/redirect \
  -e GOOGLE_CLIENT_ID=your-google-client-id \
  -e GOOGLE_CLIENT_SECRET=your-google-client-secret \
  -e GITHUB_CLIENT_ID=your-github-client-id \
  -e GITHUB_CLIENT_SECRET=your-github-client-secret \
  portfolio-backend-go
```

## Notes

- Existing plaintext passwords are preserved for compatibility.
- OAuth2 uses PKCE, provider-specific state cookies, and redirects the JWT back to `OAUTH2_REDIRECT_URI`.
- Cloudinary uploads remain client-side. The backend only stores the uploaded photo URL.
- Contact email failures are logged as non-fatal so submitted messages are still saved.