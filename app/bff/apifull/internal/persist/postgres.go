package persist

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
)

// postgresStore keeps APIFull process state in PostgreSQL. The KV table is
// intentionally small: larger domain records are owned by the domain store.
type postgresStore struct {
	db *sql.DB
}

func (s *postgresStore) Get(key string) (string, error) {
	// Story records are stored as one JSONB document per owner. Keep a
	// compatibility read from apifull_kv so deployments can roll forward from
	// the previous blob layout without losing an existing account's stories.
	if userID, ok := storyStateUserID(key); ok {
		var value []byte
		err := s.db.QueryRow(`SELECT state FROM apifull_story_state WHERE user_id = $1`, userID).Scan(&value)
		if err == nil {
			return string(value), nil
		}
		if err != sql.ErrNoRows {
			return "", err
		}
	}
	var value string
	err := s.db.QueryRow(`SELECT v FROM apifull_kv WHERE k = $1`, key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return value, err
}

func (s *postgresStore) Set(key, value string) error {
	if userID, ok := storyStateUserID(key); ok && json.Valid([]byte(value)) {
		_, err := s.db.Exec(`INSERT INTO apifull_story_state (user_id, state, updated_at)
			VALUES ($1, $2::jsonb, CURRENT_TIMESTAMP)
			ON CONFLICT (user_id) DO UPDATE SET state = EXCLUDED.state, updated_at = CURRENT_TIMESTAMP`, userID, value)
		return err
	}
	_, err := s.db.Exec(
		`INSERT INTO apifull_kv (k, v) VALUES ($1, $2)
		 ON CONFLICT (k) DO UPDATE SET v = EXCLUDED.v`,
		key,
		value,
	)
	return err
}

func (s *postgresStore) Update(key string, fn func(string) (string, error)) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err = tx.Exec(`SELECT pg_advisory_xact_lock($1)`, keyLockID(key)); err != nil {
		return err
	}
	current := ""
	if userID, ok := storyStateUserID(key); ok {
		var raw []byte
		err = tx.QueryRow(`SELECT state FROM apifull_story_state WHERE user_id = $1 FOR UPDATE`, userID).Scan(&raw)
		if err == sql.ErrNoRows {
			err = tx.QueryRow(`SELECT v FROM apifull_kv WHERE k = $1 FOR UPDATE`, key).Scan(&current)
			if err == sql.ErrNoRows {
				err = nil
			}
		} else if err == nil {
			current = string(raw)
		}
	} else {
		err = tx.QueryRow(`SELECT v FROM apifull_kv WHERE k = $1 FOR UPDATE`, key).Scan(&current)
		if err == sql.ErrNoRows {
			err = nil
		}
	}
	if err != nil {
		return err
	}
	next, err := fn(current)
	if err != nil {
		return err
	}
	if userID, ok := storyStateUserID(key); ok && json.Valid([]byte(next)) {
		_, err = tx.Exec(`INSERT INTO apifull_story_state (user_id, state, updated_at)
			VALUES ($1, $2::jsonb, CURRENT_TIMESTAMP)
			ON CONFLICT (user_id) DO UPDATE SET state = EXCLUDED.state, updated_at = CURRENT_TIMESTAMP`, userID, next)
	} else {
		_, err = tx.Exec(`INSERT INTO apifull_kv (k, v) VALUES ($1, $2)
			ON CONFLICT (k) DO UPDATE SET v = EXCLUDED.v`, key, next)
	}
	if err != nil {
		return err
	}
	return tx.Commit()
}

func storyStateUserID(key string) (int64, bool) {
	if !strings.HasPrefix(key, "story:") {
		return 0, false
	}
	value, err := strconv.ParseInt(strings.TrimPrefix(key, "story:"), 10, 64)
	return value, err == nil && value > 0
}

func (s *postgresStore) CompareAndDelete(key, expected string) (bool, error) {
	result, err := s.db.Exec(`DELETE FROM apifull_kv WHERE k = $1 AND v = $2`, key, expected)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	return rows == 1, err
}

