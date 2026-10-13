// Copyright 2026 Teamgram Authors
// All rights reserved.

package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/botapi"
	"github.com/teamgram/teamgram-server/pkg/storage/postgres"
)

const (
	defaultLimit        = 100
	maxLimit            = 100
	webhookDeliverySize = 100
)

type Store struct {
	Pool       *pgxpool.Pool
	ownsPool   bool
	deliverMu  sync.Mutex
	delivering map[int64]struct{}
	userLoader func(context.Context, int64) (*mtproto.ImmutableUser, error)
}

type Webhook struct {
	URL                          string
	MaxConnections               int
	AllowedUpdates               []string
	SecretToken                  string
	LastErrorDate                int64
	LastErrorMessage             string
	LastSynchronizationErrorDate int64
}

type encodedUpdate struct {
	data      []byte
	dedupeKey string
}

func New(ctx context.Context, cfg postgres.Config) (*Store, error) {
	pool, err := postgres.NewPool(ctx, cfg)
	if err != nil {
		return nil, err
	}
	if err := postgres.VerifySchema(ctx, pool,
		`SELECT bot_id,next_update_id FROM bot_api_update_counters LIMIT 0`,
		`SELECT bot_id,update_id,update_data,created_at FROM bot_api_updates LIMIT 0`,
		`SELECT bot_id,dedupe_key,first_update_id,created_at FROM bot_api_update_dedup LIMIT 0`,
		`SELECT bot_id,url,max_connections,allowed_updates,secret_token,last_error_date,last_error_message,last_synchronization_error_date FROM bot_api_webhooks LIMIT 0`,
		`SELECT bot_id,menu_button_type,menu_button_text,menu_button_url FROM bots LIMIT 0`,
		`SELECT bot_id,chat_id,button_type,button_text,button_url FROM bot_api_chat_menu_buttons LIMIT 0`,
		`SELECT bot_id,group_rights,broadcast_rights FROM bot_api_default_admin_rights LIMIT 0`); err != nil {
		pool.Close()
		return nil, err
	}
	return &Store{Pool: pool, ownsPool: true, delivering: make(map[int64]struct{})}, nil
}

func NewWithPool(pool *pgxpool.Pool) *Store {
	return &Store{Pool: pool, delivering: make(map[int64]struct{})}
}

// SetUserLoader configures the authoritative user lookup used to hydrate
// business connection updates before they are serialized into the Bot API
// queue. It is normally set once during sync service startup.
func (s *Store) SetUserLoader(loader func(context.Context, int64) (*mtproto.ImmutableUser, error)) {
	if s != nil {
		s.userLoader = loader
	}
}

func (s *Store) Close() {
	if s != nil && s.ownsPool && s.Pool != nil {
		s.Pool.Close()
	}
}

func (s *Store) Append(ctx context.Context, botID int64, updates *mtproto.Updates) error {
	_, err := s.AppendOnce(ctx, botID, updates)
	return err
}

