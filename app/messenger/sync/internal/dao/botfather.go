package dao

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

type BotFatherSession struct {
	UserID                int64
	State                 string
	PendingName           string
	PendingBotID          int64
	PendingMenuText       string
	PendingMenuURL        string
	PendingAppTitle       string
	PendingAppShortName   string
	PendingAppDescription string
	PendingAppURL         string
	LastMessageID         int32
	LastReply             string
}

func (d *Dao) LoadBotFatherSession(ctx context.Context, userID int64) (*BotFatherSession, error) {
	if d == nil || d.Postgres == nil || d.Pool == nil {
		return nil, errors.New("sync: BotFather session store is not configured")
	}
	session := &BotFatherSession{UserID: userID, State: "idle"}
	err := d.Pool.QueryRow(ctx, `SELECT state,pending_name,pending_bot_id,pending_menu_text,pending_menu_url,pending_app_title,pending_app_short_name,pending_app_description,pending_app_url,last_message_id,last_reply
FROM botfather_sessions WHERE user_id=$1`, userID).Scan(
		&session.State, &session.PendingName, &session.PendingBotID,
		&session.PendingMenuText, &session.PendingMenuURL, &session.PendingAppTitle,
		&session.PendingAppShortName, &session.PendingAppDescription, &session.PendingAppURL,
		&session.LastMessageID, &session.LastReply)
	if errors.Is(err, pgx.ErrNoRows) {
		return session, nil
	}
	if err != nil {
		return nil, err
	}
	return session, nil
}

func (d *Dao) SaveBotFatherSession(ctx context.Context, session *BotFatherSession) error {
	if d == nil || d.Postgres == nil || d.Pool == nil {
		return errors.New("sync: BotFather session store is not configured")
	}
	if session == nil || session.UserID <= 0 || session.State == "" {
		return errors.New("sync: invalid BotFather session")
	}
	tx, err := d.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	_, err = tx.Exec(ctx, `INSERT INTO botfather_sessions
	(user_id,state,pending_name,pending_bot_id,pending_menu_text,pending_menu_url,pending_app_title,pending_app_short_name,pending_app_description,pending_app_url,last_message_id,last_reply)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
ON CONFLICT (user_id) DO UPDATE SET
 state=EXCLUDED.state,
 pending_name=EXCLUDED.pending_name,
 pending_bot_id=EXCLUDED.pending_bot_id,
 pending_menu_text=EXCLUDED.pending_menu_text,
 pending_menu_url=EXCLUDED.pending_menu_url,
	 pending_app_title=EXCLUDED.pending_app_title,
	 pending_app_short_name=EXCLUDED.pending_app_short_name,
	 pending_app_description=EXCLUDED.pending_app_description,
	 pending_app_url=EXCLUDED.pending_app_url,
 last_message_id=EXCLUDED.last_message_id,
 last_reply=EXCLUDED.last_reply,
 updated_at=CURRENT_TIMESTAMP`, session.UserID, session.State, session.PendingName,
		session.PendingBotID, session.PendingMenuText, session.PendingMenuURL,
		session.PendingAppTitle, session.PendingAppShortName, session.PendingAppDescription,
		session.PendingAppURL, session.LastMessageID, session.LastReply)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}