// keyLockID maps a key to PostgreSQL's transaction-scoped advisory lock
// namespace. A hash is sufficient here because collisions only serialize
// unrelated keys; they cannot change their values or correctness.
func keyLockID(key string) int64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(key))
	return int64(h.Sum64())
}

func (s *postgresStore) CompareAndSwap(key, expected, replacement string) (bool, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()

	// SELECT FOR UPDATE cannot lock an absent row in PostgreSQL. The
	// transaction-scoped advisory lock closes that race across processes before
	// the row is read or inserted.
	if _, err = tx.Exec(`SELECT pg_advisory_xact_lock($1)`, keyLockID(key)); err != nil {
		return false, err
	}

	var current string
	err = tx.QueryRow(`SELECT v FROM apifull_kv WHERE k = $1 FOR UPDATE`, key).Scan(&current)
	if err == sql.ErrNoRows {
		if expected != "" {
			return false, nil
		}
		_, err = tx.Exec(`INSERT INTO apifull_kv (k, v) VALUES ($1, $2)`, key, replacement)
	} else if err != nil {
		return false, err
	} else if current != expected {
		return false, nil
	} else {
		_, err = tx.Exec(`UPDATE apifull_kv SET v = $1 WHERE k = $2`, replacement, key)
	}
	if err != nil {
		return false, err
	}
	if err = tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

var postgresOpenOnce sync.Mutex

var (
	postgresInsertIgnorePattern = regexp.MustCompile(`(?is)^\s*INSERT\s+IGNORE\s+INTO\b`)
	postgresDuplicatePattern    = regexp.MustCompile(`(?is)\s+ON\s+DUPLICATE\s+KEY\s+UPDATE\s+(.+?)\s*;?\s*$`)
	postgresValuesPattern       = regexp.MustCompile(`(?i)VALUES\s*\(\s*([a-z0-9_]+)\s*\)`)
	postgresInsertTablePattern  = regexp.MustCompile(`(?is)\bINSERT\s+INTO\s+([a-z0-9_]+)`)
	postgresGetLockPattern      = regexp.MustCompile(`(?is)GET_LOCK\s*\(\s*([^,()]+)\s*,\s*([^)]*)\)`)
	postgresReleaseLockPattern  = regexp.MustCompile(`(?is)RELEASE_LOCK\s*\(\s*([^()]+)\s*\)`)
)

var postgresConflictTargets = map[string]string{
	"apifull_channel":                 "id",
	"apifull_channel_member":          "channel_id, user_id",
	"apifull_channel_message":         "channel_id, message_id",
	"apifull_channel_read_state":      "user_id, channel_id",
	"apifull_call":                    "id",
	"apifull_group_call":              "id",
	"apifull_group_call_conference":   "call_id",
	"apifull_group_call_invite":       "call_id",
	"apifull_group_call_participant":  "call_id, user_id",
	"apifull_group_call_send_as":      "call_id, user_id",
	"apifull_group_call_settings":     "call_id",
	"apifull_group_call_subscription": "call_id, user_id",
	"apifull_payment_ledger":          "request_id, state",
	"apifull_report":                  "dedupe_key",
	"apifull_stars":                   "user_id",
	"apifull_stars_offer":             "kind, stars, store_product, currency, amount",
	"apifull_star_tx":                 "user_id, idem",
	"apifull_username":                "username",
}

// postgresCompatConnector keeps the existing APIFull query surface portable
// while its stores move from MySQL to PostgreSQL. The legacy code uses the
// database/sql `?` placeholder form; pgx requires PostgreSQL's `$1` form.
// Translating at the driver boundary lets the domain code migrate in focused
// slices without maintaining two copies of every query.
type postgresCompatConnector struct {
	connector driver.Connector
}

func (c postgresCompatConnector) Connect(ctx context.Context) (driver.Conn, error) {
	conn, err := c.connector.Connect(ctx)
	if err != nil {
		return nil, err
	}
	return postgresCompatConn{Conn: conn}, nil
}

func (c postgresCompatConnector) Driver() driver.Driver { return c.connector.Driver() }

type postgresCompatConn struct {
	driver.Conn
}

func (c postgresCompatConn) Prepare(query string) (driver.Stmt, error) {
	return c.Conn.Prepare(rewritePostgresSQL(query))
}

func (c postgresCompatConn) PrepareContext(ctx context.Context, query string) (driver.Stmt, error) {
	if conn, ok := c.Conn.(driver.ConnPrepareContext); ok {
		return conn.PrepareContext(ctx, rewritePostgresSQL(query))
	}
	return c.Prepare(query)
}

func (c postgresCompatConn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	if conn, ok := c.Conn.(driver.ConnBeginTx); ok {
		return conn.BeginTx(ctx, opts)
	}
	return nil, driver.ErrSkip
}

func (c postgresCompatConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	if conn, ok := c.Conn.(driver.ExecerContext); ok {
		return conn.ExecContext(ctx, rewritePostgresSQL(query), args)
	}
	return nil, driver.ErrSkip
}

func (c postgresCompatConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	if conn, ok := c.Conn.(driver.QueryerContext); ok {
		return conn.QueryContext(ctx, rewritePostgresSQL(query), args)
	}
	return nil, driver.ErrSkip
}

func (c postgresCompatConn) Ping(ctx context.Context) error {
	if conn, ok := c.Conn.(driver.Pinger); ok {
		return conn.Ping(ctx)
	}
	return driver.ErrSkip
}

func (c postgresCompatConn) ResetSession(ctx context.Context) error {
	if conn, ok := c.Conn.(driver.SessionResetter); ok {
		return conn.ResetSession(ctx)
	}
	return nil
}

func (c postgresCompatConn) CheckNamedValue(value *driver.NamedValue) error {
	if conn, ok := c.Conn.(driver.NamedValueChecker); ok {
		return conn.CheckNamedValue(value)
	}
	return nil
}

// rewritePostgresSQL handles the small set of MySQL expressions still present
// in the transitional APIFull domain and then changes unquoted `?` markers to
// positional markers. Quoted values and SQL comments are copied verbatim so
// question marks in message text and LIKE patterns remain literal.
func rewritePostgresSQL(query string) string {
	// MySQL treats '\\' as one escaped backslash in a string literal. With
	// PostgreSQL standard_conforming_strings enabled, use an escape string so
	// LIKE ... ESCAPE continues to receive exactly one character.
	query = strings.ReplaceAll(query, `ESCAPE '\\'`, `ESCAPE E'\\'`)
	// Keep the retry schedule in SQL while translating the MySQL date and
	// numeric helpers used by the legacy APIFull query surface.
	query = strings.ReplaceAll(query, `UNIX_TIMESTAMP()`, `(EXTRACT(EPOCH FROM CURRENT_TIMESTAMP)::bigint)`)
	query = strings.ReplaceAll(query, `CAST(POW(2, LEAST(attempts+1, 10)) AS UNSIGNED)`, `power(2::numeric, LEAST(attempts+1, 10))::bigint`)
	query = postgresGetLockPattern.ReplaceAllStringFunc(query, func(expression string) string {
		match := postgresGetLockPattern.FindStringSubmatch(expression)
		return "CASE WHEN " + match[2] + " < 0 THEN NULL::bigint WHEN pg_try_advisory_lock(hashtextextended(" + match[1] + ", 0)) THEN 1 ELSE 0 END"
	})
	query = postgresReleaseLockPattern.ReplaceAllStringFunc(query, func(expression string) string {
		match := postgresReleaseLockPattern.FindStringSubmatch(expression)
		return "CASE WHEN pg_advisory_unlock(hashtextextended(" + match[1] + ", 0)) THEN 1 ELSE 0 END"
	})
	if postgresInsertIgnorePattern.MatchString(query) {
		query = postgresInsertIgnorePattern.ReplaceAllString(query, "INSERT INTO")
		query = appendPostgresConflict(query, "DO NOTHING")
	}
	if match := postgresDuplicatePattern.FindStringSubmatch(query); len(match) == 2 {
		assignments := postgresValuesPattern.ReplaceAllString(match[1], "EXCLUDED.$1")
		target := ""
		if table := postgresInsertTablePattern.FindStringSubmatch(query); len(table) == 2 {
			target = postgresConflictTargets[strings.ToLower(table[1])]
		}
		if target != "" {
			target = " (" + target + ")"
		}
		query = postgresDuplicatePattern.ReplaceAllString(query, " ON CONFLICT"+target+" DO UPDATE SET "+assignments)
	}
	return rewritePostgresPlaceholders(query)
}

func appendPostgresConflict(query, action string) string {
	trimmed := strings.TrimSpace(query)
	semicolon := strings.HasSuffix(trimmed, ";")
	if semicolon {
		trimmed = strings.TrimSpace(strings.TrimSuffix(trimmed, ";"))
	}
	trimmed += " ON CONFLICT " + action
	if semicolon {
		trimmed += ";"
	}
	return trimmed
}

func rewritePostgresPlaceholders(query string) string {
	var out strings.Builder
	out.Grow(len(query) + 8)
	placeholder := 1
	state := byte(0) // 0 normal, 1 single quote, 2 double quote, 3 line comment, 4 block comment
	for i := 0; i < len(query); i++ {
		ch := query[i]
		switch state {
		case 0:
			switch ch {
			case '\'':
				state = 1
			case '"':
				state = 2
			case '-':
				if i+1 < len(query) && query[i+1] == '-' {
					state = 3
				}
			case '/':
				if i+1 < len(query) && query[i+1] == '*' {
					state = 4
				}
			case '?':
				out.WriteByte('$')
				out.WriteString(strconv.Itoa(placeholder))
				placeholder++
				continue
			}
		case 1:
			if ch == '\'' {
				if i+1 < len(query) && query[i+1] == '\'' {
					out.WriteByte(ch)
					i++
					out.WriteByte(query[i])
					continue
				}
				state = 0
			}
		case 2:
			if ch == '"' {
				if i+1 < len(query) && query[i+1] == '"' {
					out.WriteByte(ch)
					i++
					out.WriteByte(query[i])
					continue
				}
				state = 0
			}
		case 3:
			if ch == '\n' {
				state = 0
			}
		case 4:
			if ch == '*' && i+1 < len(query) && query[i+1] == '/' {
				out.WriteByte(ch)
				i++
				out.WriteByte(query[i])
				state = 0
				continue
			}
		}
		out.WriteByte(ch)
	}
	return out.String()
}

// OpenPostgres creates the APIFull KV table if needed and points Default at
// it. A second call with the same process replaces Default.
func OpenPostgres(dsn string) error {
	return openPostgres(dsn, true)
}

// OpenPostgresReadOnly connects to an already-provisioned APIFull schema
// without issuing DDL. Production processes can use this path when migrations
// are owned by deployment.
func OpenPostgresReadOnly(dsn string) error {
	return openPostgres(dsn, false)
}

func openPostgres(dsn string, createSchema bool) error {
	db, err := OpenPostgresDB(dsn)
	if err != nil {
		return err
	}
	db.SetMaxOpenConns(8)
	db.SetMaxIdleConns(8)
	if err = db.Ping(); err != nil {
		_ = db.Close()
		return err
	}
	var serverVersionNum int
	if err = db.QueryRow(`SELECT current_setting('server_version_num')::integer`).Scan(&serverVersionNum); err != nil {
		_ = db.Close()
		return err
	}
	if serverVersionNum/10000 != 18 {
		_ = db.Close()
		return fmt.Errorf("apifull: PostgreSQL 18 is required (server_version_num=%d)", serverVersionNum)
	}
	if createSchema {
		if _, err = db.Exec(`CREATE TABLE IF NOT EXISTS apifull_kv (
			k TEXT PRIMARY KEY,
			v TEXT NOT NULL
		)`); err != nil {
			_ = db.Close()
			return err
		}
		if _, err = db.Exec(`CREATE TABLE IF NOT EXISTS apifull_game_score (
			scope TEXT NOT NULL CHECK (scope IN ('peer', 'inline')),
			game_key TEXT NOT NULL,
			user_id BIGINT NOT NULL,
			score INTEGER NOT NULL CHECK (score >= 0),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (scope, game_key, user_id)
		)`); err != nil {
			_ = db.Close()
			return err
		}
		if _, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_apifull_game_score_board
			ON apifull_game_score (scope, game_key, score DESC, user_id ASC)`); err != nil {
			_ = db.Close()
			return err
		}
		if _, err = db.Exec(`CREATE TABLE IF NOT EXISTS apifull_quick_reply (
			user_id BIGINT NOT NULL,
			shortcut_id INTEGER NOT NULL CHECK (shortcut_id > 0),
			shortcut VARCHAR(64) NOT NULL CHECK (char_length(shortcut) > 0),
			message_ids JSONB NOT NULL DEFAULT '[]'::jsonb,
			position INTEGER NOT NULL DEFAULT 0 CHECK (position >= 0),
			PRIMARY KEY (user_id, shortcut_id),
			CONSTRAINT uq_apifull_quick_reply_shortcut UNIQUE (user_id, shortcut)
		)`); err != nil {
			_ = db.Close()
			return err
		}
		if _, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_apifull_quick_reply_user_position
			ON apifull_quick_reply (user_id, position, shortcut_id)`); err != nil {
			_ = db.Close()
			return err
		}
		if err = ensureStickerSchema(db); err != nil {
			_ = db.Close()
			return err
		}
		if err = ensureEmojiSchema(db); err != nil {
			_ = db.Close()
			return err
		}
		if err = ensureMiniBotAppSchema(db); err != nil {
			_ = db.Close()
			return err
		}
	} else {
		// Production processes use the deployment-owned schema and must fail
		// before serving requests when migrations were not applied.
		for _, query := range []string{
			`SELECT k, v FROM apifull_kv LIMIT 0`,
			`SELECT scope, game_key, user_id, score FROM apifull_game_score LIMIT 0`,
			`SELECT user_id, shortcut_id, shortcut, message_ids FROM apifull_quick_reply LIMIT 0`,
			`SELECT id, autotranslation, emoji_status_document_id, emoji_status_until FROM apifull_channel LIMIT 0`,
			`SELECT channel_id, user_id, joined_at, admin_rights, banned_rights FROM apifull_channel_member LIMIT 0`,
			`SELECT channel_id, message_id, sender_user_id, date, message FROM apifull_channel_message LIMIT 0`,
			`SELECT user_id, channel_id, read_max_id FROM apifull_channel_read_state LIMIT 0`,
			`SELECT user_id, state FROM apifull_chatlist_state LIMIT 0`,
			`SELECT slug, owner_user_id FROM apifull_chatlist_invite LIMIT 0`,
			`SELECT user_id, id FROM apifull_ai_compose_tone LIMIT 0`,
			`SELECT id, short_name FROM apifull_sticker_set LIMIT 0`,
			`SELECT id, set_id FROM apifull_sticker LIMIT 0`,
			`SELECT user_id, set_id FROM apifull_sticker_user_set LIMIT 0`,
			`SELECT user_id, sticker_id FROM apifull_sticker_user_recent LIMIT 0`,
			`SELECT user_id, sticker_id FROM apifull_sticker_user_favourite LIMIT 0`,
			`SELECT lang_code, version FROM apifull_emoji_language LIMIT 0`,
			`SELECT lang_code, keyword FROM apifull_emoji_keyword LIMIT 0`,
			`SELECT id, access_hash FROM apifull_emoji_document LIMIT 0`,
			`SELECT kind, title FROM apifull_emoji_group LIMIT 0`,
			`SELECT user_id, message_id, received_at FROM apifull_message_delivery_report LIMIT 0`,
			`SELECT id, actor_user_id, kind, target_type, target_id, dedupe_key, payload, state FROM apifull_report LIMIT 0`,
			`SELECT user_id, takeout_id FROM apifull_takeout_session LIMIT 0`,
			`SELECT user_id, uploaded FROM apifull_wallpaper_state LIMIT 0`,
			`SELECT user_id, state FROM apifull_story_state LIMIT 0`,
			`SELECT user_id, balance FROM apifull_stars LIMIT 0`,
			`SELECT id, user_id, amount, idem FROM apifull_star_tx LIMIT 0`,
			`SELECT id, kind, stars, store_product, currency, amount, extended, active FROM apifull_stars_offer LIMIT 0`,
			`SELECT id, user_id, request_key, provider, fingerprint, state FROM apifull_payment_request LIMIT 0`,
			`SELECT id, request_id, user_id, state, provider, transaction_id FROM apifull_payment_ledger LIMIT 0`,
			`SELECT request_id, user_id, provider, transaction_id, receipt FROM apifull_payment_receipt LIMIT 0`,
			`SELECT request_id, user_id, provider, transaction_id, months, state FROM apifull_payment_entitlement_outbox LIMIT 0`,
			`SELECT user_id, name, phone, email, credentials_saved FROM apifull_payment_saved_info LIMIT 0`,
			`SELECT id, from_user, to_user, slug, stars, saved FROM apifull_gift LIMIT 0`,
			`SELECT username, owner_user_id, kind FROM apifull_username LIMIT 0`,
			`SELECT id, access_hash, admin_id, participant_id, state FROM apifull_call LIMIT 0`,
			`SELECT community_id, owner_user_id FROM apifull_community LIMIT 0`,
			`SELECT community_id, peer_type, peer_id FROM apifull_community_peer LIMIT 0`,
			`SELECT user_id, community_id FROM apifull_community_dialog_state LIMIT 0`,
			`SELECT user_id, bot_id, can_send FROM apifull_mini_bot_permission LIMIT 0`,
			`SELECT request_id, user_id, bot_id, kind, payload, expires_at FROM apifull_webview_request LIMIT 0`,
			`SELECT owner_user_id, bot_user_id, button FROM apifull_bot_menu_button LIMIT 0`,
			`SELECT bot_user_id, group_admin_rights, broadcast_admin_rights FROM apifull_bot_default_admin_rights LIMIT 0`,
			`SELECT channel_id, sticker_set_id, updated_by_user_id FROM apifull_channel_sticker_set LIMIT 0`,
			`SELECT channel_id, sticker_set_id, updated_by_user_id FROM apifull_channel_emoji_sticker_set LIMIT 0`,
			`SELECT id, access_hash, creator_user_id, channel_id, title FROM apifull_group_call LIMIT 0`,
			`SELECT call_id, join_muted, messages_enabled FROM apifull_group_call_settings LIMIT 0`,
			`SELECT call_id, creator_user_id, token_hash FROM apifull_group_call_invite LIMIT 0`,
			`SELECT call_id, user_id, media_source, muted FROM apifull_group_call_participant LIMIT 0`,
			`SELECT call_id, user_id, subscribed FROM apifull_group_call_subscription LIMIT 0`,
			`SELECT call_id, user_id, send_as FROM apifull_group_call_send_as LIMIT 0`,
			`SELECT call_id, public_key, block, params FROM apifull_group_call_conference LIMIT 0`,
			`SELECT peer_key, topic_id, title, position FROM apifull_forum_topic LIMIT 0`,
			`SELECT user_id, title FROM apifull_forum_user_title LIMIT 0`,
			`SELECT channel_id, enabled, tabs, view_as_messages FROM apifull_forum_channel_settings LIMIT 0`,
			`SELECT scope, peer_type, peer_id, boosts, blocked_boosts FROM apifull_boost_target LIMIT 0`,
			`SELECT user_id, slot, scope, peer_type, peer_id, expires FROM apifull_boost_slot LIMIT 0`,
			`SELECT user_id, joined, allow_international, recent_sent FROM apifull_sms_job_member LIMIT 0`,
			`SELECT job_id, user_id, phone_number, text, state FROM apifull_sms_job LIMIT 0`,
			`SELECT id, user_id, event_time, event_type, peer_id, data FROM apifull_app_log LIMIT 0`,
			`SELECT owner_user_id, url_hash, url, status, match_code FROM apifull_url_auth LIMIT 0`,
			`SELECT id, call_id, user_id FROM apifull_call_artifact LIMIT 0`,
			`SELECT id, channel_id, actor_user_id FROM apifull_channel_admin_log LIMIT 0`,
			`SELECT id, channel_id, pts_from FROM apifull_channel_delivery_outbox LIMIT 0`,
			`SELECT delivery_id, user_id, state FROM apifull_channel_delivery_recipient LIMIT 0`,
			`SELECT channel_id, pts, pts_count FROM apifull_channel_event LIMIT 0`,
			`SELECT user_id, channel_id, message_id FROM apifull_channel_message_content_read LIMIT 0`,
			`SELECT user_id, channel_id, message_id FROM apifull_channel_message_hidden LIMIT 0`,
			`SELECT channel_id, sender_user_id, random_id FROM apifull_channel_message_request LIMIT 0`,
			`SELECT channel_id, last_message_id, pts FROM apifull_channel_message_seq LIMIT 0`,
			`SELECT owner_user_id, bot_user_id, can_reply FROM apifull_connected_bot LIMIT 0`,
			`SELECT owner_user_id, peer_type, peer_id FROM apifull_connected_bot_peer LIMIT 0`,
			`SELECT id, call_id, sender_user_id FROM apifull_group_call_encrypted_message LIMIT 0`,
			`SELECT id, call_id, sender_user_id FROM apifull_group_call_message LIMIT 0`,
			`SELECT credential_id, user_id, name FROM apifull_passkey_credential LIMIT 0`,
			`SELECT challenge, user_id, kind FROM apifull_passkey_session LIMIT 0`,
			`SELECT id, access_hash, admin_user_id FROM apifull_secret_chat LIMIT 0`,
			`SELECT chat_id, user_id, device_id FROM apifull_secret_chat_device_key LIMIT 0`,
			`SELECT id, chat_id, sender_user_id FROM apifull_secret_message LIMIT 0`,
			`SELECT user_id, last_qts, confirmed_qts FROM apifull_secret_user_state LIMIT 0`,
			`SELECT id, user_id, charge_id FROM apifull_stars_refund LIMIT 0`,
		} {
			if _, err = db.Exec(query); err != nil {
				_ = db.Close()
				return fmt.Errorf("apifull: required PostgreSQL schema is unavailable: %w", err)
			}
		}
	}
	postgresOpenOnce.Lock()
	previous, _ := Default.(*postgresStore)
	Default = &postgresStore{db: db}
	postgresOpenOnce.Unlock()
	if previous != nil {
		_ = previous.db.Close()
	}
	return nil
}

