package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	identity "github.com/lynx/lynx-os/backend/internal/domain/integration"
	telegram "github.com/lynx/lynx-os/backend/internal/domain/telegram"
)

const telegramLeaderLock int64 = 774201662

type TelegramRepository struct{ pool *pgxpool.Pool }

func NewTelegramRepository(pool *pgxpool.Pool) *TelegramRepository {
	return &TelegramRepository{pool: pool}
}

func (r *TelegramRepository) TryAcquireLeader(ctx context.Context) (func(), bool, error) {
	conn, err := r.pool.Acquire(ctx)
	if err != nil {
		return nil, false, err
	}
	var acquired bool
	if err = conn.QueryRow(ctx, `SELECT pg_try_advisory_lock($1)`, telegramLeaderLock).Scan(&acquired); err != nil {
		conn.Release()
		return nil, false, err
	}
	if !acquired {
		conn.Release()
		return nil, false, nil
	}
	return func() {
		_, _ = conn.Exec(context.Background(), `SELECT pg_advisory_unlock($1)`, telegramLeaderLock)
		conn.Release()
	}, true, nil
}

func (r *TelegramRepository) GetConversation(ctx context.Context, telegramID string) (telegram.Conversation, identity.SealedSecret, error) {
	var c telegram.Conversation
	var secret identity.SealedSecret
	err := r.pool.QueryRow(ctx, `SELECT telegram_user_id,admin_user_id,state,bot_message_id,expires_at,draft_ciphertext,draft_iv,draft_auth_tag FROM telegram_bot_conversations WHERE telegram_user_id=$1`, telegramID).
		Scan(&c.TelegramUserID, &c.AdminUserID, &c.State, &c.BotMessageID, &c.ExpiresAt, &secret.Ciphertext, &secret.IV, &secret.AuthTag)
	if errors.Is(err, pgx.ErrNoRows) {
		return telegram.Conversation{}, identity.SealedSecret{}, telegram.ErrConversationNotFound
	}
	return c, secret, err
}

func (r *TelegramRepository) SaveConversation(ctx context.Context, c telegram.Conversation, secret identity.SealedSecret) error {
	_, err := r.pool.Exec(ctx, `INSERT INTO telegram_bot_conversations(telegram_user_id,admin_user_id,state,bot_message_id,expires_at,draft_ciphertext,draft_iv,draft_auth_tag,updated_at)
	VALUES($1,$2,$3,$4,$5,$6,$7,$8,now()) ON CONFLICT(telegram_user_id) DO UPDATE SET admin_user_id=EXCLUDED.admin_user_id,state=EXCLUDED.state,bot_message_id=EXCLUDED.bot_message_id,expires_at=EXCLUDED.expires_at,draft_ciphertext=EXCLUDED.draft_ciphertext,draft_iv=EXCLUDED.draft_iv,draft_auth_tag=EXCLUDED.draft_auth_tag,updated_at=now()`, c.TelegramUserID, c.AdminUserID, c.State, c.BotMessageID, c.ExpiresAt, secret.Ciphertext, secret.IV, secret.AuthTag)
	return err
}

func (r *TelegramRepository) DeleteConversation(ctx context.Context, telegramID string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM telegram_bot_conversations WHERE telegram_user_id=$1`, telegramID)
	return err
}
func (r *TelegramRepository) GetUpdateOffset(ctx context.Context) (int64, error) {
	var offset int64
	err := r.pool.QueryRow(ctx, `SELECT update_offset FROM telegram_bot_state WHERE id=1`).Scan(&offset)
	return offset, err
}
func (r *TelegramRepository) AdvanceUpdateOffset(ctx context.Context, id int64) error {
	_, err := r.pool.Exec(ctx, `UPDATE telegram_bot_state SET update_offset=GREATEST(update_offset,$1),updated_at=now() WHERE id=1`, id+1)
	return err
}
func (r *TelegramRepository) BeginUpdate(ctx context.Context, id int64, chatID string) (bool, error) {
	tag, err := r.pool.Exec(ctx, `INSERT INTO telegram_bot_updates(update_id,chat_id,state) VALUES($1,$2,'processing') ON CONFLICT(update_id) DO UPDATE SET state='processing',created_at=now(),chat_id=EXCLUDED.chat_id WHERE telegram_bot_updates.state='processing' AND telegram_bot_updates.created_at<now()-interval '2 minutes'`, id, chatID)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}
func (r *TelegramRepository) CompleteUpdate(ctx context.Context, id int64, _ string, ignored bool) error {
	state := "processed"
	if ignored {
		state = "ignored"
	}
	_, err := r.pool.Exec(ctx, `UPDATE telegram_bot_updates SET state=$2,processed_at=now() WHERE update_id=$1`, id, state)
	return err
}

var _ telegram.ConversationRepository = (*TelegramRepository)(nil)
