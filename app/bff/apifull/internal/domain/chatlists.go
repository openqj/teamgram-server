package domain

import (
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

// ChatlistInviteRecord is the durable index entry for one chatlist invite.
// PeersJSON contains the serialized InputPeer vector owned by the TL layer.
type ChatlistInviteRecord struct {
	Slug      string
	FilterID  int32
	Title     string
	PeersJSON []byte
}

// LoadChatlistState reads the caller-owned chatlist state. An absent row is an
// empty state; callers decide the TL defaults for the individual fields.
func LoadChatlistState(userID int64) ([]byte, error) {
	if db == nil {
		return nil, errors.New("domain PostgreSQL is not open")
	}
	var raw []byte
	err := db.QueryRow(`SELECT state FROM apifull_chatlist_state WHERE user_id=$1`, userID).Scan(&raw)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return raw, err
}

// SaveChatlistState atomically replaces the state and its slug index. Keeping
// both records in one transaction prevents an invite from becoming visible
// through the public slug after its owner state failed to commit.
func SaveChatlistState(userID int64, state []byte, invites []ChatlistInviteRecord) error {
	if db == nil {
		return errors.New("domain PostgreSQL is not open")
	}
	if len(state) == 0 {
		state = []byte(`{}`)
	}
	if !json.Valid(state) {
		return errors.New("invalid chatlist state JSON")
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err = tx.Exec(`
		INSERT INTO apifull_chatlist_state (user_id, state, updated_at)
		VALUES ($1, $2::jsonb, $3)
		ON CONFLICT (user_id) DO UPDATE SET state=EXCLUDED.state, updated_at=EXCLUDED.updated_at`, userID, string(state), time.Now().UTC()); err != nil {
		return err
	}
	if _, err = tx.Exec(`DELETE FROM apifull_chatlist_invite WHERE owner_user_id=$1`, userID); err != nil {
		return err
	}
	for _, invite := range invites {
		if invite.Slug == "" || !json.Valid(invite.PeersJSON) {
			return errors.New("invalid chatlist invite")
		}
		if _, err = tx.Exec(`
			INSERT INTO apifull_chatlist_invite (slug, owner_user_id, filter_id, title, peers, created_at)
			VALUES ($1, $2, $3, $4, $5::jsonb, $6)
			ON CONFLICT (slug) DO UPDATE SET owner_user_id=EXCLUDED.owner_user_id,
				filter_id=EXCLUDED.filter_id, title=EXCLUDED.title, peers=EXCLUDED.peers`,
			invite.Slug, userID, invite.FilterID, invite.Title, string(invite.PeersJSON), time.Now().UTC()); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// FindChatlistInvite resolves a public slug without loading another user's
// private state first. It is the sole production lookup for invite URLs.
func FindChatlistInvite(slug string) (int64, ChatlistInviteRecord, bool, error) {
	if db == nil {
		return 0, ChatlistInviteRecord{}, false, errors.New("domain PostgreSQL is not open")
	}
	var owner int64
	var invite ChatlistInviteRecord
	var peers []byte
	err := db.QueryRow(`SELECT owner_user_id, slug, filter_id, title, peers
		FROM apifull_chatlist_invite WHERE slug=$1`, slug).Scan(&owner, &invite.Slug, &invite.FilterID, &invite.Title, &peers)
	if err == sql.ErrNoRows {
		return 0, ChatlistInviteRecord{}, false, nil
	}
	if err != nil {
		return 0, ChatlistInviteRecord{}, false, err
	}
	invite.PeersJSON = append([]byte(nil), peers...)
	return owner, invite, true, nil
}