// AppendOnce persists bot updates in one transaction and reports whether at
// least one update was newly appended. Payment query updates carry a durable
// query key, so retries after an outbox claim or process crash are ignored
// even when the original queue row was already consumed by getUpdates or a
// webhook.
func (s *Store) AppendOnce(ctx context.Context, botID int64, updates *mtproto.Updates) (bool, error) {
	if s == nil || s.Pool == nil {
		return false, errors.New("botapi: update store is not configured")
	}
	if botID <= 0 || updates == nil {
		return false, mtproto.ErrInputRequestInvalid
	}
	encoded := make([]encodedUpdate, 0)
	mtproto.VisitUpdates(botID, updates, map[string]mtproto.UpdateVisitedFunc{
		mtproto.Predicate_updateNewMessage: func(_ int64, update *mtproto.Update, users []*mtproto.User, chats []*mtproto.Chat, _ int32) {
			s.appendConverted(ctx, &encoded, update, users, chats)
		},
		mtproto.Predicate_updateNewChannelMessage: func(_ int64, update *mtproto.Update, users []*mtproto.User, chats []*mtproto.Chat, _ int32) {
			s.appendConverted(ctx, &encoded, update, users, chats)
		},
		mtproto.Predicate_updateEditMessage: func(_ int64, update *mtproto.Update, users []*mtproto.User, chats []*mtproto.Chat, _ int32) {
			s.appendConverted(ctx, &encoded, update, users, chats)
		},
		mtproto.Predicate_updateEditChannelMessage: func(_ int64, update *mtproto.Update, users []*mtproto.User, chats []*mtproto.Chat, _ int32) {
			s.appendConverted(ctx, &encoded, update, users, chats)
		},
		mtproto.Predicate_updateBotInlineQuery: func(_ int64, update *mtproto.Update, users []*mtproto.User, chats []*mtproto.Chat, _ int32) {
			s.appendConverted(ctx, &encoded, update, users, chats)
		},
		mtproto.Predicate_updateBotInlineSend: func(_ int64, update *mtproto.Update, users []*mtproto.User, chats []*mtproto.Chat, _ int32) {
			s.appendConverted(ctx, &encoded, update, users, chats)
		},
		mtproto.Predicate_updateBotCallbackQuery: func(_ int64, update *mtproto.Update, users []*mtproto.User, chats []*mtproto.Chat, _ int32) {
			s.appendConverted(ctx, &encoded, update, users, chats)
		},
		mtproto.Predicate_updateBotWebhookJSONQuery: func(_ int64, update *mtproto.Update, users []*mtproto.User, chats []*mtproto.Chat, _ int32) {
			s.appendConverted(ctx, &encoded, update, users, chats)
		},
		mtproto.Predicate_updateBotShippingQuery: func(_ int64, update *mtproto.Update, users []*mtproto.User, chats []*mtproto.Chat, _ int32) {
			s.appendConverted(ctx, &encoded, update, users, chats)
		},
		mtproto.Predicate_updateBotPrecheckoutQuery: func(_ int64, update *mtproto.Update, users []*mtproto.User, chats []*mtproto.Chat, _ int32) {
			s.appendConverted(ctx, &encoded, update, users, chats)
		},
		mtproto.Predicate_updateBotBusinessConnect: func(_ int64, update *mtproto.Update, users []*mtproto.User, chats []*mtproto.Chat, _ int32) {
			s.appendConverted(ctx, &encoded, update, users, chats)
		},
		mtproto.Predicate_updateBotNewBusinessMessage: func(_ int64, update *mtproto.Update, users []*mtproto.User, chats []*mtproto.Chat, _ int32) {
			s.appendConverted(ctx, &encoded, update, users, chats)
		},
		mtproto.Predicate_updateBotEditBusinessMessage: func(_ int64, update *mtproto.Update, users []*mtproto.User, chats []*mtproto.Chat, _ int32) {
			s.appendConverted(ctx, &encoded, update, users, chats)
		},
		mtproto.Predicate_updateBotDeleteBusinessMessage: func(_ int64, update *mtproto.Update, users []*mtproto.User, chats []*mtproto.Chat, _ int32) {
			s.appendConverted(ctx, &encoded, update, users, chats)
		},
		mtproto.Predicate_updateBusinessBotCallbackQuery: func(_ int64, update *mtproto.Update, users []*mtproto.User, chats []*mtproto.Chat, _ int32) {
			s.appendConverted(ctx, &encoded, update, users, chats)
		},
	})
	if len(encoded) == 0 {
		return false, nil
	}
	inserted := false
	err := postgres.WithTx(ctx, s.Pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO bot_api_update_counters(bot_id,next_update_id)
VALUES($1,1) ON CONFLICT (bot_id) DO NOTHING`, botID); err != nil {
			return err
		}
		// Serialize allocation and the durable query-key check per bot. This
		// prevents concurrent retries from both allocating an update for the
		// same shipping/precheckout query.
		if _, err := tx.Exec(ctx, `SELECT next_update_id FROM bot_api_update_counters
WHERE bot_id=$1 FOR UPDATE`, botID); err != nil {
			return err
		}
		for _, item := range encoded {
			if item.dedupeKey != "" {
				var exists bool
				if err := tx.QueryRow(ctx, `SELECT EXISTS(
SELECT 1 FROM bot_api_update_dedup WHERE bot_id=$1 AND dedupe_key=$2
)`, botID, item.dedupeKey).Scan(&exists); err != nil {
					return err
				}
				if exists {
					continue
				}
			}
			var updateID int64
			if err := tx.QueryRow(ctx, `UPDATE bot_api_update_counters
SET next_update_id=next_update_id+1 WHERE bot_id=$1 RETURNING next_update_id-1`, botID).Scan(&updateID); err != nil {
				return err
			}
			var value map[string]json.RawMessage
			if err := json.Unmarshal(item.data, &value); err != nil {
				return err
			}
			value["update_id"], _ = json.Marshal(updateID)
			data, err := json.Marshal(value)
			if err != nil {
				return err
			}
			if item.dedupeKey != "" {
				if _, err := tx.Exec(ctx, `INSERT INTO bot_api_update_dedup(bot_id,dedupe_key,first_update_id)
VALUES($1,$2,$3)`, botID, item.dedupeKey, updateID); err != nil {
					return err
				}
			}
			if _, err := tx.Exec(ctx, `INSERT INTO bot_api_updates(bot_id,update_id,update_data)
VALUES($1,$2,$3::jsonb)`, botID, updateID, string(data)); err != nil {
				return err
			}
			inserted = true
		}
		return nil
	})
	return inserted, err
}

func (s *Store) appendConverted(ctx context.Context, encoded *[]encodedUpdate, update *mtproto.Update, users []*mtproto.User, chats []*mtproto.Chat) {
	if update != nil && update.GetPredicateName() == mtproto.Predicate_updateBotBusinessConnect && s.userLoader != nil {
		if connection := update.GetConnection(); connection != nil && connection.GetUserId() > 0 {
			if immutable, err := s.userLoader(ctx, connection.GetUserId()); err == nil && immutable != nil && immutable.GetUser() != nil {
				hydrated := immutable.ToUser(0)
				if hydrated != nil {
					users = append([]*mtproto.User{hydrated}, users...)
				}
			}
		}
	}

	converted, ok := botapi.UpdateFromMTProto(update, users, chats)
	if !ok {
		return
	}
	data, err := botapi.EncodeUpdate(converted)
	if err == nil && len(data) > 0 {
		*encoded = append(*encoded, encodedUpdate{data: data, dedupeKey: botUpdateDedupeKey(update)})
	}
}

func botUpdateDedupeKey(update *mtproto.Update) string {
	if update == nil {
		return ""
	}
	switch update.GetPredicateName() {
	case mtproto.Predicate_updateBotShippingQuery:
		if queryID := update.GetQueryId_INT64(); queryID > 0 {
			return fmt.Sprintf("shipping_query:%d", queryID)
		}
	case mtproto.Predicate_updateBotPrecheckoutQuery:
		if queryID := update.GetQueryId_INT64(); queryID > 0 {
			return fmt.Sprintf("precheckout_query:%d", queryID)
		}
	default:
	}
	return ""
}

func (s *Store) GetUpdates(ctx context.Context, botID, offset int64, limit int, allowed []string) ([]json.RawMessage, error) {
	if s == nil || s.Pool == nil {
		return nil, errors.New("botapi: update store is not configured")
	}
	if botID <= 0 {
		return nil, mtproto.ErrInputRequestInvalid
	}
	for _, name := range allowed {
		if !botapi.IsValidAllowedUpdateType(name) {
			return nil, mtproto.ErrInputRequestInvalid
		}
	}
	if limit <= 0 {
		limit = defaultLimit
	}
	if limit > maxLimit {
		limit = maxLimit
	}
	return queryUpdates(ctx, s.Pool, botID, offset, limit, allowed)
}

func queryUpdates(ctx context.Context, db interface {
	Begin(context.Context) (pgx.Tx, error)
}, botID, offset int64, limit int, allowed []string) ([]json.RawMessage, error) {
	tx, err := db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if offset > 0 {
		if _, err := tx.Exec(ctx, `DELETE FROM bot_api_updates WHERE bot_id=$1 AND update_id<$2`, botID, offset); err != nil {
			return nil, err
		}
	} else if offset < 0 {
		if _, err := tx.Exec(ctx, `DELETE FROM bot_api_updates
WHERE bot_id=$1 AND update_id < COALESCE((SELECT MAX(update_id) FROM bot_api_updates WHERE bot_id=$1),0)+$2+1`, botID, offset); err != nil {
			return nil, err
		}
	}
	rows, err := tx.Query(ctx, `SELECT update_data FROM bot_api_updates
WHERE bot_id=$1 ORDER BY update_id LIMIT $2`, botID, limit*4)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	allowedSet := make(map[string]struct{}, len(allowed))
	for _, name := range allowed {
		allowedSet[name] = struct{}{}
	}
	result := make([]json.RawMessage, 0, limit)
	for rows.Next() && len(result) < limit {
		var data []byte
		if err := rows.Scan(&data); err != nil {
			return nil, err
		}
		if len(allowedSet) != 0 && !allowedUpdate(data, allowedSet) {
			continue
		}
		result = append(result, json.RawMessage(data))
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return result, nil
}

func allowedUpdate(data []byte, allowed map[string]struct{}) bool {
	var value map[string]json.RawMessage
	if json.Unmarshal(data, &value) != nil {
		return false
	}
	for name := range allowed {
		if _, ok := value[name]; ok {
			return true
		}
	}
	return false
}

func (s *Store) DropPending(ctx context.Context, botID int64) error {
	if s == nil || s.Pool == nil {
		return errors.New("botapi: update store is not configured")
	}
	_, err := s.Pool.Exec(ctx, `DELETE FROM bot_api_updates WHERE bot_id=$1`, botID)
	return err
}

func (s *Store) SetWebhook(ctx context.Context, botID int64, webhook Webhook, dropPending bool) error {
	if s == nil || s.Pool == nil || botID <= 0 || strings.TrimSpace(webhook.URL) == "" {
		return mtproto.ErrInputRequestInvalid
	}
	parsedURL, err := url.ParseRequestURI(webhook.URL)
	if err != nil || parsedURL.Host == "" || parsedURL.Scheme != "https" {
		return mtproto.ErrInputRequestInvalid
	}
	if webhook.MaxConnections <= 0 {
		webhook.MaxConnections = 40
	}
	if webhook.MaxConnections > 100 {
		return mtproto.ErrInputRequestInvalid
	}
	for _, name := range webhook.AllowedUpdates {
		if !botapi.IsValidAllowedUpdateType(name) {
			return mtproto.ErrInputRequestInvalid
		}
	}
	allowed, err := json.Marshal(webhook.AllowedUpdates)
	if err != nil {
		return err
	}
	return postgres.WithTx(ctx, s.Pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		if dropPending {
			if _, err := tx.Exec(ctx, `DELETE FROM bot_api_updates WHERE bot_id=$1`, botID); err != nil {
				return err
			}
		}
		_, err := tx.Exec(ctx, `INSERT INTO bot_api_webhooks
(bot_id,url,max_connections,allowed_updates,secret_token,last_error_date,last_error_message,last_synchronization_error_date)
VALUES($1,$2,$3,$4::jsonb,$5,0,'',0)
ON CONFLICT(bot_id) DO UPDATE SET url=EXCLUDED.url,max_connections=EXCLUDED.max_connections,
allowed_updates=EXCLUDED.allowed_updates,secret_token=EXCLUDED.secret_token`,
			botID, webhook.URL, webhook.MaxConnections, string(allowed), webhook.SecretToken)
		return err
	})
}

func (s *Store) DeleteWebhook(ctx context.Context, botID int64, dropPending bool) error {
	if s == nil || s.Pool == nil || botID <= 0 {
		return mtproto.ErrInputRequestInvalid
	}
	return postgres.WithTx(ctx, s.Pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		if dropPending {
			if _, err := tx.Exec(ctx, `DELETE FROM bot_api_updates WHERE bot_id=$1`, botID); err != nil {
				return err
			}
		}
		_, err := tx.Exec(ctx, `DELETE FROM bot_api_webhooks WHERE bot_id=$1`, botID)
		return err
	})
}

func (s *Store) GetWebhook(ctx context.Context, botID int64) (*Webhook, int, error) {
	if s == nil || s.Pool == nil || botID <= 0 {
		return nil, 0, mtproto.ErrInputRequestInvalid
	}
	webhook := new(Webhook)
	var allowed []byte
	err := s.Pool.QueryRow(ctx, `SELECT url,max_connections,allowed_updates,secret_token,last_error_date,last_error_message,last_synchronization_error_date
FROM bot_api_webhooks WHERE bot_id=$1`, botID).Scan(&webhook.URL, &webhook.MaxConnections, &allowed, &webhook.SecretToken,
		&webhook.LastErrorDate, &webhook.LastErrorMessage, &webhook.LastSynchronizationErrorDate)
	if errors.Is(err, pgx.ErrNoRows) {
		webhook = &Webhook{}
	} else if err != nil {
		return nil, 0, err
	} else if len(allowed) > 0 {
		if err := json.Unmarshal(allowed, &webhook.AllowedUpdates); err != nil {
			return nil, 0, err
		}
	}
	var pending int
	if err := s.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM bot_api_updates WHERE bot_id=$1`, botID).Scan(&pending); err != nil {
		return nil, 0, err
	}
	return webhook, pending, nil
}

