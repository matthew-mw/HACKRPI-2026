# Volunteer Together API

This reference describes the current Go handlers, including authentication,
browser routes, and all `/api/*` endpoints. Paths are relative to the application
origin (locally `http://localhost:8080`). There is no version prefix or bearer-token API.

## How a request works

1. `cmd/server/main.go` opens PostgreSQL through pgx, checks connectivity, and
   constructs the session manager and HTTP router. Apply migrations separately
   before starting the server.
2. Cross-origin protection rejects unsafe browser requests from other origins.
3. Session middleware reads the cookie, looks up its SHA-256 hash in PostgreSQL,
   checks expiration, and puts the authenticated user ID in the request context.
   Routes are private unless explicitly allowlisted.
4. The router serves embedded HTML/assets or dispatches JSON requests. Product
   handlers validate inputs, enforce authorization, and execute parameterized SQL.
   Multi-step changes such as profile replacement, registration, publishing, and
   event completion use transactions.
5. SQL builds most JSON read models directly. The browser's
   `web/static/js/api.js` sends same-origin credentials and parses these responses;
   `app.js` renders them. There is no separate frontend API server.

Product requests have a 10-second context deadline; legacy event searches use
5 seconds. Geocoder HTTP requests have a 7-second timeout. All session-middleware
responses use `Cache-Control: no-store`. `/healthz` is a process health response,
not a fresh database connectivity test.

## Common conventions

- Send `Accept: application/json`, especially for routes that also serve HTML.
- POST bodies use `Content-Type: application/json`, except `/logout`, which does
  not inspect a body. Unknown JSON fields and trailing JSON values are rejected.
  Login/signup bodies are limited to 8,192 bytes; product bodies to 32,768 bytes.
- IDs are UUID strings. Times are RFC3339 strings with a timezone, such as
  `2026-12-05T10:00:00-05:00`. Numeric values and booleans are JSON numbers and
  booleans, not strings. Go string-length checks generally count UTF-8 bytes.
- Every GET route also accepts HEAD. HEAD returns headers without a response body.
- Successful product requests return **200**, including creation. Most writes
  return `{"ok":true}`; exceptions are documented below. Signup returns **201**.
- List responses are arrays, generally `[]` when empty. Optional database values
  can be `null`. There are no pagination cursors or total-count envelopes.
- Boolean action fields default to `false` when omitted: explicitly send them
  to avoid unintentionally leaving, unsaving, or unfollowing.
- Unsupported `/api/*` methods return 405 with `Allow: GET, HEAD, POST`.
  A known product path called with the wrong GET/POST method returns 404 because
  reads and writes have separate dispatch tables. Authentication can reject a
  request before route or method lookup.

Errors have the shape `{"error":"message"}`:

| Status | Meaning |
| --- | --- |
| 400 | Invalid fields, UUID, dates, JSON, or database constraint/reference |
| 401 | Missing/expired session, invalid credentials, or anonymous nearby search |
| 403 | Cross-origin rejection or insufficient permission |
| 404 | Missing route/record, or a record hidden by visibility rules |
| 405 | Unsupported method |
| 409 | Duplicate/conflicting state, capacity reached, or invalid lifecycle transition |
| 415 | Required JSON content type missing or incorrect |
| 422 | Matching location missing or street address ambiguous/unmatched |
| 429 | All four per-process login/signup bcrypt slots occupied; `Retry-After: 1` |
| 503 | Database, authentication, or upstream lookup unavailable |

## Authentication

Signup and login set an opaque 24-hour cookie and rotate the current browser
session. Only its hash is stored server-side. Production uses `__Host-session`
with `Secure`, `HttpOnly`, `SameSite=Lax`, and `Path=/`. Explicit local development
with `AUTH_INSECURE_LOCAL_HTTP=true` uses a non-Secure cookie named `session`.
There is no refresh endpoint; sign in again after expiration. Expired sessions
are cleaned up hourly. Browser writes must pass same-origin protection; there
is no separate CSRF-token parameter.

### POST /signup — public

```json
{"first_name":"Alex","last_name":"Rivera","email":"alex@example.com","password":"a long example phrase"}
```

