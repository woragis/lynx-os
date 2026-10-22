package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	domain "github.com/lynx/lynx-os/backend/internal/domain/identity"
)

type IdentityRepository struct{ pool *pgxpool.Pool }

func NewIdentityRepository(pool *pgxpool.Pool) *IdentityRepository {
	return &IdentityRepository{pool: pool}
}

func (r *IdentityRepository) CreateInitialOwner(ctx context.Context, user domain.User) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(913570129)`); err != nil {
		return err
	}
	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM admin_users WHERE role='owner')`).Scan(&exists); err != nil {
		return err
	}
	if exists {
		return domain.ErrOwnerAlreadyExists
	}
	_, err = tx.Exec(ctx, `INSERT INTO admin_users (id,email,password_hash,display_name,role,active,created_at,updated_at) VALUES ($1,$2,$3,$4,'owner',true,$5,$5)`,
		user.ID, user.Email, user.PasswordHash, user.DisplayName, user.CreatedAt)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *IdentityRepository) FindByEmail(ctx context.Context, email string) (domain.User, error) {
	var user domain.User
	err := r.pool.QueryRow(ctx, `SELECT id,email,password_hash,display_name,role,active,created_at,last_login_at FROM admin_users WHERE lower(email)=lower($1)`, email).
		Scan(&user.ID, &user.Email, &user.PasswordHash, &user.DisplayName, &user.Role, &user.Active, &user.CreatedAt, &user.LastLoginAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.User{}, domain.ErrUserNotFound
	}
	return user, err
}

func (r *IdentityRepository) FindByID(ctx context.Context, id string) (domain.User, error) {
	var user domain.User
	err := r.pool.QueryRow(ctx, `SELECT id,email,password_hash,display_name,role,active,created_at,last_login_at FROM admin_users WHERE id=$1`, id).
		Scan(&user.ID, &user.Email, &user.PasswordHash, &user.DisplayName, &user.Role, &user.Active, &user.CreatedAt, &user.LastLoginAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.User{}, domain.ErrUserNotFound
	}
	return user, err
}

func (r *IdentityRepository) TouchLastLogin(ctx context.Context, id string) error {
	_, err := r.pool.Exec(ctx, `UPDATE admin_users SET last_login_at=now(),updated_at=now() WHERE id=$1 AND active=true`, id)
	return err
}

func (r *IdentityRepository) CreateSession(ctx context.Context, session domain.Session) error {
	_, err := r.pool.Exec(ctx, `INSERT INTO admin_sessions (id,user_id,family_id,refresh_token_hash,expires_at,ip_address,user_agent,created_at,last_used_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		session.ID, session.UserID, session.FamilyID, session.RefreshTokenHash, session.ExpiresAt, session.IPAddress, session.UserAgent, session.CreatedAt, session.LastUsedAt)
	return err
}

func (r *IdentityRepository) GetActiveSession(ctx context.Context, id string) (domain.Session, domain.User, error) {
	var session domain.Session
	var user domain.User
	err := r.pool.QueryRow(ctx, `
		SELECT s.id,s.family_id,s.user_id,s.refresh_token_hash,s.expires_at,s.created_at,s.last_used_at,s.revoked_at,s.ip_address,s.user_agent,
		       u.id,u.email,u.password_hash,u.display_name,u.role,u.active,u.created_at,u.last_login_at
		FROM admin_sessions s JOIN admin_users u ON u.id=s.user_id
		WHERE s.id=$1 AND s.revoked_at IS NULL AND s.expires_at>now() AND u.active=true`, id,
	).Scan(&session.ID, &session.FamilyID, &session.UserID, &session.RefreshTokenHash, &session.ExpiresAt, &session.CreatedAt, &session.LastUsedAt, &session.RevokedAt, &session.IPAddress, &session.UserAgent,
		&user.ID, &user.Email, &user.PasswordHash, &user.DisplayName, &user.Role, &user.Active, &user.CreatedAt, &user.LastLoginAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Session{}, domain.User{}, domain.ErrSessionRevoked
	}
	if err == nil {
		_, _ = r.pool.Exec(ctx, `UPDATE admin_sessions SET last_used_at=now() WHERE id=$1`, id)
	}
	return session, user, err
}

func (r *IdentityRepository) RotateSession(ctx context.Context, oldHash string, next domain.Session) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var old domain.Session
	var userActive bool
	err = tx.QueryRow(ctx, `
		SELECT s.id,s.user_id,s.family_id,s.expires_at,s.revoked_at,u.active
		FROM admin_sessions s JOIN admin_users u ON u.id=s.user_id
		WHERE s.refresh_token_hash=$1 FOR UPDATE OF s`, oldHash).
		Scan(&old.ID, &old.UserID, &old.FamilyID, &old.ExpiresAt, &old.RevokedAt, &userActive)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrInvalidRefresh
	}
	if err != nil {
		return err
	}
	if old.RevokedAt != nil {
		if _, err := tx.Exec(ctx, `UPDATE admin_sessions SET revoked_at=COALESCE(revoked_at,now()) WHERE family_id=$1`, old.FamilyID); err != nil {
			return err
		}
		if err := tx.Commit(ctx); err != nil {
			return err
		}
		return domain.ErrRefreshReuse
	}
	if !userActive || !old.ExpiresAt.After(time.Now()) {
		return domain.ErrInvalidRefresh
	}
	if _, err := tx.Exec(ctx, `UPDATE admin_sessions SET revoked_at=now(),last_used_at=now() WHERE id=$1`, old.ID); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO admin_sessions (id,user_id,family_id,refresh_token_hash,expires_at,ip_address,user_agent,created_at,last_used_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		next.ID, old.UserID, old.FamilyID, next.RefreshTokenHash, next.ExpiresAt, next.IPAddress, next.UserAgent, next.CreatedAt, next.LastUsedAt)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *IdentityRepository) RevokeSession(ctx context.Context, userID, sessionID string) error {
	_, err := r.pool.Exec(ctx, `UPDATE admin_sessions SET revoked_at=COALESCE(revoked_at,now()) WHERE user_id=$1 AND id=$2`, userID, sessionID)
	return err
}

func (r *IdentityRepository) RevokeSessionByRefreshHash(ctx context.Context, tokenHash string) error {
	_, err := r.pool.Exec(ctx, `UPDATE admin_sessions SET revoked_at=COALESCE(revoked_at,now()) WHERE refresh_token_hash=$1`, tokenHash)
	return err
}

func (r *IdentityRepository) RevokeAllSessions(ctx context.Context, userID string) error {
	_, err := r.pool.Exec(ctx, `UPDATE admin_sessions SET revoked_at=COALESCE(revoked_at,now()) WHERE user_id=$1 AND revoked_at IS NULL`, userID)
	return err
}

func (r *IdentityRepository) ListSessions(ctx context.Context, userID string) ([]domain.Session, error) {
	rows, err := r.pool.Query(ctx, `SELECT id,family_id,user_id,expires_at,created_at,last_used_at,revoked_at,ip_address,user_agent FROM admin_sessions WHERE user_id=$1 ORDER BY created_at DESC LIMIT 100`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	sessions := make([]domain.Session, 0)
	for rows.Next() {
		var session domain.Session
		if err := rows.Scan(&session.ID, &session.FamilyID, &session.UserID, &session.ExpiresAt, &session.CreatedAt, &session.LastUsedAt, &session.RevokedAt, &session.IPAddress, &session.UserAgent); err != nil {
			return nil, err
		}
		sessions = append(sessions, session)
	}
	return sessions, rows.Err()
}