func (s *Store) WebhookInfo(ctx context.Context, botID int64) (*botapi.WebhookInfo, error) {
	webhook, pending, err := s.GetWebhook(ctx, botID)
	if err != nil {
		return nil, err
	}
	info := &botapi.WebhookInfo{
		URL:                          webhook.URL,
		PendingUpdateCount:           pending,
		LastErrorDate:                webhook.LastErrorDate,
		LastErrorMessage:             webhook.LastErrorMessage,
		LastSynchronizationErrorDate: webhook.LastSynchronizationErrorDate,
		MaxConnections:               webhook.MaxConnections,
		AllowedUpdates:               webhook.AllowedUpdates,
	}
	if parsed, err := url.Parse(webhook.URL); err == nil {
		if host, _, err := net.SplitHostPort(parsed.Host); err == nil {
			info.IPAddress = host
		} else {
			info.IPAddress = parsed.Hostname()
		}
	}
	return info, nil
}

// SetBotMenuButton persists either the default button or a chat-scoped button.
// chatID=0 addresses the bot's default scope.
func (s *Store) SetBotMenuButton(ctx context.Context, botID, chatID int64, button botapi.MenuButton) error {
	if s == nil || s.Pool == nil || botID <= 0 {
		return mtproto.ErrInputRequestInvalid
	}
	if button.Type == "" {
		button.Type = "default"
	}
	if err := validateMenuButton(button); err != nil {
		return err
	}
	return postgres.WithTx(ctx, s.Pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		text, rawURL := menuButtonValues(button)
		if chatID != 0 {
			_, err := tx.Exec(ctx, `INSERT INTO bot_api_chat_menu_buttons
(bot_id,chat_id,button_type,button_text,button_url)
VALUES($1,$2,$3,$4,$5)
ON CONFLICT(bot_id,chat_id) DO UPDATE SET button_type=EXCLUDED.button_type,
button_text=EXCLUDED.button_text,button_url=EXCLUDED.button_url,updated_at=CURRENT_TIMESTAMP`,
				botID, chatID, button.Type, text, rawURL)
			return err
		}
		hasButton := button.Type == "web_app"
		result, err := tx.Exec(ctx, `UPDATE bots SET has_menu_button=$2,menu_button_type=$3,
menu_button_text=$4,menu_button_url=$5,
main_app_url=CASE WHEN $2 THEN $5 ELSE main_app_url END,
bot_has_main_app=CASE WHEN $2 THEN TRUE ELSE bot_has_main_app END WHERE bot_id=$1`,
			botID, hasButton, button.Type, text, rawURL)
		if err != nil {
			return err
		}
		if result.RowsAffected() == 0 {
			return mtproto.ErrBotInvalid
		}
		return nil
	})
}