All fields are required. Trimmed first/last names must be 1–100 bytes each.
Email must be a valid address up to 254 bytes and is trimmed and lowercased.
Password must contain at least 12 Unicode characters and at most 72 bytes.
Passwords are bcrypt-hashed. Returns **201** `{"user_id":"UUID"}` and a cookie.
Duplicate email returns 409. If account creation succeeds but session creation
fails, 503 explicitly asks the client to sign in; the account already exists.

### POST /login — public

Body: `{"email":"alex@example.com","password":"a long example phrase"}`.
Email and password must be nonempty, at most 254 and 72 bytes respectively.
Email matching is case-insensitive. Returns **200** `{"user_id":"UUID"}` and
a cookie. Wrong email/password returns 401 with a generic credentials error.

### POST /logout — session required

No body required. Deletes the current session and clears the cookie. Returns
`{"ok":true}`. Other browser sessions are not revoked.

## Browser, health, and legacy JSON routes

| GET path | Access | JSON response / behavior |
| --- | --- | --- |
| `/` | Public | `{"service":"Volunteer Together"}` |
| `/healthz` | Public | `{"status":"ok"}` |
| `/events` | Public | Array of upcoming public events, described below |
| `/matches` | Session | Upcoming events within the saved location/radius |
| `/profile` | Session | `{"user_id":"UUID"}`; full profile is in dashboard |
| `/login` | Public | Always serves the login HTML page |
| `/signup` | Public | Always serves the signup HTML page |
| `/static/*` | Public | Embedded static file; missing files/directories return 404 |

`/`, `/events`, `/matches`, and `/profile` serve HTML when `Accept` contains
`text/html`, and JSON otherwise. Anonymous HTML requests to `/matches` and
`/profile` redirect to `/login` with 303; JSON requests return 401.

### GET /events and GET /matches

Optional query parameters: `from`, `to` (RFC3339; `to > from`), `weekday`
(0 Sunday–6 Saturday in America/New_York), and `max_distance_km` (integer 1–500).
`from` bounds the event start inclusively; `to` bounds its end inclusively.

Both return published, uncompleted events starting now or later, sorted by
`starts_at`, then `id`, with no explicit result limit. Each row contains:
`id`, `organization_id`, `location_id`, `title`, `description`, `starts_at`,
`ends_at`, `capacity`, `status`, `organization_name`, `location_label`, `city`,
`address`, `distance_km`.

`/events` deliberately passes no user to its query even when signed in: only
public events appear, `distance_km` is null, and the distance filter is validated
but unused. `/matches` includes audience-accessible events and uses PostGIS
`ST_DWithin` with the lesser of the saved radius and requested radius. Missing
profile location returns 422. These are hard geographic/time filters, not AI scores.

## Read endpoints

### GET /api/community — public; GET /api/dashboard — session

Both use the same aggregate response builder. Anonymous community responses
contain `feed` and `crews`. With a valid session, **both** endpoints also include
all personalized keys below. There are no query parameters.

| Key | Response shape and meaning |
| --- | --- |
| `feed` | Up to 50 visible non-rejected activities, newest creation first; Activity rows below |
| `crews` | Visible volunteer groups: `id`, `name`, `description`, `owner_id`, `visibility`, `joined`, `membership_status`, `members`, `verified_hours` |
| `profile` | `id`, `first_name`, `last_name`, `email`, `bio`, `max_distance_km`, `activity_visibility`, `hours_visibility`, `is_site_admin`, `volunteer_with_friends`, `city`, `location_label`, `address`, `latitude`, `longitude`, `tags`, `availability`, `challenges` |
| `stats` | `hours`, `verified_hours`, `events`, `organizations`, `causes`, `active_weeks`, `monthly: [{month,hours}]`, `by_cause: [{cause,hours}]` |
| `upcoming` | Future published, uncompleted registered/waitlisted events: `id`, `title`, `starts_at`, `ends_at`, `city`, `location_label`, `address`, `organization_name`, `status` (registration status) |
| `invitations` | `{events:[{id,title,invited_by}],crews:[{id,name}]}`; event invitations exclude already-registered and past/completed events |
| `my_activity` | Same Activity shape, restricted to own non-rejected activity, maximum 50 |
| `registrations` | Active registrations, newest event start first: `id`, `title`, `starts_at`, `ends_at`, `status` (event status), `registration_status` |
| `organizations` | Publishable organizations: `{id,name}`; includes approved charities where the user is an active member |
| `my_events` | Same rows as `/api/my-events` |
| `charities` | `{id,name,description,approval_status,joined}`; approved charities plus own charities, or all charities for site admins |

