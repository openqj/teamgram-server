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

type ChannelDeliveryRecipientResult struct {
	UserID int64
	State  string
	Err    error
}

const (
	ChannelDeliveryRecipientDelivered = "delivered"
	ChannelDeliveryRecipientSkipped   = "skipped"
	ChannelDeliveryRecipientRetry     = "retry"
)

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
	started := time.Now()
	defer observeChannelPersistence("outbox_write", started)
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
	var deliveryID int64
	err = tx.QueryRow(`INSERT INTO apifull_channel_delivery_outbox
		(channel_id, pts_from, pts_to, sender_user_id, exclude_auth_key_id, payload, created_at)
		VALUES (?,?,?,?,?,?,?)
		ON CONFLICT (channel_id, pts_from, pts_to) DO NOTHING
		RETURNING id`, channelID, key.PTSFrom, key.PTSTo, senderID, excludeAuthKeyID, payload, time.Now().Unix()).Scan(&deliveryID)
	if errors.Is(err, sql.ErrNoRows) {
		var state string
		if err = tx.QueryRow(`SELECT state FROM apifull_channel_delivery_outbox
			WHERE channel_id=? AND pts_from=? AND pts_to=? FOR UPDATE`, channelID, key.PTSFrom, key.PTSTo).Scan(&state); err != nil {
			return err
		}
		return nil
	}
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
	lockHash := sha256.Sum256([]byte(fmt.Sprintf("%d:%d:%d", key.ChannelID, key.PTSFrom, key.PTSTo)))
	lockName := "apifull:ch:" + hex.EncodeToString(lockHash[:20])
	return lockPostgresTransaction(ctx, lockName, wait)
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
	results, err := ChannelDeliveryRecipientsCanReceive(ctx, channelID, creatorID, []ChannelDeliveryRecipient{{UserID: userID}}, messageIDs)
	if err != nil {
		return false, err
	}
	return results[userID], nil
}

