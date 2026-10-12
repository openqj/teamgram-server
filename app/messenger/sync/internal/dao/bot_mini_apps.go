package dao

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/teamgram/proto/mtproto"
)

type BotMiniApp struct {
	ID          int64
	BotID       int64
	ShortName   string
	Title       string
	Description string
	URL         string
	Domain      string
}

func (d *Postgres) ListBotMiniApps(ctx context.Context, userID int64) ([]BotMiniApp, error) {
	if d == nil || d.Pool == nil || userID <= 0 {
		return nil, mtproto.ErrMethodNotImpl
	}
	rows, err := d.Pool.Query(ctx, `SELECT a.id,a.bot_id,a.short_name,a.title,a.description,a.url,a.domain
FROM bot_mini_apps a
JOIN bots b ON b.bot_id = a.bot_id
WHERE b.creator_user_id = $1
ORDER BY a.updated_at DESC, a.id DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	apps := make([]BotMiniApp, 0)
	for rows.Next() {
		var app BotMiniApp
		if err := rows.Scan(&app.ID, &app.BotID, &app.ShortName, &app.Title, &app.Description, &app.URL, &app.Domain); err != nil {
			return nil, err
		}
		apps = append(apps, app)
	}
	return apps, rows.Err()
}

func (d *Postgres) UpsertBotMiniApp(ctx context.Context, userID int64, app BotMiniApp) error {
	if d == nil || d.Pool == nil || userID <= 0 || app.BotID <= 0 || strings.TrimSpace(app.ShortName) == "" || strings.TrimSpace(app.Title) == "" || strings.TrimSpace(app.URL) == "" {
		return mtproto.ErrInputRequestInvalid
	}
	tx, err := d.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var creatorID int64
	err = tx.QueryRow(ctx, `SELECT creator_user_id FROM bots WHERE bot_id = $1 FOR UPDATE`, app.BotID).Scan(&creatorID)
	if errors.Is(err, pgx.ErrNoRows) {
		return mtproto.ErrBotInvalid
	}
	if err != nil {
		return err
	}
	if creatorID != userID {
		return mtproto.ErrForbiddenUserBotInvalid
	}
	_, err = tx.Exec(ctx, `INSERT INTO bot_mini_apps(bot_id,short_name,title,description,url,domain,created_at,updated_at)
VALUES($1,$2,$3,$4,$5,$6,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)
ON CONFLICT (bot_id,short_name) DO UPDATE SET title=EXCLUDED.title,
description=EXCLUDED.description,url=EXCLUDED.url,domain=EXCLUDED.domain,updated_at=CURRENT_TIMESTAMP`,
		app.BotID, app.ShortName, app.Title, app.Description, app.URL, app.Domain)
	if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE bots SET main_app_url=$1,bot_has_main_app=TRUE WHERE bot_id=$2`, app.URL, app.BotID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (d *Postgres) DeleteBotMiniApp(ctx context.Context, userID, botID int64, shortName string) error {
	if d == nil || d.Pool == nil || userID <= 0 || botID <= 0 {
		return mtproto.ErrInputRequestInvalid
	}
	tx, err := d.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var creatorID int64
	err = tx.QueryRow(ctx, `SELECT creator_user_id FROM bots WHERE bot_id = $1 FOR UPDATE`, botID).Scan(&creatorID)
	if errors.Is(err, pgx.ErrNoRows) {
		return mtproto.ErrBotInvalid
	}
	if err != nil {
		return err
	}
	if creatorID != userID {
		return mtproto.ErrForbiddenUserBotInvalid
	}
	var appID int64
	if strings.TrimSpace(shortName) == "" {
		err = tx.QueryRow(ctx, `SELECT id FROM bot_mini_apps WHERE bot_id=$1 ORDER BY updated_at DESC,id DESC LIMIT 1 FOR UPDATE`, botID).Scan(&appID)
	} else {
		err = tx.QueryRow(ctx, `SELECT id FROM bot_mini_apps WHERE bot_id=$1 AND short_name=$2 FOR UPDATE`, botID, shortName).Scan(&appID)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return mtproto.ErrBotInvalid
	}
	if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `DELETE FROM bot_mini_apps WHERE id=$1`, appID); err != nil {
		return err
	}
	var nextURL string
	err = tx.QueryRow(ctx, `SELECT url FROM bot_mini_apps WHERE bot_id=$1 ORDER BY updated_at DESC,id DESC LIMIT 1`, botID).Scan(&nextURL)
	if errors.Is(err, pgx.ErrNoRows) {
		nextURL = ""
	} else if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE bots SET main_app_url=$1,bot_has_main_app=$2 WHERE bot_id=$3`, nextURL, nextURL != "", botID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (d *Postgres) SetBotMiniAppDomain(ctx context.Context, userID, botID int64, domain string) error {
	if d == nil || d.Pool == nil || userID <= 0 || botID <= 0 || strings.TrimSpace(domain) == "" {
		return mtproto.ErrInputRequestInvalid
	}
	tx, err := d.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var creatorID int64
	err = tx.QueryRow(ctx, `SELECT creator_user_id FROM bots WHERE bot_id=$1 FOR UPDATE`, botID).Scan(&creatorID)
	if errors.Is(err, pgx.ErrNoRows) {
		return mtproto.ErrBotInvalid
	}
	if err != nil {
		return err
	}
	if creatorID != userID {
		return mtproto.ErrForbiddenUserBotInvalid
	}
	var appID int64
	err = tx.QueryRow(ctx, `SELECT id FROM bot_mini_apps WHERE bot_id=$1 ORDER BY updated_at DESC,id DESC LIMIT 1 FOR UPDATE`, botID).Scan(&appID)
	if errors.Is(err, pgx.ErrNoRows) {
		return mtproto.ErrBotInvalid
	}
	if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE bot_mini_apps SET domain=$1,updated_at=CURRENT_TIMESTAMP WHERE id=$2`, domain, appID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
