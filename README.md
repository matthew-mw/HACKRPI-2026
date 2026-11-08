# Volunteer Together — Do Good. Together.

A social volunteering application using the existing Go/PostgreSQL/PostGIS schema,
semantic HTML, CSS, HTMX, and vanilla JavaScript. All community data comes from
PostgreSQL. There are no sample people, events, registrations, feed posts, fake
rankings, AI match scores, or localStorage application records.

## Run locally

Requires Go 1.26+ and the existing PostgreSQL database with pgcrypto, pgvector,
and PostGIS installed.

```sh
cp .env.example .env
# Set DATABASE_URL in .env, then:
sh scripts/dev.sh migrate
sh scripts/dev.sh run
```

Open http://localhost:8080. For local HTTP only, set
`AUTH_INSECURE_LOCAL_HTTP=true`. Deployment always uses secure session cookies.
Templates and assets are embedded; restart/rebuild after edits. There is no
frontend build step. A static file server cannot provide database features.

## Actual product behavior

- Home loads shared activities and your upcoming registrations. Private activity
  is visible only to its author; friends-only means mutually followed accounts.
- Discover searches actual published future events by title, organization, city,
  cause, day, date/time range, and your saved travel radius. Results are ordered
  by upcoming time. There is no scoring or AI ranking.
- Registration locks the event row before checking capacity. Join/leave, saved
  events, kudos, comments, follows, invitations, and challenge membership persist
  in PostgreSQL and work across devices after signing in.
- My Crews uses `organizations` with `organization_type=volunteer_group` and
  `organization_members`. Group matching requires enough spaces plus a matching
  location, travel distance, weekday, timezone, and time window for every active
  member. Private crew schedules are visible to active members only.
- Create an organization or crew, then publish its events through Discover.
  Publishing is authorized for owners/admins. Event photographs are illustrative
  stock assets, clearly labeled in accessible descriptions, not uploaded event photos.
- Events can be public or restricted to active members of the hosting crew. My Events lists hosted and registered events, including past events awaiting organizer completion.
- Organizers explicitly mark an event done, confirm the attendance checklist and actual service period, then verified hours are logged for every selected participant in one transaction. Repeated completion requests cannot award hours twice. Prior individual submissions for that event are retained for audit and excluded from totals.
- The legacy individual service-log API remains available for existing integrations. Time must fall within
  the event, cannot be in the future, and cannot overlap an existing non-rejected log.
  Logged time starts pending. Event organizers can verify/reject other volunteers'
  service from My Impact; nobody can verify their own hours.
- My Impact, goals, badges, charts, and individual/crew/organization leaderboards
  derive from stored activity. Verified metrics exclude pending/rejected hours.
  Personal challenge targets are product definitions, not seeded community records.
- Profile edits, tags, availability, street addresses, travel distance, and privacy settings
  are saved to the database. Activity and hour visibility are independently private
  by default. Street addresses are resolved by the U.S. Census Geocoder, then stored in the existing locations address fields. Coordinates remain internal for PostGIS distance matching; the forms use addresses.
- Notifications contain actual in-app event and crew invitations. No email is sent.
- Empty databases show helpful empty states. Creating an account does not seed
  a volunteer history, friends, crews, events, or achievements.

## Schema

`001_init.sql` and `002_sessions.sql` remain unchanged. The additive migration
`003_social_product.sql` adds caption and privacy fields plus `activity_comments`,
`saved_events`, `event_invitations`, and `user_challenges`. It inserts no seed rows. `004_event_lifecycle.sql` adds per-event audience, creator and completion metadata, confirmed attendance, and a unique completion-credit index. It preserves the audience of existing restricted events.
The migration runner records checksums and applies changes transactionally.

## API

Browser pages: `/`, `/events`, `/matches`, `/profile`, `/login`, `/signup`.
Existing authentication routes are preserved. All writes require a session and
same-origin JSON requests. Errors use `{"error":"message"}`.

