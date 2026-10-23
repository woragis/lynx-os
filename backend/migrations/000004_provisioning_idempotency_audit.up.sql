ALTER TABLE provisioning_records
    DROP CONSTRAINT IF EXISTS provisioning_records_idempotency_key_key;

ALTER TABLE provisioning_records
    ADD COLUMN IF NOT EXISTS operation TEXT NOT NULL DEFAULT 'unknown',
    ADD COLUMN IF NOT EXISTS actor_user_id TEXT REFERENCES admin_users(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS request_id TEXT;

CREATE UNIQUE INDEX IF NOT EXISTS provisioning_records_integration_idempotency_unique
    ON provisioning_records(integration_id, idempotency_key);

ALTER TABLE provisioning_records
    DROP CONSTRAINT IF EXISTS provisioning_records_status_check;
ALTER TABLE provisioning_records
    ADD CONSTRAINT provisioning_records_status_check CHECK (status IN ('pending', 'completed', 'failed'));

CREATE INDEX IF NOT EXISTS provisioning_records_status_updated_idx
    ON provisioning_records(status, updated_at);

ALTER TABLE audit_logs
    ADD COLUMN IF NOT EXISTS resource_type TEXT,
    ADD COLUMN IF NOT EXISTS resource_id TEXT;
