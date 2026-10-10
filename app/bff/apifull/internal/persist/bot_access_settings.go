package persist

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

// BotAccessSettings is the durable access policy owned by a bot. User IDs are
// stored in request order; the API layer hydrates them into Telegram User
// constructors after validating the caller's access hash.
type BotAccessSettings struct {
	BotID      int64
	Restricted bool
	AddUserIDs []int64
}

func botAccessDB() (*postgresStore, error) {
	store, ok := Default.(*postgresStore)
	if !ok || store == nil || store.db == nil {
		return nil, errors.New("apifull: bot access settings require PostgreSQL")
	}
	return store, nil
}

func decodeBotAccessSettings(botID int64, restricted bool, raw []byte) (*BotAccessSettings, error) {
	ids := []int64{}
	if len(raw) != 0 {
		if err := json.Unmarshal(raw, &ids); err != nil {
			return nil, fmt.Errorf("apifull: decode bot access users: %w", err)
		}
	}
	if ids == nil {
		ids = []int64{}
	}
	return &BotAccessSettings{BotID: botID, Restricted: restricted, AddUserIDs: ids}, nil
}

// GetBotAccessSettings returns the Telegram default (unrestricted and no
// explicit users) when no policy row exists.
func GetBotAccessSettings(ctx context.Context, botID int64) (*BotAccessSettings, error) {
	store, err := botAccessDB()
	if err != nil {
		return nil, err
	}
	if botID <= 0 {
		return nil, errors.New("apifull: invalid bot id")
	}
	var restricted bool
	var raw []byte
	err = store.db.QueryRowContext(ctx, `SELECT restricted, add_users
		FROM apifull_bot_access_settings WHERE bot_user_id = $1`, botID).Scan(&restricted, &raw)
	if err == sql.ErrNoRows {
		return &BotAccessSettings{BotID: botID, AddUserIDs: []int64{}}, nil
	}
	if err != nil {
		return nil, err
	}
	return decodeBotAccessSettings(botID, restricted, raw)
}

// EditBotAccessSettings updates one bot policy atomically. A nil addUserIDs
// means the TL add_users flag was absent and therefore preserves the prior
// list; a non-nil empty slice explicitly clears it.
func EditBotAccessSettings(ctx context.Context, botID int64, restricted bool, addUserIDs []int64) (*BotAccessSettings, error) {
	store, err := botAccessDB()
	if err != nil {
		return nil, err
	}
	if botID <= 0 {
		return nil, errors.New("apifull: invalid bot id")
	}
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`,
		fmt.Sprintf("apifull-bot-access:%d", botID)); err != nil {
		return nil, err
	}
	current := []int64{}
	var oldRaw []byte
	err = tx.QueryRowContext(ctx, `SELECT add_users FROM apifull_bot_access_settings
		WHERE bot_user_id = $1 FOR UPDATE`, botID).Scan(&oldRaw)
	if err == sql.ErrNoRows {
		err = nil
	} else if err != nil {
		return nil, err
	} else if len(oldRaw) != 0 {
		if err = json.Unmarshal(oldRaw, &current); err != nil {
			return nil, fmt.Errorf("apifull: decode existing bot access users: %w", err)
		}
	}
	if addUserIDs == nil {
		addUserIDs = current
	}
	if addUserIDs == nil {
		addUserIDs = []int64{}
	}
	encoded, err := json.Marshal(addUserIDs)
	if err != nil {
		return nil, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO apifull_bot_access_settings
		(bot_user_id, restricted, add_users, updated_at)
		VALUES ($1, $2, $3::jsonb, CURRENT_TIMESTAMP)
		ON CONFLICT (bot_user_id) DO UPDATE SET restricted = EXCLUDED.restricted,
		add_users = EXCLUDED.add_users, updated_at = CURRENT_TIMESTAMP`, botID, restricted, string(encoded))
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return &BotAccessSettings{BotID: botID, Restricted: restricted, AddUserIDs: append([]int64(nil), addUserIDs...)}, nil
}
