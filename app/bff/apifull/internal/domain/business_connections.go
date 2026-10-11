package domain

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// BusinessConnectionRecord is the canonical BotBusinessConnection state
// addressed by the bot-visible connection ID.
type BusinessConnectionRecord struct {
	ConnectionID string
	OwnerUserID  int64
	BotUserID    int64
	DCID         int32
	Date         int32
	QTS          int32
	Disabled     bool
	CanReply     bool
	Rights       []byte
}

// BusinessConnectionDelivery is one durable updateBotBusinessConnect
// delivery intent claimed by the APIFull worker.
type BusinessConnectionDelivery struct {
	ID         int64
	BotUserID  int64
	Connection BusinessConnectionRecord
}

type businessConnectionPayload struct {
	ConnectionID string          `json:"connection_id"`
	OwnerUserID  int64           `json:"owner_user_id"`
	BotUserID    int64           `json:"bot_user_id"`
	DCID         int32           `json:"dc_id"`
	Date         int32           `json:"date"`
	QTS          int32           `json:"qts"`
	Disabled     bool            `json:"disabled"`
	CanReply     bool            `json:"can_reply"`
	Rights       json.RawMessage `json:"rights"`
}

func validBusinessConnectionRights(rights []byte) bool {
	if len(rights) == 0 || !json.Valid(rights) || string(rights) == "null" {
		return false
	}
	var object map[string]any
	return json.Unmarshal(rights, &object) == nil
}

func newBusinessConnectionID() (string, error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw[:]), nil
}

func lockBusinessConnectionQTS(tx *sql.Tx, botUserID int64) error {
	if _, err := tx.Exec(`SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`,
		fmt.Sprintf("apifull-business-connection-qts:%d", botUserID)); err != nil {
		return err
	}
	return nil
}

func nextBusinessConnectionQTS(tx *sql.Tx, botUserID int64) (int32, error) {
	var next int64
	if err := tx.QueryRow(`SELECT COALESCE(MAX(qts), 0) + 1
		FROM apifull_business_connection WHERE bot_user_id=$1`, botUserID).Scan(&next); err != nil {
		return 0, err
	}
	if next <= 0 || next > int64(^uint32(0)>>1) {
		return 0, errors.New("business connection qts exhausted")
	}
	return int32(next), nil
}

