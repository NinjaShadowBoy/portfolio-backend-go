# Portfolio Backend Go

The JSON API behind my portfolio site. It serves the projects the site lists,
takes the feedback and contact messages readers send, and handles who they are
when they send them. Originally a Kotlin/Spring Boot service, rewritten in Go
and kept as its own repository.

What it does:

- **Projects.** Public reads, authenticated writes, plus the photo records
  (Cloudinary does the uploading client-side; this only stores the URL).
- **Feedback.** A typed note (`critique`, `suggestion` or `feature_request`)
  attached to either a project or an article slug, never both, enforced by an
  XOR check constraint. Articles have no database row of their own: they are
  files in the frontend repo, addressed by the slug the build assigns them.
- **Identity without an account.** Feedback needs no login. `EnsureIdentity`
  resolves a bearer JWT, and failing that mints an `anon_token` cookie backed by
  an auto-provisioned anonymous user. Only the SHA-256 hash of the token is
  stored, the cookie lasts a year, and logging in later claims everything that
  anonymous user wrote.
- **Auth.** Email and password, Google and GitHub OAuth2 over PKCE with
  per-provider state cookies, and the GitHub device flow. Sessions are JWTs.
- **Contact.** Submissions are stored and emailed over SMTP. A send failure is
  logged and non-fatal, so the message is never lost to a bad mail server.

The frontend that consumes it is
[portfolio-app](https://github.com/NinjaShadowBoy/portfolio-app).

## Stack

- Go 1.26
- `github.com/gin-gonic/gin`
- `sqlc`
- PostgreSQL
- Goose migrations (`github.com/pressly/goose/v3`)
- JWT auth
- Google and GitHub OAuth2 login
- SMTP email notifications for contact form submissions

## Quick start

You need Go 1.26 and either Docker or a local PostgreSQL.

```bash
git clone https://github.com/NinjaShadowBoy/portfolio-backend-go.git
cd portfolio-backend-go
docker compose up -d db          # PostgreSQL on localhost:5432, password 123456789
cp .env.example .env             # then set DB_URL to match (see below)
go run ./cmd/api                 # API on http://localhost:8081/portfolio/v1
```

Migrations run on startup, so there is no separate database step.

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
OAUTH_BASE_URL=http://localhost:8081/portfolio/v1
OAUTH2_REDIRECT_URI=http://localhost:4200/oauth2/redirect
GOOGLE_CLIENT_ID=your-google-client-id
GOOGLE_CLIENT_SECRET=your-google-client-secret
GITHUB_CLIENT_ID=your-github-client-id
GITHUB_CLIENT_SECRET=your-github-client-secret
```

Register these authorized redirect URIs:

```text
http://localhost:8081/portfolio/v1/auth/google/callback
http://localhost:8081/portfolio/v1/auth/github/callback
```

Local database seeding is enabled when `APP_ENV` is `local`, `dev`, or `development` and `SEED_ENABLED=true`. Seeding is skipped for any other `APP_ENV`, including production.

## Run

```bash
go run ./cmd/api
```

On startup, migrations run first. If `APP_ENV` is `local`, `dev`, or `development`, `SEED_ENABLED=true`, and `SEEDS_PATH` points to SQL files, the server runs those seed files automatically before accepting requests.

The API is served under:

```text
/portfolio/v1
```

That path is `config.APIPrefix + "/" + config.APIVersion` (`internal/config/config.go`). Bumping `APIVersion` means adding a new route group and updating the frontend's environment files, which hardcode the same string.

Health endpoint:

```text
GET /portfolio/v1/cron
```

OAuth2 flows:

```text
GET /portfolio/v1/auth/google
GET /portfolio/v1/auth/google/callback
GET /portfolio/v1/auth/github
GET /portfolio/v1/auth/github/callback
```

Each callback redirects to `OAUTH2_REDIRECT_URI` with either `token=<jwt>` or `error=<message>`.

## Generate sqlc code

`db/queries/*.sql` is the source of truth for `internal/db/*.sql.go`. Edit the SQL, then:

```bash
make sqlc          # sqlc -f db/sqlc.yaml generate
```

Never hand-edit the generated files. Schema changes are numbered goose migrations in `db/migrations/`.

## Test

```bash
make test          # go test ./...
go vet ./...       # CI gate
make fmt           # gofmt -w cmd internal
```

## Build

```bash
make build         # static linux binary -> bin/portfolio-backend
```

Deployment is over SSH to a VPS, under a templated systemd user unit
(`portfolio-backend@<env>.service`). See [deploy/DEPLOY.md](deploy/DEPLOY.md).

## Notes

- Existing plaintext passwords are preserved for compatibility.
- OAuth2 uses PKCE, provider-specific state cookies, and redirects the JWT back to `OAUTH2_REDIRECT_URI`.
- Cloudinary uploads remain client-side. The backend only stores the uploaded photo URL.
- Contact email failures are logged as non-fatal so submitted messages are still saved.
- `binding.EnableDecoderDisallowUnknownFields = true` (`internal/router/router.go`): an unexpected field in a request body is a 400, so frontend DTOs must match the Go structs exactly.
