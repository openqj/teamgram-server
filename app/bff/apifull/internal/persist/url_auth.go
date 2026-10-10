package persist

import (
	"database/sql"
	"fmt"
	"time"
)

// URLAuthRecord is the durable state used by messages.*UrlAuth and the
// account web-authorizations methods. Only accepted rows are exposed by
// ListURLAuth; requested rows remain available for match-code checks.
type URLAuthRecord struct {
	Hash        int64
	URL         string
	Status      string
	MatchCode   string
	BotID       int64
	MsgID       int32
	ButtonID    int32
	InAppOrigin string
	DateCreated int32
	DateActive  int32
}

// URLAuthPostgresEnabled reports whether APIFull's process store is the
// authoritative PostgreSQL implementation.
func URLAuthPostgresEnabled() bool {
	store, ok := Default.(*postgresStore)
	return ok && store != nil && store.db != nil
}

func urlAuthLockName(userID int64) string {
	return fmt.Sprintf("apifull-url-auth:%d", userID)
}

// RequestURLAuth records a pending URL authorization. A repeated request for
// an already accepted user/URL is a no-op, preserving the active web
// authorization while the client asks to open the same button again.
func RequestURLAuth(userID int64, record URLAuthRecord) error {
	store, ok := Default.(*postgresStore)
	if !ok || store == nil || store.db == nil {
		return errURLAuthPostgresUnavailable
	}
	record.Status = "requested"
	if record.DateCreated == 0 {
		record.DateCreated = unixNow()
	}
	if record.DateActive == 0 {
		record.DateActive = record.DateCreated
	}
	return store.mutateURLAuth(userID, func(tx *sql.Tx) error {
		_, err := tx.Exec(`INSERT INTO apifull_url_auth
			(owner_user_id, url_hash, url, status, match_code, bot_id, msg_id,
			 button_id, in_app_origin, date_created, date_active)
			VALUES ($1,$2,$3,'requested',$4,$5,$6,$7,$8,$9,$10)
			ON CONFLICT (owner_user_id, url_hash) DO UPDATE SET
			 url=EXCLUDED.url, status='requested', match_code=EXCLUDED.match_code,
			 bot_id=EXCLUDED.bot_id, msg_id=EXCLUDED.msg_id,
			 button_id=EXCLUDED.button_id, in_app_origin=EXCLUDED.in_app_origin,
			 date_active=EXCLUDED.date_active, updated_at=CURRENT_TIMESTAMP
			WHERE apifull_url_auth.status <> 'accepted'`,
			userID, record.Hash, record.URL, record.MatchCode, record.BotID,
			record.MsgID, record.ButtonID, record.InAppOrigin, record.DateCreated,
			record.DateActive)
		return err
	})
}

// AcceptURLAuth atomically marks an authorization accepted and returns the
// canonical stored row. Retries update the same row instead of creating a
// second web authorization.
func AcceptURLAuth(userID int64, record URLAuthRecord) (URLAuthRecord, error) {
	store, ok := Default.(*postgresStore)
	if !ok || store == nil || store.db == nil {
		return URLAuthRecord{}, errURLAuthPostgresUnavailable
	}
	record.Status = "accepted"
	if record.DateCreated == 0 {
		record.DateCreated = unixNow()
	}
	if record.DateActive == 0 {
		record.DateActive = record.DateCreated
	}
	var out URLAuthRecord
	err := store.mutateURLAuth(userID, func(tx *sql.Tx) error {
		return tx.QueryRow(`INSERT INTO apifull_url_auth
			(owner_user_id, url_hash, url, status, match_code, bot_id, msg_id,
			 button_id, in_app_origin, date_created, date_active)
			VALUES ($1,$2,$3,'accepted',$4,$5,$6,$7,$8,$9,$10)
			ON CONFLICT (owner_user_id, url_hash) DO UPDATE SET
			 url=EXCLUDED.url, status='accepted', match_code=EXCLUDED.match_code,
			 bot_id=EXCLUDED.bot_id, msg_id=EXCLUDED.msg_id,
			 button_id=EXCLUDED.button_id, in_app_origin=EXCLUDED.in_app_origin,
			 date_created=apifull_url_auth.date_created,
			 date_active=EXCLUDED.date_active, updated_at=CURRENT_TIMESTAMP
			RETURNING url_hash, url, status, match_code, bot_id, msg_id,
			 button_id, in_app_origin, date_created, date_active`,
			userID, record.Hash, record.URL, record.MatchCode, record.BotID,
			record.MsgID, record.ButtonID, record.InAppOrigin, record.DateCreated,
			record.DateActive).Scan(&out.Hash, &out.URL, &out.Status, &out.MatchCode,
			&out.BotID, &out.MsgID, &out.ButtonID, &out.InAppOrigin,
			&out.DateCreated, &out.DateActive)
	})
	return out, err
}