// UpsertConnectedBotAndConnection atomically persists account.updateConnectedBot
// state, creates or updates the bot-visible connection registry, and records
// the corresponding update delivery intent.
func UpsertConnectedBotAndConnection(ownerUserID, botUserID int64, canReply bool, rights, recipientsBot, recipients []byte, dcID int32, deleted bool) (BusinessConnectionRecord, error) {
	if db == nil {
		return BusinessConnectionRecord{}, errors.New("domain PostgreSQL is not open")
	}
	if ownerUserID <= 0 || botUserID <= 0 || dcID <= 0 {
		return BusinessConnectionRecord{}, errors.New("invalid business connection identity")
	}
	if !validBusinessConnectionRights(rights) || !validBusinessConnectionRights(recipientsBot) || (len(recipients) > 0 && (!json.Valid(recipients) || string(recipients) == "null")) {
		return BusinessConnectionRecord{}, errors.New("invalid connected bot JSON")
	}
	tx, err := db.Begin()
	if err != nil {
		return BusinessConnectionRecord{}, err
	}
	defer func() { _ = tx.Rollback() }()
	if err = lockBusinessConnectionQTS(tx, botUserID); err != nil {
		return BusinessConnectionRecord{}, err
	}

	var record BusinessConnectionRecord
	var storedRights []byte
	err = tx.QueryRow(`SELECT connection_id, owner_user_id, bot_user_id, dc_id,
		connection_date, qts, disabled, can_reply, rights
		FROM apifull_business_connection
		WHERE owner_user_id=$1 AND bot_user_id=$2 FOR UPDATE`, ownerUserID, botUserID).
		Scan(&record.ConnectionID, &record.OwnerUserID, &record.BotUserID, &record.DCID,
			&record.Date, &record.QTS, &record.Disabled, &record.CanReply, &storedRights)
	existing := err == nil
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return BusinessConnectionRecord{}, err
	}
	if !existing {
		record.ConnectionID, err = newBusinessConnectionID()
		if err != nil {
			return BusinessConnectionRecord{}, err
		}
		record.OwnerUserID, record.BotUserID = ownerUserID, botUserID
		record.DCID = dcID
		record.QTS, err = nextBusinessConnectionQTS(tx, botUserID)
		if err != nil {
			return BusinessConnectionRecord{}, err
		}
	} else {
		record.QTS++
		if record.QTS <= 0 {
			return BusinessConnectionRecord{}, errors.New("business connection qts exhausted")
		}
		record.DCID = dcID
	}
	record.Date = int32(time.Now().Unix())
	record.Disabled = deleted
	record.CanReply = canReply
	if deleted && len(storedRights) > 0 {
		record.Rights = append([]byte(nil), storedRights...)
	} else {
		record.Rights = append([]byte(nil), rights...)
	}

	if existing {
		_, err = tx.Exec(`UPDATE apifull_business_connection SET owner_user_id=$1,
			bot_user_id=$2, dc_id=$3, connection_date=$4, qts=$5, disabled=$6,
			can_reply=$7, rights=$8::jsonb, updated_at=CURRENT_TIMESTAMP
			WHERE connection_id=$9`, record.OwnerUserID, record.BotUserID, record.DCID,
			record.Date, record.QTS, record.Disabled, record.CanReply,
			string(record.Rights), record.ConnectionID)
	} else {
		_, err = tx.Exec(`INSERT INTO apifull_business_connection
			(connection_id, owner_user_id, bot_user_id, dc_id, connection_date,
			 qts, disabled, can_reply, rights, updated_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9::jsonb,CURRENT_TIMESTAMP)`,
			record.ConnectionID, record.OwnerUserID, record.BotUserID, record.DCID,
			record.Date, record.QTS, record.Disabled, record.CanReply,
			string(record.Rights))
	}
	if err != nil {
		return BusinessConnectionRecord{}, err
	}
	if deleted {
		_, err = tx.Exec(`DELETE FROM apifull_connected_bot
			WHERE owner_user_id=$1 AND bot_user_id=$2`, ownerUserID, botUserID)
	} else {
		_, err = tx.Exec(`INSERT INTO apifull_connected_bot
			(owner_user_id, bot_user_id, can_reply, rights, recipients_bot,
			recipients, updated_at)
			VALUES ($1,$2,$3,$4::jsonb,$5::jsonb,$6::jsonb,CURRENT_TIMESTAMP)
			ON CONFLICT (owner_user_id, bot_user_id) DO UPDATE SET
			can_reply=EXCLUDED.can_reply, rights=EXCLUDED.rights,
			recipients_bot=EXCLUDED.recipients_bot, recipients=EXCLUDED.recipients,
			updated_at=CURRENT_TIMESTAMP`, ownerUserID, botUserID, canReply,
			string(rights), string(recipientsBot), nullableJSON(recipients))
	}
	if err != nil {
		return BusinessConnectionRecord{}, err
	}
	payload, err := json.Marshal(businessConnectionPayload{
		ConnectionID: record.ConnectionID, OwnerUserID: record.OwnerUserID,
		BotUserID: record.BotUserID, DCID: record.DCID, Date: record.Date,
		QTS: record.QTS, Disabled: record.Disabled, CanReply: record.CanReply,
		Rights: json.RawMessage(record.Rights),
	})
	if err != nil {
		return BusinessConnectionRecord{}, err
	}
	_, err = tx.Exec(`INSERT INTO apifull_business_connection_outbox
		(bot_user_id, connection_id, qts, payload)
		VALUES ($1,$2,$3,$4::jsonb)
		ON CONFLICT (bot_user_id, connection_id, qts) DO NOTHING`,
		record.BotUserID, record.ConnectionID, record.QTS, string(payload))
	if err != nil {
		return BusinessConnectionRecord{}, err
	}
	if err = tx.Commit(); err != nil {
		return BusinessConnectionRecord{}, err
	}
	return record, nil
}

