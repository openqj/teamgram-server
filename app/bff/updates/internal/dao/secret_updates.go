package dao

import (
	"context"
	"database/sql"
	"errors"

	_ "github.com/go-sql-driver/mysql"
)

const defaultSecretDifferenceLimit int32 = 5000

var (
	ErrMaxQTSInvalid         = errors.New("secret update qts is beyond the current state")
	ErrSecretUpdatesDisabled = errors.New("secret updates mysql is not configured")
)

type SecretFile struct {
	ID             int64
	AccessHash     int64
	Size           int64
	DCID           int32
	KeyFingerprint int32
}

type SecretMessage struct {
	ChatID   int32
	RandomID int64
	QTS      int32
	Date     int32
	Data     []byte
	Service  bool
	File     *SecretFile
}

type SecretDifference struct {
	CurrentQTS int32
	Messages   []SecretMessage
	HasMore    bool
}

type SecretUpdatesReader interface {
	CurrentQTS(ctx context.Context, userID int64) (int32, error)
	GetDifference(ctx context.Context, userID int64, qts, limit int32) (SecretDifference, error)
}

type mysqlSecretUpdatesReader struct {
	db *sql.DB
}

type unavailableSecretUpdatesReader struct {
	err error
}

func NewSecretUpdatesReader(dsn string) (SecretUpdatesReader, error) {
	if dsn == "" {
		return nil, ErrSecretUpdatesDisabled
	}
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}
	return &mysqlSecretUpdatesReader{db: db}, nil
}

func (r *unavailableSecretUpdatesReader) CurrentQTS(context.Context, int64) (int32, error) {
	return 0, r.err
}

func (r *unavailableSecretUpdatesReader) GetDifference(context.Context, int64, int32, int32) (SecretDifference, error) {
	return SecretDifference{}, r.err
}

func (r *mysqlSecretUpdatesReader) CurrentQTS(ctx context.Context, userID int64) (int32, error) {
	var qts int32
	err := r.db.QueryRowContext(ctx, `SELECT last_qts FROM apifull_secret_user_state WHERE user_id=?`, userID).Scan(&qts)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return qts, err
}

func (r *mysqlSecretUpdatesReader) GetDifference(ctx context.Context, userID int64, qts, limit int32) (SecretDifference, error) {
	if qts < 0 {
		return SecretDifference{}, ErrMaxQTSInvalid
	}
	if limit <= 0 || limit > defaultSecretDifferenceLimit {
		limit = defaultSecretDifferenceLimit
	}

	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return SecretDifference{}, err
	}
	defer tx.Rollback()

	var result SecretDifference
	err = tx.QueryRowContext(ctx, `SELECT last_qts FROM apifull_secret_user_state WHERE user_id=?`, userID).Scan(&result.CurrentQTS)
	if errors.Is(err, sql.ErrNoRows) {
		result.CurrentQTS = 0
	} else if err != nil {
		return SecretDifference{}, err
	}
	if qts > result.CurrentQTS {
		return SecretDifference{}, ErrMaxQTSInvalid
	}

	rows, err := tx.QueryContext(ctx, `SELECT chat_id, random_id, qts, date, encrypted_data, service,
		file_id, file_access_hash, file_size, file_dc_id, file_key_fingerprint
		FROM apifull_secret_message
		WHERE recipient_user_id=? AND qts>? AND qts<=?
		ORDER BY qts LIMIT ?`, userID, qts, result.CurrentQTS, limit+1)
	if err != nil {
		return SecretDifference{}, err
	}
	defer rows.Close()

	result.Messages = make([]SecretMessage, 0, limit)
	for rows.Next() {
		var (
			message                          SecretMessage
			service                          int8
			fileID, fileAccessHash, fileSize sql.NullInt64
			fileDCID, fileKeyFingerprint     sql.NullInt32
		)
		if err = rows.Scan(
			&message.ChatID, &message.RandomID, &message.QTS, &message.Date, &message.Data, &service,
			&fileID, &fileAccessHash, &fileSize, &fileDCID, &fileKeyFingerprint,
		); err != nil {
			return SecretDifference{}, err
		}
		message.Service = service != 0
		if fileID.Valid {
			message.File = &SecretFile{
				ID:             fileID.Int64,
				AccessHash:     fileAccessHash.Int64,
				Size:           fileSize.Int64,
				DCID:           fileDCID.Int32,
				KeyFingerprint: fileKeyFingerprint.Int32,
			}
		}
		result.Messages = append(result.Messages, message)
	}
	if err = rows.Err(); err != nil {
		return SecretDifference{}, err
	}
	if len(result.Messages) > int(limit) {
		result.Messages = result.Messages[:limit]
		result.HasMore = true
	}
	if err = rows.Close(); err != nil {
		return SecretDifference{}, err
	}
	if err = tx.Commit(); err != nil {
		return SecretDifference{}, err
	}
	return result, nil
}

func (d *Dao) CurrentSecretQTS(ctx context.Context, userID int64) (int32, error) {
	if d.SecretUpdates == nil {
		return 0, ErrSecretUpdatesDisabled
	}
	return d.SecretUpdates.CurrentQTS(ctx, userID)
}

func (d *Dao) GetSecretDifference(ctx context.Context, userID int64, qts, limit int32) (SecretDifference, error) {
	if d.SecretUpdates == nil {
		return SecretDifference{}, ErrSecretUpdatesDisabled
	}
	return d.SecretUpdates.GetDifference(ctx, userID, qts, limit)
}
