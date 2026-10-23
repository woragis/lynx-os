CREATE TABLE integrations (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    slug TEXT NOT NULL UNIQUE,
    base_url TEXT NOT NULL,
    encrypted_api_key TEXT NOT NULL,
    api_key_iv TEXT NOT NULL,
    api_key_auth_tag TEXT NOT NULL,
    active BOOLEAN NOT NULL DEFAULT TRUE,
    revoked_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT integrations_revocation_consistent CHECK (revoked_at IS NULL OR active = FALSE)
);

CREATE TABLE provisioning_records (
    id TEXT PRIMARY KEY,
    idempotency_key TEXT NOT NULL UNIQUE,
    integration_id TEXT NOT NULL REFERENCES integrations(id) ON DELETE RESTRICT,
    payload_hash CHAR(64) NOT NULL,
    payload_ciphertext TEXT NOT NULL,
    payload_iv TEXT NOT NULL,
    payload_auth_tag TEXT NOT NULL,
    status TEXT NOT NULL,
    response_ciphertext TEXT,
    response_iv TEXT,
    response_auth_tag TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT provisioning_response_ciphertext_complete CHECK (
      (response_ciphertext IS NULL AND response_iv IS NULL AND response_auth_tag IS NULL)
      OR (response_ciphertext IS NOT NULL AND response_iv IS NOT NULL AND response_auth_tag IS NOT NULL)
    )
);
CREATE INDEX provisioning_records_integration_created_idx ON provisioning_records(integration_id, created_at DESC);

CREATE TABLE telegram_conversations (
    chat_id TEXT PRIMARY KEY,
    state TEXT NOT NULL,
    draft_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE audit_logs (
    id TEXT PRIMARY KEY,
    action TEXT NOT NULL,
    integration_id TEXT REFERENCES integrations(id) ON DELETE SET NULL,
    request_id TEXT,
    result TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX audit_logs_created_idx ON audit_logs(created_at DESC);
CREATE INDEX audit_logs_integration_created_idx ON audit_logs(integration_id, created_at DESC);