Activity rows contain `id`, `user_id`, `name`, `caption`, `verification_status`,
`created_at`, `event_id`, `title`, `organization_name`, `city`, `hours`, `kudos`,
`liked`, `causes: string[]`, and
`comments: [{id,name,body,created_at}]` in creation order. Hidden hours are null.

Profile `tags` contain `{name,category}`; `availability` contains
`{weekday,start_time,end_time,timezone}`; `challenges` is an array of keys.
The signed-in profile response includes coordinates, even though the UI uses
address/area inputs. Availability read times may include seconds; write HH:MM.

**Metric semantics:** `hours`, monthly/cause totals, and other general statistics
include pending and verified activity, excluding rejected activity.
`verified_hours` and leaderboards include only verified activity. Multi-cause
events contribute their full hours to each cause, so cause totals can overlap.

### GET /api/location — session

Required query: `q`, a trimmed city/state or ZIP string of 2–120 bytes.
Returns `{city,label,latitude,longitude}`; `city` is the submitted query and
`label` is the provider's display name. Searches U.S. areas through Nominatim,
uses the first result, caches results in-process, and spaces uncached upstream
requests at least one second apart. No match returns 400; provider failure 503.
This resolves coordinates but does not save a profile location.

### GET /api/opportunities — public

| Optional query | Behavior |
| --- | --- |
| `q` | Trimmed case-insensitive SQL substring search across title, organization, city, venue label, and address |
| `cause` | Exact cause-tag name |
| `from`, `to` | RFC3339, start >= from and end <= to; to must follow from |
| `weekday` | 0–6 using America/New_York |
| `near` | Only literal `true` enables nearby filtering; requires session and saved location |
| `distance` | Integer 1–500 km; narrows saved radius only when near=true |

Returns at most 200 published, uncompleted, audience-accessible future events,
ordered by start and ID. Rows contain `id`, `title`, `description`, `starts_at`,
`ends_at`, `capacity`, `visibility`, `organization_id`, `organization_name`,
`location_label`, `city`, `address`, `distance_km` (rounded to one decimal, or null),
`spots`, `joined`, `saved`, `friends`, and `causes: string[]`.
`spots` is capacity minus registered users, clamped to zero. `friends` counts
registered people the viewer follows; it does not require mutual following.
Nearby anonymous requests return 401; missing saved location returns 422.

### GET /api/my-events — session

No parameters. Returns managed events or any event with a registration record
for the user (including cancelled registrations), newest start first.
Rows: `id`, `title`, `description`, `starts_at`, `ends_at`, `visibility`, `status`,
`completed_at`, `actual_started_at`, `actual_ended_at`, `organization_name`,
`location_label`, `city`, `address`, `can_manage`, `joined`, `registrations`,
`participants`. Counts distinguish registered users from confirmed participants.

### GET /api/event-details — public

Required query: `id=UUID`. Returns `id`, `title`, `description`, `starts_at`,
`ends_at`, `status`, `visibility`, `completed_at`, `organization_name`,
`location_label`, `city`, `address`, `can_manage`, `joined`, `spots`, `causes`.
Only published events or events the viewer manages qualify, and audience checks
still apply. Missing/inaccessible events return 404. Past/completed events can
still be read; this is not restricted to upcoming opportunities.

### GET /api/event-attendance — session and event manager

Required query: `id=UUID`. Returns
`{id,title,starts_at,ends_at,completed_at,attendees:[{id,name,registered}]}`.
Attendees are current registered users plus the requesting manager if not
registered, sorted by name. Non-managers receive 403. This is the checklist
input for completion, not a historical participation report.

### GET /api/crew — session

Required query: `id=UUID` for a `volunteer_group`. Public crews are readable;
private crews require ownership or active/invited membership. Hidden crews
return 404. Response always has `events`, upcoming accessible crew events with
`id`, `title`, `starts_at`, `ends_at`, `visibility`, `city`, `address`.

Only active members also receive:

- `members`: `{id,name,role,has_location,availability:[{weekday,start_time,end_time,timezone}]}`.
- `matches`: at most 30 upcoming events with `id`, `title`, `starts_at`,
  `ends_at`, `organization_name`, `city`, `address`.

