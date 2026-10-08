package domain

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

type ChannelDeliveryKey struct {
	ChannelID int64
	PTSFrom   int32
	PTSTo     int32
}

type ChannelDelivery struct {
	ID  int64
	Key ChannelDeliveryKey
}

type ChannelDeliveryPayload struct {
	EventType        string           `json:"event_type,omitempty"`
	Rows             []ChannelMessage `json:"rows,omitempty"`
	MessageIDs       []int32          `json:"message_ids,omitempty"`
	Pinned           bool             `json:"pinned,omitempty"`
	ExcludeAuthKeyID int64
	Complete         bool
}

type ChannelDeliveryRecipient struct {
	UserID int64
}

func insertChannelDeliveryTx(tx *sql.Tx, channelID, senderID, excludeAuthKeyID int64, rows []ChannelMessage) error {
	if len(rows) == 0 {
		return ErrInvalidMessageID
	}
	for i, row := range rows {
		if row.ChannelID != channelID || row.Sender != senderID || row.Pts <= 0 || (i > 0 && row.Pts <= rows[i-1].Pts) {
			return ErrInvalidMessageID
		}
	}
	return insertChannelDeliveryEventTx(tx, channelID, senderID, excludeAuthKeyID, channelEventNew, rows, nil, false, rows[0].Pts, rows[len(rows)-1].Pts)
}

// insertChannelDeliveryEventTx stores every channel event that must reach the
// member snapshot. New-message delivery uses rows to hydrate the message;
// edit, delete, and pin events use the event-specific message IDs and flags.
// The recipient snapshot is created in the same transaction as the canonical
// write so a worker restart cannot lose an event between those two steps.
func insertChannelDeliveryEventTx(tx *sql.Tx, channelID, senderID, excludeAuthKeyID int64, eventType string, rows []ChannelMessage, messageIDs []int32, pinned bool, ptsFrom, ptsTo int32) error {
	if eventType == "" || channelID <= 0 || ptsFrom <= 0 || ptsTo < ptsFrom {
		return ErrInvalidMessageID
	}
	if len(rows) == 0 && len(messageIDs) == 0 {
		return ErrInvalidMessageID
	}
	if eventType == channelEventNew {
		if len(rows) == 0 {
			return ErrInvalidMessageID
		}
		for i, row := range rows {
			if row.ChannelID != channelID || row.Sender != senderID || row.Pts <= 0 || (i > 0 && row.Pts <= rows[i-1].Pts) {
				return ErrInvalidMessageID
			}
		}
	}
	payload, err := json.Marshal(ChannelDeliveryPayload{
		EventType:  eventType,
		Rows:       rows,
		MessageIDs: append([]int32(nil), messageIDs...),
		Pinned:     pinned,
	})
	if err != nil {
		return err
	}
	key := ChannelDeliveryKey{ChannelID: channelID, PTSFrom: ptsFrom, PTSTo: ptsTo}
	result, err := tx.Exec(`INSERT IGNORE INTO apifull_channel_delivery_outbox
		(channel_id, pts_from, pts_to, sender_user_id, exclude_auth_key_id, payload, created_at)
		VALUES (?,?,?,?,?,?,?)`, channelID, key.PTSFrom, key.PTSTo, senderID, excludeAuthKeyID, payload, time.Now().Unix())
	if err != nil {
		return err
	}
	inserted, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if inserted == 0 {
		var state string
		if err = tx.QueryRow(`SELECT state FROM apifull_channel_delivery_outbox
			WHERE channel_id=? AND pts_from=? AND pts_to=? FOR UPDATE`, channelID, key.PTSFrom, key.PTSTo).Scan(&state); err != nil {
			return err
		}
		return nil
	}
	deliveryID, err := result.LastInsertId()
	if err != nil {
		return err
	}
	var creatorID int64
	if err = tx.QueryRow(`SELECT creator_user_id FROM apifull_channel WHERE id=?`, channelID).Scan(&creatorID); err != nil {
		return err
	}
	now := time.Now().Unix()
	if _, err = tx.Exec(`INSERT INTO apifull_channel_delivery_recipient (delivery_id, user_id, next_attempt_at)
		VALUES (?,?,?)`, deliveryID, creatorID, now); err != nil {
		return err
	}
	_, err = tx.Exec(`INSERT INTO apifull_channel_delivery_recipient (delivery_id, user_id, next_attempt_at)
		SELECT ?, user_id, ? FROM apifull_channel_member WHERE channel_id=? AND user_id<>?`, deliveryID, now, channelID, creatorID)
	return err
}

