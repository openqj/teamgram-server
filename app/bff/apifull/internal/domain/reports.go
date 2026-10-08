package domain

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"strings"
	"time"
)

// Report is the durable moderation intake record.  It is deliberately an
// intake record only: a separate moderation worker must transition state and
// apply any enforcement action.
type Report struct {
	ID         int64
	ActorID    int64
	Kind       string
	TargetType string
	TargetID   int64
	DedupeKey  string
	Payload    []byte
	State      string
	CreatedAt  int64
	UpdatedAt  int64
}

var (
	ErrInvalidReport = errors.New("invalid report")
)

// SaveReport stores a report idempotently. The dedupe key is supplied by the
// caller so protocol retries do not create duplicate moderation work.
func SaveReport(actorID int64, kind, targetType string, targetID int64, dedupeKey string, payload []byte) error {
	if db == nil {
		return errors.New("domain PostgreSQL is not open")
	}
	if actorID <= 0 || strings.TrimSpace(kind) == "" || strings.TrimSpace(targetType) == "" || targetID < 0 || strings.TrimSpace(dedupeKey) == "" || len(payload) == 0 {
		return ErrInvalidReport
	}
	if len(dedupeKey) != sha256.Size*2 {
		return ErrInvalidReport
	}
	if _, err := hex.DecodeString(dedupeKey); err != nil {
		return ErrInvalidReport
	}
	now := time.Now().Unix()
	_, err := db.Exec(`INSERT INTO apifull_report
		(actor_user_id, kind, target_type, target_id, dedupe_key, payload, state, created_at, updated_at)
		VALUES (?,?,?,?,?,?, 'pending', ?, ?)
		ON DUPLICATE KEY UPDATE updated_at=VALUES(updated_at)`,
		actorID, kind, targetType, targetID, dedupeKey, payload, now, now)
	return err
}

// LoadReportByDedupe returns the canonical record for an idempotency key.
func LoadReportByDedupe(dedupeKey string) (Report, bool, error) {
	var out Report
	if db == nil {
		return out, false, errors.New("domain PostgreSQL is not open")
	}
	if len(dedupeKey) != sha256.Size*2 {
		return out, false, ErrInvalidReport
	}
	if _, err := hex.DecodeString(dedupeKey); err != nil {
		return out, false, ErrInvalidReport
	}
	err := db.QueryRow(`SELECT id, actor_user_id, kind, target_type, target_id, dedupe_key, payload, state, created_at, updated_at
		FROM apifull_report WHERE dedupe_key=?`, dedupeKey).Scan(
		&out.ID, &out.ActorID, &out.Kind, &out.TargetType, &out.TargetID, &out.DedupeKey,
		&out.Payload, &out.State, &out.CreatedAt, &out.UpdatedAt)
	if err == sql.ErrNoRows {
		return Report{}, false, nil
	}
	if err != nil {
		return Report{}, false, err
	}
	return out, true, nil
}