A match needs enough capacity for members not already registered, audience
access for every member, and a location within every member's saved radius.
Every member must have availability containing the entire event on one local
calendar day in that availability row's timezone. Results are earliest first.

### GET /api/people — session

Optional `q` searches full names using case-insensitive SQL substring matching.
Returns up to 50 other accounts as `{id,name,following,follows_you}`, sorted by
name. Empty query lists people without a name filter; email is not returned.

### GET /api/leaderboard — session

Required `period=week|month|all`; optional `type=individuals|crews|organizations`
(default `individuals`). Returns up to 50 `{id,name,hours}` rows ranked by
verified hours descending, with name ordering for returned ties.
Week/month use PostgreSQL `date_trunc(period, now())`, not a rolling 7/30-day
window; boundaries follow the database session timezone.
Individual entries respect hours visibility. Crew entries aggregate events
hosted by volunteer groups; `organizations` currently means companies, not
charities. Group entries enforce event audience visibility and do not apply
individual hours-privacy filtering to the aggregate.

### GET /api/review — session

No parameters. Returns pending activity by other users at organizations where
the viewer is owner or active owner/admin. Rows are
`{id,caption,started_at,ended_at,created_at,name,title}`, oldest creation first.
Use each activity ID with `/api/verify`.

## Write endpoints

All endpoints below require a session and JSON. Unless otherwise stated,
success is **200** `{"ok":true}`. Fields shown are the complete accepted field
set; unknown fields are rejected. Optional text defaults to empty and optional
arrays default to empty; explicit required values and defaults are noted.

### POST /api/profile

```json
{
  "first_name": "Alex", "last_name": "Rivera", "bio": "Weekend volunteer",
  "max_distance_km": 25, "activity_visibility": "friends",
  "hours_visibility": "private", "volunteer_with_friends": true,
  "clear_location": false,
  "location": {"city":"Troy, NY","label":"Troy","latitude":42.7284,"longitude":-73.6918},
  "tags": [{"name":"Environment","category":"cause"}],
  "availability": [{"weekday":6,"start_time":"09:00","end_time":"17:00","timezone":"America/New_York"}]
}
```

This is a **replacement of editable profile fields**, not a partial patch.
First/last names are required, trimmed, at most 100 bytes; bio at most 1,000.
Radius is required, integer 1–500. Both visibility fields are required and must
be `private`, `friends`, or `public`. Tags and availability are replaced entirely;
omitting them clears them. At most 30 tags and 21 availability rows are accepted.
Tag names are trimmed, 1–80 bytes; category is `interest`, `skill`, or `cause`.
Weekday is 0–6, times must be HH:MM with end after start, and timezone must load
as a timezone name. Overnight availability must be split into separate days.

Omit/null `location` to retain it. Set `clear_location:true` to remove it; do not
also supply `location`. A location accepts `address`, `label`, `city`, `latitude`,
`longitude`. Nonempty address is geocoded and overrides city/coordinates.
Otherwise provide city/coordinates (for example from `/api/location`). City is
required, at most 120 bytes; label at most 200; latitude -90–90, longitude -180–180.
Profile changes never grant site-admin status.

### POST /api/organization

Body: `{name,description,visibility,organization_type,address}`.
Required trimmed name: 1–120 bytes; description at most 2,000.
Required visibility: `public`, `invite_only`, or `closed`.
Required type: `volunteer_group`, `charity`, or `company`.
Charities/companies require a geocodable street address; volunteer groups inherit
the creator's saved location and do not use the address input.
Creates the organization and an active owner membership atomically.
Returns `{"id":"UUID"}`. Charities require approval before publishing.

### POST /api/charity-approval

Body: `{"organization_id":"UUID"}`. Only users whose database record has
`is_site_admin=true` can approve a charity. Returns 403 for unauthorized users
or a target that is not an existing charity. Sets approval to `approved`;
there is no rejection/revocation action here.

### POST /api/event

```json
{
  "organization_id":"11111111-1111-4111-8111-111111111111",
  "title":"Park cleanup", "description":"Help clean the park.",
  "starts_at":"2026-12-05T10:00:00-05:00", "ends_at":"2026-12-05T12:00:00-05:00",
  "capacity":20, "label":"Meeting point", "address":"Full street address, city, state ZIP",
  "causes":["Environment"], "visibility":"public"
}
```

