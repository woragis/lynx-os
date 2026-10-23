CREATE TABLE telegram_bot_conversations (
    telegram_user_id TEXT PRIMARY KEY,
    admin_user_id TEXT NOT NULL REFERENCES admin_users(id) ON DELETE CASCADE,
    state TEXT NOT NULL,
    draft_ciphertext TEXT NOT NULL,
    draft_iv TEXT NOT NULL,
    draft_auth_tag TEXT NOT NULL,
    bot_message_id BIGINT,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX telegram_bot_conversations_expiry_idx ON telegram_bot_conversations(expires_at);

CREATE TABLE telegram_bot_state (
    id SMALLINT PRIMARY KEY CHECK (id = 1),
    update_offset BIGINT NOT NULL DEFAULT 0,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
INSERT INTO telegram_bot_state(id, update_offset) VALUES (1, 0);

CREATE TABLE telegram_bot_updates (
    update_id BIGINT PRIMARY KEY,
    chat_id TEXT,
    state TEXT NOT NULL CHECK (state IN ('processing', 'processed', 'ignored')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    processed_at TIMESTAMPTZ
);
CREATE INDEX telegram_bot_updates_created_idx ON telegram_bot_updates(created_at);