// LoadBusinessConnection returns the latest authoritative connection state
// for a bot and connection ID. A missing row is not a successful empty result.
func LoadBusinessConnection(botUserID int64, connectionID string) (BusinessConnectionRecord, bool, error) {
	if db == nil {
		return BusinessConnectionRecord{}, false, errors.New("domain PostgreSQL is not open")
	}
	if botUserID <= 0 || connectionID == "" {
		return BusinessConnectionRecord{}, false, errors.New("invalid business connection lookup")
	}
	var record BusinessConnectionRecord
	if err := db.QueryRow(`SELECT connection_id, owner_user_id, bot_user_id, dc_id,
		connection_date, qts, disabled, can_reply, rights
		FROM apifull_business_connection WHERE bot_user_id=$1 AND connection_id=$2`,
		botUserID, connectionID).Scan(&record.ConnectionID, &record.OwnerUserID,
		&record.BotUserID, &record.DCID, &record.Date, &record.QTS,
		&record.Disabled, &record.CanReply, &record.Rights); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return BusinessConnectionRecord{}, false, nil
		}
		return BusinessConnectionRecord{}, false, err
	}
	return record, true, nil
}

// ListDueBusinessConnectionDeliveries claims a bounded batch for one worker.
// Inflight rows are leased so a process restart can retry them.
func ListDueBusinessConnectionDeliveries(ctx context.Context, limit int) ([]BusinessConnectionDelivery, error) {
	if db == nil {
		return nil, errors.New("domain PostgreSQL is not open")
	}
	if limit <= 0 || limit > 1000 {
		return nil, errors.New("invalid business connection delivery limit")
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	rows, err := tx.QueryContext(ctx, `SELECT id, bot_user_id, payload
		FROM apifull_business_connection_outbox
		WHERE (state='pending' OR state='inflight') AND next_attempt_at <= CURRENT_TIMESTAMP
		ORDER BY id FOR UPDATE SKIP LOCKED LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []BusinessConnectionDelivery
	for rows.Next() {
		var id, botUserID int64
		var raw []byte
		if err = rows.Scan(&id, &botUserID, &raw); err != nil {
			return nil, err
		}
		var payload businessConnectionPayload
		if err = json.Unmarshal(raw, &payload); err != nil || payload.ConnectionID == "" || payload.OwnerUserID <= 0 || payload.BotUserID != botUserID || payload.DCID <= 0 || payload.Date <= 0 || payload.QTS <= 0 || !validBusinessConnectionRights(payload.Rights) {
			return nil, errors.New("invalid business connection delivery payload")
		}
		if _, err = tx.ExecContext(ctx, `UPDATE apifull_business_connection_outbox
			SET state='inflight', attempts=attempts+1,
				next_attempt_at=CURRENT_TIMESTAMP + INTERVAL '30 seconds',
				updated_at=CURRENT_TIMESTAMP WHERE id=$1`, id); err != nil {
			return nil, err
		}
		out = append(out, BusinessConnectionDelivery{ID: id, BotUserID: botUserID, Connection: BusinessConnectionRecord{
			ConnectionID: payload.ConnectionID, OwnerUserID: payload.OwnerUserID,
			BotUserID: payload.BotUserID, DCID: payload.DCID, Date: payload.Date,
			QTS: payload.QTS, Disabled: payload.Disabled, CanReply: payload.CanReply,
			Rights: append([]byte(nil), payload.Rights...),
		}})
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return out, nil
}

func CompleteBusinessConnectionDelivery(ctx context.Context, id int64) error {
	if db == nil || id <= 0 {
		return errors.New("invalid business connection delivery")
	}
	_, err := db.ExecContext(ctx, `UPDATE apifull_business_connection_outbox
		SET state='complete', completed_at=CURRENT_TIMESTAMP,
			updated_at=CURRENT_TIMESTAMP, last_error=''
		WHERE id=$1 AND state='inflight'`, id)
	return err
}

func RetryBusinessConnectionDelivery(ctx context.Context, id int64, deliveryErr error) error {
	if db == nil || id <= 0 {
		return errors.New("invalid business connection delivery")
	}
	detail := "business connection delivery failed"
	if deliveryErr != nil {
		detail = deliveryErr.Error()
	}
	_, err := db.ExecContext(ctx, `UPDATE apifull_business_connection_outbox
		SET state='pending', next_attempt_at=$2, last_error=$3,
			updated_at=CURRENT_TIMESTAMP WHERE id=$1 AND state='inflight'`,
		id, time.Now().Add(30*time.Second), detail)
	return err
}
