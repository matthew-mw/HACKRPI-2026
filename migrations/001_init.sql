-- AI volunteer / charity matching application
-- PostgreSQL 15+ with pgvector and PostGIS enabled
-- Gemini Embedding 2 configured to output 768 dimensions.
-- Execute in order on a NEW database.

CREATE EXTENSION IF NOT EXISTS pgcrypto;
CREATE EXTENSION IF NOT EXISTS vector;
CREATE EXTENSION IF NOT EXISTS postgis;

-- LOCATION DATA -----------------------------------------------------------
CREATE TABLE locations (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    label          text,
    address_line1  text,
    city           text NOT NULL,
    region         text,
    postal_code    text,
    country_code   char(2) NOT NULL DEFAULT 'US',
    coordinates    geography(Point, 4326) NOT NULL,
    created_at     timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_locations_coordinates ON locations USING gist (coordinates);

-- USERS AND ORGS ---------------------------------------------------------
CREATE TABLE users (
    id                   uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    first_name           text NOT NULL,
    last_name            text NOT NULL,
    email                text NOT NULL,
    phone                text,
    bio                  text NOT NULL DEFAULT '',
    password_hash        text,
    location_id          uuid REFERENCES locations(id) ON DELETE SET NULL,
    max_distance_km      integer NOT NULL DEFAULT 25 CHECK (max_distance_km BETWEEN 1 AND 500),
    embedding            vector(768),
    embedding_model      text,
    embedding_updated_at timestamptz,
    created_at           timestamptz NOT NULL DEFAULT now(),
    updated_at           timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX uq_users_email_ci ON users (lower(email));

CREATE TABLE organizations (
    id                   uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name                 text NOT NULL,
    description          text NOT NULL DEFAULT '',
    organization_type    text NOT NULL CHECK (organization_type IN ('charity', 'volunteer_group', 'company')),
    visibility           text NOT NULL DEFAULT 'public' CHECK (visibility IN ('public', 'invite_only', 'closed')),
    owner_id             uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    location_id          uuid REFERENCES locations(id) ON DELETE SET NULL,
    embedding            vector(768),
    embedding_model      text,
    embedding_updated_at timestamptz,
    created_at           timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_organizations_owner ON organizations(owner_id);
CREATE INDEX idx_organizations_type ON organizations(organization_type);

CREATE TABLE organization_members (
    organization_id  uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    user_id          uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role             text NOT NULL DEFAULT 'member' CHECK (role IN ('owner', 'admin', 'member')),
    status           text NOT NULL DEFAULT 'active' CHECK (status IN ('invited', 'requested', 'active', 'removed')),
    joined_at        timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (organization_id, user_id)
);
CREATE INDEX idx_organization_members_user ON organization_members(user_id);

-- TAGS -------------------------------------------------------------------
CREATE TABLE tags (
    id        uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name      text NOT NULL,
    category  text NOT NULL DEFAULT 'interest' CHECK (category IN ('interest', 'skill', 'cause', 'activity'))
);
CREATE UNIQUE INDEX uq_tags_category_name_ci ON tags(category, lower(name));

CREATE TABLE user_tags (
    user_id  uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    tag_id   uuid NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
    PRIMARY KEY (user_id, tag_id)
);
CREATE INDEX idx_user_tags_tag ON user_tags(tag_id);

CREATE TABLE organization_tags (
    organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    tag_id          uuid NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
    PRIMARY KEY (organization_id, tag_id)
);
CREATE INDEX idx_org_tags_tag ON organization_tags(tag_id);

-- Recurring template and event instances ---------------------------------
CREATE TABLE recurring_events (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id  uuid NOT NULL REFERENCES organizations(id) ON DELETE RESTRICT,
    location_id      uuid NOT NULL REFERENCES locations(id) ON DELETE RESTRICT,
    title            text NOT NULL,
    description      text NOT NULL DEFAULT '',
    capacity         integer NOT NULL DEFAULT 1 CHECK (capacity > 0),
    recurrence_rule  text NOT NULL,
    timezone         text NOT NULL DEFAULT 'UTC',
    next_run_at      timestamptz,
    active           boolean NOT NULL DEFAULT true,
    created_at       timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_recurring_due ON recurring_events(next_run_at) WHERE active;

CREATE TABLE events (
    id                   uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id      uuid NOT NULL REFERENCES organizations(id) ON DELETE RESTRICT,
    location_id          uuid NOT NULL REFERENCES locations(id) ON DELETE RESTRICT,
    recurring_event_id   uuid REFERENCES recurring_events(id) ON DELETE SET NULL,
    title                text NOT NULL,
    description          text NOT NULL DEFAULT '',
    starts_at            timestamptz NOT NULL,
    ends_at              timestamptz NOT NULL,
    capacity             integer NOT NULL DEFAULT 1 CHECK (capacity > 0),
    status               text NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'published', 'cancelled')),
    embedding            vector(768),
    embedding_model      text,
    embedding_updated_at timestamptz,
    created_at           timestamptz NOT NULL DEFAULT now(),
    CHECK (ends_at > starts_at)
);
CREATE UNIQUE INDEX uq_events_recurring_instance ON events(recurring_event_id, starts_at)
    WHERE recurring_event_id IS NOT NULL;
CREATE INDEX idx_events_org ON events(organization_id);
CREATE INDEX idx_events_published_start ON events(starts_at) WHERE status = 'published';
CREATE INDEX idx_events_location ON events(location_id);

CREATE TABLE event_tags (
    event_id  uuid NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    tag_id    uuid NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
    PRIMARY KEY (event_id, tag_id)
);
CREATE INDEX idx_event_tags_tag ON event_tags(tag_id);

CREATE TABLE event_registrations (
    event_id       uuid NOT NULL REFERENCES events(id) ON DELETE RESTRICT,
    user_id        uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    status         text NOT NULL DEFAULT 'registered'
                   CHECK (status IN ('registered', 'waitlisted', 'cancelled')),
    registered_at  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (event_id, user_id)
);
CREATE INDEX idx_event_registrations_user ON event_registrations(user_id);

CREATE TABLE user_availability (
    user_id      uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    weekday      smallint NOT NULL CHECK (weekday BETWEEN 0 AND 6),
    start_time   time NOT NULL,
    end_time     time NOT NULL,
    timezone     text NOT NULL DEFAULT 'UTC',
    PRIMARY KEY (user_id, weekday, start_time),
    CHECK (end_time > start_time)
);

-- HOURS AND SOCIAL --------------------------------------------------------
CREATE TABLE activities (
    id                   uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id              uuid NOT NULL,
    event_id             uuid NOT NULL,
    started_at           timestamptz NOT NULL,
    ended_at             timestamptz,
    verification_status  text NOT NULL DEFAULT 'pending'
                         CHECK (verification_status IN ('pending', 'verified', 'rejected')),
    verified_by          uuid REFERENCES users(id) ON DELETE SET NULL,
    created_at           timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (event_id, user_id)
        REFERENCES event_registrations(event_id, user_id) ON DELETE RESTRICT,
    CHECK (ended_at IS NULL OR ended_at >= started_at)
);
CREATE INDEX idx_activities_user ON activities(user_id);
CREATE INDEX idx_activities_event ON activities(event_id);

CREATE TABLE reactions (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    activity_id    uuid NOT NULL REFERENCES activities(id) ON DELETE CASCADE,
    user_from      uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    reaction_type  text NOT NULL CHECK (reaction_type IN ('like', 'clap', 'heart')),
    created_at     timestamptz NOT NULL DEFAULT now(),
    UNIQUE (activity_id, user_from)
);

CREATE TABLE following (
    follower_id   uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    following_id  uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at    timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (follower_id, following_id),
    CHECK (follower_id <> following_id)
);
CREATE INDEX idx_following_target ON following(following_id);

CREATE TABLE organization_follows (
    user_id          uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    organization_id  uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    created_at       timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, organization_id)
);

-- Add these optional indexes later when there are enough embeddings to justify ANN.
-- CREATE INDEX idx_events_embedding_hnsw ON events
--    USING hnsw (embedding vector_cosine_ops);
-- CREATE INDEX idx_org_embedding_hnsw ON organizations
--    USING hnsw (embedding vector_cosine_ops);

-- Suggested query to test embedding retrieval (supply a valid 768D vector):
-- SELECT id, title, 1 - (embedding <=> $1::vector) AS cosine_similarity
-- FROM events
-- WHERE embedding IS NOT NULL AND status = 'published' AND starts_at > now()
-- ORDER BY embedding <=> $1::vector
-- LIMIT 50;
