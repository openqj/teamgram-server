package persist

import (
	"encoding/json"
	"errors"
	"fmt"
)

// QuickReplyRecord is the durable PostgreSQL representation of one shortcut
// and its saved message associations. The API layer owns Telegram validation;
// this package only provides transactional storage.
type QuickReplyRecord struct {
	ShortcutID int32
	Shortcut   string
	MessageIDs []int32
}

var ErrQuickReplyStateUnavailable = errors.New("apifull: quick reply state requires PostgreSQL")

func quickReplyStore() (*postgresStore, error) {
	store, ok := Default.(*postgresStore)
	if !ok || store == nil || store.db == nil {
		return nil, ErrQuickReplyStateUnavailable
	}
	return store, nil
}

// LoadQuickReplyRecords reads all shortcuts for one user in deterministic
// order. An absent user has an empty vector.
func LoadQuickReplyRecords(userID int64) ([]QuickReplyRecord, error) {
	store, err := quickReplyStore()
	if err != nil {
		return nil, err
	}
	rows, err := store.db.Query(`SELECT shortcut_id, shortcut, message_ids
		FROM apifull_quick_reply WHERE user_id = $1 ORDER BY position, shortcut_id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	records := make([]QuickReplyRecord, 0)
	for rows.Next() {
		var record QuickReplyRecord
		var raw []byte
		if err = rows.Scan(&record.ShortcutID, &record.Shortcut, &raw); err != nil {
			return nil, err
		}
		if len(raw) == 0 {
			raw = []byte("[]")
		}
		if err = json.Unmarshal(raw, &record.MessageIDs); err != nil {
			return nil, fmt.Errorf("apifull: invalid quick reply message_ids: %w", err)
		}
		records = append(records, record)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return records, nil
}

// MutateQuickReplyRecords performs one user-scoped read/modify/write under a
// PostgreSQL transaction. The advisory lock closes the absent-row race and
// the row lock protects existing shortcuts across API instances.
func MutateQuickReplyRecords(userID int64, mutate func([]QuickReplyRecord) ([]QuickReplyRecord, error)) ([]QuickReplyRecord, error) {
	store, err := quickReplyStore()
	if err != nil {
		return nil, err
	}
	tx, err := store.db.Begin()
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err = tx.Exec(`SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`,
		fmt.Sprintf("apifull-quick-reply:%d", userID)); err != nil {
		return nil, err
	}
	rows, err := tx.Query(`SELECT shortcut_id, shortcut, message_ids
		FROM apifull_quick_reply WHERE user_id = $1 ORDER BY position, shortcut_id FOR UPDATE`, userID)
	if err != nil {
		return nil, err
	}
	current := make([]QuickReplyRecord, 0)
	for rows.Next() {
		var record QuickReplyRecord
		var raw []byte
		if err = rows.Scan(&record.ShortcutID, &record.Shortcut, &raw); err != nil {
			_ = rows.Close()
			return nil, err
		}
		if len(raw) == 0 {
			raw = []byte("[]")
		}
		if err = json.Unmarshal(raw, &record.MessageIDs); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("apifull: invalid quick reply message_ids: %w", err)
		}
		current = append(current, record)
	}
	if err = rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err = rows.Close(); err != nil {
		return nil, err
	}
	next, err := mutate(current)
	if err != nil {
		return nil, err
	}
	if _, err = tx.Exec(`DELETE FROM apifull_quick_reply WHERE user_id = $1`, userID); err != nil {
		return nil, err
	}
	for position, record := range next {
		if record.ShortcutID <= 0 {
			return nil, errors.New("apifull: invalid quick reply record")
		}
		raw, marshalErr := json.Marshal(record.MessageIDs)
		if marshalErr != nil {
			return nil, marshalErr
		}
		if _, err = tx.Exec(`INSERT INTO apifull_quick_reply
			(user_id, shortcut_id, shortcut, message_ids, position)
			VALUES ($1, $2, $3, $4::jsonb, $5)`,
			userID, record.ShortcutID, record.Shortcut, raw, position); err != nil {
			return nil, err
		}
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return next, nil
}