func (s *Store) GetBotMenuButton(ctx context.Context, botID, chatID int64) (botapi.MenuButton, error) {
	if s == nil || s.Pool == nil || botID <= 0 {
		return botapi.MenuButton{}, mtproto.ErrInputRequestInvalid
	}
	var buttonType, text, rawURL string
	if chatID != 0 {
		err := s.Pool.QueryRow(ctx, `SELECT button_type,button_text,button_url
FROM bot_api_chat_menu_buttons WHERE bot_id=$1 AND chat_id=$2`, botID, chatID).
			Scan(&buttonType, &text, &rawURL)
		if err == nil {
			return menuButtonFromValues(buttonType, text, rawURL), nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return botapi.MenuButton{}, err
		}
	}
	if err := s.Pool.QueryRow(ctx, `SELECT menu_button_type,menu_button_text,menu_button_url
FROM bots WHERE bot_id=$1`, botID).Scan(&buttonType, &text, &rawURL); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return botapi.MenuButton{Type: "default"}, nil
		}
		return botapi.MenuButton{}, err
	}
	if buttonType == "" {
		buttonType = "default"
	}
	if buttonType != "web_app" || strings.TrimSpace(rawURL) == "" {
		if buttonType != "commands" {
			buttonType = "default"
		}
		return botapi.MenuButton{Type: buttonType}, nil
	}
	return menuButtonFromValues(buttonType, text, rawURL), nil
}

