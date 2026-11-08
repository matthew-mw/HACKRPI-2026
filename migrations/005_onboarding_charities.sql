ALTER TABLE users ADD COLUMN is_site_admin boolean NOT NULL DEFAULT false;
ALTER TABLE organizations ADD COLUMN approval_status text NOT NULL DEFAULT 'pending' CHECK (approval_status IN ('pending','approved','rejected'));
-- Existing charities also require review before publishing further events.
