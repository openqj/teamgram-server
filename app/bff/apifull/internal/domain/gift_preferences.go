package domain

import (
	"database/sql"
	"errors"
	"strconv"
	"strings"
	"time"
)

var ErrGiftPreferenceInvalid = errors.New("invalid gift preference")

// SetGiftNotificationSetting atomically upserts one owner-scoped peer
// preference. The peer identity is already normalized by the TL boundary.
func SetGiftNotificationSetting(owner int64, peerType int32, peerID int64, enabled bool) error {
	if db == nil {
		return errors.New("domain PostgreSQL is not open")
	}
	if owner <= 0 || peerID <= 0 || (peerType != 1 && peerType != 2 && peerType != 3 && peerType != 4) {
		return ErrGiftPreferenceInvalid
	}
	_, err := execPostgresMutation(`INSERT INTO apifull_gift_notification_setting
		(owner_user_id, peer_type, peer_id, enabled, updated_at)
		VALUES ($1,$2,$3,$4,$5)
		ON CONFLICT (owner_user_id, peer_type, peer_id) DO UPDATE
		SET enabled=EXCLUDED.enabled, updated_at=EXCLUDED.updated_at`, owner, peerType, peerID, enabled, time.Now().Unix())
	return err
}

// SetGiftPinnedOrder replaces the owner's complete pinned list in one
// transaction. Every reference is resolved and locked before existing pins
// are removed, so a bad reference cannot partially clear the list.
func SetGiftPinnedOrder(owner int64, refs []GiftRef) error {
	if db == nil {
		return errors.New("domain PostgreSQL is not open")
	}
	if owner <= 0 {
		return ErrGiftPreferenceInvalid
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	// Serialize complete-list replacements for one owner. Without an owner
	// lock, concurrent requests could both clear the old list and then race on
	// the unique (owner, position) constraint while inserting new positions.
	if _, err = tx.Exec(`SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, "apifull_gift_pin:"+strconv.FormatInt(owner, 10)); err != nil {
		return err
	}
	ids := make([]int64, 0, len(refs))
	seen := make(map[int64]struct{}, len(refs))
	for _, ref := range refs {
		if ref.ID <= 0 && strings.TrimSpace(ref.Slug) == "" {
			return ErrGiftPreferenceInvalid
		}
		var id int64
		if ref.ID > 0 {
			err = tx.QueryRow(`SELECT id FROM apifull_gift WHERE id=$1 AND to_user=$2 AND saved=1 FOR UPDATE`, ref.ID, owner).Scan(&id)
		} else {
			err = tx.QueryRow(`SELECT id FROM apifull_gift WHERE slug=$1 AND to_user=$2 AND saved=1 ORDER BY id LIMIT 1 FOR UPDATE`, strings.TrimSpace(ref.Slug), owner).Scan(&id)
		}
		if errors.Is(err, sql.ErrNoRows) {
			return ErrGiftPreferenceInvalid
		}
		if err != nil {
			return err
		}
		if _, duplicate := seen[id]; duplicate {
			return ErrGiftPreferenceInvalid
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	if _, err = tx.Exec(`DELETE FROM apifull_gift_pin WHERE owner_user_id=$1`, owner); err != nil {
		return err
	}
	for position, id := range ids {
		if _, err = tx.Exec(`INSERT INTO apifull_gift_pin (owner_user_id, gift_id, position) VALUES ($1,$2,$3)`, owner, id, position); err != nil {
			return err
		}
	}
	return tx.Commit()
}
