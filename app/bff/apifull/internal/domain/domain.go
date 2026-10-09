package domain

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha1"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
)

// Relay is the configured TURN endpoint returned inside phoneCall connections.
var Relay RelayConfig

type RelayConfig struct {
	IP                   string
	Port                 int32
	Username             string
	Password             string
	SharedSecret         string
	CredentialTTLSeconds int
	PeerTag              []byte
}

// SetRelay points call connections at a reachable TURN listener.
// An empty ip leaves the current address. A non-positive port keeps the current port.
func SetRelay(ip string, port int32) {
	if ip == "" {
		return
	}
	Relay.IP = ip
	if port > 0 {
		Relay.Port = port
	}
}

func SetRelayCredentials(username, password string) {
	Relay.Username = username
	Relay.Password = password
}

func SetRelaySharedSecret(secret string, ttlSeconds int) {
	Relay.SharedSecret = strings.TrimSpace(secret)
	if ttlSeconds <= 0 {
		ttlSeconds = 3600
	}
	Relay.CredentialTTLSeconds = ttlSeconds
}

func RelayCredentials(userID int64) (username, password string, ok bool) {
	if userID <= 0 {
		return "", "", false
	}
	if Relay.SharedSecret != "" {
		if Relay.CredentialTTLSeconds > 86400 {
			return "", "", false
		}
		username = strconv.FormatInt(time.Now().Unix()+int64(Relay.CredentialTTLSeconds), 10) + ":" + strconv.FormatInt(userID, 10)
		mac := hmac.New(sha1.New, []byte(Relay.SharedSecret))
		_, _ = mac.Write([]byte(username))
		return username, base64.StdEncoding.EncodeToString(mac.Sum(nil)), true
	}
	if strings.TrimSpace(Relay.Username) == "" || strings.TrimSpace(Relay.Password) == "" {
		return "", "", false
	}
	return Relay.Username, Relay.Password, true
}

func RelayConfigured() bool {
	host := strings.TrimSpace(Relay.IP)
	credentialsConfigured := strings.TrimSpace(Relay.Username) != "" && strings.TrimSpace(Relay.Password) != ""
	if Relay.SharedSecret != "" {
		credentialsConfigured = Relay.CredentialTTLSeconds > 0 && Relay.CredentialTTLSeconds <= 86400
	}
	if host == "" || Relay.Port <= 0 || Relay.Port > 65535 || !credentialsConfigured ||
		strings.EqualFold(host, "localhost") {
		return false
	}
	if ip := net.ParseIP(strings.Trim(host, "[]")); ip != nil && ip.IsLoopback() {
		return false
	}
	return true
}

var db *sql.DB

// Close releases the process-owned PostgreSQL handle. It is called during
// session shutdown after the RPC server has stopped accepting requests.
func Close() error {
	storeErr := persist.ClosePostgres()
	if db == nil {
		return storeErr
	}
	err := db.Close()
	db = nil
	return errors.Join(err, storeErr)
}

// Open is the single APIFull storage entry point. The independent deployment
// uses PostgreSQL 18 exclusively; callers that still pass a MySQL DSN fail
// fast instead of silently selecting a legacy backend.
func Open(dsn string) error {
	if !isPostgresDSN(dsn) {
		return errors.New("apifull: PostgreSQL DSN is required")
	}
	return openPostgresDomain(dsn, schemaReadOnly())
}

func isPostgresDSN(dsn string) bool {
	dsn = strings.TrimSpace(strings.ToLower(dsn))
	return strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://")
}

func schemaReadOnly() bool {
	value := strings.TrimSpace(os.Getenv("TEAMGRAM_APIFULL_SCHEMA_READONLY"))
	return value == "1" || strings.EqualFold(value, "true") || strings.EqualFold(value, "yes")
}

func migrate(conn *sql.DB) error {
	return migrateDialect(conn, false)
}

// migratePostgres provisions a fresh APIFull schema using PostgreSQL-native
// types and indexes. It shares the authoritative table definition below with
// the transitional MySQL path, then applies a deterministic DDL conversion.
func migratePostgres(conn *sql.DB) error {
	tx, err := conn.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := migrateDialect(tx, true); err != nil {
		return err
	}
	if _, err := tx.Exec(`CREATE TABLE IF NOT EXISTS apifull_payment_saved_info (
		user_id BIGINT PRIMARY KEY,
		name TEXT NOT NULL DEFAULT '',
		phone TEXT NOT NULL DEFAULT '',
		email TEXT NOT NULL DEFAULT '',
		credentials_saved BOOLEAN NOT NULL DEFAULT FALSE,
		updated_at BIGINT NOT NULL
	)`); err != nil {
		return err
	}
	if _, err := tx.Exec(`CREATE TABLE IF NOT EXISTS apifull_ai_compose_tone (
		user_id BIGINT NOT NULL,
		id BIGINT NOT NULL,
		title TEXT NOT NULL DEFAULT '',
		prompt TEXT NOT NULL DEFAULT '',
		tone TEXT NOT NULL DEFAULT '',
		slug TEXT NOT NULL DEFAULT '',
		emoji_id BIGINT NOT NULL DEFAULT 0,
		access_hash BIGINT NOT NULL,
		creator BOOLEAN NOT NULL DEFAULT TRUE,
		saved BOOLEAN NOT NULL DEFAULT FALSE,
		display_author BOOLEAN NOT NULL DEFAULT FALSE,
		created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
		updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
		PRIMARY KEY (user_id, id)
	)`); err != nil {
		return err
	}
	if _, err := tx.Exec(`CREATE INDEX IF NOT EXISTS idx_apifull_ai_compose_tone_user_saved
		ON apifull_ai_compose_tone (user_id, saved, id)`); err != nil {
		return err
	}
	if _, err := tx.Exec(`CREATE TABLE IF NOT EXISTS apifull_community (
		community_id BIGINT PRIMARY KEY,
		owner_user_id BIGINT NOT NULL,
		title TEXT NOT NULL,
		about TEXT NOT NULL DEFAULT '',
		hidden BOOLEAN NOT NULL DEFAULT FALSE,
		created_at TIMESTAMPTZ NOT NULL DEFAULT now()
	)`); err != nil {
		return err
	}
	if _, err := tx.Exec(`CREATE INDEX IF NOT EXISTS idx_apifull_community_owner
		ON apifull_community (owner_user_id, community_id)`); err != nil {
		return err
	}
	if _, err := tx.Exec(`CREATE TABLE IF NOT EXISTS apifull_community_peer (
		community_id BIGINT NOT NULL,
		peer_type SMALLINT NOT NULL,
		peer_id BIGINT NOT NULL,
		access_hash BIGINT NOT NULL DEFAULT 0,
		visible BOOLEAN,
		approved BOOLEAN NOT NULL DEFAULT TRUE,
		requested_by BIGINT NOT NULL DEFAULT 0,
		requested_at BIGINT NOT NULL DEFAULT 0,
		banned BOOLEAN NOT NULL DEFAULT FALSE,
		can_view_history BOOLEAN NOT NULL DEFAULT FALSE,
		PRIMARY KEY (community_id, peer_type, peer_id)
	)`); err != nil {
		return err
	}
	if _, err := tx.Exec(`CREATE INDEX IF NOT EXISTS idx_apifull_community_peer_pending
		ON apifull_community_peer (community_id, approved, requested_at, peer_id)`); err != nil {
		return err
	}
	if _, err := tx.Exec(`CREATE TABLE IF NOT EXISTS apifull_community_dialog_state (
		user_id BIGINT NOT NULL,
		community_id BIGINT NOT NULL,
		collapsed BOOLEAN NOT NULL DEFAULT FALSE,
		updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
		PRIMARY KEY (user_id, community_id)
	)`); err != nil {
		return err
	}
	if _, err := tx.Exec(`CREATE TABLE IF NOT EXISTS apifull_boost_target (
		scope TEXT NOT NULL,
		peer_type SMALLINT NOT NULL,
		peer_id BIGINT NOT NULL,
		owner_user_id BIGINT NOT NULL DEFAULT 0,
		boosts INTEGER NOT NULL DEFAULT 0 CHECK (boosts >= 0),
		blocked_boosts INTEGER NOT NULL DEFAULT 0 CHECK (blocked_boosts >= 0),
		updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (scope, peer_type, peer_id)
	)`); err != nil {
		return err
	}
	if _, err := tx.Exec(`CREATE TABLE IF NOT EXISTS apifull_boost_slot (
		user_id BIGINT NOT NULL,
		slot INTEGER NOT NULL CHECK (slot > 0),
		scope TEXT,
		peer_type SMALLINT,
		peer_id BIGINT,
		gift BOOLEAN NOT NULL DEFAULT FALSE,
		giveaway BOOLEAN NOT NULL DEFAULT FALSE,
		unclaimed BOOLEAN NOT NULL DEFAULT FALSE,
		boost_date INTEGER NOT NULL DEFAULT 0,
		expires INTEGER NOT NULL DEFAULT 0,
		cooldown_until INTEGER NOT NULL DEFAULT 0,
		used_gift_slug TEXT NOT NULL DEFAULT '',
		multiplier INTEGER NOT NULL DEFAULT 1,
		stars BIGINT NOT NULL DEFAULT 0,
		updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (user_id, slot),
		CHECK ((scope IS NULL AND peer_type IS NULL AND peer_id IS NULL)
			OR (scope IS NOT NULL AND peer_type IS NOT NULL AND peer_id IS NOT NULL))
	)`); err != nil {
		return err
	}
	if _, err := tx.Exec(`CREATE INDEX IF NOT EXISTS idx_apifull_boost_slot_target
		ON apifull_boost_slot (scope, peer_type, peer_id, user_id, slot)`); err != nil {
		return err
	}
	if _, err := tx.Exec(`CREATE TABLE IF NOT EXISTS apifull_bot_menu_button (
		owner_user_id BIGINT NOT NULL,
		bot_user_id BIGINT NOT NULL,
		button JSONB NOT NULL DEFAULT '{}'::jsonb,
		updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (owner_user_id, bot_user_id)
	)`); err != nil {
		return err
	}
	if _, err := tx.Exec(`CREATE INDEX IF NOT EXISTS idx_apifull_bot_menu_button_bot
		ON apifull_bot_menu_button (bot_user_id, owner_user_id)`); err != nil {
		return err
	}
	if _, err := tx.Exec(`CREATE TABLE IF NOT EXISTS apifull_connected_bot (
		owner_user_id BIGINT NOT NULL,
		bot_user_id BIGINT NOT NULL,
		can_reply BOOLEAN NOT NULL DEFAULT FALSE,
		rights JSONB NOT NULL DEFAULT '{}'::jsonb,
		recipients_bot JSONB NOT NULL DEFAULT '{}'::jsonb,
		recipients JSONB,
		updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (owner_user_id, bot_user_id)
	)`); err != nil {
		return err
	}
	if _, err := tx.Exec(`CREATE INDEX IF NOT EXISTS idx_apifull_connected_bot_bot
		ON apifull_connected_bot (bot_user_id, owner_user_id)`); err != nil {
		return err
	}
	if _, err := tx.Exec(`CREATE TABLE IF NOT EXISTS apifull_connected_bot_peer (
		owner_user_id BIGINT NOT NULL,
		peer_type SMALLINT NOT NULL CHECK (peer_type IN (0, 1, 2)),
		peer_id BIGINT NOT NULL,
		paused BOOLEAN NOT NULL DEFAULT FALSE,
		disabled BOOLEAN NOT NULL DEFAULT FALSE,
		updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (owner_user_id, peer_type, peer_id)
	)`); err != nil {
		return err
	}
	if _, err := tx.Exec(`CREATE TABLE IF NOT EXISTS apifull_bot_default_admin_rights (
		bot_user_id BIGINT PRIMARY KEY,
		group_admin_rights JSONB NOT NULL DEFAULT '{}'::jsonb,
		broadcast_admin_rights JSONB NOT NULL DEFAULT '{}'::jsonb,
		updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`); err != nil {
		return err
	}
	if _, err := tx.Exec(`CREATE TABLE IF NOT EXISTS apifull_channel_sticker_set (
		channel_id BIGINT PRIMARY KEY,
		sticker_set_id BIGINT NOT NULL DEFAULT 0 REFERENCES apifull_sticker_set(id) ON DELETE CASCADE,
		updated_by_user_id BIGINT NOT NULL,
		updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`); err != nil {
		return err
	}
	if _, err := tx.Exec(`CREATE TABLE IF NOT EXISTS apifull_channel_emoji_sticker_set (
		channel_id BIGINT PRIMARY KEY,
		sticker_set_id BIGINT NOT NULL REFERENCES apifull_sticker_set(id) ON DELETE CASCADE,
		updated_by_user_id BIGINT NOT NULL,
		updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`); err != nil {
		return err
	}
	if _, err := tx.Exec(`CREATE TABLE IF NOT EXISTS apifull_sms_job_member (
		user_id BIGINT PRIMARY KEY,
		joined BOOLEAN NOT NULL DEFAULT FALSE,
		allow_international BOOLEAN NOT NULL DEFAULT FALSE,
		recent_sent INTEGER NOT NULL DEFAULT 0 CHECK (recent_sent >= 0),
		recent_since BIGINT NOT NULL DEFAULT 0,
		total_sent INTEGER NOT NULL DEFAULT 0 CHECK (total_sent >= 0),
		total_since BIGINT NOT NULL DEFAULT 0,
		last_gift_slug TEXT,
		updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`); err != nil {
		return err
	}
	if _, err := tx.Exec(`CREATE TABLE IF NOT EXISTS apifull_sms_job (
		job_id TEXT PRIMARY KEY,
		user_id BIGINT NOT NULL,
		phone_number TEXT NOT NULL,
		text TEXT NOT NULL,
		state TEXT NOT NULL DEFAULT 'pending',
		error_text TEXT,
		assigned_at BIGINT NOT NULL DEFAULT 0,
		finished_at BIGINT NOT NULL DEFAULT 0,
		created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
		CHECK (state IN ('pending', 'finished'))
	)`); err != nil {
		return err
	}
	if _, err := tx.Exec(`CREATE INDEX IF NOT EXISTS idx_apifull_sms_job_user_state
		ON apifull_sms_job (user_id, state, created_at, job_id)`); err != nil {
		return err
	}
	for _, stmt := range []string{
		`CREATE TABLE IF NOT EXISTS apifull_forum_topic (
			peer_key TEXT NOT NULL,
			topic_id INTEGER GENERATED BY DEFAULT AS IDENTITY,
			title TEXT NOT NULL,
			closed BOOLEAN NOT NULL DEFAULT FALSE,
			pinned BOOLEAN NOT NULL DEFAULT FALSE,
			hidden BOOLEAN NOT NULL DEFAULT FALSE,
			topic_date INTEGER NOT NULL DEFAULT 0,
			creator_user_id BIGINT NOT NULL,
			icon_emoji_id BIGINT NOT NULL DEFAULT 0,
			top_message INTEGER NOT NULL DEFAULT 0,
			position BIGINT NOT NULL,
			PRIMARY KEY (peer_key, topic_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_apifull_forum_topic_order
			ON apifull_forum_topic (peer_key, position, topic_id)`,
		`CREATE TABLE IF NOT EXISTS apifull_forum_user_title (
			user_id BIGINT PRIMARY KEY,
			title TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE TABLE IF NOT EXISTS apifull_forum_channel_settings (
			channel_id BIGINT PRIMARY KEY,
			enabled BOOLEAN NOT NULL DEFAULT FALSE,
			tabs BOOLEAN NOT NULL DEFAULT FALSE,
			view_as_messages BOOLEAN NOT NULL DEFAULT FALSE,
			updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS apifull_stars_refund (
			id BIGINT GENERATED BY DEFAULT AS IDENTITY PRIMARY KEY,
			user_id BIGINT NOT NULL,
			charge_id VARCHAR(191) NOT NULL,
			provider VARCHAR(32) NOT NULL,
			stars BIGINT NOT NULL CHECK (stars > 0),
			refund_transaction_id VARCHAR(191) NOT NULL,
			state VARCHAR(16) NOT NULL CHECK (state IN ('complete')),
			receipt BYTEA NOT NULL,
			created_at BIGINT NOT NULL,
			updated_at BIGINT NOT NULL,
			CONSTRAINT uniq_apifull_stars_refund_charge UNIQUE (user_id, charge_id),
			CONSTRAINT uniq_apifull_stars_refund_transaction UNIQUE (provider, refund_transaction_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_apifull_stars_refund_user
			ON apifull_stars_refund (user_id, id)`,
	} {
		if _, err := tx.Exec(stmt); err != nil {
			return err
		}
	}
	return tx.Commit()
}

