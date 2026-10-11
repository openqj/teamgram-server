package persist

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

func userEmojiPermissionDB() (*postgresStore, error) {
	store, ok := Default.(*postgresStore)
	if !ok || store == nil || store.db == nil {
		return nil, errors.New("apifull: user emoji status permissions require PostgreSQL")
	}
	return store, nil
}

// SetUserEmojiStatusPermission atomically records the owner's decision for a
// bot. Repeating the same decision is idempotent.
func SetUserEmojiStatusPermission(ctx context.Context, ownerUserID, botUserID int64, enabled bool) error {
	store, err := userEmojiPermissionDB()
	if err != nil {
		return err
	}
	if ownerUserID <= 0 || botUserID <= 0 {
		return errors.New("apifull: invalid user emoji status permission identity")
	}
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`,
		fmt.Sprintf("apifull-user-emoji-permission:%d:%d", ownerUserID, botUserID)); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO apifull_user_emoji_status_permission
		(owner_user_id, bot_user_id, enabled, updated_at) VALUES ($1, $2, $3, CURRENT_TIMESTAMP)
		ON CONFLICT (owner_user_id, bot_user_id) DO UPDATE SET enabled = EXCLUDED.enabled,
		updated_at = CURRENT_TIMESTAMP`, ownerUserID, botUserID, enabled)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func UserEmojiStatusPermission(ctx context.Context, ownerUserID, botUserID int64) (bool, error) {
	store, err := userEmojiPermissionDB()
	if err != nil {
		return false, err
	}
	if ownerUserID <= 0 || botUserID <= 0 {
		return false, errors.New("apifull: invalid user emoji status permission identity")
	}
	var enabled bool
	err = store.db.QueryRowContext(ctx, `SELECT enabled FROM apifull_user_emoji_status_permission
		WHERE owner_user_id = $1 AND bot_user_id = $2`, ownerUserID, botUserID).Scan(&enabled)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return enabled, err
}