| Endpoint | Behavior |
| --- | --- |
| `POST /signup`, `/login`, `/logout` | Database-backed accounts and sessions |
| `GET /events` | Published upcoming events, earliest first |
| `GET /matches` | Same, filtered with ST_DWithin using the user's location and max_distance_km |
| `GET /api/community` | Public feed/crews; includes own dashboard for signed-in accounts |
| `GET /api/dashboard` | Authenticated profile, statistics, activity, registrations, crews, invitations |
| `GET /api/opportunities` | Searchable event cards with actual capacity, saved/registration state, distance, causes |
| `GET /api/crew?id=UUID` | Crew events; member availability and group matches for active members |
| `GET /api/people?q=NAME` | Registered people and follow relationship state |
| `GET /api/leaderboard?period=week|month|all&type=individuals|crews|organizations` | Verified service rankings, respecting visibility |
| `GET /api/review` | Pending service at events the current account can review |
| `POST /api/profile` | Profile, preferences, availability, privacy, optional chosen street address |
| `POST /api/organization`, `/api/event` | Create a crew/organization; publish its event |
| `POST /api/registration`, `/api/save` | Join/leave or save/unsave an event |
| `GET /api/my-events`, `/api/event-details?id=UUID` | Owned/attending event history and audience-checked event details |
| `GET /api/event-attendance?id=UUID` | Organizer-only attendance checklist |
| `POST /api/event-complete` | Confirm actual time/participants and atomically finalize hours |
| `POST /api/activity`, `/api/verify` | Log completed service; organizer verification |
| `POST /api/reaction`, `/api/comment` | Kudos and comments on visible activity |
| `POST /api/membership`, `/api/invite` | Crew membership and in-app invitations |
| `POST /api/follow`, `/api/challenge` | Follow people and join personal goals |

`/events` and `/matches` support `from`/`to` RFC3339 timestamps and `weekday`
(0 Sunday through 6 Saturday, America/New_York). `/matches` optionally accepts
`max_distance_km` to narrow, never expand, the stored profile radius.
`/api/opportunities` accepts `q`, `cause`, `from`, `to`, `weekday`, `near=true`,
and optional `distance`. Missing matching coordinates returns 422 with an instruction
to set location in Preferences. UI datetime controls use the browser's timezone.

## Verification

```sh
go test ./...
go vet ./...
node --check web/static/js/data.js
node --check web/static/js/api.js
node --check web/static/js/app.js
# Against an authorized test deployment with an existing test account:
python scripts/smoke_live.py https://YOUR_DOMAIN path/to/test-credentials.txt
```

For database tests set `TEST_DATABASE_URL` to a test PostgreSQL/PostGIS database.
`TestProductPostgres` creates a unique isolated schema, applies the actual migrations,
then exercises signup, profiles, publishing, capacity, search, membership, invitations,
group matching, comments, kudos, verification, and privacy. It drops only that schema.
The spatial tests use connection-local temporary tables. No fixture rows are inserted
into the application's production schema by the tests.

The frontend modules are `data.js` (product vocabulary/rules), `api.js` (HTTP client),
and `app.js` (shared UI). Page views live in `web/static/js/pages/` and are loaded only for the current URL. Page data uses `/api/community?page=...` so unrelated dashboard queries are not executed. No frontend framework or build dependency is needed. HTMX loads
the About disclosure from `/static/fragments/about.html` on the same server.

## Third-party assets

HTMX 2.0.8 is vendored with its Zero-Clause BSD license in
`web/static/js/htmx.LICENSE.txt`. Illustrative photos are bundled from Unsplash image
IDs `photo-1559027615-cd4628902d4a` and `photo-1542601906990-b4d3fb778b09`. Runtime
requests do not depend on a CDN. These images do not represent particular events.

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

Address lookup uses the official Census locations/onelineaddress API with Public_AR_Current. Unmatched or ambiguous addresses return 422; service failures return 503. Saving preferences without changing an address does not perform another lookup. No additional migration or API key is needed.


## Onboarding and charity approval

Signup opens location, causes, availability, and suggested opportunities in order.
Volunteers can use a city/state or ZIP without supplying a street address or location ID.
City/ZIP search uses OpenStreetMap Nominatim; street addresses for event venues retain Census lookup.
Migration `005_onboarding_charities.sql` adds charity approval and site administrator roles.
New and existing charities need approval before publishing additional events. Active members of
approved charities can publish; clubs and companies require an owner or organization admin.
A site administrator sees pending charities in the Charities tab and can approve them.
Provision the initial trusted site administrator through your database administration console:
`UPDATE users SET is_site_admin=true WHERE lower(email)=lower('YOUR_ADMIN_EMAIL');`
This role is never set by signup or profile editing.