type sqlExecer interface {
	Exec(query string, args ...any) (sql.Result, error)
}

func migrateDialect(conn sqlExecer, postgres bool) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS apifull_channel (
			id BIGINT NOT NULL PRIMARY KEY,
			access_hash BIGINT NOT NULL,
			migrated_from_chat_id BIGINT NULL,
			creator_user_id BIGINT NOT NULL,
			title VARCHAR(255) NOT NULL,
			about VARCHAR(1024) NOT NULL DEFAULT '',
			broadcast TINYINT NOT NULL DEFAULT 0,
			megagroup TINYINT NOT NULL DEFAULT 0,
			signatures_enabled TINYINT NOT NULL DEFAULT 0,
			signature_profiles_enabled TINYINT NOT NULL DEFAULT 0,
			antispam TINYINT NOT NULL DEFAULT 0,
			autotranslation TINYINT NOT NULL DEFAULT 0,
			hidden_prehistory TINYINT NOT NULL DEFAULT 0,
			participants_hidden TINYINT NOT NULL DEFAULT 0,
			slowmode_seconds INT NOT NULL DEFAULT 0,
			color INT NULL,
			background_emoji_id BIGINT NULL,
			profile_color INT NULL,
			profile_background_emoji_id BIGINT NULL,
			photo_id BIGINT NOT NULL DEFAULT 0,
			photo_dc_id INT NOT NULL DEFAULT 0,
			photo_has_video TINYINT NOT NULL DEFAULT 0,
			location_lat DOUBLE NULL,
			location_long DOUBLE NULL,
			location_address VARCHAR(255) NOT NULL DEFAULT '',
			username VARCHAR(64) NOT NULL DEFAULT '',
			discussion_group_id BIGINT NULL,
			created_at INT NOT NULL,
			KEY idx_apifull_channel_creator (creator_user_id),
			UNIQUE KEY uniq_apifull_channel_migrated_chat (migrated_from_chat_id)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS apifull_channel_member (
			channel_id BIGINT NOT NULL,
			user_id BIGINT NOT NULL,
			invited_by_user_id BIGINT NOT NULL DEFAULT 0,
			joined_at INT NOT NULL,
			admin_rights VARCHAR(2048) NOT NULL DEFAULT '',
			admin_rank VARCHAR(64) NOT NULL DEFAULT '',
			banned_rights VARCHAR(2048) NOT NULL DEFAULT '',
			banned_by_user_id BIGINT NOT NULL DEFAULT 0,
			banned_at INT NOT NULL DEFAULT 0,
			PRIMARY KEY (channel_id, user_id),
			KEY idx_apifull_channel_member_user (user_id)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS apifull_channel_admin_log (
			id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
			channel_id BIGINT NOT NULL,
			actor_user_id BIGINT NOT NULL,
			target_user_id BIGINT NOT NULL DEFAULT 0,
			action VARCHAR(64) NOT NULL,
			prev_member MEDIUMTEXT NOT NULL,
			new_member MEDIUMTEXT NOT NULL,
			date INT NOT NULL,
			KEY idx_apifull_channel_admin_log_channel (channel_id, id),
			KEY idx_apifull_channel_admin_log_action (channel_id, action, id)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS apifull_call (
			id BIGINT NOT NULL PRIMARY KEY,
			access_hash BIGINT NOT NULL,
			admin_id BIGINT NOT NULL,
			participant_id BIGINT NOT NULL,
			state VARCHAR(32) NOT NULL,
			video TINYINT NOT NULL DEFAULT 0,
			ga_hash VARBINARY(256) NULL,
			gb VARBINARY(256) NULL,
			ga VARBINARY(256) NULL,
			protocol MEDIUMTEXT NULL,
			key_fingerprint BIGINT NOT NULL DEFAULT 0,
			received_at INT NOT NULL DEFAULT 0,
			accepted_at INT NOT NULL DEFAULT 0,
			confirmed_at INT NOT NULL DEFAULT 0,
			discarded_at INT NOT NULL DEFAULT 0,
			discarded_by BIGINT NOT NULL DEFAULT 0,
			duration INT NOT NULL DEFAULT 0,
			reason VARCHAR(64) NOT NULL DEFAULT '',
			created_at INT NOT NULL,
			KEY idx_apifull_call_participant (participant_id)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS apifull_call_artifact (
			id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
			call_id BIGINT NOT NULL,
			user_id BIGINT NOT NULL,
			kind VARCHAR(32) NOT NULL,
			payload MEDIUMBLOB NOT NULL,
			created_at INT NOT NULL,
			KEY idx_apifull_call_artifact_call (call_id, kind, id),
			KEY idx_apifull_call_artifact_user (user_id, kind, id)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS apifull_stars (
			user_id BIGINT NOT NULL PRIMARY KEY,
			balance BIGINT NOT NULL
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS apifull_star_tx (
			id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
			user_id BIGINT NOT NULL,
			amount BIGINT NOT NULL,
			idem VARCHAR(191) NOT NULL,
			UNIQUE KEY uniq_apifull_star_tx (user_id, idem)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS apifull_stars_offer (
			id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
			kind VARCHAR(16) NOT NULL,
			stars BIGINT NOT NULL,
			store_product VARCHAR(191) NOT NULL DEFAULT '',
			currency VARCHAR(16) NOT NULL DEFAULT '',
			amount BIGINT NOT NULL DEFAULT 0,
			extended TINYINT NOT NULL DEFAULT 0,
			active TINYINT NOT NULL DEFAULT 1,
			UNIQUE KEY uniq_apifull_stars_offer (kind, stars, store_product, currency, amount),
			KEY idx_apifull_stars_offer_active (kind, active, id)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS apifull_payment_request (
			id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
			user_id BIGINT NOT NULL,
			request_key VARCHAR(191) NOT NULL,
			provider VARCHAR(32) NOT NULL,
			fingerprint CHAR(64) NOT NULL,
			state VARCHAR(16) NOT NULL,
			transaction_id VARCHAR(191) NOT NULL DEFAULT '',
			currency VARCHAR(16) NOT NULL DEFAULT '',
			amount BIGINT NOT NULL DEFAULT 0,
			peer_id BIGINT NOT NULL DEFAULT 0,
			msg_id INT NOT NULL DEFAULT 0,
			error_text VARCHAR(255) NOT NULL DEFAULT '',
			created_at INT NOT NULL,
			updated_at INT NOT NULL,
			UNIQUE KEY uniq_apifull_payment_request (user_id, request_key),
			KEY idx_apifull_payment_request_state (state, updated_at),
			KEY idx_apifull_payment_request_transaction (provider, transaction_id)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS apifull_payment_ledger (
			id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
			request_id BIGINT NOT NULL,
			user_id BIGINT NOT NULL,
			state VARCHAR(16) NOT NULL,
			provider VARCHAR(32) NOT NULL,
			transaction_id VARCHAR(191) NOT NULL DEFAULT '',
			currency VARCHAR(16) NOT NULL DEFAULT '',
			amount BIGINT NOT NULL DEFAULT 0,
			receipt_hash CHAR(64) NOT NULL DEFAULT '',
			created_at INT NOT NULL,
			UNIQUE KEY uniq_apifull_payment_ledger_state (request_id, state),
			KEY idx_apifull_payment_ledger_user (user_id, id)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS apifull_payment_receipt (
			request_id BIGINT NOT NULL PRIMARY KEY,
			user_id BIGINT NOT NULL,
			provider VARCHAR(32) NOT NULL,
			transaction_id VARCHAR(191) NOT NULL,
			currency VARCHAR(16) NOT NULL DEFAULT '',
			amount BIGINT NOT NULL DEFAULT 0,
			peer_id BIGINT NOT NULL DEFAULT 0,
			msg_id INT NOT NULL DEFAULT 0,
			title VARCHAR(255) NOT NULL DEFAULT '',
			receipt MEDIUMBLOB NOT NULL,
			created_at INT NOT NULL,
			UNIQUE KEY uniq_apifull_payment_receipt_transaction (provider, transaction_id),
			KEY idx_apifull_payment_receipt_message (user_id, peer_id, msg_id)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS apifull_payment_entitlement_outbox (
			request_id BIGINT NOT NULL PRIMARY KEY,
			user_id BIGINT NOT NULL,
			provider VARCHAR(32) COLLATE utf8mb4_bin NOT NULL,
			transaction_id VARCHAR(191) COLLATE utf8mb4_bin NOT NULL,
			months TINYINT UNSIGNED NOT NULL,
			state VARCHAR(16) NOT NULL DEFAULT 'pending',
			attempts INT NOT NULL DEFAULT 0,
			next_attempt_at BIGINT NOT NULL,
			last_error VARCHAR(255) NOT NULL DEFAULT '',
			created_at BIGINT NOT NULL,
			updated_at BIGINT NOT NULL,
			UNIQUE KEY uniq_apifull_payment_entitlement_transaction (provider, transaction_id),
			KEY idx_apifull_payment_entitlement_due (state, next_attempt_at, request_id)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS apifull_report (
			id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
			actor_user_id BIGINT NOT NULL,
			kind VARCHAR(64) NOT NULL,
			target_type VARCHAR(32) NOT NULL,
			target_id BIGINT NOT NULL DEFAULT 0,
			dedupe_key CHAR(64) NOT NULL,
			payload MEDIUMBLOB NOT NULL,
			state VARCHAR(16) NOT NULL DEFAULT 'pending',
			created_at INT NOT NULL,
			updated_at INT NOT NULL,
			UNIQUE KEY uniq_apifull_report_dedupe (dedupe_key),
			KEY idx_apifull_report_actor (actor_user_id, id),
			KEY idx_apifull_report_state (state, id),
			KEY idx_apifull_report_target (target_type, target_id, id)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS apifull_gift (
			id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
			from_user BIGINT NOT NULL,
			to_user BIGINT NOT NULL,
			slug VARCHAR(191) NOT NULL,
			stars BIGINT NOT NULL DEFAULT 0,
			saved TINYINT NOT NULL DEFAULT 1,
			KEY idx_apifull_gift_to (to_user)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS apifull_username (
			username VARCHAR(64) NOT NULL PRIMARY KEY,
			owner_user_id BIGINT NOT NULL,
			kind VARCHAR(16) NOT NULL
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS apifull_group_call (
			id BIGINT NOT NULL PRIMARY KEY,
			access_hash BIGINT NOT NULL,
			creator_user_id BIGINT NOT NULL,
			channel_id BIGINT NOT NULL DEFAULT 0,
			title VARCHAR(255) NOT NULL DEFAULT '',
			rtmp_stream TINYINT NOT NULL DEFAULT 0,
			conference TINYINT NOT NULL DEFAULT 0,
			schedule_date INT NULL,
			participants MEDIUMTEXT NOT NULL,
			created_at INT NOT NULL
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS apifull_group_call_settings (
			call_id BIGINT NOT NULL PRIMARY KEY,
			join_muted TINYINT NOT NULL DEFAULT 0,
			messages_enabled TINYINT NOT NULL DEFAULT 1,
			send_paid_messages_stars BIGINT NULL,
			record_active TINYINT NOT NULL DEFAULT 0,
			record_video TINYINT NOT NULL DEFAULT 0,
			record_title VARCHAR(255) NOT NULL DEFAULT '',
			record_video_portrait TINYINT NOT NULL DEFAULT 0,
			scheduled_started TINYINT NOT NULL DEFAULT 0,
			updated_at INT NOT NULL
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS apifull_group_call_invite (
			call_id BIGINT NOT NULL PRIMARY KEY,
			creator_user_id BIGINT NOT NULL,
			token_hash VARBINARY(32) NOT NULL,
			can_self_unmute TINYINT NOT NULL DEFAULT 0,
			revoked_at INT NOT NULL DEFAULT 0,
			created_at INT NOT NULL,
			UNIQUE KEY uniq_apifull_group_call_invite_token (token_hash),
			KEY idx_apifull_group_call_invite_creator (creator_user_id)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS apifull_group_call_conference (
			call_id BIGINT NOT NULL PRIMARY KEY,
			public_key VARBINARY(256) NOT NULL,
			block MEDIUMBLOB NOT NULL,
			params MEDIUMTEXT NOT NULL,
			updated_at INT NOT NULL
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS apifull_group_call_participant (
			call_id BIGINT NOT NULL,
			user_id BIGINT NOT NULL,
			media_source INT NULL,
			muted TINYINT NOT NULL DEFAULT 0,
			volume INT NULL,
			raise_hand TINYINT NOT NULL DEFAULT 0,
			video_stopped TINYINT NOT NULL DEFAULT 0,
			video_paused TINYINT NOT NULL DEFAULT 0,
			presentation_paused TINYINT NOT NULL DEFAULT 0,
			presentation_active TINYINT NOT NULL DEFAULT 0,
			presentation_params MEDIUMTEXT NOT NULL,
			join_params MEDIUMTEXT NOT NULL,
			updated_at INT NOT NULL,
			PRIMARY KEY (call_id, user_id),
			UNIQUE KEY uniq_apifull_group_call_participant_source (call_id, media_source),
			KEY idx_apifull_group_call_participant_user (user_id)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS apifull_group_call_subscription (
			call_id BIGINT NOT NULL,
			user_id BIGINT NOT NULL,
			subscribed TINYINT NOT NULL DEFAULT 0,
			updated_at INT NOT NULL,
			PRIMARY KEY (call_id, user_id)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS apifull_group_call_send_as (
			call_id BIGINT NOT NULL,
			user_id BIGINT NOT NULL,
			send_as MEDIUMTEXT NOT NULL,
			updated_at INT NOT NULL,
			PRIMARY KEY (call_id, user_id)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS apifull_group_call_message (
			id INT NOT NULL AUTO_INCREMENT PRIMARY KEY,
			call_id BIGINT NOT NULL,
			sender_user_id BIGINT NOT NULL,
			random_id BIGINT NOT NULL,
			message MEDIUMTEXT NOT NULL,
			send_as MEDIUMTEXT NOT NULL,
			paid_stars BIGINT NULL,
			date INT NOT NULL,
			deleted TINYINT NOT NULL DEFAULT 0,
			UNIQUE KEY uniq_apifull_group_call_message_random (call_id, random_id),
			KEY idx_apifull_group_call_message_call (call_id, id),
			KEY idx_apifull_group_call_message_sender (call_id, sender_user_id, id)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS apifull_channel_message (
				channel_id BIGINT NOT NULL,
				message_id INT NOT NULL,
			sender_user_id BIGINT NOT NULL,
			date INT NOT NULL,
			message TEXT NOT NULL,
			edited TINYINT NOT NULL DEFAULT 0,
			edited_at INT NOT NULL DEFAULT 0,
			reply_to_msg_id INT NOT NULL DEFAULT 0,
			reply_to_top_id INT NOT NULL DEFAULT 0,
			content_json MEDIUMTEXT NULL,
			pinned TINYINT NOT NULL DEFAULT 0,
				PRIMARY KEY (channel_id, message_id)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS apifull_channel_message_request (
				channel_id BIGINT NOT NULL,
				sender_user_id BIGINT NOT NULL,
				random_id BIGINT NOT NULL,
				message_id INT NOT NULL,
				pts INT NOT NULL,
				request_hash BINARY(32) NOT NULL,
				created_at INT NOT NULL,
				PRIMARY KEY (channel_id, sender_user_id, random_id),
				KEY idx_apifull_channel_message_request_message (channel_id, message_id)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS apifull_channel_delivery_outbox (
			id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
			channel_id BIGINT NOT NULL,
			pts_from INT NOT NULL,
			pts_to INT NOT NULL,
			sender_user_id BIGINT NOT NULL,
			exclude_auth_key_id BIGINT NOT NULL DEFAULT 0,
			state VARCHAR(16) NOT NULL DEFAULT 'pending',
			payload MEDIUMBLOB NOT NULL,
			created_at BIGINT NOT NULL,
			UNIQUE KEY uniq_apifull_channel_delivery_event (channel_id, pts_from, pts_to),
			KEY idx_apifull_channel_delivery_channel (channel_id, id)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS apifull_channel_delivery_recipient (
			delivery_id BIGINT NOT NULL,
			user_id BIGINT NOT NULL,
			state VARCHAR(16) NOT NULL DEFAULT 'pending',
			attempts INT NOT NULL DEFAULT 0,
			next_attempt_at BIGINT NOT NULL,
			delivered_at BIGINT NOT NULL DEFAULT 0,
			last_error VARCHAR(255) NOT NULL DEFAULT '',
			PRIMARY KEY (delivery_id, user_id),
			KEY idx_apifull_channel_delivery_due (state, next_attempt_at, delivery_id)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS apifull_channel_message_hidden (
			user_id BIGINT NOT NULL,
			channel_id BIGINT NOT NULL,
			message_id INT NOT NULL,
			PRIMARY KEY (user_id, channel_id, message_id),
			KEY idx_apifull_channel_hidden_message (channel_id, message_id)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS apifull_channel_message_seq (
			channel_id BIGINT NOT NULL PRIMARY KEY,
			last_message_id INT NOT NULL,
			pts INT NOT NULL
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS apifull_channel_event (
			channel_id BIGINT NOT NULL,
			pts INT NOT NULL,
			pts_count INT NOT NULL DEFAULT 1,
			event_type VARCHAR(16) NOT NULL,
			message_ids MEDIUMTEXT NOT NULL,
			sender_user_id BIGINT NOT NULL DEFAULT 0,
			date INT NOT NULL DEFAULT 0,
			message TEXT NOT NULL,
			content_json MEDIUMTEXT NULL,
			edited_at INT NOT NULL DEFAULT 0,
			pinned TINYINT NOT NULL DEFAULT 0,
			PRIMARY KEY (channel_id, pts),
			KEY idx_apifull_channel_event_cursor (channel_id, pts)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS apifull_channel_read_state (
			user_id BIGINT NOT NULL,
			channel_id BIGINT NOT NULL,
			read_max_id INT NOT NULL DEFAULT 0,
			PRIMARY KEY (user_id, channel_id)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS apifull_channel_message_content_read (
			user_id BIGINT NOT NULL,
			channel_id BIGINT NOT NULL,
			message_id INT NOT NULL,
			read_at INT NOT NULL,
			PRIMARY KEY (user_id, channel_id, message_id),
			KEY idx_apifull_channel_content_read_message (channel_id, message_id)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS apifull_secret_chat (
			id INT NOT NULL PRIMARY KEY,
			access_hash BIGINT NOT NULL,
			admin_user_id BIGINT NOT NULL,
			participant_user_id BIGINT NOT NULL,
			state VARCHAR(16) NOT NULL,
			g_a VARBINARY(256) NOT NULL,
			g_b VARBINARY(256) NULL,
			key_fingerprint BIGINT NOT NULL DEFAULT 0,
			created_at INT NOT NULL,
			accepted_at INT NOT NULL DEFAULT 0,
			discarded_at INT NOT NULL DEFAULT 0,
			discarded_by_user_id BIGINT NOT NULL DEFAULT 0,
			history_deleted TINYINT NOT NULL DEFAULT 0,
			KEY idx_apifull_secret_chat_admin (admin_user_id),
			KEY idx_apifull_secret_chat_participant (participant_user_id)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS apifull_secret_chat_device_key (
			chat_id INT NOT NULL,
			user_id BIGINT NOT NULL,
			device_id BIGINT NOT NULL,
			epoch INT NOT NULL,
			public_key VARBINARY(256) NOT NULL,
			fingerprint BIGINT NOT NULL DEFAULT 0,
			created_at INT NOT NULL,
			PRIMARY KEY (chat_id, user_id, device_id, epoch),
			KEY idx_apifull_secret_device_key (chat_id, user_id, device_id, epoch)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS apifull_secret_user_state (
			user_id BIGINT NOT NULL PRIMARY KEY,
			last_qts INT NOT NULL DEFAULT 0,
			confirmed_qts INT NOT NULL DEFAULT 0
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS apifull_secret_message (
			id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
			chat_id INT NOT NULL,
			sender_user_id BIGINT NOT NULL,
			recipient_user_id BIGINT NOT NULL,
			random_id BIGINT NOT NULL,
			qts INT NOT NULL,
			date INT NOT NULL,
			encrypted_data MEDIUMBLOB NOT NULL,
			service TINYINT NOT NULL DEFAULT 0,
			file_id BIGINT NULL,
			file_access_hash BIGINT NULL,
			file_size BIGINT NULL,
			file_dc_id INT NULL,
			file_key_fingerprint INT NULL,
			acknowledged_at INT NOT NULL DEFAULT 0,
			read_at INT NOT NULL DEFAULT 0,
			UNIQUE KEY uniq_apifull_secret_sender_random (sender_user_id, random_id),
			UNIQUE KEY uniq_apifull_secret_recipient_qts (recipient_user_id, qts),
			KEY idx_apifull_secret_message_chat (chat_id),
			KEY idx_apifull_secret_message_recipient (recipient_user_id, acknowledged_at)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
	}
	for _, stmt := range stmts {
		if postgres {
			converted, indexes, err := postgresDDL(stmt)
			if err != nil {
				return err
			}
			stmt = converted
			if _, err := conn.Exec(stmt); err != nil {
				return err
			}
			for _, index := range indexes {
				if _, err := conn.Exec(index); err != nil {
					return err
				}
			}
			continue
		}
		if _, err := conn.Exec(stmt); err != nil {
			return err
		}
	}
	if postgres {
		// Chatlist state and public invite slugs are PostgreSQL-owned records.
		// Keep the state document and its slug index separate so invite lookup
		// never requires reading another user's private state blob.
		for _, stmt := range []string{
			`CREATE TABLE IF NOT EXISTS apifull_chatlist_state (
				user_id BIGINT PRIMARY KEY,
				state JSONB NOT NULL DEFAULT '{}'::jsonb,
				updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
			)`,
			`CREATE TABLE IF NOT EXISTS apifull_chatlist_invite (
				slug TEXT PRIMARY KEY,
				owner_user_id BIGINT NOT NULL,
				filter_id INTEGER NOT NULL,
				title TEXT NOT NULL DEFAULT '',
				peers JSONB NOT NULL DEFAULT '[]'::jsonb,
				created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
			)`,
			`CREATE INDEX IF NOT EXISTS apifull_chatlist_invite_owner_idx ON apifull_chatlist_invite (owner_user_id, slug)`,
		} {
			if _, err := conn.Exec(stmt); err != nil {
				return err
			}
		}
		return nil
	}
	if _, err := conn.Exec(`ALTER TABLE apifull_channel_message ADD COLUMN pinned TINYINT NOT NULL DEFAULT 0`); err != nil && !duplicateColumn(err) {
		return err
	}
	for _, stmt := range []string{
		`ALTER TABLE apifull_group_call ADD COLUMN conference TINYINT NOT NULL DEFAULT 0`,
		`ALTER TABLE apifull_channel ADD COLUMN migrated_from_chat_id BIGINT NULL`,
		`ALTER TABLE apifull_channel_member ADD COLUMN admin_rights VARCHAR(2048) NOT NULL DEFAULT ''`,
		`ALTER TABLE apifull_channel_member ADD COLUMN admin_rank VARCHAR(64) NOT NULL DEFAULT ''`,
		`ALTER TABLE apifull_channel_member ADD COLUMN banned_rights VARCHAR(2048) NOT NULL DEFAULT ''`,
		`ALTER TABLE apifull_channel_member ADD COLUMN banned_by_user_id BIGINT NOT NULL DEFAULT 0`,
		`ALTER TABLE apifull_channel_member ADD COLUMN banned_at INT NOT NULL DEFAULT 0`,
		`ALTER TABLE apifull_channel ADD COLUMN signatures_enabled TINYINT NOT NULL DEFAULT 0`,
		`ALTER TABLE apifull_channel ADD COLUMN signature_profiles_enabled TINYINT NOT NULL DEFAULT 0`,
		`ALTER TABLE apifull_channel ADD COLUMN antispam TINYINT NOT NULL DEFAULT 0`,
		`ALTER TABLE apifull_channel ADD COLUMN autotranslation TINYINT NOT NULL DEFAULT 0`,
		`ALTER TABLE apifull_channel ADD COLUMN hidden_prehistory TINYINT NOT NULL DEFAULT 0`,
		`ALTER TABLE apifull_channel ADD COLUMN participants_hidden TINYINT NOT NULL DEFAULT 0`,
		`ALTER TABLE apifull_channel ADD COLUMN slowmode_seconds INT NOT NULL DEFAULT 0`,
		`ALTER TABLE apifull_channel ADD COLUMN color INT NULL`,
		`ALTER TABLE apifull_channel ADD COLUMN background_emoji_id BIGINT NULL`,
		`ALTER TABLE apifull_channel ADD COLUMN profile_color INT NULL`,
		`ALTER TABLE apifull_channel ADD COLUMN profile_background_emoji_id BIGINT NULL`,
		`ALTER TABLE apifull_channel ADD COLUMN photo_id BIGINT NOT NULL DEFAULT 0`,
		`ALTER TABLE apifull_channel ADD COLUMN photo_dc_id INT NOT NULL DEFAULT 0`,
		`ALTER TABLE apifull_channel ADD COLUMN photo_has_video TINYINT NOT NULL DEFAULT 0`,
		`ALTER TABLE apifull_channel ADD COLUMN location_lat DOUBLE NULL`,
		`ALTER TABLE apifull_channel ADD COLUMN location_long DOUBLE NULL`,
		`ALTER TABLE apifull_channel ADD COLUMN location_address VARCHAR(255) NOT NULL DEFAULT ''`,
		`ALTER TABLE apifull_channel ADD COLUMN username VARCHAR(64) NOT NULL DEFAULT ''`,
		`ALTER TABLE apifull_channel ADD COLUMN discussion_group_id BIGINT NULL`,
		`ALTER TABLE apifull_channel_message ADD COLUMN reply_to_msg_id INT NOT NULL DEFAULT 0`,
		`ALTER TABLE apifull_channel_message ADD COLUMN reply_to_top_id INT NOT NULL DEFAULT 0`,
		`ALTER TABLE apifull_channel_message ADD COLUMN content_json MEDIUMTEXT NULL`,
		`ALTER TABLE apifull_channel_event ADD COLUMN content_json MEDIUMTEXT NULL`,
		`ALTER TABLE apifull_group_call ADD COLUMN title VARCHAR(255) NOT NULL DEFAULT ''`,
		`ALTER TABLE apifull_group_call ADD COLUMN rtmp_stream TINYINT NOT NULL DEFAULT 0`,
		`ALTER TABLE apifull_group_call ADD COLUMN schedule_date INT NULL`,
		`ALTER TABLE apifull_group_call_participant ADD COLUMN presentation_active TINYINT NOT NULL DEFAULT 0`,
		`ALTER TABLE apifull_group_call_participant ADD COLUMN join_params MEDIUMTEXT NOT NULL`,
		`ALTER TABLE apifull_group_call_participant ADD COLUMN media_source INT NULL`,
		`ALTER TABLE apifull_call ADD COLUMN ga_hash VARBINARY(256) NULL`,
		`ALTER TABLE apifull_call ADD COLUMN gb VARBINARY(256) NULL`,
		`ALTER TABLE apifull_call ADD COLUMN ga VARBINARY(256) NULL`,
		`ALTER TABLE apifull_call ADD COLUMN protocol MEDIUMTEXT NULL`,
		`ALTER TABLE apifull_call ADD COLUMN key_fingerprint BIGINT NOT NULL DEFAULT 0`,
		`ALTER TABLE apifull_call ADD COLUMN received_at INT NOT NULL DEFAULT 0`,
		`ALTER TABLE apifull_call ADD COLUMN accepted_at INT NOT NULL DEFAULT 0`,
		`ALTER TABLE apifull_call ADD COLUMN confirmed_at INT NOT NULL DEFAULT 0`,
		`ALTER TABLE apifull_call ADD COLUMN discarded_at INT NOT NULL DEFAULT 0`,
		`ALTER TABLE apifull_call ADD COLUMN discarded_by BIGINT NOT NULL DEFAULT 0`,
		`ALTER TABLE apifull_call ADD COLUMN duration INT NOT NULL DEFAULT 0`,
		`ALTER TABLE apifull_call ADD COLUMN reason VARCHAR(64) NOT NULL DEFAULT ''`,
	} {
		if _, err := conn.Exec(stmt); err != nil && !duplicateColumn(err) {
			return err
		}
	}
	if _, err := conn.Exec(`CREATE UNIQUE INDEX uniq_apifull_group_call_participant_source ON apifull_group_call_participant (call_id, media_source)`); err != nil && !duplicateIndex(err) {
		return err
	}
	// Older deployments used NOT NULL DEFAULT 0 for this optional link. Make
	// it nullable before enforcing the unique index so ordinary channels do not
	// collide on the same sentinel value.
	if _, err := conn.Exec(`ALTER TABLE apifull_channel MODIFY COLUMN migrated_from_chat_id BIGINT NULL DEFAULT NULL`); err != nil {
		return err
	}
	if _, err := conn.Exec(`CREATE UNIQUE INDEX uniq_apifull_channel_migrated_chat ON apifull_channel (migrated_from_chat_id)`); err != nil && !duplicateIndex(err) {
		return err
	}
	return nil
}

func duplicateColumn(err error) bool {
	var pe *pgconn.PgError
	return errors.As(err, &pe) && pe.Code == "42701"
}

// undefinedTable reports a missing optional legacy projection. APIFull owns
// the PostgreSQL channel tables, while installations that still expose the
// transitional channel service may also have the legacy tables. A missing
// legacy table must select the APIFull path instead of turning a valid request
// into an internal database error.
func undefinedTable(err error) bool {
	var pe *pgconn.PgError
	return errors.As(err, &pe) && pe.Code == "42P01"
}

func duplicateIndex(err error) bool {
	var pe *pgconn.PgError
	return errors.As(err, &pe) && pe.Code == "42P07"
}

func isDuplicateKey(err error) bool {
	var pe *pgconn.PgError
	return errors.As(err, &pe) && pe.Code == "23505"
}

func Ready() bool { return db != nil }

var (
	ErrInvalidStarsTransaction  = errors.New("invalid stars transaction")
	ErrStarsIdempotencyConflict = errors.New("stars idempotency key reused with a different amount")
	ErrStarsBalanceOverflow     = errors.New("stars balance overflow")
	ErrStarsBalanceExceeded     = errors.New("stars balance exceeded")
	ErrGiftNotConvertible       = errors.New("gift cannot be converted")
)

type Channel struct {
	ID                       int64
	AccessHash               int64
	MigratedFromChatID       int64
	Creator                  int64
	Title                    string
	About                    string
	Broadcast                bool
	Megagroup                bool
	Signatures               bool
	SignatureProfiles        bool
	Antispam                 bool
	Autotranslation          bool
	HiddenPrehistory         bool
	ParticipantsHidden       bool
	SlowmodeSeconds          int32
	Color                    *int32
	BackgroundEmojiID        *int64
	ProfileColor             *int32
	ProfileBackgroundEmojiID *int64
	PhotoID                  int64
	PhotoDCID                int32
	PhotoHasVideo            bool
	LocationLat              *float64
	LocationLong             *float64
	LocationAddress          string
	Username                 string
	DiscussionGroupID        int64
	CreatedAt                int64
}

// InactiveChannel is a canonical channel together with the last activity
// timestamp used by channels.getInactiveChannels.  A channel with no message
// activity uses its creation time as the last activity.
type InactiveChannel struct {
	Channel    Channel
	LastActive int64
}

type ChannelSettings struct {
	Signatures         *bool
	SignatureProfiles  *bool
	Antispam           *bool
	Autotranslation    *bool
	HiddenPrehistory   *bool
	ParticipantsHidden *bool
	SlowmodeSeconds    *int32
}

func SaveChannel(ch Channel) error {
	if db == nil {
		return errors.New("domain PostgreSQL is not open")
	}
	if ch.CreatedAt == 0 {
		ch.CreatedAt = time.Now().Unix()
	}
	_, err := execPostgresMutation(`INSERT INTO apifull_channel
		(id, access_hash, migrated_from_chat_id, creator_user_id, title, about, broadcast, megagroup, signatures_enabled, signature_profiles_enabled, antispam, autotranslation,
		hidden_prehistory, participants_hidden, slowmode_seconds, location_lat, location_long, location_address, username, created_at)
		VALUES ($1,$2,NULL,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19)
		ON CONFLICT (id) DO UPDATE SET title=EXCLUDED.title, about=EXCLUDED.about, username=EXCLUDED.username,
			broadcast=EXCLUDED.broadcast, megagroup=EXCLUDED.megagroup, signatures_enabled=EXCLUDED.signatures_enabled,
			signature_profiles_enabled=EXCLUDED.signature_profiles_enabled, antispam=EXCLUDED.antispam, autotranslation=EXCLUDED.autotranslation,
			hidden_prehistory=EXCLUDED.hidden_prehistory,
			participants_hidden=EXCLUDED.participants_hidden, slowmode_seconds=EXCLUDED.slowmode_seconds,
			location_lat=EXCLUDED.location_lat, location_long=EXCLUDED.location_long, location_address=EXCLUDED.location_address`,
		ch.ID, ch.AccessHash, ch.Creator, ch.Title, ch.About, boolInt(ch.Broadcast), boolInt(ch.Megagroup),
		boolInt(ch.Signatures), boolInt(ch.SignatureProfiles), boolInt(ch.Antispam), boolInt(ch.Autotranslation), boolInt(ch.HiddenPrehistory), boolInt(ch.ParticipantsHidden),
		ch.SlowmodeSeconds, ch.LocationLat, ch.LocationLong, ch.LocationAddress, ch.Username, ch.CreatedAt)
	return err
}

func LoadChannel(id int64) (Channel, bool, error) {
	var ch Channel
	if db == nil {
		return ch, false, errors.New("domain PostgreSQL is not open")
	}
	var broadcast, megagroup, signatures, signatureProfiles, antispam, autotranslation, hiddenPrehistory, participantsHidden int
	var color, profileColor sql.NullInt32
	var backgroundEmojiID, profileBackgroundEmojiID sql.NullInt64
	var locationLat, locationLong sql.NullFloat64
	err := db.QueryRow(`SELECT id, access_hash, COALESCE(migrated_from_chat_id, 0), creator_user_id, title, about, broadcast, megagroup, signatures_enabled,
		signature_profiles_enabled, antispam, autotranslation, hidden_prehistory, participants_hidden, slowmode_seconds, color, background_emoji_id,
		profile_color, profile_background_emoji_id, photo_id, photo_dc_id, photo_has_video,
		location_lat, location_long, location_address, username, COALESCE(discussion_group_id, 0), created_at
		FROM apifull_channel WHERE id=?`, id).Scan(
		&ch.ID, &ch.AccessHash, &ch.MigratedFromChatID, &ch.Creator, &ch.Title, &ch.About, &broadcast, &megagroup, &signatures, &signatureProfiles,
		&antispam, &autotranslation, &hiddenPrehistory, &participantsHidden, &ch.SlowmodeSeconds, &color, &backgroundEmojiID, &profileColor,
		&profileBackgroundEmojiID, &ch.PhotoID, &ch.PhotoDCID, &ch.PhotoHasVideo,
		&locationLat, &locationLong, &ch.LocationAddress, &ch.Username, &ch.DiscussionGroupID, &ch.CreatedAt)
	if err == sql.ErrNoRows {
		return Channel{}, false, nil
	}
	if err != nil {
		return Channel{}, false, err
	}
	ch.Broadcast = broadcast != 0
	ch.Megagroup = megagroup != 0
	ch.Signatures = signatures != 0
	ch.SignatureProfiles = signatureProfiles != 0
	ch.Antispam = antispam != 0
	ch.Autotranslation = autotranslation != 0
	ch.HiddenPrehistory = hiddenPrehistory != 0
	ch.ParticipantsHidden = participantsHidden != 0
	if color.Valid {
		ch.Color = &color.Int32
	}
	if backgroundEmojiID.Valid {
		ch.BackgroundEmojiID = &backgroundEmojiID.Int64
	}
	if profileColor.Valid {
		ch.ProfileColor = &profileColor.Int32
	}
	if profileBackgroundEmojiID.Valid {
		ch.ProfileBackgroundEmojiID = &profileBackgroundEmojiID.Int64
	}
	if locationLat.Valid {
		ch.LocationLat = &locationLat.Float64
	}
	if locationLong.Valid {
		ch.LocationLong = &locationLong.Float64
	}
	return ch, true, nil
}

// ListInactiveChannels returns channels owned by creatorID whose last
// persisted message (or creation time when they have no messages) is at or
// before cutoff.  The method intentionally uses the canonical channel and
// message tables instead of caller-local KV state.
func ListInactiveChannels(creatorID, cutoff int64) ([]InactiveChannel, error) {
	if db == nil {
		return nil, errors.New("domain PostgreSQL is not open")
	}
	rows, err := db.Query(`
		SELECT c.id, COALESCE(MAX(m.date), c.created_at) AS last_active
		FROM apifull_channel c
		LEFT JOIN apifull_channel_message m ON m.channel_id=c.id
		WHERE c.creator_user_id=?
		GROUP BY c.id, c.created_at
		HAVING COALESCE(MAX(m.date), c.created_at) <= ?
		ORDER BY last_active ASC, c.id ASC`, creatorID, cutoff)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]InactiveChannel, 0)
	for rows.Next() {
		var id, lastActive int64
		if err = rows.Scan(&id, &lastActive); err != nil {
			return nil, err
		}
		channel, ok, loadErr := LoadChannel(id)
		if loadErr != nil {
			return nil, loadErr
		}
		if !ok {
			continue
		}
		out = append(out, InactiveChannel{Channel: channel, LastActive: lastActive})
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// UpdateChannelPhoto stores the canonical photo reference after the media/DFS
// service has accepted it. Clearing the photo uses a zero id and dc id.
func UpdateChannelPhoto(userID, channelID, photoID int64, photoDCID int32, hasVideo bool) error {
	if db == nil {
		return errors.New("domain PostgreSQL is not open")
	}
	if photoID < 0 || photoDCID < 0 || (photoID == 0) != (photoDCID == 0) {
		return errors.New("invalid channel photo")
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = lockChannelOwner(tx, channelID, userID); err != nil {
		return err
	}
	if _, err = tx.Exec(`UPDATE apifull_channel SET photo_id=?, photo_dc_id=?, photo_has_video=? WHERE id=?`,
		photoID, photoDCID, boolInt(hasVideo), channelID); err != nil {
		return err
	}
	return tx.Commit()
}

func UpdateChannelColor(userID, channelID int64, forProfile bool, color *int32, backgroundEmojiID *int64) error {
	if db == nil {
		return errors.New("domain PostgreSQL is not open")
	}
	if color == nil && backgroundEmojiID == nil {
		return errors.New("no channel color fields supplied")
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = lockChannelOwner(tx, channelID, userID); err != nil {
		return err
	}
	colorColumn, emojiColumn := "color", "background_emoji_id"
	if forProfile {
		colorColumn, emojiColumn = "profile_color", "profile_background_emoji_id"
	}
	sets := make([]string, 0, 2)
	args := make([]any, 0, 3)
	if color != nil {
		sets = append(sets, colorColumn+"=?")
		args = append(args, *color)
	}
	if backgroundEmojiID != nil {
		sets = append(sets, emojiColumn+"=?")
		args = append(args, *backgroundEmojiID)
	}
	args = append(args, channelID)
	if _, err = tx.Exec(`UPDATE apifull_channel SET `+strings.Join(sets, ",")+` WHERE id=?`, args...); err != nil {
		return err
	}
	return tx.Commit()
}

func UpdateChannelSettings(userID, channelID int64, settings ChannelSettings) error {
	if db == nil {
		return errors.New("domain PostgreSQL is not open")
	}
	sets := []string{}
	args := []any{}
	if settings.Signatures != nil {
		sets = append(sets, "signatures_enabled=$"+strconv.Itoa(len(args)+1))
		args = append(args, boolInt(*settings.Signatures))
	}
	if settings.SignatureProfiles != nil {
		sets = append(sets, "signature_profiles_enabled=$"+strconv.Itoa(len(args)+1))
		args = append(args, boolInt(*settings.SignatureProfiles))
	}
	if settings.Antispam != nil {
		sets = append(sets, "antispam=$"+strconv.Itoa(len(args)+1))
		args = append(args, boolInt(*settings.Antispam))
	}
	if settings.Autotranslation != nil {
		sets = append(sets, "autotranslation=$"+strconv.Itoa(len(args)+1))
		args = append(args, boolInt(*settings.Autotranslation))
	}
	if settings.HiddenPrehistory != nil {
		sets = append(sets, "hidden_prehistory=$"+strconv.Itoa(len(args)+1))
		args = append(args, boolInt(*settings.HiddenPrehistory))
	}
	if settings.ParticipantsHidden != nil {
		sets = append(sets, "participants_hidden=$"+strconv.Itoa(len(args)+1))
		args = append(args, boolInt(*settings.ParticipantsHidden))
	}
	if settings.SlowmodeSeconds != nil {
		if *settings.SlowmodeSeconds < 0 {
			return errors.New("invalid channel slowmode seconds")
		}
		sets = append(sets, "slowmode_seconds=$"+strconv.Itoa(len(args)+1))
		args = append(args, *settings.SlowmodeSeconds)
	}
	if len(sets) == 0 {
		return errors.New("no channel settings supplied")
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = lockChannelOwner(tx, channelID, userID); err != nil {
		return err
	}
	args = append(args, channelID)
	if _, err = tx.Exec(`UPDATE apifull_channel SET `+strings.Join(sets, ",")+` WHERE id=$`+strconv.Itoa(len(args)), args...); err != nil {
		return err
	}
	return tx.Commit()
}

func UpdateChannelTitle(userID, channelID int64, title string) error {
	if db == nil {
		return errors.New("domain PostgreSQL is not open")
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = lockChannelOwner(tx, channelID, userID); err != nil {
		return err
	}
	if _, err = tx.Exec(`UPDATE apifull_channel SET title=? WHERE id=?`, title, channelID); err != nil {
		return err
	}
	return tx.Commit()
}

// UpdateChannelUsername stores the public username after the username service
// has accepted the change. Authorization belongs to that caller.
func UpdateChannelUsername(channelID int64, username string) error {
	if db == nil {
		return errors.New("domain PostgreSQL is not open")
	}
	result, err := execPostgresMutation(`UPDATE apifull_channel SET username=$1 WHERE id=$2`, username, channelID)
	if err != nil {
		return err
	}
	updated, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if updated == 0 {
		_, found, err := LoadChannel(channelID)
		if err != nil {
			return err
		}
		if !found {
			return ErrChannelMissing
		}
	}
	return nil
}

// UpdateChannelLocation replaces the channel's public location. A nil pair
// clears the location, matching inputGeoPointEmpty.
func UpdateChannelLocation(userID, channelID int64, lat, long *float64, address string) error {
	if db == nil {
		return errors.New("domain PostgreSQL is not open")
	}
	if (lat == nil) != (long == nil) {
		return ErrInvalidLocation
	}
	if lat != nil && (*lat < -90 || *lat > 90 || *long < -180 || *long > 180) {
		return ErrInvalidLocation
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = lockChannelOwner(tx, channelID, userID); err != nil {
		return err
	}
	if _, err = tx.Exec(`UPDATE apifull_channel SET location_lat=?, location_long=?, location_address=? WHERE id=?`, lat, long, address, channelID); err != nil {
		return err
	}
	return tx.Commit()
}

// DeleteChannel removes a channel and all APIFull-owned rows scoped to it.
// Only the channel creator can delete the channel. The owner lock keeps the
// authorization check and the cleanup in one transaction.
func DeleteChannel(userID, channelID int64) error {
	if db == nil {
		return errors.New("domain PostgreSQL is not open")
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = lockChannelOwner(tx, channelID, userID); err != nil {
		return err
	}
	for _, stmt := range []string{
		`DELETE FROM apifull_community_dialog_state WHERE community_id=?`,
		`DELETE FROM apifull_community_peer WHERE community_id=?`,
		`DELETE FROM apifull_community WHERE community_id=?`,
		`UPDATE apifull_boost_slot SET scope=NULL, peer_type=NULL, peer_id=NULL, updated_at=CURRENT_TIMESTAMP WHERE scope='premium' AND peer_type=4 AND peer_id=?`,
		`DELETE FROM apifull_boost_target WHERE peer_type=? AND peer_id=?`,
		`UPDATE apifull_channel SET discussion_group_id=NULL WHERE discussion_group_id=?`,
		`DELETE FROM apifull_channel_delivery_recipient WHERE delivery_id IN (SELECT id FROM apifull_channel_delivery_outbox WHERE channel_id=?)`,
		`DELETE FROM apifull_channel_delivery_outbox WHERE channel_id=?`,
		`DELETE FROM apifull_channel_event WHERE channel_id=?`,
		`DELETE FROM apifull_channel_message_request WHERE channel_id=?`,
		`DELETE FROM apifull_channel_message_hidden WHERE channel_id=?`,
		`DELETE FROM apifull_channel_message_content_read WHERE channel_id=?`,
		`DELETE FROM apifull_channel_message WHERE channel_id=?`,
		`DELETE FROM apifull_channel_message_seq WHERE channel_id=?`,
		`DELETE FROM apifull_channel_read_state WHERE channel_id=?`,
		`DELETE FROM apifull_forum_topic WHERE peer_key=?`,
		`DELETE FROM apifull_forum_channel_settings WHERE channel_id=?`,
		`DELETE FROM apifull_channel_admin_log WHERE channel_id=?`,
		// chat_invites is shared with basic chats. This row has already been
		// established as an APIFull channel under the creator lock above.
		`DELETE FROM chat_invite_participants WHERE chat_id=?`,
		`DELETE FROM chat_invites WHERE chat_id=?`,
		`DELETE FROM apifull_channel_member WHERE channel_id=?`,
		`DELETE FROM apifull_group_call_settings WHERE call_id IN (SELECT id FROM apifull_group_call WHERE channel_id=?)`,
		`DELETE FROM apifull_group_call_participant WHERE call_id IN (SELECT id FROM apifull_group_call WHERE channel_id=?)`,
		`DELETE FROM apifull_group_call_subscription WHERE call_id IN (SELECT id FROM apifull_group_call WHERE channel_id=?)`,
		`DELETE FROM apifull_group_call_send_as WHERE call_id IN (SELECT id FROM apifull_group_call WHERE channel_id=?)`,
		`DELETE FROM apifull_group_call_message WHERE call_id IN (SELECT id FROM apifull_group_call WHERE channel_id=?)`,
		`DELETE FROM apifull_group_call WHERE channel_id=?`,
		`DELETE FROM apifull_channel WHERE id=?`,
	} {
		if stmt == `DELETE FROM apifull_boost_target WHERE peer_type=? AND peer_id=?` {
			if _, err = tx.Exec(stmt, 4, channelID); err != nil {
				return err
			}
			continue
		}
		if stmt == `DELETE FROM apifull_forum_topic WHERE peer_key=?` {
			if _, err = tx.Exec(stmt, "channel:"+strconv.FormatInt(channelID, 10)); err != nil {
				return err
			}
			continue
		}
		if _, err = tx.Exec(stmt, channelID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func lockChannelOwner(tx *sql.Tx, channelID, userID int64) error {
	var creator int64
	err := tx.QueryRow(`SELECT creator_user_id FROM apifull_channel WHERE id=$1 FOR UPDATE`, channelID).Scan(&creator)
	if err == sql.ErrNoRows {
		return ErrChannelMissing
	}
	if err != nil {
		return err
	}
	if creator != userID {
		return ErrNotCreator
	}
	return nil
}

func ListByCreator(creator int64) ([]Channel, error) {
	if db == nil {
		return nil, errors.New("domain PostgreSQL is not open")
	}
	rows, err := db.Query(`SELECT id, access_hash, creator_user_id, title, about, broadcast, megagroup,
		signatures_enabled, signature_profiles_enabled, hidden_prehistory, participants_hidden, slowmode_seconds, username, created_at
		FROM apifull_channel WHERE creator_user_id=? ORDER BY created_at DESC`, creator)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Channel
	for rows.Next() {
		var ch Channel
		var broadcast, megagroup, signatures, signatureProfiles, hiddenPrehistory, participantsHidden int
		if err = rows.Scan(&ch.ID, &ch.AccessHash, &ch.Creator, &ch.Title, &ch.About, &broadcast, &megagroup,
			&signatures, &signatureProfiles, &hiddenPrehistory, &participantsHidden, &ch.SlowmodeSeconds, &ch.Username, &ch.CreatedAt); err != nil {
			return nil, err
		}
		ch.Broadcast = broadcast != 0
		ch.Megagroup = megagroup != 0
		ch.Signatures = signatures != 0
		ch.SignatureProfiles = signatureProfiles != 0
		ch.HiddenPrehistory = hiddenPrehistory != 0
		ch.ParticipantsHidden = participantsHidden != 0
		out = append(out, ch)
	}
	return out, rows.Err()
}

type Call struct {
	ID             int64
	AccessHash     int64
	AdminID        int64
	ParticipantID  int64
	State          string
	Video          bool
	GAHash         []byte
	GB             []byte
	GA             []byte
	Protocol       string
	KeyFingerprint int64
	ReceivedAt     int64
	AcceptedAt     int64
	ConfirmedAt    int64
	DiscardedAt    int64
	DiscardedBy    int64
	Duration       int32
	Reason         string
	CreatedAt      int64
}

var (
	ErrCallNotFound     = errors.New("call not found")
	ErrCallForbidden    = errors.New("call access denied")
	ErrCallInvalidState = errors.New("invalid call state")
	ErrCallProtocol     = errors.New("invalid call protocol")
)

// CallTransition describes one server-authorized call state transition. The
// row is locked while the actor, access hash, and expected state are checked.
type CallTransition struct {
	ActorID        int64
	AccessHash     int64
	FromStates     []string
	ToState        string
	GB             []byte
	GA             []byte
	Protocol       string
	KeyFingerprint int64
	ReceivedAt     int64
	AcceptedAt     int64
	ConfirmedAt    int64
	DiscardedAt    int64
	DiscardedBy    int64
	Duration       int32
	Reason         string
}

// SaveCallArtifact authorizes a call participant and stores a client-produced
// call artifact. Artifacts are append-only except for ratings, where a retry
// replaces the caller's previous rating for the same call.
func SaveCallArtifact(callID, accessHash, userID int64, kind string, payload []byte) (Call, error) {
	if db == nil {
		return Call{}, errors.New("domain PostgreSQL is not open")
	}
	if callID <= 0 || accessHash == 0 || userID <= 0 || strings.TrimSpace(kind) == "" || len(payload) == 0 || len(payload) > 1<<20 {
		return Call{}, ErrCallInvalidState
	}
	switch kind {
	case "rating", "debug", "signaling", "log":
	default:
		return Call{}, ErrCallInvalidState
	}
	tx, err := db.Begin()
	if err != nil {
		return Call{}, err
	}
	defer func() { _ = tx.Rollback() }()
	var call Call
	var video int
	var protocol sql.NullString
	err = tx.QueryRow(`SELECT id, access_hash, admin_id, participant_id, state, video, ga_hash, gb, ga, protocol,
		key_fingerprint, received_at, accepted_at, confirmed_at, discarded_at, discarded_by, duration, reason, created_at
		FROM apifull_call WHERE id=? FOR UPDATE`, callID).
		Scan(&call.ID, &call.AccessHash, &call.AdminID, &call.ParticipantID, &call.State, &video,
			&call.GAHash, &call.GB, &call.GA, &protocol, &call.KeyFingerprint, &call.ReceivedAt,
			&call.AcceptedAt, &call.ConfirmedAt, &call.DiscardedAt, &call.DiscardedBy, &call.Duration,
			&call.Reason, &call.CreatedAt)
	if err == sql.ErrNoRows {
		return Call{}, ErrCallNotFound
	}
	if err != nil {
		return Call{}, err
	}
	call.Video = video != 0
	if protocol.Valid {
		call.Protocol = protocol.String
	}
	if call.AccessHash != accessHash || (call.AdminID != userID && call.ParticipantID != userID) {
		return Call{}, ErrCallForbidden
	}
	if kind == "signaling" && call.State == "discarded" {
		return Call{}, ErrCallInvalidState
	}
	if kind == "rating" {
		if _, err = tx.Exec(`DELETE FROM apifull_call_artifact WHERE call_id=? AND user_id=? AND kind=?`, callID, userID, kind); err != nil {
			return Call{}, err
		}
	}
	if _, err = tx.Exec(`INSERT INTO apifull_call_artifact (call_id, user_id, kind, payload, created_at) VALUES (?,?,?,?,?)`,
		callID, userID, kind, payload, time.Now().Unix()); err != nil {
		return Call{}, err
	}
	if err = tx.Commit(); err != nil {
		return Call{}, err
	}
	return call, nil
}

func SaveCall(call Call) error {
	if db == nil {
		return errors.New("domain PostgreSQL is not open")
	}
	if call.CreatedAt == 0 {
		call.CreatedAt = time.Now().Unix()
	}
	_, err := execPostgresMutation(`INSERT INTO apifull_call
		(id, access_hash, admin_id, participant_id, state, video, ga_hash, gb, ga, protocol,
		 key_fingerprint, received_at, accepted_at, confirmed_at, discarded_at, discarded_by, duration, reason, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19)
		ON CONFLICT (id) DO UPDATE SET state=EXCLUDED.state, participant_id=EXCLUDED.participant_id,
			video=EXCLUDED.video, ga_hash=EXCLUDED.ga_hash, gb=EXCLUDED.gb, ga=EXCLUDED.ga,
			protocol=EXCLUDED.protocol, key_fingerprint=EXCLUDED.key_fingerprint,
			received_at=EXCLUDED.received_at, accepted_at=EXCLUDED.accepted_at,
			confirmed_at=EXCLUDED.confirmed_at, discarded_at=EXCLUDED.discarded_at,
			discarded_by=EXCLUDED.discarded_by, duration=EXCLUDED.duration, reason=EXCLUDED.reason`,
		call.ID, call.AccessHash, call.AdminID, call.ParticipantID, call.State, boolInt(call.Video),
		call.GAHash, call.GB, call.GA, nullableString(call.Protocol), call.KeyFingerprint,
		call.ReceivedAt, call.AcceptedAt, call.ConfirmedAt, call.DiscardedAt, call.DiscardedBy,
		call.Duration, call.Reason, call.CreatedAt)
	return err
}

func LoadCall(id int64) (Call, bool, error) {
	var call Call
	if db == nil {
		return call, false, errors.New("domain PostgreSQL is not open")
	}
	var video int
	var protocol sql.NullString
	err := db.QueryRow(`SELECT id, access_hash, admin_id, participant_id, state, video, ga_hash, gb, ga, protocol,
		key_fingerprint, received_at, accepted_at, confirmed_at, discarded_at, discarded_by, duration, reason, created_at
		FROM apifull_call WHERE id=?`, id).
		Scan(&call.ID, &call.AccessHash, &call.AdminID, &call.ParticipantID, &call.State, &video,
			&call.GAHash, &call.GB, &call.GA, &protocol, &call.KeyFingerprint, &call.ReceivedAt,
			&call.AcceptedAt, &call.ConfirmedAt, &call.DiscardedAt, &call.DiscardedBy, &call.Duration,
			&call.Reason, &call.CreatedAt)
	if err == sql.ErrNoRows {
		return Call{}, false, nil
	}
	if err != nil {
		return Call{}, false, err
	}
	call.Video = video != 0
	if protocol.Valid {
		call.Protocol = protocol.String
	}
	return call, true, nil
}

// TransitionCall atomically authorizes and applies one call state transition.
// It is intentionally the only lifecycle write used by the VoIP handlers.
func TransitionCall(id int64, transition CallTransition) (Call, error) {
	if db == nil {
		return Call{}, errors.New("domain PostgreSQL is not open")
	}
	tx, err := db.Begin()
	if err != nil {
		return Call{}, err
	}
	defer func() { _ = tx.Rollback() }()
	var call Call
	var video int
	var protocol sql.NullString
	err = tx.QueryRow(`SELECT id, access_hash, admin_id, participant_id, state, video, ga_hash, gb, ga, protocol,
		key_fingerprint, received_at, accepted_at, confirmed_at, discarded_at, discarded_by, duration, reason, created_at
		FROM apifull_call WHERE id=? FOR UPDATE`, id).
		Scan(&call.ID, &call.AccessHash, &call.AdminID, &call.ParticipantID, &call.State, &video,
			&call.GAHash, &call.GB, &call.GA, &protocol, &call.KeyFingerprint, &call.ReceivedAt,
			&call.AcceptedAt, &call.ConfirmedAt, &call.DiscardedAt, &call.DiscardedBy, &call.Duration,
			&call.Reason, &call.CreatedAt)
	if err == sql.ErrNoRows {
		return Call{}, ErrCallNotFound
	}
	if err != nil {
		return Call{}, err
	}
	call.Video = video != 0
	if protocol.Valid {
		call.Protocol = protocol.String
	}
	if transition.AccessHash == 0 || transition.AccessHash != call.AccessHash ||
		(transition.ActorID != call.AdminID && transition.ActorID != call.ParticipantID) {
		return Call{}, ErrCallForbidden
	}
	if len(transition.FromStates) > 0 && !containsState(transition.FromStates, call.State) {
		return Call{}, ErrCallInvalidState
	}
	if transition.ToState == "received" || transition.ToState == "accepted" {
		if transition.ActorID != call.ParticipantID {
			return Call{}, ErrCallForbidden
		}
	}
	if transition.ToState == "confirmed" && transition.ActorID != call.AdminID {
		return Call{}, ErrCallForbidden
	}
	if transition.GA != nil && len(call.GAHash) > 0 {
		digest := sha256.Sum256(transition.GA)
		if !bytes.Equal(digest[:], call.GAHash) {
			return Call{}, ErrCallProtocol
		}
	}
	if transition.ToState == "" {
		return Call{}, ErrCallInvalidState
	}
	sets := []string{"state=?"}
	args := []any{transition.ToState}
	if transition.GB != nil {
		sets = append(sets, "gb=?")
		args = append(args, transition.GB)
	}
	if transition.GA != nil {
		sets = append(sets, "ga=?")
		args = append(args, transition.GA)
	}
	if transition.Protocol != "" {
		sets = append(sets, "protocol=?")
		args = append(args, transition.Protocol)
	}
	if transition.KeyFingerprint != 0 {
		sets = append(sets, "key_fingerprint=?")
		args = append(args, transition.KeyFingerprint)
	}
	if transition.ReceivedAt != 0 {
		sets = append(sets, "received_at=?")
		args = append(args, transition.ReceivedAt)
	}
	if transition.AcceptedAt != 0 {
		sets = append(sets, "accepted_at=?")
		args = append(args, transition.AcceptedAt)
	}
	if transition.ConfirmedAt != 0 {
		sets = append(sets, "confirmed_at=?")
		args = append(args, transition.ConfirmedAt)
	}
	if transition.DiscardedAt != 0 {
		sets = append(sets, "discarded_at=?")
		args = append(args, transition.DiscardedAt)
	}
	if transition.DiscardedBy != 0 {
		sets = append(sets, "discarded_by=?")
		args = append(args, transition.DiscardedBy)
	}
	if transition.Duration != 0 {
		sets = append(sets, "duration=?")
		args = append(args, transition.Duration)
	}
	if transition.Reason != "" {
		sets = append(sets, "reason=?")
		args = append(args, transition.Reason)
	}
	args = append(args, id)
	if _, err = tx.Exec(`UPDATE apifull_call SET `+strings.Join(sets, ", ")+` WHERE id=?`, args...); err != nil {
		return Call{}, err
	}
	if transition.ToState == "discarded" {
		if _, err = tx.Exec(`DELETE FROM apifull_call_artifact WHERE call_id=? AND kind='signaling'`, id); err != nil {
			return Call{}, err
		}
	}
	if err = tx.Commit(); err != nil {
		return Call{}, err
	}
	call.State = transition.ToState
	if transition.GB != nil {
		call.GB = append([]byte(nil), transition.GB...)
	}
	if transition.GA != nil {
		call.GA = append([]byte(nil), transition.GA...)
	}
	if transition.Protocol != "" {
		call.Protocol = transition.Protocol
	}
	if transition.KeyFingerprint != 0 {
		call.KeyFingerprint = transition.KeyFingerprint
	}
	call.ReceivedAt = maxInt64(call.ReceivedAt, transition.ReceivedAt)
	call.AcceptedAt = maxInt64(call.AcceptedAt, transition.AcceptedAt)
	call.ConfirmedAt = maxInt64(call.ConfirmedAt, transition.ConfirmedAt)
	call.DiscardedAt = maxInt64(call.DiscardedAt, transition.DiscardedAt)
	if transition.DiscardedBy != 0 {
		call.DiscardedBy = transition.DiscardedBy
	}
	if transition.Duration != 0 {
		call.Duration = transition.Duration
	}
	if transition.Reason != "" {
		call.Reason = transition.Reason
	}
	return call, nil
}

func containsState(states []string, state string) bool {
	for _, candidate := range states {
		if candidate == state {
			return true
		}
	}
	return false
}

func maxInt64(current, next int64) int64 {
	if next != 0 {
		return next
	}
	return current
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

// StarsTransaction is the durable portion of a Stars ledger entry. The local
// ledger intentionally exposes only fields stored in apifull_star_tx; provider
// metadata such as a charge title or payment date is not invented here.
type StarsTransaction struct {
	ID     int64
	Amount int64
	Idem   string
}

// StarsOffer is an operator/provider supplied Stars catalog row. Offers are
// returned to clients only when an active row exists; the API never invents a
// price or product identifier.
type StarsOffer struct {
	ID           int64
	Kind         string
	Stars        int64
	StoreProduct string
	Currency     string
	Amount       int64
	Extended     bool
}

// UpsertStarsOffer publishes one active catalog offer. Provider/bootstrap
// code may use this helper when synchronising its authoritative catalog.
func UpsertStarsOffer(kind string, stars int64, storeProduct, currency string, amount int64, extended bool) error {
	if db == nil {
		return errors.New("domain PostgreSQL is not open")
	}
	kind = strings.TrimSpace(kind)
	storeProduct = strings.TrimSpace(storeProduct)
	currency = strings.TrimSpace(currency)
	if (kind != "topup" && kind != "gift") || stars <= 0 || amount < 0 || (storeProduct == "" && currency == "") {
		return ErrInvalidStarsTransaction
	}
	_, err := execPostgresMutation(`INSERT INTO apifull_stars_offer
		(kind, stars, store_product, currency, amount, extended, active)
		VALUES ($1,$2,$3,$4,$5,$6,1)
		ON CONFLICT (kind, stars, store_product, currency, amount) DO UPDATE SET extended=EXCLUDED.extended, active=1`,
		kind, stars, storeProduct, currency, amount, boolInt(extended))
	return err
}

// ListStarsOffers returns active, provider-synchronised catalog rows in a
// stable order. An empty result is a real empty catalog.
func ListStarsOffers(kind string) ([]StarsOffer, error) {
	if db == nil {
		return nil, errors.New("domain PostgreSQL is not open")
	}
	if kind != "topup" && kind != "gift" {
		return nil, ErrInvalidStarsTransaction
	}
	rows, err := db.Query(`SELECT id, kind, stars, store_product, currency, amount, extended
		FROM apifull_stars_offer WHERE kind=$1 AND active=1 ORDER BY id`, kind)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	offers := make([]StarsOffer, 0)
	for rows.Next() {
		var offer StarsOffer
		var extended int
		if err = rows.Scan(&offer.ID, &offer.Kind, &offer.Stars, &offer.StoreProduct, &offer.Currency, &offer.Amount, &extended); err != nil {
			return nil, err
		}
		offer.Extended = extended != 0
		offers = append(offers, offer)
	}
	return offers, rows.Err()
}

func FindActiveStarsTopupOffer(stars int64, storeProduct, currency string, amount int64, extended *bool) (StarsOffer, bool, error) {
	if db == nil {
		return StarsOffer{}, false, errors.New("domain PostgreSQL is not open")
	}
	if stars <= 0 || amount < 0 {
		return StarsOffer{}, false, ErrInvalidStarsTransaction
	}
	rows, err := db.Query(`SELECT id, kind, stars, store_product, currency, amount, extended
		FROM apifull_stars_offer WHERE kind='topup' AND active=1 AND stars=$1 ORDER BY id`, stars)
	if err != nil {
		return StarsOffer{}, false, err
	}
	defer rows.Close()
	var match StarsOffer
	found := false
	for rows.Next() {
		var offer StarsOffer
		var isExtended int
		if err = rows.Scan(&offer.ID, &offer.Kind, &offer.Stars, &offer.StoreProduct, &offer.Currency, &offer.Amount, &isExtended); err != nil {
			return StarsOffer{}, false, err
		}
		offer.Extended = isExtended != 0
		if offer.StoreProduct != storeProduct || offer.Currency != currency || offer.Amount != amount || (extended != nil && offer.Extended != *extended) {
			continue
		}
		if found {
			return StarsOffer{}, false, ErrInvalidStarsTransaction
		}
		match, found = offer, true
	}
	if err = rows.Err(); err != nil {
		return StarsOffer{}, false, err
	}
	return match, found, nil
}

// ApplyStars adds delta (negative debits). The same idempotency key does not apply twice.
// A debit below zero returns an error and leaves the balance unchanged.
func ApplyStars(userID, delta int64, idem string) (int64, error) {
	if db == nil {
		return 0, errors.New("domain PostgreSQL is not open")
	}
	if userID <= 0 || delta == 0 || strings.TrimSpace(idem) == "" {
		return 0, ErrInvalidStarsTransaction
	}
	tx, err := db.Begin()
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	bal, err := applyStarsTx(tx, userID, delta, idem)
	if err != nil {
		return bal, err
	}
	if err = tx.Commit(); err != nil {
		return 0, err
	}
	return bal, nil
}

func applyStarsTx(tx *sql.Tx, userID, delta int64, idem string) (int64, error) {
	// Lock the balance before checking the ledger. A retry that waited for a
	// concurrent debit must see that debit's idempotency record before checking
	// the remaining balance. Every Stars writer uses this same lock order.
	if _, err := tx.Exec(`INSERT INTO apifull_stars (user_id, balance) VALUES ($1,0)
		ON CONFLICT (user_id) DO NOTHING`, userID); err != nil {
		return 0, err
	}
	var bal int64
	if err := tx.QueryRow(`SELECT balance FROM apifull_stars WHERE user_id=$1 FOR UPDATE`, userID).Scan(&bal); err != nil {
		return 0, err
	}
	var existing int64
	err := tx.QueryRow(`SELECT amount FROM apifull_star_tx WHERE user_id=$1 AND idem=$2`, userID, idem).Scan(&existing)
	if err == nil {
		if existing != delta {
			return bal, ErrStarsIdempotencyConflict
		}
		return bal, nil
	}
	if err != sql.ErrNoRows {
		return 0, err
	}
	if err = validateStarsDelta(bal, delta); err != nil {
		return bal, err
	}
	if _, err = tx.Exec(`INSERT INTO apifull_star_tx (user_id, amount, idem) VALUES ($1,$2,$3)`, userID, delta, idem); err != nil {
		return 0, err
	}
	if _, err = tx.Exec(`UPDATE apifull_stars SET balance=balance+$1 WHERE user_id=$2`, delta, userID); err != nil {
		return 0, err
	}
	return bal + delta, nil
}

func validateStarsDelta(balance, delta int64) error {
	if delta == 0 {
		return ErrInvalidStarsTransaction
	}
	if delta < 0 {
		if delta == math.MinInt64 || balance < -delta {
			return ErrStarsBalanceExceeded
		}
		return nil
	}
	if balance > math.MaxInt64-delta {
		return ErrStarsBalanceOverflow
	}
	return nil
}

func StarsBalance(userID int64) (int64, error) {
	if db == nil {
		return 0, errors.New("domain PostgreSQL is not open")
	}
	if userID <= 0 {
		return 0, ErrInvalidStarsTransaction
	}
	return balanceTx(db, userID)
}

// ListStarsTransactions returns durable ledger entries in id order. The
// direction filters are applied to the signed amount stored in the database.
func ListStarsTransactions(userID int64, inbound, outbound, ascending bool, offset, limit int) ([]StarsTransaction, error) {
	if db == nil {
		return nil, errors.New("domain PostgreSQL is not open")
	}
	if userID <= 0 || offset < 0 {
		return nil, ErrInvalidStarsTransaction
	}
	if limit <= 0 {
		limit = 100
	}
	order := "DESC"
	if ascending {
		order = "ASC"
	}
	where := "user_id=$1"
	args := []any{userID}
	if inbound && !outbound {
		where += " AND amount>0"
	} else if outbound && !inbound {
		where += " AND amount<0"
	}
	args = append(args, limit, offset)
	rows, err := db.Query(`SELECT id, amount, idem FROM apifull_star_tx WHERE `+where+` ORDER BY id `+order+` LIMIT $2 OFFSET $3`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	transactions := make([]StarsTransaction, 0)
	for rows.Next() {
		var item StarsTransaction
		if err = rows.Scan(&item.ID, &item.Amount, &item.Idem); err != nil {
			return nil, err
		}
		transactions = append(transactions, item)
	}
	return transactions, rows.Err()
}

func GetStarsTransaction(userID int64, id string) (StarsTransaction, bool, error) {
	if db == nil {
		return StarsTransaction{}, false, errors.New("domain PostgreSQL is not open")
	}
	if userID <= 0 || strings.TrimSpace(id) == "" {
		return StarsTransaction{}, false, ErrInvalidStarsTransaction
	}
	var item StarsTransaction
	err := db.QueryRow(`SELECT id, amount, idem FROM apifull_star_tx WHERE user_id=$1 AND idem=$2`, userID, id).
		Scan(&item.ID, &item.Amount, &item.Idem)
	if err == sql.ErrNoRows {
		return StarsTransaction{}, false, nil
	}
	if err != nil {
		return StarsTransaction{}, false, err
	}
	return item, true, nil
}

var ErrGiftNotFound = errors.New("gift not found")

type Gift struct {
	ID    int64
	From  int64
	To    int64
	Slug  string
	Stars int64
	Saved bool
}

type rower interface {
	QueryRow(query string, args ...any) *sql.Row
}

func balanceTx(q rower, userID int64) (int64, error) {
	var bal int64
	err := q.QueryRow(`SELECT balance FROM apifull_stars WHERE user_id=$1`, userID).Scan(&bal)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return bal, err
}

// SaveGift records a gift issued by an authoritative commerce/provider
// integration. Client RPCs must use SetGiftSaved instead; they must not call
// this function to create inventory.
func SaveGift(from, to int64, slug string, stars int64) error {
	if db == nil {
		return errors.New("domain PostgreSQL is not open")
	}
	if from <= 0 || to <= 0 || strings.TrimSpace(slug) == "" || stars < 0 {
		return ErrGiftNotFound
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err = tx.Exec(`INSERT INTO apifull_gift (from_user, to_user, slug, stars, saved) VALUES ($1,$2,$3,$4,1)`, from, to, slug, stars); err != nil {
		return err
	}
	return tx.Commit()
}

// FindGiftBySlug resolves an issued gift from the durable catalog. It is
// intentionally read-only; callers still need an ownership check before
// mutating inventory.
func FindGiftBySlug(slug string) (Gift, bool, error) {
	if db == nil {
		return Gift{}, false, errors.New("domain PostgreSQL is not open")
	}
	slug = strings.TrimSpace(slug)
	if slug == "" {
		return Gift{}, false, ErrGiftNotFound
	}
	var gift Gift
	var saved int
	err := db.QueryRow(`SELECT id, from_user, to_user, slug, stars, saved
		FROM apifull_gift WHERE slug=$1 ORDER BY id LIMIT 1`, slug).
		Scan(&gift.ID, &gift.From, &gift.To, &gift.Slug, &gift.Stars, &saved)
	if err == sql.ErrNoRows {
		return Gift{}, false, nil
	}
	if err != nil {
		return Gift{}, false, err
	}
	gift.Saved = saved != 0
	return gift, true, nil
}

// TransferGift moves one saved-gift row under a row lock. The ownership check
// and update happen in one transaction so a concurrent transfer cannot spend
// the same inventory twice.
func TransferGift(from, to, giftID int64, slug string) error {
	if db == nil {
		return errors.New("domain PostgreSQL is not open")
	}
	if from <= 0 || to <= 0 || from == to || (giftID <= 0 && strings.TrimSpace(slug) == "") {
		return ErrGiftNotFound
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	query := `SELECT id FROM apifull_gift WHERE id=$1 AND to_user=$2 AND saved=1 FOR UPDATE`
	args := []any{giftID, from}
	if giftID <= 0 {
		query = `SELECT id FROM apifull_gift WHERE slug=$1 AND to_user=$2 AND saved=1 ORDER BY id LIMIT 1 FOR UPDATE`
		args = []any{strings.TrimSpace(slug), from}
	}
	var id int64
	if err = tx.QueryRow(query, args...).Scan(&id); err != nil {
		if err == sql.ErrNoRows {
			return ErrGiftNotFound
		}
		return err
	}
	if _, err = tx.Exec(`UPDATE apifull_gift SET to_user=$1 WHERE id=$2 AND to_user=$3`, to, id, from); err != nil {
		return err
	}
	return tx.Commit()
}

// ConvertGift atomically consumes one saved gift and credits its recorded
// value. The conversion key is tied to the durable gift id, so a retried RPC
// cannot credit the same gift twice.
func ConvertGift(to int64, from *int64, slug string) error {
	if db == nil {
		return errors.New("domain PostgreSQL is not open")
	}
	if to <= 0 || strings.TrimSpace(slug) == "" {
		return ErrGiftNotFound
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	query := `SELECT id, stars, saved FROM apifull_gift WHERE to_user=$1 AND slug=$2 ORDER BY id LIMIT 1 FOR UPDATE`
	args := []any{to, slug}
	if from != nil {
		query = `SELECT id, stars, saved FROM apifull_gift WHERE to_user=$1 AND from_user=$2 AND slug=$3 ORDER BY id LIMIT 1 FOR UPDATE`
		args = []any{to, *from, slug}
	}
	var giftID, stars int64
	var saved int
	if err = tx.QueryRow(query, args...).Scan(&giftID, &stars, &saved); err != nil {
		if err == sql.ErrNoRows {
			return ErrGiftNotFound
		}
		return err
	}
	idem := "gift:convert:" + strconv.FormatInt(giftID, 10)
	if saved == 0 {
		var existing int64
		if err = tx.QueryRow(`SELECT amount FROM apifull_star_tx WHERE user_id=$1 AND idem=$2`, to, idem).Scan(&existing); err == nil {
			if existing != stars {
				return ErrStarsIdempotencyConflict
			}
			return tx.Commit()
		}
		if err != sql.ErrNoRows {
			return err
		}
		return ErrGiftNotFound
	}
	if stars <= 0 {
		return ErrGiftNotConvertible
	}
	if _, err = applyStarsTx(tx, to, stars, idem); err != nil {
		return err
	}
	if _, err = tx.Exec(`UPDATE apifull_gift SET saved=0 WHERE id=$1`, giftID); err != nil {
		return err
	}
	return tx.Commit()
}

// SetGiftSaved changes the saved flag on a gift that already exists for the
// recipient. It deliberately does not create a row: saving a gift is not a
// grant or purchase operation.
func SetGiftSaved(to int64, from *int64, slug string, saved bool) error {
	if db == nil {
		return errors.New("domain PostgreSQL is not open")
	}
	if slug == "" {
		return ErrGiftNotFound
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	query := `SELECT id FROM apifull_gift WHERE to_user=$1 AND slug=$2 ORDER BY id LIMIT 1 FOR UPDATE`
	args := []any{to, slug}
	if from != nil {
		query = `SELECT id FROM apifull_gift WHERE to_user=$1 AND from_user=$2 AND slug=$3 ORDER BY id LIMIT 1 FOR UPDATE`
		args = []any{to, *from, slug}
	}
	var id int64
	if err := tx.QueryRow(query, args...).Scan(&id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrGiftNotFound
		}
		return err
	}
	if _, err = tx.Exec(`UPDATE apifull_gift SET saved=$1 WHERE id=$2 AND to_user=$3`, boolInt(saved), id, to); err != nil {
		return err
	}
	return tx.Commit()
}

// GiftCatalogSlugs returns the gift identifiers that have been issued into
// the local ledger. An empty result is a real empty catalog; callers must not
// synthesize gift records when no authoritative row exists.
func GiftCatalog() ([]Gift, error) {
	if db == nil {
		return nil, errors.New("domain PostgreSQL is not open")
	}
	rows, err := db.Query(`SELECT id, from_user, to_user, slug, stars, saved FROM apifull_gift ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Gift, 0)
	seen := make(map[string]struct{})
	for rows.Next() {
		var gift Gift
		var saved int
		if err = rows.Scan(&gift.ID, &gift.From, &gift.To, &gift.Slug, &gift.Stars, &saved); err != nil {
			return nil, err
		}
		gift.Saved = saved != 0
		if gift.Slug != "" {
			if _, ok := seen[gift.Slug]; !ok {
				seen[gift.Slug] = struct{}{}
				out = append(out, gift)
			}
		}
	}
	return out, rows.Err()
}

func GiftCatalogSlugs() ([]string, error) {
	gifts, err := GiftCatalog()
	if err != nil {
		return nil, err
	}
	slugs := make([]string, 0, len(gifts))
	for _, gift := range gifts {
		slugs = append(slugs, gift.Slug)
	}
	return slugs, nil
}

func ListSavedGifts(to int64) ([]Gift, error) {
	return listGifts(to, true)
}

// ListGifts returns gifts issued to a user. When savedOnly is true, only gifts
// currently visible in the recipient's saved-gift list are returned.
func ListGifts(to int64, savedOnly bool) ([]Gift, error) {
	return listGifts(to, savedOnly)
}

func listGifts(to int64, savedOnly bool) ([]Gift, error) {
	if db == nil {
		return nil, errors.New("domain PostgreSQL is not open")
	}
	if to <= 0 {
		return nil, ErrGiftNotFound
	}
	query := `SELECT id, from_user, to_user, slug, stars, saved FROM apifull_gift WHERE to_user=$1`
	if savedOnly {
		query += ` AND saved=1`
	}
	query += ` ORDER BY id`
	rows, err := db.Query(query, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Gift, 0)
	for rows.Next() {
		var gift Gift
		var saved int
		if err = rows.Scan(&gift.ID, &gift.From, &gift.To, &gift.Slug, &gift.Stars, &saved); err != nil {
			return nil, err
		}
		gift.Saved = saved != 0
		out = append(out, gift)
	}
	return out, rows.Err()
}

func GiftSlugs(to int64) ([]string, error) {
	gifts, err := ListSavedGifts(to)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(gifts))
	for _, gift := range gifts {
		out = append(out, gift.Slug)
	}
	return out, nil
}

func ClaimUsername(name, kind string, owner int64) error {
	if db == nil {
		return errors.New("domain PostgreSQL is not open")
	}
	name = strings.ToLower(name)
	_, err := execPostgresMutation(`INSERT INTO apifull_username (username, owner_user_id, kind) VALUES ($1,$2,$3)
		ON CONFLICT (username) DO UPDATE SET owner_user_id=EXCLUDED.owner_user_id, kind=EXCLUDED.kind`, name, owner, kind)
	return err
}

func LookupUsername(name string) (owner int64, kind string, ok bool, err error) {
	if db == nil {
		return 0, "", false, errors.New("domain PostgreSQL is not open")
	}
	name = strings.ToLower(name)
	err = db.QueryRow(`SELECT owner_user_id, kind FROM apifull_username WHERE username=?`, name).Scan(&owner, &kind)
	if err == sql.ErrNoRows {
		return 0, "", false, nil
	}
	if err != nil {
		return 0, "", false, err
	}
	return owner, kind, true, nil
}

func SaveGroupCall(id, access, creator, channelID int64, title, participants string) error {
	return SaveGroupCallWithMetadata(id, access, creator, channelID, title, participants, nil, false)
}

func SaveGroupCallWithMetadata(id, access, creator, channelID int64, title, participants string, scheduleDate *int32, rtmpStream bool) error {
	return saveGroupCallWithMetadata(id, access, creator, channelID, title, participants, scheduleDate, rtmpStream, false)
}

// SaveGroupCallInvite replaces the active invite for a call. Only the token
// digest is persisted; the exported bearer token is never stored in clear.
func SaveGroupCallInvite(callID, creatorID int64, token string, canSelfUnmute bool) error {
	if db == nil {
		return errors.New("domain PostgreSQL is not open")
	}
	if callID <= 0 || creatorID <= 0 || strings.TrimSpace(token) == "" {
		return errors.New("invalid group call invite")
	}
	digest := sha256.Sum256([]byte(token))
	_, err := execPostgresMutation(`INSERT INTO apifull_group_call_invite
		(call_id, creator_user_id, token_hash, can_self_unmute, revoked_at, created_at)
		VALUES ($1,$2,$3,$4,0,$5)
		ON CONFLICT (call_id) DO UPDATE SET creator_user_id=EXCLUDED.creator_user_id,
		token_hash=EXCLUDED.token_hash, can_self_unmute=EXCLUDED.can_self_unmute,
		revoked_at=0, created_at=EXCLUDED.created_at`,
		callID, creatorID, digest[:], boolInt(canSelfUnmute), time.Now().Unix())
	return err
}

// CheckGroupCallInvite validates a bearer token against the currently active
// invite. It returns the exporter's self-unmute flag only after the digest
// matches and the invite has not been revoked.
func CheckGroupCallInvite(callID int64, token string) (bool, bool, error) {
	if db == nil {
		return false, false, errors.New("domain PostgreSQL is not open")
	}
	if callID <= 0 || strings.TrimSpace(token) == "" {
		return false, false, nil
	}
	digest := sha256.Sum256([]byte(token))
	var stored []byte
	var canSelfUnmute, revoked int
	err := db.QueryRow(`SELECT token_hash, can_self_unmute, revoked_at
		FROM apifull_group_call_invite WHERE call_id=?`, callID).
		Scan(&stored, &canSelfUnmute, &revoked)
	if err == sql.ErrNoRows {
		return false, false, nil
	}
	if err != nil {
		return false, false, err
	}
	if revoked != 0 || !bytes.Equal(stored, digest[:]) {
		return false, false, nil
	}
	return true, canSelfUnmute != 0, nil
}

// RevokeGroupCallInvite invalidates the currently exported bearer token. The
// next export replaces it with a fresh token; callers may safely invoke this
// for a call that has no invite yet.
func RevokeGroupCallInvite(callID, creatorID int64) error {
	if db == nil {
		return errors.New("domain PostgreSQL is not open")
	}
	if callID <= 0 || creatorID <= 0 {
		return errors.New("invalid group call invite")
	}
	_, err := execPostgresMutation(`UPDATE apifull_group_call_invite
		SET revoked_at=$1 WHERE call_id=$2 AND creator_user_id=$3`, time.Now().Unix(), callID, creatorID)
	return err
}

func SaveConferenceCallWithMetadata(id, access, creator, channelID int64, title, participants string, scheduleDate *int32, rtmpStream bool) error {
	return saveGroupCallWithMetadata(id, access, creator, channelID, title, participants, scheduleDate, rtmpStream, true)
}

func SaveConferenceCallControl(callID int64, publicKey, block []byte, params string) error {
	if db == nil {
		return errors.New("domain PostgreSQL is not open")
	}
	if callID <= 0 || len(publicKey) == 0 || len(publicKey) > 256 || len(block) == 0 {
		return errors.New("invalid conference control block")
	}
	_, err := execPostgresMutation(`INSERT INTO apifull_group_call_conference
		(call_id, public_key, block, params, updated_at) VALUES ($1,$2,$3,$4,$5)
		ON CONFLICT (call_id) DO UPDATE SET public_key=EXCLUDED.public_key, block=EXCLUDED.block,
		params=EXCLUDED.params, updated_at=EXCLUDED.updated_at`, callID, publicKey, block, params, time.Now().Unix())
	return err
}

// UpdateConferenceCallBlock replaces the latest conference broadcast block
// after the call has been created. The conference row remains the source of
// truth for the currently advertised chain head.
func UpdateConferenceCallBlock(callID int64, block []byte) error {
	if db == nil {
		return errors.New("domain PostgreSQL is not open")
	}
	if callID <= 0 || len(block) == 0 || len(block) > 1<<20 {
		return errors.New("invalid conference broadcast block")
	}
	result, err := execPostgresMutation(`UPDATE apifull_group_call_conference SET block=$1, updated_at=$2 WHERE call_id=$3`, block, time.Now().Unix(), callID)
	if err != nil {
		return err
	}
	if affected, rowsErr := result.RowsAffected(); rowsErr != nil {
		return rowsErr
	} else if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// LoadConferenceCallBlock returns the current broadcast chain head. A missing
// row is reported separately so callers can return a typed empty update.
func LoadConferenceCallBlock(callID int64) ([]byte, bool, error) {
	if db == nil {
		return nil, false, errors.New("domain PostgreSQL is not open")
	}
	if callID <= 0 {
		return nil, false, errors.New("invalid conference call")
	}
	var block []byte
	err := db.QueryRow(`SELECT block FROM apifull_group_call_conference WHERE call_id=?`, callID).Scan(&block)
	if err == sql.ErrNoRows {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return append([]byte(nil), block...), true, nil
}

func saveGroupCallWithMetadata(id, access, creator, channelID int64, title, participants string, scheduleDate *int32, rtmpStream, conference bool) error {
	if db == nil {
		return errors.New("domain PostgreSQL is not open")
	}
	var schedule any
	if scheduleDate != nil {
		schedule = *scheduleDate
	}
	_, err := execPostgresMutation(`INSERT INTO apifull_group_call
		(id, access_hash, creator_user_id, channel_id, title, rtmp_stream, conference, schedule_date, participants, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		ON CONFLICT (id) DO UPDATE SET access_hash=EXCLUDED.access_hash, creator_user_id=EXCLUDED.creator_user_id,
		channel_id=EXCLUDED.channel_id, title=EXCLUDED.title, rtmp_stream=EXCLUDED.rtmp_stream,
		conference=EXCLUDED.conference,
		schedule_date=EXCLUDED.schedule_date, participants=EXCLUDED.participants`,
		id, access, creator, channelID, title, boolInt(rtmpStream), boolInt(conference), schedule, participants, time.Now().Unix())
	return err
}

type GroupCall struct {
	ID           int64
	AccessHash   int64
	Creator      int64
	ChannelID    int64
	Title        string
	RtmpStream   bool
	Conference   bool
	ScheduleDate *int32
	Participants string
}

func LoadGroupCallRecord(id int64) (GroupCall, bool, error) {
	var call GroupCall
	if db == nil {
		return call, false, errors.New("domain PostgreSQL is not open")
	}
	var rtmpStream, conference int
	var scheduleDate sql.NullInt64
	err := db.QueryRow(`SELECT id, access_hash, creator_user_id, channel_id, title, rtmp_stream, conference, schedule_date, participants
		FROM apifull_group_call WHERE id=?`, id).
		Scan(&call.ID, &call.AccessHash, &call.Creator, &call.ChannelID, &call.Title, &rtmpStream, &conference, &scheduleDate, &call.Participants)
	if err == sql.ErrNoRows {
		return GroupCall{}, false, nil
	}
	if err != nil {
		return GroupCall{}, false, err
	}
	call.RtmpStream = rtmpStream != 0
	call.Conference = conference != 0
	if scheduleDate.Valid {
		value := int32(scheduleDate.Int64)
		call.ScheduleDate = &value
	}
	return call, true, nil
}

func LoadGroupCall(id int64) (participants string, ok bool, err error) {
	call, ok, err := LoadGroupCallRecord(id)
	if err != nil || !ok {
		return "", ok, err
	}
	return call.Participants, true, nil
}

func UpdateGroupCallParticipants(id int64, participants string) (bool, error) {
	if db == nil {
		return false, errors.New("domain PostgreSQL is not open")
	}
	result, err := execPostgresMutation(`UPDATE apifull_group_call SET participants=$1 WHERE id=$2`, participants, id)
	if err != nil {
		return false, err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return changed > 0, nil
}

func UpdateGroupCallTitle(id int64, title string) (bool, error) {
	if db == nil {
		return false, errors.New("domain PostgreSQL is not open")
	}
	result, err := execPostgresMutation(`UPDATE apifull_group_call SET title=$1 WHERE id=$2`, title, id)
	if err != nil {
		return false, err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return changed > 0, nil
}

type GroupCallSettings struct {
	CallID                int64
	JoinMuted             bool
	MessagesEnabled       bool
	SendPaidMessagesStars *int64
	RecordActive          bool
	RecordVideo           bool
	RecordTitle           string
	RecordVideoPortrait   bool
	ScheduledStarted      bool
}

func LoadGroupCallSettings(callID int64) (GroupCallSettings, error) {
	settings := GroupCallSettings{CallID: callID, MessagesEnabled: true}
	if db == nil {
		return settings, errors.New("domain PostgreSQL is not open")
	}
	var joinMuted, messagesEnabled, recordActive, recordVideo, recordVideoPortrait, scheduledStarted int
	var paid sql.NullInt64
	err := db.QueryRow(`SELECT join_muted, messages_enabled, send_paid_messages_stars,
		record_active, record_video, record_title, record_video_portrait, scheduled_started
		FROM apifull_group_call_settings WHERE call_id=?`, callID).
		Scan(&joinMuted, &messagesEnabled, &paid, &recordActive, &recordVideo, &settings.RecordTitle,
			&recordVideoPortrait, &scheduledStarted)
	if err == sql.ErrNoRows {
		return settings, nil
	}
	if err != nil {
		return GroupCallSettings{}, err
	}
	settings.JoinMuted = joinMuted != 0
	settings.MessagesEnabled = messagesEnabled != 0
	settings.RecordActive = recordActive != 0
	settings.RecordVideo = recordVideo != 0
	settings.RecordVideoPortrait = recordVideoPortrait != 0
	settings.ScheduledStarted = scheduledStarted != 0
	if paid.Valid {
		value := paid.Int64
		settings.SendPaidMessagesStars = &value
	}
	return settings, nil
}

func SaveGroupCallSettings(settings GroupCallSettings) error {
	if db == nil {
		return errors.New("domain PostgreSQL is not open")
	}
	if settings.CallID == 0 {
		return errors.New("invalid group call id")
	}
	var paid any
	if settings.SendPaidMessagesStars != nil {
		paid = *settings.SendPaidMessagesStars
	}
	_, err := execPostgresMutation(`INSERT INTO apifull_group_call_settings
		(call_id, join_muted, messages_enabled, send_paid_messages_stars, record_active, record_video,
		record_title, record_video_portrait, scheduled_started, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		ON CONFLICT (call_id) DO UPDATE SET join_muted=EXCLUDED.join_muted, messages_enabled=EXCLUDED.messages_enabled,
		send_paid_messages_stars=EXCLUDED.send_paid_messages_stars, record_active=EXCLUDED.record_active,
		record_video=EXCLUDED.record_video, record_title=EXCLUDED.record_title,
		record_video_portrait=EXCLUDED.record_video_portrait, scheduled_started=EXCLUDED.scheduled_started,
		updated_at=EXCLUDED.updated_at`, settings.CallID, boolInt(settings.JoinMuted), boolInt(settings.MessagesEnabled),
		paid, boolInt(settings.RecordActive), boolInt(settings.RecordVideo), settings.RecordTitle,
		boolInt(settings.RecordVideoPortrait), boolInt(settings.ScheduledStarted), time.Now().Unix())
	return err
}

type GroupCallParticipantState struct {
	CallID             int64
	UserID             int64
	MediaSource        int32
	Muted              bool
	Volume             *int32
	RaiseHand          bool
	VideoStopped       bool
	VideoPaused        bool
	PresentationPaused bool
	PresentationActive bool
	PresentationParams string
	JoinParams         string
}

func LoadGroupCallParticipant(callID, userID int64) (GroupCallParticipantState, bool, error) {
	state := GroupCallParticipantState{CallID: callID, UserID: userID}
	if db == nil {
		return state, false, errors.New("domain PostgreSQL is not open")
	}
	var muted, raiseHand, videoStopped, videoPaused, presentationPaused, presentationActive int
	var volume, mediaSource sql.NullInt64
	err := db.QueryRow(`SELECT media_source, muted, volume, raise_hand, video_stopped, video_paused,
		presentation_paused, presentation_active, presentation_params, join_params
		FROM apifull_group_call_participant WHERE call_id=? AND user_id=?`,
		callID, userID).Scan(&mediaSource, &muted, &volume, &raiseHand, &videoStopped, &videoPaused, &presentationPaused,
		&presentationActive,
		&state.PresentationParams, &state.JoinParams)
	if err == sql.ErrNoRows {
		return state, false, nil
	}
	if err != nil {
		return GroupCallParticipantState{}, false, err
	}
	state.Muted = muted != 0
	state.RaiseHand = raiseHand != 0
	state.VideoStopped = videoStopped != 0
	state.VideoPaused = videoPaused != 0
	state.PresentationPaused = presentationPaused != 0
	state.PresentationActive = presentationActive != 0
	if mediaSource.Valid {
		state.MediaSource = int32(mediaSource.Int64)
	}
	if volume.Valid {
		value := int32(volume.Int64)
		state.Volume = &value
	}
	return state, true, nil
}

func SaveGroupCallParticipant(state GroupCallParticipantState) error {
	if db == nil {
		return errors.New("domain PostgreSQL is not open")
	}
	if state.CallID == 0 || state.UserID == 0 {
		return errors.New("invalid group call participant")
	}
	var volume any
	if state.Volume != nil {
		volume = *state.Volume
	}
	_, err := execPostgresMutation(`INSERT INTO apifull_group_call_participant
		(call_id, user_id, muted, volume, raise_hand, video_stopped, video_paused, presentation_paused,
		presentation_active, presentation_params, join_params, updated_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
		ON CONFLICT (call_id, user_id) DO UPDATE SET muted=EXCLUDED.muted, volume=EXCLUDED.volume, raise_hand=EXCLUDED.raise_hand,
		video_stopped=EXCLUDED.video_stopped, video_paused=EXCLUDED.video_paused,
		presentation_paused=EXCLUDED.presentation_paused, presentation_active=EXCLUDED.presentation_active,
		presentation_params=EXCLUDED.presentation_params,
		join_params=EXCLUDED.join_params, updated_at=EXCLUDED.updated_at`, state.CallID, state.UserID, boolInt(state.Muted), volume,
		boolInt(state.RaiseHand), boolInt(state.VideoStopped), boolInt(state.VideoPaused),
		boolInt(state.PresentationPaused), boolInt(state.PresentationActive), state.PresentationParams, state.JoinParams, time.Now().Unix())
	return err
}

func DeleteGroupCallParticipant(callID, userID int64) error {
	if db == nil {
		return errors.New("domain PostgreSQL is not open")
	}
	_, err := execPostgresMutation(`DELETE FROM apifull_group_call_participant WHERE call_id=$1 AND user_id=$2`, callID, userID)
	return err
}

func JoinGroupCallParticipant(state GroupCallParticipantState) ([]int64, error) {
	if db == nil {
		return nil, errors.New("domain PostgreSQL is not open")
	}
	if state.CallID <= 0 || state.UserID <= 0 || state.MediaSource <= 0 {
		return nil, errors.New("invalid group call participant")
	}

	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var participants string
	if err = tx.QueryRow(`SELECT participants FROM apifull_group_call WHERE id=? FOR UPDATE`, state.CallID).Scan(&participants); err != nil {
		return nil, err
	}
	var ids []int64
	if err = json.Unmarshal([]byte(participants), &ids); err != nil {
		return nil, err
	}
	if ids == nil {
		ids = []int64{}
	}
	found := false
	for _, id := range ids {
		if id == state.UserID {
			found = true
			break
		}
	}
	if !found {
		ids = append(ids, state.UserID)
	}
	encodedParticipants, err := json.Marshal(ids)
	if err != nil {
		return nil, err
	}
	if _, err = tx.Exec(`UPDATE apifull_group_call SET participants=? WHERE id=?`, string(encodedParticipants), state.CallID); err != nil {
		return nil, err
	}

	var volume any
	if state.Volume != nil {
		volume = *state.Volume
	}
	var existing int
	err = tx.QueryRow(`SELECT 1 FROM apifull_group_call_participant
		WHERE call_id=? AND user_id=? FOR UPDATE`, state.CallID, state.UserID).Scan(&existing)
	participantValues := []any{
		state.MediaSource, boolInt(state.Muted), volume, boolInt(state.RaiseHand), boolInt(state.VideoStopped),
		boolInt(state.VideoPaused), boolInt(state.PresentationPaused), boolInt(state.PresentationActive),
		state.PresentationParams, state.JoinParams, time.Now().Unix(),
	}
	if err == sql.ErrNoRows {
		args := append([]any{state.CallID, state.UserID}, participantValues...)
		_, err = tx.Exec(`INSERT INTO apifull_group_call_participant
			(call_id, user_id, media_source, muted, volume, raise_hand, video_stopped, video_paused,
			presentation_paused, presentation_active, presentation_params, join_params, updated_at)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`, args...)
	} else if err == nil {
		args := append(participantValues, state.CallID, state.UserID)
		_, err = tx.Exec(`UPDATE apifull_group_call_participant SET media_source=?, muted=?, volume=?, raise_hand=?,
			video_stopped=?, video_paused=?, presentation_paused=?, presentation_active=?, presentation_params=?,
			join_params=?, updated_at=? WHERE call_id=? AND user_id=?`, args...)
	}
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return ids, nil
}

func RemoveGroupCallParticipant(callID, userID int64) ([]int64, error) {
	if db == nil {
		return nil, errors.New("domain PostgreSQL is not open")
	}
	if callID <= 0 || userID <= 0 {
		return nil, errors.New("invalid group call participant")
	}

	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var participants string
	if err = tx.QueryRow(`SELECT participants FROM apifull_group_call WHERE id=? FOR UPDATE`, callID).Scan(&participants); err != nil {
		return nil, err
	}
	var ids []int64
	if err = json.Unmarshal([]byte(participants), &ids); err != nil {
		return nil, err
	}
	next := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id != userID {
			next = append(next, id)
		}
	}
	encodedParticipants, err := json.Marshal(next)
	if err != nil {
		return nil, err
	}
	if _, err = tx.Exec(`UPDATE apifull_group_call SET participants=? WHERE id=?`, string(encodedParticipants), callID); err != nil {
		return nil, err
	}
	if _, err = tx.Exec(`DELETE FROM apifull_group_call_participant WHERE call_id=? AND user_id=?`, callID, userID); err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return next, nil
}

func GroupCallHasMediaParticipants(callID int64) (bool, error) {
	if db == nil {
		return false, errors.New("domain PostgreSQL is not open")
	}
	if callID <= 0 {
		return false, errors.New("invalid group call id")
	}
	var found bool
	err := db.QueryRow(`SELECT EXISTS(SELECT 1 FROM apifull_group_call_participant
		WHERE call_id=$1 AND media_source IS NOT NULL AND media_source>0)`, callID).Scan(&found)
	return found, err
}

func SaveGroupCallSubscription(callID, userID int64, subscribed bool) error {
	if db == nil {
		return errors.New("domain PostgreSQL is not open")
	}
	_, err := execPostgresMutation(`INSERT INTO apifull_group_call_subscription (call_id, user_id, subscribed, updated_at)
		VALUES ($1,$2,$3,$4) ON CONFLICT (call_id, user_id) DO UPDATE SET subscribed=EXCLUDED.subscribed, updated_at=EXCLUDED.updated_at`,
		callID, userID, boolInt(subscribed), time.Now().Unix())
	return err
}

func LoadGroupCallSubscription(callID, userID int64) (bool, error) {
	if db == nil {
		return false, errors.New("domain PostgreSQL is not open")
	}
	var subscribed int
	err := db.QueryRow(`SELECT subscribed FROM apifull_group_call_subscription WHERE call_id=? AND user_id=?`, callID, userID).Scan(&subscribed)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return subscribed != 0, nil
}

func SaveGroupCallSendAs(callID, userID int64, sendAs string) error {
	if db == nil {
		return errors.New("domain PostgreSQL is not open")
	}
	_, err := execPostgresMutation(`INSERT INTO apifull_group_call_send_as (call_id, user_id, send_as, updated_at)
		VALUES ($1,$2,$3,$4) ON CONFLICT (call_id, user_id) DO UPDATE SET send_as=EXCLUDED.send_as, updated_at=EXCLUDED.updated_at`,
		callID, userID, sendAs, time.Now().Unix())
	return err
}

func LoadGroupCallSendAs(callID, userID int64) (string, bool, error) {
	if db == nil {
		return "", false, errors.New("domain PostgreSQL is not open")
	}
	var sendAs string
	err := db.QueryRow(`SELECT send_as FROM apifull_group_call_send_as WHERE call_id=? AND user_id=?`, callID, userID).Scan(&sendAs)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return sendAs, true, nil
}

type GroupCallMessage struct {
	ID        int64
	CallID    int64
	SenderID  int64
	RandomID  int64
	Message   string
	SendAs    string
	PaidStars *int64
	Date      int64
	Deleted   bool
}

func scanGroupCallMessage(row *sql.Row) (GroupCallMessage, error) {
	var message GroupCallMessage
	var paid sql.NullInt64
	var deleted int
	err := row.Scan(&message.ID, &message.CallID, &message.SenderID, &message.RandomID, &message.Message,
		&message.SendAs, &paid, &message.Date, &deleted)
	if err != nil {
		return GroupCallMessage{}, err
	}
	if paid.Valid {
		value := paid.Int64
		message.PaidStars = &value
	}
	message.Deleted = deleted != 0
	return message, nil
}

func CreateGroupCallMessage(callID, senderID, randomID int64, message, sendAs string, paidStars *int64) (GroupCallMessage, bool, error) {
	if db == nil {
		return GroupCallMessage{}, false, errors.New("domain PostgreSQL is not open")
	}
	if callID == 0 || senderID == 0 || randomID == 0 {
		return GroupCallMessage{}, false, errors.New("invalid group call message")
	}
	var paid any
	if paidStars != nil {
		paid = *paidStars
	}
	now := time.Now().Unix()
	var id int64
	err := db.QueryRow(`INSERT INTO apifull_group_call_message
		(call_id, sender_user_id, random_id, message, send_as, paid_stars, date)
		VALUES (?,?,?,?,?,?,?) RETURNING id`, callID, senderID, randomID, message, sendAs, paid, now).Scan(&id)
	if err == nil {
		return GroupCallMessage{ID: id, CallID: callID, SenderID: senderID, RandomID: randomID,
			Message: message, SendAs: sendAs, PaidStars: paidStars, Date: now}, true, nil
	}
	if !isDuplicateKey(err) {
		return GroupCallMessage{}, false, err
	}
	return LoadGroupCallMessage(callID, randomID)
}

func LoadGroupCallMessage(callID, randomID int64) (GroupCallMessage, bool, error) {
	if db == nil {
		return GroupCallMessage{}, false, errors.New("domain PostgreSQL is not open")
	}
	message, err := scanGroupCallMessage(db.QueryRow(`SELECT id, call_id, sender_user_id, random_id, message,
		send_as, paid_stars, date, deleted FROM apifull_group_call_message WHERE call_id=? AND random_id=?`, callID, randomID))
	if err == sql.ErrNoRows {
		return GroupCallMessage{}, false, nil
	}
	if err != nil {
		return GroupCallMessage{}, false, err
	}
	return message, true, nil
}

func LoadGroupCallMessageByID(callID, messageID int64) (GroupCallMessage, bool, error) {
	if db == nil {
		return GroupCallMessage{}, false, errors.New("domain PostgreSQL is not open")
	}
	message, err := scanGroupCallMessage(db.QueryRow(`SELECT id, call_id, sender_user_id, random_id, message,
		send_as, paid_stars, date, deleted FROM apifull_group_call_message WHERE call_id=? AND id=?`, callID, messageID))
	if err == sql.ErrNoRows {
		return GroupCallMessage{}, false, nil
	}
	if err != nil {
		return GroupCallMessage{}, false, err
	}
	return message, true, nil
}

func DeleteGroupCallMessages(callID int64, ids []int32) ([]int32, error) {
	if db == nil {
		return nil, errors.New("domain PostgreSQL is not open")
	}
	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	deleted := make([]int32, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		result, err := tx.Exec(`UPDATE apifull_group_call_message SET deleted=1 WHERE call_id=$1 AND id=$2 AND deleted=0`, callID, id)
		if err != nil {
			return nil, err
		}
		changed, err := result.RowsAffected()
		if err != nil {
			return nil, err
		}
		if changed > 0 {
			deleted = append(deleted, id)
		}
	}
	return deleted, tx.Commit()
}

func DeleteGroupCallParticipantMessages(callID, senderID int64) ([]int32, error) {
	if db == nil {
		return nil, errors.New("domain PostgreSQL is not open")
	}
	rows, err := db.Query(`SELECT id FROM apifull_group_call_message WHERE call_id=? AND sender_user_id=? AND deleted=0`, callID, senderID)
	if err != nil {
		return nil, err
	}
	ids := make([]int32, 0)
	for rows.Next() {
		var id int32
		if err = rows.Scan(&id); err != nil {
			_ = rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	if err = rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err = rows.Close(); err != nil {
		return nil, err
	}
	return DeleteGroupCallMessages(callID, ids)
}

func DeleteGroupCall(id int64) (bool, error) {
	if db == nil {
		return false, errors.New("domain PostgreSQL is not open")
	}
	tx, err := db.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	result, err := tx.Exec(`DELETE FROM apifull_group_call WHERE id=?`, id)
	if err != nil {
		return false, err
	}
	deleted, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	if deleted > 0 {
		for _, table := range []string{
			"apifull_group_call_settings",
			"apifull_group_call_invite",
			"apifull_group_call_conference",
			"apifull_group_call_participant",
			"apifull_group_call_subscription",
			"apifull_group_call_send_as",
			"apifull_group_call_message",
		} {
			if _, err = tx.Exec("DELETE FROM "+table+" WHERE call_id=?", id); err != nil {
				return false, err
			}
		}
	}
	if err = tx.Commit(); err != nil {
		return false, err
	}
	return deleted > 0, nil
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