func ChannelDeliveryRecipientsCanReceive(ctx context.Context, channelID, creatorID int64, recipients []ChannelDeliveryRecipient, messageIDs []int32) (map[int64]bool, error) {
	allowed := make(map[int64]bool, len(recipients))
	if db == nil {
		return nil, errors.New("domain PostgreSQL is not open")
	}
	if channelID <= 0 || creatorID <= 0 || len(messageIDs) == 0 {
		return nil, ErrInvalidMessageID
	}
	if len(recipients) == 0 {
		return allowed, nil
	}

	users := make([]string, 0, len(recipients))
	args := make([]interface{}, 0, 2+len(recipients)+len(messageIDs))
	seen := make(map[int64]struct{}, len(recipients))
	for _, recipient := range recipients {
		if recipient.UserID <= 0 {
			return nil, errors.New("invalid channel delivery recipient")
		}
		if _, ok := seen[recipient.UserID]; ok {
			return nil, errors.New("duplicate channel delivery recipient")
		}
		seen[recipient.UserID] = struct{}{}
		users = append(users, "(CAST(? AS BIGINT))")
	}
	messagePlaceholders := make([]string, 0, len(messageIDs))
	for range messageIDs {
		messagePlaceholders = append(messagePlaceholders, "?")
	}
	args = append(args, channelID)
	for _, messageID := range messageIDs {
		args = append(args, messageID)
	}
	for _, recipient := range recipients {
		args = append(args, recipient.UserID)
	}
	args = append(args, channelID, creatorID)
	query := `SELECT requested.user_id, COALESCE(member.banned_rights,''), EXISTS (
			SELECT 1 FROM apifull_channel_message_hidden hidden
			WHERE hidden.user_id=requested.user_id AND hidden.channel_id=?
			AND hidden.message_id IN (` + strings.Join(messagePlaceholders, ",") + `))
		FROM (VALUES ` + strings.Join(users, ",") + `) AS requested(user_id)
		LEFT JOIN apifull_channel_member member
			ON member.channel_id=? AND member.user_id=requested.user_id
		WHERE requested.user_id=? OR member.user_id IS NOT NULL`
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	now := time.Now().Unix()
	for rows.Next() {
		var userID int64
		var rawBanned string
		var hidden bool
		if err = rows.Scan(&userID, &rawBanned, &hidden); err != nil {
			return nil, err
		}
		if hidden {
			continue
		}
		if rawBanned != "" {
			var banned ChannelBannedRights
			if err = json.Unmarshal([]byte(rawBanned), &banned); err != nil {
				return nil, err
			}
			if banned.Active(now) && banned.Kicks(now) {
				continue
			}
		}
		allowed[userID] = true
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return allowed, nil
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
	var deliveryID int64
	err = tx.QueryRowContext(ctx, `SELECT id FROM apifull_channel_delivery_outbox
		WHERE channel_id=$1 AND pts_from=$2 AND pts_to=$3 AND state='pending' FOR UPDATE`,
		key.ChannelID, key.PTSFrom, key.PTSTo).Scan(&deliveryID)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE apifull_channel_delivery_recipient r
		SET state=$1, delivered_at=$2
		WHERE delivery_id=$3 AND user_id=$4 AND state='pending'`,
		state, time.Now().Unix(), deliveryID, userID); err != nil {
		return err
	}
	var pending int
	if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM apifull_channel_delivery_recipient
		WHERE delivery_id=$1 AND state='pending'`, deliveryID).Scan(&pending); err != nil {
		return err
	}
	if pending == 0 {
		if _, err = tx.ExecContext(ctx, `UPDATE apifull_channel_delivery_outbox
			SET state='completed', payload='', exclude_auth_key_id=0
			WHERE id=$1`, deliveryID); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `DELETE FROM apifull_channel_delivery_recipient WHERE delivery_id=$1`, deliveryID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func RecordChannelDeliveryRecipientResults(ctx context.Context, key ChannelDeliveryKey, results []ChannelDeliveryRecipientResult) error {
	if db == nil {
		return errors.New("domain PostgreSQL is not open")
	}
	if len(results) == 0 {
		return nil
	}
	if key.ChannelID <= 0 || key.PTSFrom <= 0 || key.PTSTo < key.PTSFrom {
		return ErrInvalidMessageID
	}
	seen := make(map[int64]struct{}, len(results))
	for _, result := range results {
		if result.UserID <= 0 {
			return errors.New("invalid channel delivery recipient")
		}
		if _, ok := seen[result.UserID]; ok {
			return errors.New("duplicate channel delivery recipient result")
		}
		seen[result.UserID] = struct{}{}
		if result.State != ChannelDeliveryRecipientDelivered && result.State != ChannelDeliveryRecipientSkipped && result.State != ChannelDeliveryRecipientRetry {
			return errors.New("invalid channel delivery recipient state")
		}
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var deliveryID int64
	err = tx.QueryRowContext(ctx, `SELECT id FROM apifull_channel_delivery_outbox
		WHERE channel_id=$1 AND pts_from=$2 AND pts_to=$3 AND state='pending' FOR UPDATE`,
		key.ChannelID, key.PTSFrom, key.PTSTo).Scan(&deliveryID)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return err
	}

	now := time.Now().Unix()
	values := make([]string, 0, len(results))
	args := make([]interface{}, 0, 2+len(results)*3)
	args = append(args, deliveryID, now)
	for i, result := range results {
		first := 3 + i*3
		values = append(values, fmt.Sprintf("($%d::bigint,$%d::text,$%d::text)", first, first+1, first+2))
		detail := ""
		if result.State == ChannelDeliveryRecipientRetry {
			detail = "delivery failed"
			if result.Err != nil {
				detail = result.Err.Error()
			}
			if len(detail) > 255 {
				detail = strings.ToValidUTF8(detail[:255], "")
			}
		}
		args = append(args, result.UserID, result.State, detail)
	}
	query := `UPDATE apifull_channel_delivery_recipient AS r
		SET state=CASE WHEN outcome.recipient_state IN ('delivered','skipped') THEN outcome.recipient_state ELSE r.state END,
			delivered_at=CASE WHEN outcome.recipient_state IN ('delivered','skipped') THEN $2 ELSE r.delivered_at END,
			attempts=r.attempts+CASE WHEN outcome.recipient_state='retry' THEN 1 ELSE 0 END,
			next_attempt_at=CASE WHEN outcome.recipient_state='retry'
				THEN $2 + LEAST(300::bigint, (1::bigint << LEAST(r.attempts, 8))) ELSE r.next_attempt_at END,
			last_error=CASE WHEN outcome.recipient_state='retry' THEN outcome.detail ELSE r.last_error END
		FROM (VALUES ` + strings.Join(values, ",") + `) AS outcome(user_id,recipient_state,detail)
		WHERE r.delivery_id=$1 AND r.user_id=outcome.user_id AND r.state='pending'`
	if _, err = tx.ExecContext(ctx, query, args...); err != nil {
		return err
	}
	var pending int
	if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM apifull_channel_delivery_recipient
		WHERE delivery_id=$1 AND state='pending'`, deliveryID).Scan(&pending); err != nil {
		return err
	}
	if pending == 0 {
		if _, err = tx.ExecContext(ctx, `UPDATE apifull_channel_delivery_outbox
			SET state='completed', payload='', exclude_auth_key_id=0 WHERE id=$1`, deliveryID); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `DELETE FROM apifull_channel_delivery_recipient WHERE delivery_id=$1`, deliveryID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func RetryChannelDeliveryRecipient(ctx context.Context, key ChannelDeliveryKey, userID int64, cause error) error {
	if db == nil {
		return errors.New("domain PostgreSQL is not open")
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var deliveryID int64
	err = tx.QueryRowContext(ctx, `SELECT id FROM apifull_channel_delivery_outbox
		WHERE channel_id=$1 AND pts_from=$2 AND pts_to=$3 AND state='pending' FOR UPDATE`,
		key.ChannelID, key.PTSFrom, key.PTSTo).Scan(&deliveryID)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return err
	}
	var attempts int
	err = tx.QueryRowContext(ctx, `SELECT attempts FROM apifull_channel_delivery_recipient
		WHERE delivery_id=$1 AND user_id=$2 AND state='pending' FOR UPDATE`, deliveryID, userID).Scan(&attempts)
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
	_, err = tx.ExecContext(ctx, `UPDATE apifull_channel_delivery_recipient
		SET attempts=attempts+1, next_attempt_at=$1, last_error=$2
		WHERE delivery_id=$3 AND user_id=$4 AND state='pending'`,
		time.Now().Add(delay).Unix(), detail, deliveryID, userID)
	if err != nil {
		return err
	}
	return tx.Commit()
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
