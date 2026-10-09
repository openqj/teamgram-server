package dao

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/teamgram/teamgram-server/pkg/storage/postgres"
)

const defaultSecretDifferenceLimit int32 = 5000

var (
	ErrMaxQTSInvalid         = errors.New("secret update qts is beyond the current state")
	ErrSecretUpdatesDisabled = errors.New("secret updates PostgreSQL is not configured")
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

func NewSecretUpdatesReader(dsn string) (SecretUpdatesReader, error) {
	if dsn == "" {
		return nil, ErrSecretUpdatesDisabled
	}
	pool, err := postgres.NewPool(context.Background(), postgres.Config{DSN: dsn, MaxConns: 8})
	if err != nil {
		return nil, err
	}
	if err := postgres.VerifySchema(context.Background(), pool,
		`SELECT user_id,last_qts FROM apifull_secret_user_state LIMIT 0`,
		`SELECT chat_id,recipient_user_id,random_id,qts,date,encrypted_data,service,file_id,file_access_hash,file_size,file_dc_id,file_key_fingerprint FROM apifull_secret_message LIMIT 0`); err != nil {
		pool.Close()
		return nil, err
	}
	return &postgresSecretUpdatesReader{pool: pool}, nil
}

type postgresSecretUpdatesReader struct{ pool *pgxpool.Pool }

func (r *postgresSecretUpdatesReader) Close() error {
	if r != nil && r.pool != nil {
		r.pool.Close()
	}
	return nil
}

func (r *postgresSecretUpdatesReader) CurrentQTS(ctx context.Context, userID int64) (int32, error) {
	var qts int32
	err := r.pool.QueryRow(ctx, `SELECT last_qts FROM apifull_secret_user_state WHERE user_id=$1`, userID).Scan(&qts)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil
	}
	return qts, err
}

func (r *postgresSecretUpdatesReader) GetDifference(ctx context.Context, userID int64, qts, limit int32) (SecretDifference, error) {
	if qts < 0 {
		return SecretDifference{}, ErrMaxQTSInvalid
	}
	if limit <= 0 || limit > defaultSecretDifferenceLimit {
		limit = defaultSecretDifferenceLimit
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return SecretDifference{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var result SecretDifference
	err = tx.QueryRow(ctx, `SELECT last_qts FROM apifull_secret_user_state WHERE user_id=$1`, userID).Scan(&result.CurrentQTS)
	if errors.Is(err, pgx.ErrNoRows) {
		result.CurrentQTS = 0
	} else if err != nil {
		return SecretDifference{}, err
	}
	if qts > result.CurrentQTS {
		return SecretDifference{}, ErrMaxQTSInvalid
	}
	rows, err := tx.Query(ctx, `SELECT chat_id, random_id, qts, date, encrypted_data, service,
		file_id, file_access_hash, file_size, file_dc_id, file_key_fingerprint
		FROM apifull_secret_message
		WHERE recipient_user_id=$1 AND qts>$2 AND qts<=$3
		ORDER BY qts LIMIT $4`, userID, qts, result.CurrentQTS, limit+1)
	if err != nil {
		return SecretDifference{}, err
	}
	result.Messages = make([]SecretMessage, 0, limit)
	for rows.Next() {
		var message SecretMessage
		var fileID, fileAccessHash, fileSize *int64
		var fileDCID, fileKeyFingerprint *int32
		if err := rows.Scan(&message.ChatID, &message.RandomID, &message.QTS, &message.Date, &message.Data, &message.Service,
			&fileID, &fileAccessHash, &fileSize, &fileDCID, &fileKeyFingerprint); err != nil {
			rows.Close()
			return SecretDifference{}, err
		}
		if fileID != nil {
			if fileAccessHash == nil || fileSize == nil || fileDCID == nil || fileKeyFingerprint == nil {
				rows.Close()
				return SecretDifference{}, errors.New("secret updates: incomplete encrypted file metadata")
			}
			message.File = &SecretFile{ID: *fileID, AccessHash: *fileAccessHash, Size: *fileSize, DCID: *fileDCID, KeyFingerprint: *fileKeyFingerprint}
		}
		result.Messages = append(result.Messages, message)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return SecretDifference{}, err
	}
	if len(result.Messages) > int(limit) {
		result.Messages = result.Messages[:limit]
		result.HasMore = true
	}
	if err := tx.Commit(ctx); err != nil {
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
