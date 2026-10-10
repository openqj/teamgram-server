package dao

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/pkg/storage/postgres"
)

// SaveEncryptedFile records the identity returned by DFS. The file id is
// globally unique; an exact retry is idempotent while a conflicting identity
// is rejected instead of silently changing the access hash.
func (p *Postgres) SaveEncryptedFile(ctx context.Context, ownerID int64, file *mtproto.EncryptedFile) error {
	if p == nil || p.Pool == nil {
		return errors.New("media: PostgreSQL store is not initialized")
	}
	// Auth key IDs are signed int64 values on the wire; zero is the only
	// missing-owner sentinel.
	if ownerID == 0 || file == nil || file.GetId() <= 0 || file.GetAccessHash() == 0 || file.GetSize2_INT64() <= 0 || file.GetDcId() <= 0 {
		return mtproto.ErrMediaInvalid
	}
	return postgres.WithTx(ctx, p.Pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		var existing struct {
			ownerID, accessHash, size int64
			dcID, fingerprint         int32
		}
		err := tx.QueryRow(ctx, `SELECT owner_id, access_hash, file_size, dc_id, key_fingerprint
FROM encrypted_files WHERE encrypted_file_id = $1 FOR UPDATE`, file.GetId()).Scan(
			&existing.ownerID, &existing.accessHash, &existing.size, &existing.dcID, &existing.fingerprint)
		if err == nil {
			return verifyEncryptedFileIdentity(ownerID, file, existing.ownerID, existing.accessHash, existing.size, existing.dcID, existing.fingerprint)
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO encrypted_files
 (encrypted_file_id, owner_id, access_hash, file_size, dc_id, key_fingerprint)
 VALUES ($1,$2,$3,$4,$5,$6)
 ON CONFLICT (encrypted_file_id) DO NOTHING`,
			file.GetId(), ownerID, file.GetAccessHash(), file.GetSize2_INT64(), file.GetDcId(), file.GetKeyFingerprint()); err != nil {
			return err
		}
		// A concurrent insert may have won the unique key. Re-read it under the
		// same transaction before acknowledging the upload.
		err = tx.QueryRow(ctx, `SELECT owner_id, access_hash, file_size, dc_id, key_fingerprint
FROM encrypted_files WHERE encrypted_file_id = $1 FOR UPDATE`, file.GetId()).Scan(
			&existing.ownerID, &existing.accessHash, &existing.size, &existing.dcID, &existing.fingerprint)
		if err != nil {
			return err
		}
		return verifyEncryptedFileIdentity(ownerID, file, existing.ownerID, existing.accessHash, existing.size, existing.dcID, existing.fingerprint)
	})
}

func verifyEncryptedFileIdentity(ownerID int64, file *mtproto.EncryptedFile, existingOwner, existingAccessHash, existingSize int64, existingDC, existingFingerprint int32) error {
	if existingOwner != ownerID || existingAccessHash != file.GetAccessHash() || existingSize != file.GetSize2_INT64() || existingDC != file.GetDcId() || existingFingerprint != file.GetKeyFingerprint() {
		return mtproto.ErrFileIdInvalid
	}
	return nil
}

func (p *Postgres) GetEncryptedFile(ctx context.Context, id, accessHash int64) (*mtproto.EncryptedFile, error) {
	if p == nil || p.Pool == nil {
		return nil, errors.New("media: PostgreSQL store is not initialized")
	}
	if id <= 0 || accessHash == 0 {
		return nil, mtproto.ErrMediaInvalid
	}
	file := &mtproto.EncryptedFile{}
	err := p.Pool.QueryRow(ctx, `SELECT encrypted_file_id, access_hash, file_size, dc_id, key_fingerprint
FROM encrypted_files WHERE encrypted_file_id = $1 AND access_hash = $2`, id, accessHash).Scan(
		&file.Id, &file.AccessHash, &file.Size2_INT64, &file.DcId, &file.KeyFingerprint)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, mtproto.ErrFileIdInvalid
	}
	if err != nil {
		return nil, err
	}
	if file.Size2_INT64 > int64(^uint32(0)>>1) {
		file.Size2_INT32 = int32(^uint32(0) >> 1)
	} else {
		file.Size2_INT32 = int32(file.Size2_INT64)
	}
	return mtproto.MakeTLEncryptedFile(file).To_EncryptedFile(), nil
}
