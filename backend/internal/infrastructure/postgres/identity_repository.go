package postgres

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
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

func (r *IdentityRepository) CreatePasswordResetToken(ctx context.Context, userID, tokenHash string, expiresAt time.Time) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `UPDATE admin_password_reset_tokens SET used_at=now() WHERE user_id=$1 AND used_at IS NULL`, userID); err != nil {
		return err
	}
	id, err := randomTextID()
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO admin_password_reset_tokens (id,user_id,token_hash,expires_at) VALUES ($1,$2,$3,$4)`, id, userID, tokenHash, expiresAt)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *IdentityRepository) CompletePasswordReset(ctx context.Context, tokenHash, passwordHash string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var tokenID, userID string
	var expiresAt time.Time
	err = tx.QueryRow(ctx, `SELECT id,user_id,expires_at FROM admin_password_reset_tokens WHERE token_hash=$1 AND used_at IS NULL FOR UPDATE`, tokenHash).Scan(&tokenID, &userID, &expiresAt)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && !expiresAt.After(time.Now())) {
		return domain.ErrInvalidOneTimeToken
	}
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE admin_password_reset_tokens SET used_at=now() WHERE id=$1`, tokenID); err != nil {
		return err
	}
	result, err := tx.Exec(ctx, `UPDATE admin_users SET password_hash=$2,updated_at=now() WHERE id=$1 AND active=true`, userID, passwordHash)
	if err != nil {
		return err
	}
	if result.RowsAffected() != 1 {
		return domain.ErrInvalidOneTimeToken
	}
	if _, err := tx.Exec(ctx, `UPDATE admin_password_reset_tokens SET used_at=COALESCE(used_at,now()) WHERE user_id=$1`, userID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE admin_sessions SET revoked_at=COALESCE(revoked_at,now()) WHERE user_id=$1 AND revoked_at IS NULL`, userID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *IdentityRepository) CreateInvitation(ctx context.Context, invitation domain.Invitation) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended(lower($1), 913570130))`, invitation.Email); err != nil {
		return err
	}
	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM admin_users WHERE lower(email)=lower($1))`, invitation.Email).Scan(&exists); err != nil {
		return err
	}
	if exists {
		return domain.ErrEmailAlreadyUsed
	}
	if _, err := tx.Exec(ctx, `UPDATE admin_invitations SET accepted_at=now() WHERE lower(email)=lower($1) AND accepted_at IS NULL`, invitation.Email); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO admin_invitations (id,email,display_name,role,token_hash,invited_by,expires_at,created_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
		invitation.ID, invitation.Email, invitation.DisplayName, invitation.Role, invitation.TokenHash, invitation.InvitedBy, invitation.ExpiresAt, invitation.CreatedAt)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *IdentityRepository) AcceptInvitation(ctx context.Context, tokenHash, passwordHash string) (domain.User, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.User{}, err
	}
	defer tx.Rollback(ctx)
	var invitation domain.Invitation
	err = tx.QueryRow(ctx, `SELECT id,email,display_name,role,expires_at FROM admin_invitations WHERE token_hash=$1 AND accepted_at IS NULL FOR UPDATE`, tokenHash).
		Scan(&invitation.ID, &invitation.Email, &invitation.DisplayName, &invitation.Role, &invitation.ExpiresAt)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && !invitation.ExpiresAt.After(time.Now())) {
		return domain.User{}, domain.ErrInvalidOneTimeToken
	}
	if err != nil {
		return domain.User{}, err
	}
	now := time.Now().UTC()
	userID, err := randomTextID()
	if err != nil {
		return domain.User{}, err
	}
	user := domain.User{ID: userID, Email: invitation.Email, PasswordHash: passwordHash, DisplayName: invitation.DisplayName, Role: invitation.Role, Active: true, CreatedAt: now}
	_, err = tx.Exec(ctx, `INSERT INTO admin_users (id,email,password_hash,display_name,role,active,created_at,updated_at) VALUES ($1,$2,$3,$4,$5,true,$6,$6)`,
		user.ID, user.Email, user.PasswordHash, user.DisplayName, user.Role, user.CreatedAt)
	var pgError *pgconn.PgError
	if errors.As(err, &pgError) && pgError.Code == "23505" {
		return domain.User{}, domain.ErrEmailAlreadyUsed
	}
	if err != nil {
		return domain.User{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE admin_invitations SET accepted_at=now() WHERE id=$1`, invitation.ID); err != nil {
		return domain.User{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.User{}, err
	}
	return user, nil
}

