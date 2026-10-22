package postgres

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	domain "github.com/lynx/lynx-os/backend/internal/domain/integration"
)

type IntegrationRepository struct{ pool *pgxpool.Pool }

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
