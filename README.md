# CharityMatch / HACKRPI-2026

Go 1.26+ with PostgreSQL (the initial schema also requires pgvector and PostGIS).

## Run

### Tiger Data / Tiger Cloud setup

1. [Create a Tiger Cloud service](https://www.tigerdata.com/docs/get-started/quickstart/create-service)
   in your account and wait for it to show **Ready**. Save the downloaded credentials.
   Choose a region near your app and review the selected service's cost before creating it.
2. Copy its PostgreSQL connection URL from the service's connection details into
   your local `.env` as `DATABASE_URL='postgresql://…?sslmode=require'`.
   Use the supplied host, port, username, and database name (usually `tsdb`),
   and URL-encode special characters in the password. See `.env.example` for the format.
   Keep TLS enabled for Tiger Cloud. `.env` is local and must not be committed.
3. Check connectivity and available extensions, then initialize a **new database**:

   ```sh
   sh scripts/dev.sh check
   sh scripts/dev.sh init-db
   ```

   Initialization enables `pgcrypto`, `vector` (pgvector), and `postgis`, and applies
   both migrations in one transaction. It is a one-time operation, not a migration
   tracker: do not rerun it against an existing installation. No hypertables are
   needed for the current relational schema.
4. For local HTTP login, add `AUTH_INSECURE_LOCAL_HTTP=true` to `.env`, then start:

   ```sh
   sh scripts/dev.sh run
   # In another terminal:
   curl http://localhost:8080/healthz
   ```

The helper loads the project's trusted `.env`; direct `go run` still requires
exported environment variables. If you prefer Tiger Console's SQL editor, execute
the two migration files in order there instead of using the Go setup command. No local `psql` installation is required.

### Manual setup

Apply `migrations/001_init.sql` to a new database, then apply
`migrations/002_sessions.sql`. Existing installations only need migration 002.
Export `DATABASE_URL` and optionally `PORT` (default `8080`), then run:

```sh
go run ./cmd/server
```

The server reads exported environment variables; it does not load `.env` files.
Use HTTPS in production, normally through a reverse proxy that preserves the
public Host header. Cookies are Secure, HttpOnly, SameSite=Lax, host-only, and
named `__Host-session`. For **local HTTP development only**, export
`AUTH_INSECURE_LOCAL_HTTP=true`; this uses a non-Secure cookie named `session`.
Never enable that flag on a public deployment.

## Authentication

Browsers requesting `text/html` receive the bundled HTML pages at `/`, `/events`,
`/profile`, and `/matches`. Profile and matches require a session. Public assets
are served under `/static/`. Templates and assets are embedded at build time, so
restart `go run` after editing them. Login and signup pages are available at `/login` and `/signup`.
Signup accepts first name, last name, email, and a password of at least 12 characters
(up to 72 bytes), stores a bcrypt hash, and starts a session. Browser forms use
JavaScript to submit JSON; successful authentication opens `/profile`.
Unauthenticated browser visits to profile and matches redirect to login.

Clients without `Accept: text/html` continue to receive the JSON API responses
listed below. API response bodies use `application/json`. Errors have the shape
`{"error":"message"}`. HEAD responses omit the body as required by HTTP.

| Route | Access | Response |
| --- | --- | --- |
| GET `/` | Public | Service metadata |
| GET `/healthz` | Public | `{"status":"ok"}` |
| GET `/events` | Public | 501 JSON placeholder pending implementation |
| GET `/login`, `/signup` | Public | HTML authentication forms |
| POST `/signup` | Public | Creates account and session (201) |
| POST `/login` | Public | `{"user_id":"…"}` and session cookie |
| GET `/profile` | Session | Current `user_id` |
| GET `/matches` | Session | 501 JSON placeholder pending implementation |
| POST `/logout` | Session | `{"ok":true}` and cleared session cookie |

GET routes also support HEAD. All other requests require a valid session by
default. Configure the explicit method/path allowlist in `auth.PublicRequest`.
Missing authentication returns 401 JSON, without redirects or HTMX-specific
behavior. Authenticated unknown routes return 404; unsupported methods return
405 with an Allow header. Cross-origin unsafe browser requests return 403 JSON.

POST `/login` requires `Content-Type: application/json` and a single object:

```json
{"email":"user@example.com","password":"your-password"}
```

Malformed bodies, unknown fields, and missing fields return 400; unsupported
content types return 415; invalid credentials return 401. The body limit is 8 KiB.
Login uses existing users whose `users.password_hash` contains a bcrypt hash
produced with `bcrypt.GenerateFromPassword` (default cost or higher). Accounts
with NULL password hashes cannot use password login. Password reset and email verification are not implemented. Login limits concurrent password checks;
deployment-level rate limiting is still needed to limit repeated attempts.
Browser clients should send the session cookie with requests (use
`credentials: "include"` where appropriate). Cross-origin frontend hosting
requires an explicit CORS and cookie policy; it is not enabled by default.

A successful login creates a fresh random 256-bit token; only its SHA-256 hash
is stored in Postgres. Sessions have a fixed 24-hour expiry. Login replaces the
browser's old session; POST `/logout` deletes the current session and clears its
cookie. `PostgresStore.DeleteUserSessions` is available for future password reset
and sign-out-all flows. Expired records are cleaned hourly. Database errors fail
closed with 503. Auth middleware supplies the user ID through `auth.UserID(ctx)`;
handlers must separately enforce organization roles and resource ownership.

Go's `http.CrossOriginProtection` wraps all routes and rejects cross-origin
unsafe browser requests, including login/logout. GET and HEAD handlers must
remain read-only. Non-browser clients without Origin/Fetch Metadata headers
are accepted by this protection but still require authentication on private
routes.

## Verify

```sh
go test ./...
go vet ./...
# Optional: run against a disposable PostgreSQL database, creating/dropping a test schema.
TEST_DATABASE_URL='postgres://…' go test ./internal/auth -run TestPostgres -v
```

Tests cover public/private route access, login/logout, CSRF rejection, JSON request/response validation, token hashing, cookie flags, rotation, expiry, revocation, and store
errors. The PostgreSQL integration test applies migration 002 and verifies real
password authentication and session queries with a minimal users fixture.
