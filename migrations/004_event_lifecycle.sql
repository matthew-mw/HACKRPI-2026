ALTER TABLE events ADD COLUMN visibility text NOT NULL DEFAULT 'public'
    CHECK (visibility IN ('public','crew_only'));
-- Preserve the audience of existing events from restricted organizations.
UPDATE events e SET visibility='crew_only' FROM organizations o
WHERE o.id=e.organization_id AND o.visibility<>'public';
ALTER TABLE events ADD COLUMN created_by uuid REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE events ADD COLUMN completed_at timestamptz;
ALTER TABLE events ADD COLUMN completed_by uuid REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE events ADD COLUMN actual_started_at timestamptz;
ALTER TABLE events ADD COLUMN actual_ended_at timestamptz;
ALTER TABLE events ADD CONSTRAINT completed_event_times CHECK (
    (completed_at IS NULL AND completed_by IS NULL AND actual_started_at IS NULL AND actual_ended_at IS NULL)
    OR (completed_at IS NOT NULL AND actual_started_at IS NOT NULL
        AND actual_ended_at IS NOT NULL AND actual_ended_at>actual_started_at)
);
ALTER TABLE event_registrations ADD COLUMN participated boolean NOT NULL DEFAULT false;
ALTER TABLE activities ADD COLUMN completion_generated boolean NOT NULL DEFAULT false;
ALTER TABLE activities ADD COLUMN superseded_by_completion boolean NOT NULL DEFAULT false;
CREATE UNIQUE INDEX uq_event_completion_activity ON activities(event_id,user_id) WHERE completion_generated;
CREATE INDEX idx_events_creator ON events(created_by);
