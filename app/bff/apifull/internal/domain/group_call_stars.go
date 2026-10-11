package domain

import (
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"time"
)

// GroupCallStarsDonor is the durable aggregate used by phone.getGroupCallStars.
// It is derived only from settled paid group-call messages, never from a
// client supplied total.
type GroupCallStarsDonor struct {
	UserID int64
	Stars  int64
}

// CreatePaidGroupCallMessage atomically charges the sender, credits the call
// creator, and stores the message. The random ID is the idempotency key for
// both the message and the two ledger entries.
func CreatePaidGroupCallMessage(callID, senderID, randomID int64, message, sendAs string, stars int64) (GroupCallMessage, bool, error) {
	if db == nil {
		return GroupCallMessage{}, false, errors.New("domain PostgreSQL is not open")
	}
	if callID <= 0 || senderID <= 0 || randomID == 0 || stars <= 0 {
		return GroupCallMessage{}, false, errors.New("invalid paid group call message")
	}
	tx, err := db.Begin()
	if err != nil {
		return GroupCallMessage{}, false, err
	}
	defer func() { _ = tx.Rollback() }()
	// Lock by call/random ID before checking for an existing row. PostgreSQL
	// cannot lock an absent row with SELECT FOR UPDATE, so this closes the
	// first-write race and makes retries deterministic.
	if _, err = tx.Exec(`SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`,
		fmt.Sprintf("apifull-group-call-paid:%d:%d", callID, randomID)); err != nil {
		return GroupCallMessage{}, false, err
	}
	var existing GroupCallMessage
	err = scanGroupCallMessageInto(tx.QueryRow(`SELECT id, call_id, sender_user_id, random_id, message,
		send_as, paid_stars, date, deleted FROM apifull_group_call_message WHERE call_id=$1 AND random_id=$2`, callID, randomID), &existing)
	if err == nil {
		if existing.SenderID != senderID || existing.PaidStars == nil || *existing.PaidStars != stars || existing.Message != message || existing.SendAs != sendAs {
			return GroupCallMessage{}, false, ErrRandomIDConflict
		}
		if err = tx.Commit(); err != nil {
			return GroupCallMessage{}, false, err
		}
		return existing, false, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return GroupCallMessage{}, false, err
	}
	var creatorID int64
	if err = tx.QueryRow(`SELECT creator_user_id FROM apifull_group_call WHERE id=$1 FOR SHARE`, callID).Scan(&creatorID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return GroupCallMessage{}, false, ErrCallNotFound
		}
		return GroupCallMessage{}, false, err
	}
	if creatorID <= 0 || creatorID == senderID {
		return GroupCallMessage{}, false, errors.New("invalid paid group call recipient")
	}
	debitKey := fmt.Sprintf("group-call-paid:%d:%d:debit", callID, randomID)
	creditKey := fmt.Sprintf("group-call-paid:%d:%d:credit", callID, randomID)
	// Every Stars writer locks user rows in ascending order to avoid a
	// sender/creator deadlock when two calls transfer in opposite directions.
	if senderID < creatorID {
		if _, err = applyStarsTx(tx, senderID, -stars, debitKey); err != nil {
			return GroupCallMessage{}, false, err
		}
		if _, err = applyStarsTx(tx, creatorID, stars, creditKey); err != nil {
			return GroupCallMessage{}, false, err
		}
	} else {
		if _, err = applyStarsTx(tx, creatorID, stars, creditKey); err != nil {
			return GroupCallMessage{}, false, err
		}
		if _, err = applyStarsTx(tx, senderID, -stars, debitKey); err != nil {
			return GroupCallMessage{}, false, err
		}
	}
	now := time.Now().Unix()
	var id int64
	if err = tx.QueryRow(`INSERT INTO apifull_group_call_message
		(call_id, sender_user_id, random_id, message, send_as, paid_stars, date)
		VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING id`, callID, senderID, randomID, message, sendAs, stars, now).Scan(&id); err != nil {
		return GroupCallMessage{}, false, err
	}
	if err = tx.Commit(); err != nil {
		return GroupCallMessage{}, false, err
	}
	return GroupCallMessage{ID: id, CallID: callID, SenderID: senderID, RandomID: randomID,
		Message: message, SendAs: sendAs, PaidStars: &stars, Date: now}, true, nil
}

func scanGroupCallMessageInto(row interface{ Scan(...any) error }, message *GroupCallMessage) error {
	var paid sql.NullInt64
	var deleted int
	if err := row.Scan(&message.ID, &message.CallID, &message.SenderID, &message.RandomID, &message.Message,
		&message.SendAs, &paid, &message.Date, &deleted); err != nil {
		return err
	}
	if paid.Valid {
		value := paid.Int64
		message.PaidStars = &value
	}
	message.Deleted = deleted != 0
	return nil
}

// LoadGroupCallStars returns settled, non-deleted paid messages and aggregates
// donors deterministically for stable pagination/client rendering.
func LoadGroupCallStars(callID int64) (int64, []GroupCallStarsDonor, error) {
	if db == nil {
		return 0, nil, errors.New("domain PostgreSQL is not open")
	}
	if callID <= 0 {
		return 0, nil, errors.New("invalid group call id")
	}
	var total int64
	if err := db.QueryRow(`SELECT COALESCE(SUM(paid_stars),0) FROM apifull_group_call_message
		WHERE call_id=$1 AND deleted=0 AND paid_stars IS NOT NULL AND paid_stars>0`, callID).Scan(&total); err != nil {
		return 0, nil, err
	}
	rows, err := db.Query(`SELECT sender_user_id, COALESCE(SUM(paid_stars),0) AS stars
		FROM apifull_group_call_message
		WHERE call_id=$1 AND deleted=0 AND paid_stars IS NOT NULL AND paid_stars>0
		GROUP BY sender_user_id`, callID)
	if err != nil {
		return 0, nil, err
	}
	defer rows.Close()
	donors := make([]GroupCallStarsDonor, 0)
	for rows.Next() {
		var donor GroupCallStarsDonor
		if err = rows.Scan(&donor.UserID, &donor.Stars); err != nil {
			return 0, nil, err
		}
		if donor.UserID > 0 && donor.Stars > 0 {
			donors = append(donors, donor)
		}
	}
	if err = rows.Err(); err != nil {
		return 0, nil, err
	}
	sort.Slice(donors, func(i, j int) bool {
		if donors[i].Stars == donors[j].Stars {
			return donors[i].UserID < donors[j].UserID
		}
		return donors[i].Stars > donors[j].Stars
	})
	return total, donors, nil
}
