CREATE TABLE legacy_telegram_conversations (
    source_chat_id_hash CHAR(64) PRIMARY KEY,
    state TEXT NOT NULL,
    data_ciphertext TEXT NOT NULL,
    data_iv TEXT NOT NULL,
    data_auth_tag TEXT NOT NULL,
    source_updated_at TIMESTAMPTZ NOT NULL,
    imported_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE sqlite_import_runs (
    source_fingerprint CHAR(64) PRIMARY KEY,
    imported_counts JSONB NOT NULL,
    imported_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
