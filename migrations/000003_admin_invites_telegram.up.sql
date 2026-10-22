CREATE TABLE admin_invitations (
    id TEXT PRIMARY KEY,
    email TEXT NOT NULL,
    display_name TEXT NOT NULL,
    role TEXT NOT NULL CHECK (role IN ('admin', 'operator', 'viewer')),
    token_hash CHAR(64) NOT NULL UNIQUE,
    invited_by TEXT NOT NULL REFERENCES admin_users(id) ON DELETE RESTRICT,
    expires_at TIMESTAMPTZ NOT NULL,
    accepted_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX admin_invitations_pending_email_unique ON admin_invitations(lower(email)) WHERE accepted_at IS NULL;
CREATE INDEX admin_invitations_expiry_idx ON admin_invitations(expires_at);

CREATE TABLE telegram_link_codes (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES admin_users(id) ON DELETE CASCADE,
    token_hash CHAR(64) NOT NULL UNIQUE,
    expires_at TIMESTAMPTZ NOT NULL,
    used_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX telegram_link_codes_user_created_idx ON telegram_link_codes(user_id, created_at DESC);