Also accepts `city`, `latitude`, `longitude` as an alternative to address lookup.
Required: organization UUID, trimmed title (1–160 bytes), dates, capacity (1–10,000),
and a location. Description <=4,000 bytes, city <=120, label <=200; coordinates
must be in geographic bounds. An address overrides city/coordinates after lookup.
End must be after start, no more than 24 hours later, and in the future; the
handler permits an already-started event if its end remains future.
At most five cause names, each trimmed and 1–80 bytes. Visibility defaults to
`public`, otherwise must be `public` or `crew_only`.

Organization owners or active owner/admin members may publish. Any active
member of an approved charity may also publish; unapproved charities cannot.
Creates location, published event, creator metadata, and cause links in one
transaction. Returns `{"id":"UUID"}`. There is no event edit/delete endpoint.

### POST /api/registration

Body: `{"event_id":"UUID","join":true}`; false cancels registration.
Locks the event before checking capacity. Joining requires a published event
whose start is still future and access to its audience. Existing registrants
can repeat join even at capacity; new registrants receive 409 when full.
Completed events reject both join and leave with 409. Leaving otherwise marks
an existing registration cancelled. No waitlist is created by this endpoint.

### POST /api/save

Body: `{"event_id":"UUID","save":true}`; false removes the bookmark.
Saving requires an accessible published event (404 otherwise). Repeated saves
are harmless. Saved state appears on opportunity rows; no separate saved-list
endpoint exists.

### POST /api/event-complete

Body: `{event_id,participant_ids: string[],started_at,ended_at}`.
Requires an event manager: organization owner, event creator, or active
organization owner/admin. Participant IDs must be valid, unique, and currently
registered, except the requesting manager may include themselves without a
prior registration. An empty attendance list is allowed. At most 10,000 IDs
are checked, although the shared 32 KiB body limit is reached sooner.

The event must be published and have started. Actual service must start no
earlier than the scheduled start, end no later than now, and last >0 and <=24
hours. Actual end may exceed the scheduled end.

Within one transaction this endpoint locks the event, rejects/supersedes earlier
individual activity submissions for that event, resets participation flags,
creates verified activity for selected participants, marks them participated,
and stores completion time/manager/actual service period. A selected manager is
registered automatically if necessary and may receive completion credit.

Returns `{"ok":true,"participants_credited":3,"hours_each":2}`.
After authorization, an already-completed event returns
`{"ok":true,"already_completed":true}` without changing attendance or awarding
hours again. Completion is final through this API. A unique database index also
prevents duplicate completion credit per event/user.

### POST /api/activity

Legacy individual log body: `{event_id,started_at,ended_at,caption}`.
Requires a current registration and an event not already completed. Caption
is trimmed, at most 2,000 bytes. Duration must be >0 and <=24 hours, not in the
future, and entirely inside scheduled event times. Overlap with another
non-rejected activity for that user returns 409. Creates a pending activity;
no ID is returned. Completion later supersedes individual submissions.

### POST /api/verify

Body: `{"activity_id":"UUID","status":"verified"}` or status `rejected`.
Only organization owners/active owner-admins may review another user's pending
activity. Self-review is disallowed here. No eligible pending row returns 409,
including repeated verification or insufficient reviewer authority. This
endpoint differs from organizer completion, which can credit the manager.

### POST /api/reaction

Body: `{"activity_id":"UUID","like":true}`; false removes kudos.
Requires visible, non-rejected activity (404 otherwise), including event
audience checks. Stores one `clap` per activity/user; repeated likes do not add
extra reactions.

### POST /api/comment

Body: `{"activity_id":"UUID","body":"Thanks for helping!"}`.
Requires the same visibility as reactions. Trimmed body must be 1–1,000 bytes.
Creates a new comment each time; retries can duplicate comments. No comment
edit/delete endpoint exists.

### POST /api/membership

Body: `{"organization_id":"UUID","join":true}`; false leaves.
Public organizations allow joining; other visibility modes require existing
invited/active membership or ownership. Joining sets active membership; leaving
sets removed. Owners cannot leave their own organization (409).

### POST /api/invite

