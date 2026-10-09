package domain

import (
	"database/sql"
	"encoding/json"
	"errors"
)

// ConnectedBotRecord is the durable owner-scoped configuration for one
// connected bot. Payloads contain the canonical Layer 229 constructors.
type ConnectedBotRecord struct {
	BotID         int64
	CanReply      bool
	Rights        []byte
	RecipientsBot []byte
	Recipients    []byte
}

func SetConnectedBot(ownerID int64, record ConnectedBotRecord) error {
	if db == nil {
		return errors.New("domain PostgreSQL is not open")
	}
	if ownerID <= 0 || record.BotID <= 0 || !json.Valid(record.Rights) || !json.Valid(record.RecipientsBot) {
		return errors.New("invalid connected bot state")
	}
	_, err := execPostgresMutation(`INSERT INTO apifull_connected_bot
		(owner_user_id, bot_user_id, can_reply, rights, recipients_bot, recipients, updated_at)
		VALUES ($1,$2,$3,$4::jsonb,$5::jsonb,$6::jsonb,CURRENT_TIMESTAMP)
		ON CONFLICT (owner_user_id, bot_user_id) DO UPDATE SET
		can_reply=EXCLUDED.can_reply, rights=EXCLUDED.rights,
		recipients_bot=EXCLUDED.recipients_bot, recipients=EXCLUDED.recipients,
		updated_at=CURRENT_TIMESTAMP`, ownerID, record.BotID, record.CanReply,
		string(record.Rights), string(record.RecipientsBot), nullableJSON(record.Recipients))
	return err
}

func DeleteConnectedBot(ownerID, botID int64) error {
	if db == nil {
		return errors.New("domain PostgreSQL is not open")
	}
	if ownerID <= 0 {
		return errors.New("invalid connected bot owner")
	}
	if botID == 0 {
		_, err := execPostgresMutation(`DELETE FROM apifull_connected_bot WHERE owner_user_id=$1`, ownerID)
		return err
	}
	_, err := execPostgresMutation(`DELETE FROM apifull_connected_bot WHERE owner_user_id=$1 AND bot_user_id=$2`, ownerID, botID)
	return err
}

func ListConnectedBots(ownerID int64) ([]ConnectedBotRecord, error) {
	if db == nil {
		return nil, errors.New("domain PostgreSQL is not open")
	}
	if ownerID <= 0 {
		return nil, errors.New("invalid connected bot owner")
	}
	rows, err := db.Query(`SELECT bot_user_id, can_reply, rights, recipients_bot, recipients
		FROM apifull_connected_bot WHERE owner_user_id=$1 ORDER BY bot_user_id`, ownerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ConnectedBotRecord
	for rows.Next() {
		var row ConnectedBotRecord
		if err = rows.Scan(&row.BotID, &row.CanReply, &row.Rights, &row.RecipientsBot, &row.Recipients); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func SetConnectedBotPeerState(ownerID, peerType, peerID int64, paused, disabled *bool) error {
	if db == nil {
		return errors.New("domain PostgreSQL is not open")
	}
	if ownerID <= 0 || peerType < 0 || peerID <= 0 || (paused == nil && disabled == nil) {
		return errors.New("invalid connected bot peer state")
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var currentPaused, currentDisabled bool
	err = tx.QueryRow(`SELECT paused, disabled FROM apifull_connected_bot_peer
		WHERE owner_user_id=$1 AND peer_type=$2 AND peer_id=$3 FOR UPDATE`, ownerID, peerType, peerID).
		Scan(&currentPaused, &currentDisabled)
	if err == sql.ErrNoRows {
		err = nil
	}
	if err != nil {
		return err
	}
	if paused != nil {
		currentPaused = *paused
	}
	if disabled != nil {
		currentDisabled = *disabled
	}
	_, err = tx.Exec(`INSERT INTO apifull_connected_bot_peer
		(owner_user_id, peer_type, peer_id, paused, disabled, updated_at)
		VALUES ($1,$2,$3,$4,$5,CURRENT_TIMESTAMP)
		ON CONFLICT (owner_user_id, peer_type, peer_id) DO UPDATE SET
		paused=EXCLUDED.paused, disabled=EXCLUDED.disabled, updated_at=CURRENT_TIMESTAMP`,
		ownerID, peerType, peerID, currentPaused, currentDisabled)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func nullableJSON(value []byte) any {
	if len(value) == 0 {
		return nil
	}
	return string(value)
}