// DeclineURLAuth removes the caller-owned authorization for the URL. It is
// idempotent and cannot affect another user's authorization with the same URL.
func DeclineURLAuth(userID, hash int64, rawURL string) error {
	store, ok := Default.(*postgresStore)
	if !ok || store == nil || store.db == nil {
		return errURLAuthPostgresUnavailable
	}
	return store.mutateURLAuth(userID, func(tx *sql.Tx) error {
		if rawURL == "" {
			_, err := tx.Exec(`DELETE FROM apifull_url_auth WHERE owner_user_id=$1 AND url_hash=$2`, userID, hash)
			return err
		}
		_, err := tx.Exec(`DELETE FROM apifull_url_auth
			WHERE owner_user_id=$1 AND (url_hash=$2 OR url=$3)`, userID, hash, rawURL)
		return err
	})
}

// CheckURLAuthMatchCode checks pending and accepted caller-owned records.
func CheckURLAuthMatchCode(userID int64, rawURL, matchCode string) (bool, error) {
	store, ok := Default.(*postgresStore)
	if !ok || store == nil || store.db == nil {
		return false, errURLAuthPostgresUnavailable
	}
	var exists bool
	err := store.db.QueryRow(`SELECT EXISTS (
		SELECT 1 FROM apifull_url_auth
		WHERE owner_user_id=$1 AND match_code=$2
		  AND ($3='' OR url='' OR url=$3)
	)`, userID, matchCode, rawURL).Scan(&exists)
	return exists, err
}

// ListURLAuth returns only accepted records for the caller.
func ListURLAuth(userID int64) ([]URLAuthRecord, error) {
	store, ok := Default.(*postgresStore)
	if !ok || store == nil || store.db == nil {
		return nil, errURLAuthPostgresUnavailable
	}
	rows, err := store.db.Query(`SELECT url_hash, url, status, match_code, bot_id,
		msg_id, button_id, in_app_origin, date_created, date_active
		FROM apifull_url_auth WHERE owner_user_id=$1 AND status='accepted'
		ORDER BY date_active, url_hash`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]URLAuthRecord, 0)
	for rows.Next() {
		var item URLAuthRecord
		if err := rows.Scan(&item.Hash, &item.URL, &item.Status, &item.MatchCode,
			&item.BotID, &item.MsgID, &item.ButtonID, &item.InAppOrigin,
			&item.DateCreated, &item.DateActive); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// ResetURLAuth removes one accepted authorization for the caller.
func ResetURLAuth(userID, hash int64) error {
	store, ok := Default.(*postgresStore)
	if !ok || store == nil || store.db == nil {
		return errURLAuthPostgresUnavailable
	}
	return store.mutateURLAuth(userID, func(tx *sql.Tx) error {
		_, err := tx.Exec(`DELETE FROM apifull_url_auth WHERE owner_user_id=$1 AND url_hash=$2`, userID, hash)
		return err
	})
}

// ResetURLAuths removes all caller-owned URL authorizations.
func ResetURLAuths(userID int64) error {
	store, ok := Default.(*postgresStore)
	if !ok || store == nil || store.db == nil {
		return errURLAuthPostgresUnavailable
	}
	return store.mutateURLAuth(userID, func(tx *sql.Tx) error {
		_, err := tx.Exec(`DELETE FROM apifull_url_auth WHERE owner_user_id=$1`, userID)
		return err
	})
}

var errURLAuthPostgresUnavailable = fmt.Errorf("apifull: URL authorization requires PostgreSQL")

func unixNow() int32 {
	return int32(time.Now().Unix())
}

func (s *postgresStore) mutateURLAuth(userID int64, mutate func(*sql.Tx) error) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err = tx.Exec(`SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, urlAuthLockName(userID)); err != nil {
		return err
	}
	if err = mutate(tx); err != nil {
		return err
	}
	return tx.Commit()
}