func (s *Store) SetDefaultAdministratorRights(ctx context.Context, botID int64, forChannels bool, rights []byte) error {
	if s == nil || s.Pool == nil || botID <= 0 || !json.Valid(rights) {
		return mtproto.ErrInputRequestInvalid
	}
	column := "group_rights"
	if forChannels {
		column = "broadcast_rights"
	}
	return postgres.WithTx(ctx, s.Pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO bot_api_default_admin_rights (bot_id,`+column+`)
VALUES($1,$2::jsonb)
ON CONFLICT(bot_id) DO UPDATE SET `+column+`=EXCLUDED.`+column+`,updated_at=CURRENT_TIMESTAMP`, botID, string(rights))
		return err
	})
}

func (s *Store) GetDefaultAdministratorRights(ctx context.Context, botID int64, forChannels bool) ([]byte, bool, error) {
	if s == nil || s.Pool == nil || botID <= 0 {
		return nil, false, mtproto.ErrInputRequestInvalid
	}
	column := "group_rights"
	if forChannels {
		column = "broadcast_rights"
	}
	var rights []byte
	err := s.Pool.QueryRow(ctx, `SELECT `+column+` FROM bot_api_default_admin_rights WHERE bot_id=$1`, botID).Scan(&rights)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return rights, true, nil
}

func validateMenuButton(button botapi.MenuButton) error {
	if button.Type != "default" && button.Type != "commands" && button.Type != "web_app" {
		return mtproto.ErrInputRequestInvalid
	}
	if button.Type != "web_app" {
		if button.Text != "" || button.WebApp != nil {
			return mtproto.ErrInputRequestInvalid
		}
		return nil
	}
	if button.WebApp == nil || strings.TrimSpace(button.WebApp.URL) == "" || strings.TrimSpace(button.Text) == "" {
		return mtproto.ErrInputRequestInvalid
	}
	parsed, err := url.ParseRequestURI(button.WebApp.URL)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return mtproto.ErrWebdocumentUrlInvalid
	}
	return nil
}

func menuButtonValues(button botapi.MenuButton) (string, string) {
	if button.Type == "web_app" && button.WebApp != nil {
		return strings.TrimSpace(button.Text), strings.TrimSpace(button.WebApp.URL)
	}
	return "", ""
}

func menuButtonFromValues(buttonType, text, rawURL string) botapi.MenuButton {
	if buttonType == "web_app" {
		return botapi.MenuButton{Type: "web_app", Text: text, WebApp: &botapi.WebAppInfo{URL: rawURL}}
	}
	if buttonType == "commands" {
		return botapi.MenuButton{Type: "commands"}
	}
	return botapi.MenuButton{Type: "default"}
}

func (s *Store) GetBotAbout(ctx context.Context, botID int64) (string, error) {
	if s == nil || s.Pool == nil || botID <= 0 {
		return "", mtproto.ErrInputRequestInvalid
	}
	var about string
	if err := s.Pool.QueryRow(ctx, `SELECT about FROM users WHERE id=$1 AND is_bot=TRUE AND deleted=FALSE`, botID).Scan(&about); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", mtproto.ErrBotInvalid
		}
		return "", err
	}
	return about, nil
}

// DeliverWebhook drains the oldest pending updates after a successful append.
// The per-process guard prevents duplicate delivery when several sync workers
// observe the same bot at once. PostgreSQL deletion remains conditional on the
// update id, so a failed request stays queued for a later retry.
func (s *Store) DeliverWebhook(ctx context.Context, botID int64) {
	if s == nil || s.Pool == nil || botID <= 0 {
		return
	}
	s.deliverMu.Lock()
	if _, ok := s.delivering[botID]; ok {
		s.deliverMu.Unlock()
		return
	}
	s.delivering[botID] = struct{}{}
	s.deliverMu.Unlock()
	defer func() {
		s.deliverMu.Lock()
		delete(s.delivering, botID)
		s.deliverMu.Unlock()
	}()
	webhook, _, err := s.GetWebhook(ctx, botID)
	if err != nil || webhook == nil || webhook.URL == "" {
		return
	}
	rows, err := s.Pool.Query(ctx, `SELECT update_id,update_data FROM bot_api_updates
WHERE bot_id=$1 ORDER BY update_id LIMIT $2`, botID, webhookDeliverySize)
	if err != nil {
		return
	}
	defer rows.Close()
	type pendingUpdate struct {
		id   int64
		data []byte
	}
	items := make([]pendingUpdate, 0, webhookDeliverySize)
	for rows.Next() {
		var item pendingUpdate
		if err := rows.Scan(&item.id, &item.data); err != nil {
			return
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return
	}
	allowedSet := make(map[string]struct{}, len(webhook.AllowedUpdates))
	for _, name := range webhook.AllowedUpdates {
		allowedSet[name] = struct{}{}
	}
	client := &http.Client{Timeout: 10 * time.Second}
	for _, item := range items {
		if len(allowedSet) > 0 && !allowedUpdate(item.data, allowedSet) {
			// A filtered update must not block delivery of a later allowed
			// update at the head of the queue.
			_, _ = s.Pool.Exec(ctx, `DELETE FROM bot_api_updates WHERE bot_id=$1 AND update_id=$2`, botID, item.id)
			continue
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhook.URL, strings.NewReader(string(item.data)))
		if err != nil {
			s.recordWebhookError(ctx, botID, err)
			return
		}
		req.Header.Set("Content-Type", "application/json")
		if webhook.SecretToken != "" {
			req.Header.Set("X-Telegram-Bot-Api-Secret-Token", webhook.SecretToken)
		}
		resp, err := client.Do(req)
		if err != nil {
			s.recordWebhookError(ctx, botID, err)
			return
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			_ = resp.Body.Close()
			s.recordWebhookError(ctx, botID, fmt.Errorf("webhook returned HTTP %d", resp.StatusCode))
			return
		}
		// Telegram treats any 2xx response as an acknowledged update. A
		// webhook endpoint is not required to return the Bot API response
		// envelope, so an empty or plain-text 200 must also drain the queue.
		_, readErr := io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
		_ = resp.Body.Close()
		if readErr != nil {
			s.recordWebhookError(ctx, botID, readErr)
			return
		}
		_, _ = s.Pool.Exec(ctx, `DELETE FROM bot_api_updates WHERE bot_id=$1 AND update_id=$2`, botID, item.id)
	}
}

func (s *Store) recordWebhookError(ctx context.Context, botID int64, err error) {
	_, _ = s.Pool.Exec(ctx, `UPDATE bot_api_webhooks SET last_error_date=$2,last_error_message=$3,last_synchronization_error_date=$2 WHERE bot_id=$1`, botID, time.Now().Unix(), err.Error())
}
