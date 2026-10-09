package domain

import (
	"database/sql"
	"errors"
	"strings"
	"time"
)

// SavedPaymentInfo contains the buyer's reusable invoice information. Payment
// credentials are represented only by a provider-issued presence marker; raw
// card or token material never enters this table.
type SavedPaymentInfo struct {
	UserID              int64
	Name                string
	Phone               string
	Email               string
	HasSavedCredentials bool
	UpdatedAt           int64
}

// PostgresEnabled reports whether the APIFull domain has an open PostgreSQL
// handle. Callers use it to keep in-memory test doubles out of production
// request paths while preserving focused unit tests.
func PostgresEnabled() bool { return db != nil }

func LoadSavedPaymentInfo(userID int64) (SavedPaymentInfo, bool, error) {
	if db == nil {
		return SavedPaymentInfo{}, false, errors.New("domain PostgreSQL is not open")
	}
	if userID <= 0 {
		return SavedPaymentInfo{}, false, ErrInvalidPaymentRequest
	}
	var info SavedPaymentInfo
	err := db.QueryRow(`SELECT user_id, name, phone, email, credentials_saved, updated_at
		FROM apifull_payment_saved_info WHERE user_id=$1`, userID).
		Scan(&info.UserID, &info.Name, &info.Phone, &info.Email, &info.HasSavedCredentials, &info.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return SavedPaymentInfo{}, false, nil
	}
	if err != nil {
		return SavedPaymentInfo{}, false, err
	}
	return info, info.Name != "" || info.Phone != "" || info.Email != "" || info.HasSavedCredentials, nil
}

// SaveSavedPaymentInfo atomically upserts only invoice contact fields. The
// existing credentials marker is intentionally preserved.
func SaveSavedPaymentInfo(userID int64, name, phone, email string) error {
	if db == nil {
		return errors.New("domain PostgreSQL is not open")
	}
	if userID <= 0 {
		return ErrInvalidPaymentRequest
	}
	name, phone, email = strings.TrimSpace(name), strings.TrimSpace(phone), strings.TrimSpace(email)
	if name == "" && phone == "" && email == "" {
		return nil
	}
	now := time.Now().Unix()
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err = tx.Exec(`INSERT INTO apifull_payment_saved_info
		(user_id, name, phone, email, credentials_saved, updated_at)
		VALUES ($1,$2,$3,$4,FALSE,$5)
		ON CONFLICT (user_id) DO UPDATE SET name=EXCLUDED.name, phone=EXCLUDED.phone,
		email=EXCLUDED.email, updated_at=EXCLUDED.updated_at`, userID, name, phone, email, now); err != nil {
		return err
	}
	return tx.Commit()
}

// SetSavedPaymentCredentials changes only the provider-issued presence
// marker. It is separate from SaveSavedPaymentInfo so callers cannot
// accidentally persist credential bytes alongside contact information.
func SetSavedPaymentCredentials(userID int64, saved bool) error {
	if db == nil {
		return errors.New("domain PostgreSQL is not open")
	}
	if userID <= 0 {
		return ErrInvalidPaymentRequest
	}
	now := time.Now().Unix()
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err = tx.Exec(`INSERT INTO apifull_payment_saved_info
		(user_id, name, phone, email, credentials_saved, updated_at)
		VALUES ($1,'','','',$2,$3)
		ON CONFLICT (user_id) DO UPDATE SET credentials_saved=EXCLUDED.credentials_saved,
			updated_at=EXCLUDED.updated_at`, userID, saved, now); err != nil {
		return err
	}
	return tx.Commit()
}

// ClearSavedPaymentInfo clears each requested component in one transaction.
// Contact data and the credentials marker are independent per MTProto flags.
func ClearSavedPaymentInfo(userID int64, clearInfo, clearCredentials bool) error {
	if db == nil {
		return errors.New("domain PostgreSQL is not open")
	}
	if userID <= 0 {
		return ErrInvalidPaymentRequest
	}
	if !clearInfo && !clearCredentials {
		return nil
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var name, phone, email string
	var credentials bool
	err = tx.QueryRow(`SELECT name, phone, email, credentials_saved FROM apifull_payment_saved_info WHERE user_id=$1 FOR UPDATE`, userID).
		Scan(&name, &phone, &email, &credentials)
	if errors.Is(err, sql.ErrNoRows) {
		return tx.Commit()
	}
	if err != nil {
		return err
	}
	if clearInfo {
		name, phone, email = "", "", ""
	}
	if clearCredentials {
		credentials = false
	}
	if name == "" && phone == "" && email == "" && !credentials {
		if _, err = tx.Exec(`DELETE FROM apifull_payment_saved_info WHERE user_id=$1`, userID); err != nil {
			return err
		}
		return tx.Commit()
	}
	_, err = tx.Exec(`UPDATE apifull_payment_saved_info SET name=$1, phone=$2, email=$3,
		credentials_saved=$4, updated_at=$5 WHERE user_id=$6`, name, phone, email, credentials, time.Now().Unix(), userID)
	if err != nil {
		return err
	}
	return tx.Commit()
}
