package sqliteimport

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	app "github.com/lynx/lynx-os/backend/internal/application/sqliteimport"
)

const importLockID int64 = 770021007

type Target struct{ pool *pgxpool.Pool }

func NewTarget(pool *pgxpool.Pool) *Target { return &Target{pool: pool} }

func (t *Target) Counts(ctx context.Context) (app.Counts, error) {
	var c app.Counts
	err := t.pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM integrations),(SELECT count(*) FROM provisioning_records),(SELECT count(*) FROM audit_logs),(SELECT count(*) FROM legacy_telegram_conversations)`).Scan(&c.Integrations, &c.Operations, &c.AuditLogs, &c.TelegramConversations)
	if err != nil {
		return app.Counts{}, err
	}
	var runs int
	if err := t.pool.QueryRow(ctx, `SELECT count(*) FROM sqlite_import_runs`).Scan(&runs); err != nil {
		return app.Counts{}, err
	}
	var oldConversations int
	if err := t.pool.QueryRow(ctx, `SELECT count(*) FROM telegram_conversations`).Scan(&oldConversations); err != nil {
		return app.Counts{}, err
	}
	c.DestinationOccupied = runs + oldConversations
	if runs > 0 && c.Integrations+c.Operations+c.AuditLogs+c.TelegramConversations == 0 {
		return app.Counts{}, errors.New("a SQLite import has already been recorded")
	}
	return c, nil
}

func (t *Target) Import(ctx context.Context, data app.Dataset, fingerprint string, counts app.Counts) error {
	tx, err := t.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, importLockID); err != nil {
		return err
	}
	var existing int
	if err = tx.QueryRow(ctx, `SELECT (SELECT count(*) FROM integrations)+(SELECT count(*) FROM provisioning_records)+(SELECT count(*) FROM audit_logs)+(SELECT count(*) FROM legacy_telegram_conversations)+(SELECT count(*) FROM telegram_conversations)+(SELECT count(*) FROM sqlite_import_runs)`).Scan(&existing); err != nil {
		return err
	}
	if existing != 0 {
		return app.ErrTargetNotEmpty
	}
	for _, item := range data.Integrations {
		if _, err = tx.Exec(ctx, `INSERT INTO integrations(id,name,slug,base_url,app_url,encrypted_api_key,api_key_iv,api_key_auth_tag,active,revoked_at,created_at,updated_at) VALUES($1,$2,$3,$4,NULLIF($5,''),$6,$7,$8,$9,$10,$11,$12)`, item.ID, item.Name, item.Slug, item.BaseURL, item.AppURL, item.Ciphertext, item.IV, item.AuthTag, item.Active, item.RevokedAt, item.CreatedAt, item.UpdatedAt); err != nil {
			return err
		}
	}
	for _, item := range data.Operations {
		var responseCipher, responseIV, responseTag any
		if item.ResponseCiphertext != "" {
			responseCipher, responseIV, responseTag = item.ResponseCiphertext, item.ResponseIV, item.ResponseAuthTag
		}
		if _, err = tx.Exec(ctx, `INSERT INTO provisioning_records(id,idempotency_key,integration_id,payload_hash,payload_ciphertext,payload_iv,payload_auth_tag,response_ciphertext,response_iv,response_auth_tag,status,operation,request_id,created_at,updated_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$14)`, item.ID, item.IdempotencyKey, item.IntegrationID, item.PayloadHash, item.PayloadCiphertext, item.PayloadIV, item.PayloadAuthTag, responseCipher, responseIV, responseTag, item.Status, item.Operation, item.IdempotencyKey, item.CreatedAt); err != nil {
			return err
		}
	}
	for _, item := range data.AuditLogs {
		if _, err = tx.Exec(ctx, `INSERT INTO audit_logs(id,action,integration_id,request_id,result,created_at) VALUES($1,$2,NULLIF($3,''),NULLIF($4,''),$5,$6)`, item.ID, item.Action, item.IntegrationID, item.RequestID, item.Result, item.CreatedAt); err != nil {
			return err
		}
	}
	for _, item := range data.TelegramConversations {
		digest := sha256.Sum256([]byte(item.EncryptedDraft.Ciphertext))
		opaqueID := hex.EncodeToString(digest[:])
		if _, err = tx.Exec(ctx, `INSERT INTO legacy_telegram_conversations(source_chat_id_hash,state,data_ciphertext,data_iv,data_auth_tag,source_updated_at) VALUES($1,$2,$3,$4,$5,$6)`, opaqueID, item.State, item.EncryptedDraft.Ciphertext, item.EncryptedDraft.IV, item.EncryptedDraft.AuthTag, item.UpdatedAt); err != nil {
			return err
		}
	}
	var stored app.Counts
	if err = tx.QueryRow(ctx, `SELECT (SELECT count(*) FROM integrations),(SELECT count(*) FROM provisioning_records),(SELECT count(*) FROM audit_logs),(SELECT count(*) FROM legacy_telegram_conversations)`).Scan(&stored.Integrations, &stored.Operations, &stored.AuditLogs, &stored.TelegramConversations); err != nil {
		return err
	}
	if stored != counts {
		return errors.New("PostgreSQL row counts do not match the SQLite import inventory")
	}
	countsJSON, err := json.Marshal(counts)
	if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO sqlite_import_runs(source_fingerprint,imported_counts) VALUES($1,$2::jsonb)`, fingerprint, string(countsJSON)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

var _ app.Target = (*Target)(nil)
