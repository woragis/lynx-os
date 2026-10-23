package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	domain "github.com/lynx/lynx-os/backend/internal/domain/integration"
)

type IntegrationRepository struct{ pool *pgxpool.Pool }

func (r *IntegrationRepository) StartProvisioning(ctx context.Context, item domain.ProvisioningRecord) (domain.ProvisioningRecord, bool, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.ProvisioningRecord{}, false, err
	}
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, `INSERT INTO provisioning_records
		(id,idempotency_key,integration_id,payload_hash,payload_ciphertext,payload_iv,payload_auth_tag,status,operation,actor_user_id,request_id)
		VALUES($1,$2,$3,$4,$5,$6,$7,'pending',$8,NULLIF($9,''),$10)
		ON CONFLICT(integration_id,idempotency_key) DO NOTHING`,
		item.ID, item.IdempotencyKey, item.IntegrationID, item.PayloadHash, item.Payload.Ciphertext, item.Payload.IV, item.Payload.AuthTag, item.Operation, item.ActorUserID, item.RequestID)
	if err != nil {
		return domain.ProvisioningRecord{}, false, err
	}
	var stored domain.ProvisioningRecord
	var responseCipher, responseIV, responseTag *string
	var updatedAt time.Time
	err = tx.QueryRow(ctx, `SELECT id,integration_id,idempotency_key,payload_hash,payload_ciphertext,payload_iv,payload_auth_tag,
		response_ciphertext,response_iv,response_auth_tag,status,operation,COALESCE(actor_user_id,''),COALESCE(request_id,''),updated_at
		FROM provisioning_records WHERE integration_id=$1 AND idempotency_key=$2 FOR UPDATE`, item.IntegrationID, item.IdempotencyKey).
		Scan(&stored.ID, &stored.IntegrationID, &stored.IdempotencyKey, &stored.PayloadHash, &stored.Payload.Ciphertext, &stored.Payload.IV, &stored.Payload.AuthTag,
			&responseCipher, &responseIV, &responseTag, &stored.Status, &stored.Operation, &stored.ActorUserID, &stored.RequestID, &updatedAt)
	if err != nil {
		return domain.ProvisioningRecord{}, false, err
	}
	if stored.PayloadHash != item.PayloadHash {
		return domain.ProvisioningRecord{}, false, domain.ErrIdempotencyConflict
	}
	if responseCipher != nil && responseIV != nil && responseTag != nil {
		stored.Response = &domain.SealedSecret{Ciphertext: *responseCipher, IV: *responseIV, AuthTag: *responseTag}
	}
	created := stored.ID == item.ID
	if !created && (stored.Status == "failed" || (stored.Status == "pending" && time.Since(updatedAt) > 5*time.Minute)) {
		_, err = tx.Exec(ctx, `UPDATE provisioning_records SET status='pending',updated_at=now() WHERE id=$1`, stored.ID)
		if err != nil {
			return domain.ProvisioningRecord{}, false, err
		}
		stored.Status = "pending"
		// This request owns the retry after the old attempt was marked failed or abandoned.
		created = true
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.ProvisioningRecord{}, false, err
	}
	return stored, created, nil
}

func (r *IntegrationRepository) CompleteProvisioning(ctx context.Context, id string, response domain.SealedSecret, audit domain.AuditEntry) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	tag, err := tx.Exec(ctx, `UPDATE provisioning_records SET status='completed',response_ciphertext=$2,response_iv=$3,response_auth_tag=$4,updated_at=now() WHERE id=$1 AND status='pending'`, id, response.Ciphertext, response.IV, response.AuthTag)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return errors.New("provisioning record is not pending")
	}
	if err := insertAudit(ctx, tx, audit); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *IntegrationRepository) FailProvisioning(ctx context.Context, id string, audit domain.AuditEntry) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	tag, err := tx.Exec(ctx, `UPDATE provisioning_records SET status='failed',updated_at=now() WHERE id=$1 AND status='pending'`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return errors.New("provisioning record is not pending")
	}
	if err := insertAudit(ctx, tx, audit); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *IntegrationRepository) WriteAudit(ctx context.Context, entry domain.AuditEntry) error {
	return insertAudit(ctx, r.pool, entry)
}