// PostgresEnabled reports whether APIFull's process-wide store is backed by
// the deployment-owned PostgreSQL schema. Callers use this to avoid silently
// writing production Mini App state to a test or legacy store.
func PostgresEnabled() bool {
	store, ok := Default.(*postgresStore)
	return ok && store != nil && store.db != nil
}

func ClosePostgres() error {
	postgresOpenOnce.Lock()
	defer postgresOpenOnce.Unlock()
	store, ok := Default.(*postgresStore)
	if !ok {
		return nil
	}
	err := store.db.Close()
	// Leave a usable in-memory default after shutdown. This keeps test and
	// process teardown paths from retaining a closed database handle.
	Default = &mem{}
	return err
}

// OpenPostgresDB opens a PostgreSQL database/sql handle with APIFull's
// placeholder compatibility layer. Callers that own their schema can use it
// without changing the process-wide Store.
func OpenPostgresDB(dsn string) (*sql.DB, error) {
	if strings.TrimSpace(dsn) == "" {
		return nil, errors.New("apifull: PostgreSQL DSN is required")
	}
	config, err := pgx.ParseConfig(dsn)
	if err != nil {
		return nil, err
	}
	if config.ConnectTimeout <= 0 {
		config.ConnectTimeout = 10 * time.Second
	}
	db := sql.OpenDB(postgresCompatConnector{connector: stdlib.GetConnector(*config)})
	db.SetMaxOpenConns(8)
	db.SetMaxIdleConns(8)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("apifull: PostgreSQL ping: %w", err)
	}
	var serverVersionNum int
	if err := db.QueryRowContext(ctx, `SELECT current_setting('server_version_num')::integer`).Scan(&serverVersionNum); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("apifull: read PostgreSQL server version: %w", err)
	}
	if serverVersionNum/10000 != 18 {
		_ = db.Close()
		return nil, fmt.Errorf("apifull: PostgreSQL 18 is required (server_version_num=%d)", serverVersionNum)
	}
	return db, nil
}