func (r *IdentityRepository) ListAdmins(ctx context.Context) ([]domain.AdminSummary, error) {
	rows, err := r.pool.Query(ctx, `SELECT id,email,display_name,role,active,created_at,last_login_at FROM admin_users ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	users := make([]domain.AdminSummary, 0)
	for rows.Next() {
		var user domain.AdminSummary
		if err := rows.Scan(&user.ID, &user.Email, &user.DisplayName, &user.Role, &user.Active, &user.CreatedAt, &user.LastLoginAt); err != nil {
			return nil, err
		}
		users = append(users, user)
	}
	return users, rows.Err()
}

func (r *IdentityRepository) UpdateAdmin(ctx context.Context, id, displayName string, role domain.Role, active bool) (domain.AdminSummary, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.AdminSummary{}, err
	}
	defer tx.Rollback(ctx)
	var currentRole domain.Role
	err = tx.QueryRow(ctx, `SELECT role FROM admin_users WHERE id=$1 FOR UPDATE`, id).Scan(&currentRole)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.AdminSummary{}, domain.ErrUserNotFound
	}
	if err != nil {
		return domain.AdminSummary{}, err
	}
	if currentRole == domain.RoleOwner {
		return domain.AdminSummary{}, domain.ErrCannotModifyOwner
	}
	var user domain.AdminSummary
	err = tx.QueryRow(ctx, `UPDATE admin_users SET display_name=$2,role=$3,active=$4,updated_at=now() WHERE id=$1 RETURNING id,email,display_name,role,active,created_at,last_login_at`, id, displayName, role, active).
		Scan(&user.ID, &user.Email, &user.DisplayName, &user.Role, &user.Active, &user.CreatedAt, &user.LastLoginAt)
	if err != nil {
		return domain.AdminSummary{}, err
	}
	if !active {
		if _, err := tx.Exec(ctx, `UPDATE admin_sessions SET revoked_at=COALESCE(revoked_at,now()) WHERE user_id=$1 AND revoked_at IS NULL`, id); err != nil {
			return domain.AdminSummary{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.AdminSummary{}, err
	}
	return user, nil
}

func (r *IdentityRepository) CreateTelegramLinkCode(ctx context.Context, id, userID, tokenHash string, expiresAt time.Time) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var active bool
	if err := tx.QueryRow(ctx, `SELECT active FROM admin_users WHERE id=$1`, userID).Scan(&active); errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrUserNotFound
	} else if err != nil {
		return err
	} else if !active {
		return domain.ErrUserInactive
	}
	if _, err := tx.Exec(ctx, `UPDATE telegram_link_codes SET used_at=now() WHERE user_id=$1 AND used_at IS NULL`, userID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO telegram_link_codes (id,user_id,token_hash,expires_at) VALUES ($1,$2,$3,$4)`, id, userID, tokenHash, expiresAt); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *IdentityRepository) ConsumeTelegramLinkCode(ctx context.Context, tokenHash, telegramUserID string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var codeID, userID string
	err = tx.QueryRow(ctx, `SELECT c.id,c.user_id FROM telegram_link_codes c JOIN admin_users u ON u.id=c.user_id WHERE c.token_hash=$1 AND c.used_at IS NULL AND c.expires_at>now() AND u.active=true FOR UPDATE OF c`, tokenHash).Scan(&codeID, &userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrInvalidOneTimeToken
	}
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	_, err = tx.Exec(ctx, `
		INSERT INTO telegram_identities (id,user_id,telegram_user_id,linked_at,last_seen_at)
		VALUES ($1,$2,$3,$4,$4)
		ON CONFLICT (user_id) DO UPDATE SET telegram_user_id=EXCLUDED.telegram_user_id,linked_at=EXCLUDED.linked_at,last_seen_at=EXCLUDED.last_seen_at`, codeID, userID, telegramUserID, now)
	var pgError *pgconn.PgError
	if errors.As(err, &pgError) && pgError.Code == "23505" {
		return domain.ErrTelegramAlreadyLinked
	}
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE telegram_link_codes SET used_at=now() WHERE id=$1`, codeID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *IdentityRepository) FindByTelegramUserID(ctx context.Context, telegramUserID string) (domain.User, error) {
	var user domain.User
	err := r.pool.QueryRow(ctx, `SELECT u.id,u.email,u.password_hash,u.display_name,u.role,u.active,u.created_at,u.last_login_at
		FROM telegram_identities t JOIN admin_users u ON u.id=t.user_id
		WHERE t.telegram_user_id=$1 AND u.active=true`, telegramUserID).
		Scan(&user.ID,&user.Email,&user.PasswordHash,&user.DisplayName,&user.Role,&user.Active,&user.CreatedAt,&user.LastLoginAt)
	if errors.Is(err, pgx.ErrNoRows) { return domain.User{}, domain.ErrUserNotFound }
	return user,err
}

func randomTextID() (string, error) {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return hex.EncodeToString(value), nil
}
