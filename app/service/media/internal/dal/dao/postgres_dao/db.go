// Package postgres_dao contains the PostgreSQL persistence boundary for media.
// It uses pgx directly so media writes can participate in a caller-owned
// transaction during the migration from Teamgram's MySQL wrapper.
package postgres_dao

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/teamgram/teamgram-server/app/service/media/internal/dal/dataobject"
)

type DB interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

type Store struct {
	Pool       *pgxpool.Pool
	Documents  *DocumentsDAO
	Photos     *PhotosDAO
	PhotoSizes *PhotoSizesDAO
	VideoSizes *VideoSizesDAO
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{
		Pool:       pool,
		Documents:  NewDocumentsDAO(pool),
		Photos:     NewPhotosDAO(pool),
		PhotoSizes: NewPhotoSizesDAO(pool),
		VideoSizes: NewVideoSizesDAO(pool),
	}
}

func commandRows(tag pgconn.CommandTag) int64 { return tag.RowsAffected() }

const documentColumns = `id, document_id, access_hash, dc_id, file_path, file_size,
uploaded_file_name, ext, mime_type, thumb_id, video_thumb_id, attributes, version,
date2, import_document_id, deleted, sha256`

func scanDocument(row interface{ Scan(...any) error }) (*dataobject.DocumentsDO, error) {
	do := new(dataobject.DocumentsDO)
	var attributes []byte
	err := row.Scan(&do.Id, &do.DocumentId, &do.AccessHash, &do.DcId, &do.FilePath,
		&do.FileSize, &do.UploadedFileName, &do.Ext, &do.MimeType, &do.ThumbId,
		&do.VideoThumbId, &attributes, &do.Version, &do.Date2, &do.ImportDocumentId,
		&do.Deleted, &do.Sha256)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	do.Attributes = string(attributes)
	return do, nil
}

func scanDocumentRows(rows pgx.Rows) ([]dataobject.DocumentsDO, error) {
	defer rows.Close()
	result := make([]dataobject.DocumentsDO, 0)
	for rows.Next() {
		var do dataobject.DocumentsDO
		var attributes []byte
		if err := rows.Scan(&do.Id, &do.DocumentId, &do.AccessHash, &do.DcId, &do.FilePath,
			&do.FileSize, &do.UploadedFileName, &do.Ext, &do.MimeType, &do.ThumbId,
			&do.VideoThumbId, &attributes, &do.Version, &do.Date2, &do.ImportDocumentId,
			&do.Deleted, &do.Sha256); err != nil {
			return nil, err
		}
		do.Attributes = string(attributes)
		result = append(result, do)
	}
	return result, rows.Err()
}

func jsonbValue(value string) (any, error) {
	if value == "" {
		return nil, nil
	}
	var raw json.RawMessage
	if err := json.Unmarshal([]byte(value), &raw); err != nil {
		return nil, err
	}
	return raw, nil
}

const photoColumns = `id, photo_id, access_hash, has_stickers, dc_id, date2, has_video,
size_id, video_size_id, input_file_name, ext`

func scanPhoto(row interface{ Scan(...any) error }) (*dataobject.PhotosDO, error) {
	do := new(dataobject.PhotosDO)
	err := row.Scan(&do.Id, &do.PhotoId, &do.AccessHash, &do.HasStickers, &do.DcId,
		&do.Date2, &do.HasVideo, &do.SizeId, &do.VideoSizeId, &do.InputFileName, &do.Ext)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return do, nil
}

const photoSizeColumns = `id, photo_size_id, size_type, width, height, file_size,
file_path, cached_type, cached_bytes`

func scanPhotoSizeRows(rows pgx.Rows) ([]dataobject.PhotoSizesDO, error) {
	defer rows.Close()
	result := make([]dataobject.PhotoSizesDO, 0)
	for rows.Next() {
		var do dataobject.PhotoSizesDO
		if err := rows.Scan(&do.Id, &do.PhotoSizeId, &do.SizeType, &do.Width, &do.Height,
			&do.FileSize, &do.FilePath, &do.CachedType, &do.CachedBytes); err != nil {
			return nil, err
		}
		result = append(result, do)
	}
	return result, rows.Err()
}

const videoSizeColumns = `id, video_size_id, size_type, width, height, file_size,
video_start_ts, file_path`

func scanVideoSizeRows(rows pgx.Rows) ([]dataobject.VideoSizesDO, error) {
	defer rows.Close()
	result := make([]dataobject.VideoSizesDO, 0)
	for rows.Next() {
		var do dataobject.VideoSizesDO
		if err := rows.Scan(&do.Id, &do.VideoSizeId, &do.SizeType, &do.Width, &do.Height,
			&do.FileSize, &do.VideoStartTs, &do.FilePath); err != nil {
			return nil, err
		}
		result = append(result, do)
	}
	return result, rows.Err()
}