Body: `{organization_id,user_ids: string[]}` **or** `{event_id,user_ids: string[]}`.
Exactly one target is required. Supply 1–30 valid user UUIDs; no self-invites.
Organization invitations require ownership or active membership and set invited
membership, preserving existing active memberships. Event invitations require
ownership, event creation, or current registration, and the event must be
published, uncompleted, and future. Crew-only event invitees must already be
active hosting-organization members. Invitations are persisted in-app; no email
is sent, and an invitation does not register the recipient for an event.

### POST /api/follow

Body: `{"user_id":"UUID","follow":true}`; false unfollows.
Self-follow is invalid. Creates/removes a directional follow; repeat actions
are harmless. Mutual follows define friends for privacy access.

### POST /api/challenge

Body: `{"key":"ten-hours","join":true}`; false leaves.
Accepted keys: `ten-hours`, `three-events`, `fifty-hours`, `three-causes`,
`three-organizations`, `four-weeks`. Stores challenge membership only; progress
comes from activity statistics and frontend goal definitions.

## Visibility and location behavior

Activity and hours visibility are independent: self is allowed, public is
allowed, friends requires mutual follows. Feed/comment/reaction access also
requires the event audience. Crew-only events allow organization owner, event
creator, or active organization members. An invitation alone does not grant
crew-only event access. Review/completion endpoints use their own organizer
authorization rather than feed privacy rules.

Street address resolution first reuses a case-insensitive matching stored address,
then calls the U.S. Census Geocoder with `Public_AR_Current`. Input must be
8–300 bytes; exactly one complete match is required (422 otherwise); upstream
failures return 503. This happens within profile/organization/event writes,
not through a separate address endpoint. Area lookup uses Nominatim instead.
Neither integration needs an API key in this implementation.

## Example client workflow

From a same-origin browser page, create a session and read the dashboard:

```js
async function request(path, body) {
  const response = await fetch(path, {
    method: body === undefined ? "GET" : "POST",
    credentials: "same-origin",
    headers: {
      Accept: "application/json",
      ...(body === undefined ? {} : {"Content-Type": "application/json"})
    },
    ...(body === undefined ? {} : {body: JSON.stringify(body)})
  });
  const data = await response.json();
  if (!response.ok) throw new Error(`${response.status}: ${data.error}`);
  return data;
}

await request("/login", {email: "alex@example.com", password: "a long example phrase"});
const dashboard = await request("/api/dashboard");
const events = await request("/api/opportunities?near=true&distance=10");
// Select an actual returned event ID before joining:
// await request("/api/registration", {event_id: selectedId, join: true});
```

For a new account, use `/signup`, resolve an area with `/api/location`, and save
a complete `/api/profile` payload before nearby search. To host service, create
an organization, obtain charity approval if needed, publish an event, then use
the attendance checklist and `/api/event-complete` after service ends. Refresh
dashboard/my-events after writes to retrieve updated state.

## Implementation map

| Source | Responsibility |
| --- | --- |
| `internal/httpserver/router.go` | Top-level routes, embedded pages/assets, cross-origin protection |
| `internal/auth/session.go` | Public-route allowlist, cookie authentication, session lifecycle |
| `internal/auth/login.go`, `signup.go`, `postgres.go` | Credentials, account creation, session persistence |
| `internal/httpserver/product.go` | Product dispatch, errors, profile, opportunity search, registration, activity interactions |
| `internal/httpserver/product_groups.go` | Organizations, membership, invitations, publishing, verification, crew matching, charity approval |
| `internal/httpserver/product_queries.go` | Dashboard, feeds, privacy, statistics, leaderboards |
| `internal/httpserver/event_lifecycle.go` | Event history/details, manager authorization, attendance/completion |
| `internal/httpserver/events.go` | Legacy events/matches filters |
| `internal/httpserver/geocode.go` | Census street and Nominatim area resolution |
| `internal/api/json.go` | JSON success/error serialization |
| `migrations/001_init.sql` through `005_onboarding_charities.sql` | Tables, constraints, sessions, social features, completion, charity approval |

When adding endpoints, update both dispatch and the authentication allowlist
(only if public access is intended), then update this reference. The existing
router, auth, geocoder, and event tests cover protocol behavior; PostgreSQL
integration tests require `TEST_DATABASE_URL`.