func ListDueChannelDeliveries(ctx context.Context, limit int) ([]ChannelDelivery, error) {
	if db == nil {
		return nil, errors.New("domain PostgreSQL is not open")
	}
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := db.QueryContext(ctx, `SELECT DISTINCT o.id, o.channel_id, o.pts_from, o.pts_to
		FROM apifull_channel_delivery_outbox o
		JOIN apifull_channel_delivery_recipient r ON r.delivery_id=o.id
		WHERE o.state='pending' AND r.state='pending' AND r.next_attempt_at<=?
		ORDER BY o.id LIMIT ?`, time.Now().Unix(), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	deliveries := make([]ChannelDelivery, 0, limit)
	for rows.Next() {
		var delivery ChannelDelivery
		if err = rows.Scan(&delivery.ID, &delivery.Key.ChannelID, &delivery.Key.PTSFrom, &delivery.Key.PTSTo); err != nil {
			return nil, err
		}
		deliveries = append(deliveries, delivery)
	}
	return deliveries, rows.Err()
}

func LockChannelDelivery(ctx context.Context, key ChannelDeliveryKey, wait time.Duration) (func(), bool, error) {
	if db == nil {
		return nil, false, errors.New("domain PostgreSQL is not open")
	}
	if key.ChannelID <= 0 || key.PTSFrom <= 0 || key.PTSTo < key.PTSFrom {
		return nil, false, ErrInvalidMessageID
	}
	if wait < 0 {
		wait = 0
	}
	seconds := int(wait / time.Second)
	if wait%time.Second != 0 {
		seconds++
	}
	lockHash := sha256.Sum256([]byte(fmt.Sprintf("%d:%d:%d", key.ChannelID, key.PTSFrom, key.PTSTo)))
	lockName := "apifull:ch:" + hex.EncodeToString(lockHash[:20])
	conn, err := db.Conn(ctx)
	if err != nil {
		return nil, false, err
	}
	var acquired sql.NullInt64
	if err = conn.QueryRowContext(ctx, `SELECT GET_LOCK(?, ?)`, lockName, seconds).Scan(&acquired); err != nil {
		_ = conn.Close()
		return nil, false, err
	}
	if !acquired.Valid || acquired.Int64 != 1 {
		_ = conn.Close()
		return nil, false, nil
	}
	return func() {
		releaseCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		var released sql.NullInt64
		_ = conn.QueryRowContext(releaseCtx, `SELECT RELEASE_LOCK(?)`, lockName).Scan(&released)
		_ = conn.Close()
	}, true, nil
}

func LoadChannelDelivery(ctx context.Context, key ChannelDeliveryKey) (ChannelDeliveryPayload, bool, error) {
	var payload ChannelDeliveryPayload
	if db == nil {
		return payload, false, errors.New("domain PostgreSQL is not open")
	}
	var encoded []byte
	var state string
	err := db.QueryRowContext(ctx, `SELECT payload, exclude_auth_key_id, state FROM apifull_channel_delivery_outbox
		WHERE channel_id=? AND pts_from=? AND pts_to=?`, key.ChannelID, key.PTSFrom, key.PTSTo).
		Scan(&encoded, &payload.ExcludeAuthKeyID, &state)
	if err == sql.ErrNoRows {
		return payload, false, nil
	}
	if err != nil {
		return payload, false, err
	}
	if state == "completed" {
		payload.Complete = true
		return payload, true, nil
	}
	if err = json.Unmarshal(encoded, &payload); err != nil {
		// Older deployments stored new-message payloads as a bare JSON array.
		// Keep those rows deliverable while new writes use the typed envelope.
		if err = json.Unmarshal(encoded, &payload.Rows); err != nil {
			return payload, false, err
		}
	}
	if payload.EventType == "" {
		payload.EventType = channelEventNew
	}
	return payload, true, nil
}

func ListChannelDeliveryRecipients(ctx context.Context, key ChannelDeliveryKey, dueOnly bool) ([]ChannelDeliveryRecipient, error) {
	if db == nil {
		return nil, errors.New("domain PostgreSQL is not open")
	}
	query := `SELECT r.user_id FROM apifull_channel_delivery_recipient r
		JOIN apifull_channel_delivery_outbox o ON o.id=r.delivery_id
		WHERE o.channel_id=? AND o.pts_from=? AND o.pts_to=? AND o.state='pending' AND r.state='pending'`
	args := []interface{}{key.ChannelID, key.PTSFrom, key.PTSTo}
	if dueOnly {
		query += ` AND r.next_attempt_at<=?`
		args = append(args, time.Now().Unix())
	}
	query += ` ORDER BY r.user_id`
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	recipients := make([]ChannelDeliveryRecipient, 0)
	for rows.Next() {
		var recipient ChannelDeliveryRecipient
		if err = rows.Scan(&recipient.UserID); err != nil {
			return nil, err
		}
		recipients = append(recipients, recipient)
	}
	return recipients, rows.Err()
}

func ChannelDeliveryRecipientCanReceive(ctx context.Context, channelID, userID int64, messageIDs []int32) (bool, error) {
	if db == nil {
		return false, errors.New("domain PostgreSQL is not open")
	}
	var creatorID int64
	err := db.QueryRowContext(ctx, `SELECT creator_user_id FROM apifull_channel WHERE id=?`, channelID).Scan(&creatorID)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if creatorID != userID {
		var rawBanned string
		err = db.QueryRowContext(ctx, `SELECT banned_rights FROM apifull_channel_member WHERE channel_id=? AND user_id=?`, channelID, userID).Scan(&rawBanned)
		if err == sql.ErrNoRows {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		if rawBanned != "" {
			var banned ChannelBannedRights
			if err = json.Unmarshal([]byte(rawBanned), &banned); err != nil {
				return false, err
			}
			if banned.Active(time.Now().Unix()) && banned.Kicks(time.Now().Unix()) {
				return false, nil
			}
		}
	}
	if len(messageIDs) == 0 {
		return false, ErrInvalidMessageID
	}
	query := `SELECT COUNT(*) FROM apifull_channel_message_hidden WHERE user_id=? AND channel_id=? AND message_id IN (` + strings.TrimRight(strings.Repeat("?,", len(messageIDs)), ",") + `)`
	args := make([]interface{}, 0, len(messageIDs)+2)
	args = append(args, userID, channelID)
	for _, id := range messageIDs {
		args = append(args, id)
	}
	var hidden int
	if err = db.QueryRowContext(ctx, query, args...).Scan(&hidden); err != nil {
		return false, err
	}
	return hidden == 0, nil
}

func CompleteChannelDeliveryRecipient(ctx context.Context, key ChannelDeliveryKey, userID int64, state string) error {
	if db == nil {
		return errors.New("domain PostgreSQL is not open")
	}
	if state != "delivered" && state != "skipped" {
		return errors.New("invalid channel delivery recipient state")
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `UPDATE apifull_channel_delivery_recipient r
		JOIN apifull_channel_delivery_outbox o ON o.id=r.delivery_id
		SET r.state=?, r.delivered_at=?
		WHERE o.channel_id=? AND o.pts_from=? AND o.pts_to=? AND o.state='pending' AND r.user_id=? AND r.state='pending'`,
		state, time.Now().Unix(), key.ChannelID, key.PTSFrom, key.PTSTo, userID); err != nil {
		return err
	}
	var pending int
	if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM apifull_channel_delivery_recipient r
		JOIN apifull_channel_delivery_outbox o ON o.id=r.delivery_id
		WHERE o.channel_id=? AND o.pts_from=? AND o.pts_to=? AND r.state='pending'`,
		key.ChannelID, key.PTSFrom, key.PTSTo).Scan(&pending); err != nil {
		return err
	}
	if pending == 0 {
		if _, err = tx.ExecContext(ctx, `UPDATE apifull_channel_delivery_outbox
			SET state='completed', payload='', exclude_auth_key_id=0
			WHERE channel_id=? AND pts_from=? AND pts_to=?`, key.ChannelID, key.PTSFrom, key.PTSTo); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `DELETE r FROM apifull_channel_delivery_recipient r
			JOIN apifull_channel_delivery_outbox o ON o.id=r.delivery_id
			WHERE o.channel_id=? AND o.pts_from=? AND o.pts_to=?`, key.ChannelID, key.PTSFrom, key.PTSTo); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func RetryChannelDeliveryRecipient(ctx context.Context, key ChannelDeliveryKey, userID int64, cause error) error {
	if db == nil {
		return errors.New("domain PostgreSQL is not open")
	}
	var attempts int
	err := db.QueryRowContext(ctx, `SELECT r.attempts FROM apifull_channel_delivery_recipient r
		JOIN apifull_channel_delivery_outbox o ON o.id=r.delivery_id
		WHERE o.channel_id=? AND o.pts_from=? AND o.pts_to=? AND o.state='pending' AND r.user_id=? AND r.state='pending'`,
		key.ChannelID, key.PTSFrom, key.PTSTo, userID).Scan(&attempts)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return err
	}
	delay := time.Second << minInt(attempts, 8)
	if delay > 5*time.Minute {
		delay = 5 * time.Minute
	}
	detail := "delivery failed"
	if cause != nil {
		detail = cause.Error()
	}
	if len(detail) > 255 {
		detail = strings.ToValidUTF8(detail[:255], "")
	}
	_, err = db.ExecContext(ctx, `UPDATE apifull_channel_delivery_recipient r
		JOIN apifull_channel_delivery_outbox o ON o.id=r.delivery_id
		SET r.attempts=r.attempts+1, r.next_attempt_at=?, r.last_error=?
		WHERE o.channel_id=? AND o.pts_from=? AND o.pts_to=? AND o.state='pending' AND r.user_id=? AND r.state='pending'`,
		time.Now().Add(delay).Unix(), detail, key.ChannelID, key.PTSFrom, key.PTSTo, userID)
	return err
}

func minInt(value, maximum int) int {
	if value < 0 {
		return 0
	}
	if value > maximum {
		return maximum
	}
	return value
}