type sqlExecutor interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

func insertAudit(ctx context.Context, executor sqlExecutor, entry domain.AuditEntry) error {
	if entry.ID == "" {
		return errors.New("could not allocate audit id")
	}
	_, err := executor.Exec(ctx, `INSERT INTO audit_logs(id,action,integration_id,request_id,result,actor_user_id,resource_type,resource_id) VALUES($1,$2,$3,$4,$5,NULLIF($6,''),NULLIF($7,''),NULLIF($8,''))`, entry.ID, entry.Action, entry.IntegrationID, entry.RequestID, entry.Result, entry.ActorUserID, entry.ResourceType, entry.ResourceID)
	return err
}

func NewIntegrationRepository(pool *pgxpool.Pool) *IntegrationRepository {
	return &IntegrationRepository{pool: pool}
}

func (r *IntegrationRepository) Create(ctx context.Context, item domain.Record, secret domain.SealedSecret) (domain.Record, error) {
	err := r.pool.QueryRow(ctx, `
		INSERT INTO integrations (id, name, slug, base_url, encrypted_api_key, api_key_iv, api_key_auth_tag, active, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,true,$8,$8)
		RETURNING created_at, updated_at`,
		item.ID, item.Name, item.Slug, item.BaseURL, secret.Ciphertext, secret.IV, secret.AuthTag, item.CreatedAt,
	).Scan(&item.CreatedAt, &item.UpdatedAt)
	return item, err
}

func (r *IntegrationRepository) List(ctx context.Context) ([]domain.Record, error) {
	rows, err := r.pool.Query(ctx, `SELECT id,name,slug,base_url,active,revoked_at,created_at,updated_at FROM integrations ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]domain.Record, 0)
	for rows.Next() {
		var item domain.Record
		if err := rows.Scan(&item.ID, &item.Name, &item.Slug, &item.BaseURL, &item.Active, &item.RevokedAt, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *IntegrationRepository) Get(ctx context.Context, id string) (domain.Record, domain.SealedSecret, error) {
	var item domain.Record
	var secret domain.SealedSecret
	err := r.pool.QueryRow(ctx, `
		SELECT id,name,slug,base_url,active,revoked_at,created_at,updated_at,encrypted_api_key,api_key_iv,api_key_auth_tag
		FROM integrations WHERE id=$1`, id,
	).Scan(&item.ID, &item.Name, &item.Slug, &item.BaseURL, &item.Active, &item.RevokedAt, &item.CreatedAt, &item.UpdatedAt, &secret.Ciphertext, &secret.IV, &secret.AuthTag)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Record{}, domain.SealedSecret{}, domain.ErrIntegrationNotFound
	}
	return item, secret, err
}

func (r *IntegrationRepository) ReplaceSecret(ctx context.Context, id string, secret domain.SealedSecret) (domain.Record, error) {
	var item domain.Record
	err := r.pool.QueryRow(ctx, `
		UPDATE integrations SET encrypted_api_key=$2,api_key_iv=$3,api_key_auth_tag=$4,updated_at=now()
		WHERE id=$1 AND active=true AND revoked_at IS NULL
		RETURNING id,name,slug,base_url,active,revoked_at,created_at,updated_at`,
		id, secret.Ciphertext, secret.IV, secret.AuthTag,
	).Scan(&item.ID, &item.Name, &item.Slug, &item.BaseURL, &item.Active, &item.RevokedAt, &item.CreatedAt, &item.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Record{}, domain.ErrIntegrationNotFound
	}
	return item, err
}

func (r *IntegrationRepository) Revoke(ctx context.Context, id string) (domain.Record, error) {
	var item domain.Record
	err := r.pool.QueryRow(ctx, `
		UPDATE integrations SET active=false,revoked_at=COALESCE(revoked_at,now()),encrypted_api_key='',api_key_iv='',api_key_auth_tag='',updated_at=now()
		WHERE id=$1
		RETURNING id,name,slug,base_url,active,revoked_at,created_at,updated_at`, id,
	).Scan(&item.ID, &item.Name, &item.Slug, &item.BaseURL, &item.Active, &item.RevokedAt, &item.CreatedAt, &item.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Record{}, domain.ErrIntegrationNotFound
	}
	return item, err
}
