# CharityMatch

A volunteer and charity matching application built with Go and PostgreSQL.
Includes signup, login, database-backed sessions, and HTML pages. Event browsing
and matching are currently placeholders.

## Local development

Requires Go 1.26+ and PostgreSQL with `pgcrypto`, `vector` (pgvector), and `postgis`.

```sh
cp .env.example .env
# Edit .env with your database connection URL.
sh scripts/dev.sh migrate
sh scripts/dev.sh run
```

Open http://localhost:8080. The helper loads `.env`; direct `go run ./cmd/server`
requires exported environment variables. Set `AUTH_INSECURE_LOCAL_HTTP=true` only
for local HTTP development. Templates and static files are embedded in the binary;
restart after editing them.

## Deploy to Vultr

The scripts deploy to an existing Ubuntu server over SSH. They install Docker,
build the application on the server, apply database migrations, and start the app
behind Caddy with automatic HTTPS. PostgreSQL remains hosted on Tiger Data.

### 1. Prepare the server and domain

- Create a dedicated Ubuntu **24.04, 26.04, or 26.10** Vultr instance with your SSH
  public key. Use at least 2 GB RAM for building the image.
- Verify `ssh root@YOUR_SERVER_IP` works. Confirm the host key when first connecting.
- Point your domain's **A record** to the server IPv4 address. On Porkbun, leave
  the Host field blank for the root domain. Remove conflicting root A/ALIAS records.
  Only publish an AAAA record if IPv6 is configured for this server.
- Allow TCP **80 and 443** in both the Vultr firewall and the server firewall.
  Allow SSH from your IP. UDP 443 is optional. Leave port 8080 closed.
- Ports 80/443 must be available; the script does not change existing firewall rules.

If UFW is enabled, run on the server:

```sh
ufw allow 80/tcp
ufw allow 443/tcp
```

Your local machine needs Bash, Python 3, SSH, SCP, and tar. Local Docker and Go
are not required to deploy. Use `~/.ssh/config` to configure a custom SSH key or port.

### 2. Configure the database

Create a Tiger Data service, then copy the example:

```sh
cp deploy/production.env.example .env.production
chmod 600 .env.production
```

Edit `.env.production` with your service's host, port, and database password:

```env
DATABASE_URL=postgresql://tsdbadmin@YOUR_HOST:YOUR_PORT/tsdb?sslmode=require
DATABASE_PASSWORD='your actual database password'
```

`DATABASE_PASSWORD` is literal text; no URL encoding is needed. The deployment
helper encodes it automatically. If your URL already contains an encoded password,
omit `DATABASE_PASSWORD`. Keep this file out of Git.

Tiger Console's passwordless SQL editor does not authenticate this external app.
Use the database password from the service credentials or reset it in Tiger Data.
If a database IP allowlist is enabled, allow the Vultr server's outbound IP.

### 3. Deploy

From the repository root:

```sh
./deploy.sh root@YOUR_SERVER_IP yourdomain.com
```

The command uploads the source and database configuration, builds the image,
applies pending migrations, starts both containers, and checks HTTPS. On success,
it prints the site's URL. No separate `docker compose` command is needed.

To save your server settings locally:

```sh
cp deploy/settings.env.example .deploy.env
# Edit .deploy.env with your server and domain.
./deploy.sh
```

`.deploy.env` and `.env.production` are ignored by Git. Scripts have no built-in
server or domain defaults. You can also export `DEPLOY_TARGET` and `DEPLOY_DOMAIN`,
or pass the target and domain to `deploy.sh` directly. Set `DEPLOY_ENV_FILE` to use
a different credentials
file. Server files live in `/opt/charitymatch`; credentials have mode 600 and are
excluded from the image. Containers restart automatically after a reboot.

### Updates and operations

With `.deploy.env` configured (or `DEPLOY_TARGET` exported):

```sh
./deploy.sh          # Publish local changes, migrate, and start
./run.sh             # Start the existing deployment without rebuilding
./run.sh restart     # Recreate and restart containers
./run.sh status      # Show container status
./run.sh logs        # Show recent application and proxy logs
```

Migrations run in a transaction and are tracked with checksums. Repeat deployments
skip applied migrations. The original manually initialized schema can be adopted;
partial legacy schemas stop with a repair error. Updates may briefly interrupt
requests; there is no automatic rollback. Back up your database before schema changes.

### Troubleshooting

| Problem | Check |
| --- | --- |
| `password authentication failed` | Verify the service username/password in `.env.production`, then deploy again. Schema initialization cannot fix authentication failures. |
| Connection refused | Run `./run.sh status`, then `./run.sh`; confirm DNS and TCP 80/443 firewall rules. |
| HTTPS or certificate failure | Check DNS, conflicting AAAA records, ports 80/443, and `./run.sh logs`. |
| Migration failure | Read the database error and repair the schema; do not delete existing tables to retry. |

## Authentication and API

Browser pages: `/`, `/events`, `/login`, `/signup`, `/profile`, and `/matches`.
Profile and matches require a session. API clients receive JSON; errors use
`{"error":"message"}`. Browser forms submit JSON with JavaScript.

| Endpoint | Behavior |
| --- | --- |
| `GET /healthz` | Service health |
| `POST /signup` | Create an account and session; accepts `first_name`, `last_name`, `email`, `password` |
| `POST /login` | Start a session; accepts `email`, `password` |
| `POST /logout` | Revoke the session |
| `GET /profile` | Current user ID for API clients; HTML for browsers |

Passwords use bcrypt. Sessions expire after 24 hours; only token hashes are stored
in the database. Production cookies are Secure, HttpOnly, and SameSite=Lax.
Cross-origin unsafe browser requests are rejected. Password reset, email verification,
and deployment-level rate limiting are not implemented.

## Tests

```sh
go test ./...
go vet ./...
PYTHONDONTWRITEBYTECODE=1 python3 scripts/test_prepare_deploy_env.py
```

For database integration tests, set `TEST_DATABASE_URL` to a disposable PostgreSQL
database and run `go test ./internal/auth ./internal/database -v`. Tests create and remove
isolated schemas.

## Project structure

```text
cmd/
  server/             Server startup and database connection
  db/                 Database migration and check commands
internal/
  api/                JSON response helpers
  auth/               Accounts, passwords, and sessions
  database/           Migration runner and integration tests
  httpserver/         Routes and HTTP tests
migrations/           Embedded, versioned SQL migrations
web/
  templates/          Embedded HTML pages
  static/             Embedded CSS and JavaScript
scripts/              Local development and deployment credential helpers
deploy/               Docker Compose, Caddy, and production env example
```

`deploy.sh` publishes the application; `run.sh` manages the deployed containers.
Tests live beside the code they verify. Add new packages when functionality is
implemented, rather than creating empty placeholders.
