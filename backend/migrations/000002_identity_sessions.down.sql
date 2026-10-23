DROP INDEX IF EXISTS audit_logs_actor_created_idx;
ALTER TABLE audit_logs DROP COLUMN IF EXISTS actor_user_id;
DROP TABLE IF EXISTS telegram_identities;
DROP TABLE IF EXISTS admin_password_reset_tokens;
DROP TABLE IF EXISTS admin_sessions;
DROP TABLE IF EXISTS admin_users;
