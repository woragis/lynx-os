DROP INDEX IF EXISTS provisioning_records_status_updated_idx;
ALTER TABLE provisioning_records DROP CONSTRAINT IF EXISTS provisioning_records_status_check;
DROP INDEX IF EXISTS provisioning_records_integration_idempotency_unique;
ALTER TABLE provisioning_records ADD CONSTRAINT provisioning_records_idempotency_key_key UNIQUE (idempotency_key);
ALTER TABLE provisioning_records DROP COLUMN IF EXISTS request_id;
ALTER TABLE provisioning_records DROP COLUMN IF EXISTS actor_user_id;
ALTER TABLE provisioning_records DROP COLUMN IF EXISTS operation;
ALTER TABLE audit_logs DROP COLUMN IF EXISTS resource_id;
ALTER TABLE audit_logs DROP COLUMN IF EXISTS resource_type;
