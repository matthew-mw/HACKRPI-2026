-- Product state extends the original schema; no seed/demo records are inserted.
ALTER TABLE users ADD COLUMN activity_visibility text NOT NULL DEFAULT 'private'
    CHECK (activity_visibility IN ('private', 'friends', 'public'));
ALTER TABLE users ADD COLUMN hours_visibility text NOT NULL DEFAULT 'private'
    CHECK (hours_visibility IN ('private', 'friends', 'public'));
ALTER TABLE users ADD COLUMN volunteer_with_friends boolean NOT NULL DEFAULT true;
ALTER TABLE activities ADD COLUMN caption text NOT NULL DEFAULT '';

CREATE TABLE activity_comments (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    activity_id uuid NOT NULL REFERENCES activities(id) ON DELETE CASCADE,
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    body text NOT NULL CHECK (char_length(body) BETWEEN 1 AND 1000),
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_activity_comments_activity ON activity_comments(activity_id, created_at);

CREATE TABLE saved_events (
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    event_id uuid NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, event_id)
);

CREATE TABLE user_challenges (
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    challenge_key text NOT NULL CHECK (challenge_key IN ('ten-hours','three-events','fifty-hours','three-causes','three-organizations','four-weeks')),
    joined_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, challenge_key)
);

CREATE TABLE event_invitations (
    event_id uuid NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    invited_by uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (event_id, user_id, invited_by),
    CHECK (user_id <> invited_by)
);
