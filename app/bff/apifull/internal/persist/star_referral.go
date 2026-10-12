// Copyright 2026 Teamgram Authors
//  All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package persist

import (
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// StarRefProgramRecord is the authoritative local affiliate-program state.
// Revenue and participant counters are ledger-owned values and are never
// synthesized by the API layer.
type StarRefProgramRecord struct {
	BotID              int64
	CommissionPermille int32
	DurationMonths     *int32
	EndDate            *int32
	DailyRevenueAmount *int64
	DailyRevenueNanos  *int32
	UpdatedAt          time.Time
}

// StarRefConnectionRecord is one user's connection of a bot's active
// referral program to a peer. The unique key makes retries idempotent.
type StarRefConnectionRecord struct {
	OwnerUserID        int64
	PeerType           int32
	PeerID             int64
	BotID              int64
	Revoked            bool
	URL                string
	Date               int32
	CommissionPermille int32
	DurationMonths     *int32
	Participants       int64
	Revenue            int64
}

type starRefProgramCursor struct {
	Mode      string    `json:"mode"`
	Revenue   int64     `json:"revenue"`
	UpdatedAt time.Time `json:"updated_at"`
	BotID     int64     `json:"bot_id"`
}

var (
	ErrStarRefStateUnavailable  = errors.New("apifull: Star referral state requires PostgreSQL")
	ErrStarRefProgramMissing    = errors.New("apifull: Star referral program is not active")
	ErrStarRefConnectionMissing = errors.New("apifull: Star referral connection not found")
	ErrStarRefLinkInvalid       = errors.New("apifull: invalid Star referral link")
)

func ensureStarReferralSchema(db *sql.DB) error {
	for _, statement := range []string{
		`CREATE TABLE IF NOT EXISTS apifull_star_ref_program (
			bot_user_id BIGINT PRIMARY KEY CHECK (bot_user_id > 0),
			commission_permille INTEGER NOT NULL CHECK (commission_permille BETWEEN 0 AND 1000),
			duration_months INTEGER CHECK (duration_months IS NULL OR duration_months BETWEEN 1 AND 120),
			end_date INTEGER CHECK (end_date IS NULL OR end_date >= 0),
			daily_revenue_amount BIGINT CHECK (daily_revenue_amount IS NULL OR daily_revenue_amount >= 0),
			daily_revenue_nanos INTEGER CHECK (daily_revenue_nanos IS NULL OR daily_revenue_nanos BETWEEN 0 AND 999999999),
			active BOOLEAN NOT NULL DEFAULT FALSE,
			updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_apifull_star_ref_program_active
			ON apifull_star_ref_program (active, updated_at DESC, bot_user_id)`,
		`CREATE TABLE IF NOT EXISTS apifull_star_ref_connection (
			owner_user_id BIGINT NOT NULL CHECK (owner_user_id > 0),
			peer_type INTEGER NOT NULL CHECK (peer_type > 0),
			peer_id BIGINT NOT NULL CHECK (peer_id > 0),
			bot_user_id BIGINT NOT NULL CHECK (bot_user_id > 0),
			revoked BOOLEAN NOT NULL DEFAULT FALSE,
			referral_url TEXT NOT NULL CHECK (char_length(referral_url) BETWEEN 1 AND 2048),
			connected_at INTEGER NOT NULL CHECK (connected_at > 0),
			participants BIGINT NOT NULL DEFAULT 0 CHECK (participants >= 0),
			revenue BIGINT NOT NULL DEFAULT 0 CHECK (revenue >= 0),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (owner_user_id, peer_type, peer_id, bot_user_id),
			UNIQUE (owner_user_id, peer_type, peer_id, referral_url)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_apifull_star_ref_connection_owner
			ON apifull_star_ref_connection (owner_user_id, peer_type, peer_id, revoked, connected_at DESC, referral_url)`,
	} {
		if _, err := db.Exec(statement); err != nil {
			return err
		}
	}
	return nil
}

func starReferralStore() (*postgresStore, error) {
	store, ok := Default.(*postgresStore)
	if !ok || store == nil || store.db == nil {
		return nil, ErrStarRefStateUnavailable
	}
	return store, nil
}

func UpsertStarRefProgram(botID int64, commissionPermille int32, durationMonths *int32, now time.Time) (StarRefProgramRecord, error) {
	store, err := starReferralStore()
	if err != nil {
		return StarRefProgramRecord{}, err
	}
	if botID <= 0 || commissionPermille < 0 || commissionPermille > 1000 || (durationMonths != nil && (*durationMonths <= 0 || *durationMonths > 120)) {
		return StarRefProgramRecord{}, errors.New("apifull: invalid Star referral program")
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	active := commissionPermille > 0
	var record StarRefProgramRecord
	tx, err := store.db.Begin()
	if err != nil {
		return StarRefProgramRecord{}, err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err = tx.Exec(`SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, fmt.Sprintf("apifull-star-ref-program:%d", botID)); err != nil {
		return StarRefProgramRecord{}, err
	}
	_, err = tx.Exec(`INSERT INTO apifull_star_ref_program
		(bot_user_id, commission_permille, duration_months, end_date, active, updated_at)
		VALUES ($1, $2, $3, CASE WHEN $3::INTEGER IS NULL THEN NULL ELSE EXTRACT(EPOCH FROM ($4::TIMESTAMPTZ + make_interval(months => $3::INTEGER)))::INTEGER END, $5::BOOLEAN, $4::TIMESTAMPTZ)
		ON CONFLICT (bot_user_id) DO UPDATE SET commission_permille = EXCLUDED.commission_permille,
			duration_months = EXCLUDED.duration_months, end_date = EXCLUDED.end_date,
			active = EXCLUDED.active, updated_at = EXCLUDED.updated_at`, botID, commissionPermille, durationMonths, now, active)
	if err != nil {
		return StarRefProgramRecord{}, err
	}
	if err = tx.QueryRow(`SELECT bot_user_id, commission_permille, duration_months, end_date,
		daily_revenue_amount, daily_revenue_nanos, updated_at
		FROM apifull_star_ref_program WHERE bot_user_id = $1`, botID).
		Scan(&record.BotID, &record.CommissionPermille, &record.DurationMonths, &record.EndDate,
			&record.DailyRevenueAmount, &record.DailyRevenueNanos, &record.UpdatedAt); err != nil {
		return StarRefProgramRecord{}, err
	}
	if err = tx.Commit(); err != nil {
		return StarRefProgramRecord{}, err
	}
	return record, nil
}

func LoadActiveStarRefProgram(botID int64) (StarRefProgramRecord, bool, error) {
	store, err := starReferralStore()
	if err != nil {
		return StarRefProgramRecord{}, false, err
	}
	var record StarRefProgramRecord
	err = store.db.QueryRow(`SELECT bot_user_id, commission_permille, duration_months, end_date,
		daily_revenue_amount, daily_revenue_nanos, updated_at
		FROM apifull_star_ref_program WHERE bot_user_id = $1 AND active = TRUE
		AND (end_date IS NULL OR end_date > EXTRACT(EPOCH FROM CURRENT_TIMESTAMP)::INTEGER)`, botID).
		Scan(&record.BotID, &record.CommissionPermille, &record.DurationMonths, &record.EndDate,
			&record.DailyRevenueAmount, &record.DailyRevenueNanos, &record.UpdatedAt)
	if err == sql.ErrNoRows {
		return StarRefProgramRecord{}, false, nil
	}
	return record, err == nil, err
}

func ListActiveStarRefPrograms(offset string, limit int, orderByRevenue, orderByDate bool) ([]StarRefProgramRecord, int, string, error) {
	store, err := starReferralStore()
	if err != nil {
		return nil, 0, "", err
	}
	if limit <= 0 || limit > 100 {
		return nil, 0, "", errors.New("apifull: invalid Star referral limit")
	}
	mode := "date"
	order := "updated_at DESC, bot_user_id ASC"
	if orderByRevenue {
		mode = "revenue"
		order = "COALESCE(daily_revenue_amount, 0) DESC, updated_at DESC, bot_user_id ASC"
	} else if orderByDate {
		mode = "date"
		order = "updated_at DESC, bot_user_id ASC"
	}
	query := `SELECT bot_user_id, commission_permille, duration_months, end_date,
		daily_revenue_amount, daily_revenue_nanos, updated_at
		FROM apifull_star_ref_program WHERE active = TRUE
		AND (end_date IS NULL OR end_date > EXTRACT(EPOCH FROM CURRENT_TIMESTAMP)::INTEGER)`
	args := []any{}
	if strings.TrimSpace(offset) != "" {
		decoded, decodeErr := decodeStarRefCursor(offset)
		if decodeErr != nil || decoded.Mode != mode || decoded.BotID <= 0 || decoded.UpdatedAt.IsZero() {
			return nil, 0, "", errors.New("apifull: invalid Star referral offset")
		}
		if mode == "revenue" {
			query += ` AND (COALESCE(daily_revenue_amount, 0) < $1 OR
				(COALESCE(daily_revenue_amount, 0) = $1 AND (updated_at < $2 OR
				(updated_at = $2 AND bot_user_id > $3))))`
			args = append(args, decoded.Revenue, decoded.UpdatedAt, decoded.BotID)
		} else {
			query += ` AND (updated_at < $1 OR (updated_at = $1 AND bot_user_id > $2))`
			args = append(args, decoded.UpdatedAt, decoded.BotID)
		}
	}
	query += ` ORDER BY ` + order + ` LIMIT $` + strconv.Itoa(len(args)+1)
	args = append(args, limit+1)
	rows, err := store.db.Query(query, args...)
	if err != nil {
		return nil, 0, "", err
	}
	defer rows.Close()
	programs := make([]StarRefProgramRecord, 0, limit)
	for rows.Next() {
		var record StarRefProgramRecord
		if err = rows.Scan(&record.BotID, &record.CommissionPermille, &record.DurationMonths, &record.EndDate,
			&record.DailyRevenueAmount, &record.DailyRevenueNanos, &record.UpdatedAt); err != nil {
			return nil, 0, "", err
		}
		programs = append(programs, record)
	}
	if err = rows.Err(); err != nil {
		return nil, 0, "", err
	}
	var count int
	if err = store.db.QueryRow(`SELECT COUNT(*) FROM apifull_star_ref_program
		WHERE active = TRUE AND (end_date IS NULL OR end_date > EXTRACT(EPOCH FROM CURRENT_TIMESTAMP)::INTEGER)`).Scan(&count); err != nil {
		return nil, 0, "", err
	}
	next := ""
	if len(programs) > limit {
		last := programs[limit-1]
		cursor := starRefProgramCursor{Mode: mode, BotID: last.BotID, UpdatedAt: last.UpdatedAt}
		if last.DailyRevenueAmount != nil {
			cursor.Revenue = *last.DailyRevenueAmount
		}
		next = encodeStarRefCursor(cursor)
		programs = programs[:limit]
	}
	return programs, count, next, nil
}

func encodeStarRefCursor(cursor starRefProgramCursor) string {
	raw, _ := json.Marshal(cursor)
	return base64.RawURLEncoding.EncodeToString(raw)
}

func decodeStarRefCursor(value string) (starRefProgramCursor, error) {
	raw, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return starRefProgramCursor{}, err
	}
	var cursor starRefProgramCursor
	if err = json.Unmarshal(raw, &cursor); err != nil {
		return starRefProgramCursor{}, err
	}
	return cursor, nil
}

func ConnectStarRefBot(ownerID, peerType, peerID, botID int64, referralURL string, now time.Time) (StarRefConnectionRecord, error) {
	store, err := starReferralStore()
	if err != nil {
		return StarRefConnectionRecord{}, err
	}
	if ownerID <= 0 || peerType <= 0 || peerID <= 0 || botID <= 0 {
		return StarRefConnectionRecord{}, errors.New("apifull: invalid Star referral connection")
	}
	referralURL = strings.TrimSpace(referralURL)
	if !validStarReferralURL(referralURL) {
		return StarRefConnectionRecord{}, ErrStarRefLinkInvalid
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	var record StarRefConnectionRecord
	tx, err := store.db.Begin()
	if err != nil {
		return StarRefConnectionRecord{}, err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err = tx.Exec(`SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, fmt.Sprintf("apifull-star-ref-connection:%d:%d:%d:%d", ownerID, peerType, peerID, botID)); err != nil {
		return StarRefConnectionRecord{}, err
	}
	var active bool
	if err = tx.QueryRow(`SELECT active FROM apifull_star_ref_program WHERE bot_user_id = $1 FOR SHARE`, botID).Scan(&active); err == sql.ErrNoRows || !active {
		return StarRefConnectionRecord{}, ErrStarRefProgramMissing
	} else if err != nil {
		return StarRefConnectionRecord{}, err
	}
	_, err = tx.Exec(`INSERT INTO apifull_star_ref_connection
		(owner_user_id, peer_type, peer_id, bot_user_id, revoked, referral_url, connected_at, updated_at)
			VALUES ($1,$2,$3,$4,FALSE,$5,EXTRACT(EPOCH FROM $6::TIMESTAMPTZ)::INTEGER,$6::TIMESTAMPTZ)
		ON CONFLICT (owner_user_id, peer_type, peer_id, bot_user_id) DO UPDATE SET revoked = FALSE,
			referral_url = EXCLUDED.referral_url, updated_at = EXCLUDED.updated_at`, ownerID, peerType, peerID, botID, referralURL, now)
	if err != nil {
		return StarRefConnectionRecord{}, err
	}
	if err = tx.QueryRow(`SELECT c.owner_user_id, c.peer_type, c.peer_id, c.bot_user_id, c.revoked,
		c.referral_url, c.connected_at, p.commission_permille, p.duration_months, c.participants, c.revenue
		FROM apifull_star_ref_connection c JOIN apifull_star_ref_program p ON p.bot_user_id = c.bot_user_id
		WHERE c.owner_user_id=$1 AND c.peer_type=$2 AND c.peer_id=$3 AND c.bot_user_id=$4`, ownerID, peerType, peerID, botID).
		Scan(&record.OwnerUserID, &record.PeerType, &record.PeerID, &record.BotID, &record.Revoked, &record.URL,
			&record.Date, &record.CommissionPermille, &record.DurationMonths, &record.Participants, &record.Revenue); err != nil {
		return StarRefConnectionRecord{}, err
	}
	if err = tx.Commit(); err != nil {
		return StarRefConnectionRecord{}, err
	}
	return record, nil
}

func ListStarRefConnections(ownerID int64, peerType int32, peerID int64, offsetDate int32, offsetLink string, limit int) ([]StarRefConnectionRecord, int, error) {
	store, err := starReferralStore()
	if err != nil {
		return nil, 0, err
	}
	if ownerID <= 0 || peerType <= 0 || peerID <= 0 || limit <= 0 || limit > 100 {
		return nil, 0, errors.New("apifull: invalid Star referral connection list")
	}
	query := `SELECT c.owner_user_id, c.peer_type, c.peer_id, c.bot_user_id, c.revoked,
		c.referral_url, c.connected_at, p.commission_permille, p.duration_months, c.participants, c.revenue
		FROM apifull_star_ref_connection c JOIN apifull_star_ref_program p ON p.bot_user_id = c.bot_user_id
		WHERE c.owner_user_id=$1 AND c.peer_type=$2 AND c.peer_id=$3`
	args := []any{ownerID, peerType, peerID}
	if offsetDate > 0 || strings.TrimSpace(offsetLink) != "" {
		query += ` AND (c.connected_at, c.referral_url) < ($4, $5)`
		args = append(args, offsetDate, offsetLink)
	}
	query += ` ORDER BY c.connected_at DESC, c.referral_url DESC LIMIT $` + strconv.Itoa(len(args)+1)
	args = append(args, limit+1)
	rows, err := store.db.Query(query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	result := make([]StarRefConnectionRecord, 0, limit)
	for rows.Next() {
		var record StarRefConnectionRecord
		if err = rows.Scan(&record.OwnerUserID, &record.PeerType, &record.PeerID, &record.BotID, &record.Revoked,
			&record.URL, &record.Date, &record.CommissionPermille, &record.DurationMonths, &record.Participants, &record.Revenue); err != nil {
			return nil, 0, err
		}
		result = append(result, record)
	}
	if err = rows.Err(); err != nil {
		return nil, 0, err
	}
	var count int
	if err = store.db.QueryRow(`SELECT COUNT(*) FROM apifull_star_ref_connection WHERE owner_user_id=$1 AND peer_type=$2 AND peer_id=$3`, ownerID, peerType, peerID).Scan(&count); err != nil {
		return nil, 0, err
	}
	return result, count, nil
}

func LoadStarRefConnection(ownerID int64, peerType, peerID, botID int64) (StarRefConnectionRecord, bool, error) {
	store, err := starReferralStore()
	if err != nil {
		return StarRefConnectionRecord{}, false, err
	}
	var record StarRefConnectionRecord
	err = store.db.QueryRow(`SELECT c.owner_user_id, c.peer_type, c.peer_id, c.bot_user_id, c.revoked,
		c.referral_url, c.connected_at, p.commission_permille, p.duration_months, c.participants, c.revenue
		FROM apifull_star_ref_connection c JOIN apifull_star_ref_program p ON p.bot_user_id = c.bot_user_id
		WHERE c.owner_user_id=$1 AND c.peer_type=$2 AND c.peer_id=$3 AND c.bot_user_id=$4`, ownerID, peerType, peerID, botID).
		Scan(&record.OwnerUserID, &record.PeerType, &record.PeerID, &record.BotID, &record.Revoked, &record.URL,
			&record.Date, &record.CommissionPermille, &record.DurationMonths, &record.Participants, &record.Revenue)
	if err == sql.ErrNoRows {
		return StarRefConnectionRecord{}, false, nil
	}
	return record, err == nil, err
}

func EditStarRefConnection(ownerID, peerType, peerID int64, referralURL string, revoked bool) (StarRefConnectionRecord, error) {
	store, err := starReferralStore()
	if err != nil {
		return StarRefConnectionRecord{}, err
	}
	referralURL = strings.TrimSpace(referralURL)
	if ownerID <= 0 || peerType <= 0 || peerID <= 0 || !validStarReferralURL(referralURL) {
		return StarRefConnectionRecord{}, ErrStarRefLinkInvalid
	}
	var record StarRefConnectionRecord
	tx, err := store.db.Begin()
	if err != nil {
		return StarRefConnectionRecord{}, err
	}
	defer func() { _ = tx.Rollback() }()
	if err = tx.QueryRow(`SELECT c.owner_user_id, c.peer_type, c.peer_id, c.bot_user_id, c.revoked,
		c.referral_url, c.connected_at, p.commission_permille, p.duration_months, c.participants, c.revenue
		FROM apifull_star_ref_connection c JOIN apifull_star_ref_program p ON p.bot_user_id = c.bot_user_id
		WHERE c.owner_user_id=$1 AND c.peer_type=$2 AND c.peer_id=$3 AND c.referral_url=$4 FOR UPDATE`, ownerID, peerType, peerID, referralURL).
		Scan(&record.OwnerUserID, &record.PeerType, &record.PeerID, &record.BotID, &record.Revoked, &record.URL,
			&record.Date, &record.CommissionPermille, &record.DurationMonths, &record.Participants, &record.Revenue); err == sql.ErrNoRows {
		return StarRefConnectionRecord{}, ErrStarRefConnectionMissing
	} else if err != nil {
		return StarRefConnectionRecord{}, err
	}
	if _, err = tx.Exec(`UPDATE apifull_star_ref_connection SET revoked=$1, updated_at=CURRENT_TIMESTAMP
		WHERE owner_user_id=$2 AND peer_type=$3 AND peer_id=$4 AND referral_url=$5`, revoked, ownerID, peerType, peerID, referralURL); err != nil {
		return StarRefConnectionRecord{}, err
	}
	record.Revoked = revoked
	if err = tx.Commit(); err != nil {
		return StarRefConnectionRecord{}, err
	}
	return record, nil
}

func NewStarReferralURL(botUsername string, botID int64) (string, error) {
	botUsername = strings.TrimPrefix(strings.TrimSpace(botUsername), "@")
	if botID <= 0 || botUsername == "" || len(botUsername) > 64 {
		return "", ErrStarRefLinkInvalid
	}
	for _, r := range botUsername {
		if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') && r != '_' {
			return "", ErrStarRefLinkInvalid
		}
	}
	var raw [12]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return "https://t.me/" + botUsername + "?start=" + base64.RawURLEncoding.EncodeToString(raw[:]) + "_" + strconv.FormatInt(botID, 10), nil
}

func validStarReferralURL(raw string) bool {
	parsed, err := url.ParseRequestURI(raw)
	return err == nil && parsed.Scheme == "https" && parsed.Host == "t.me" && parsed.Path != "" && len(raw) <= 2048
}

// ValidStarReferralLinkForAPI lets the API validate cursor links without
// duplicating the URL policy used by the persistence boundary.
func ValidStarReferralLinkForAPI(raw string) bool {
	return validStarReferralURL(strings.TrimSpace(raw))
}
