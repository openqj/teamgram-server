package postgres_dao

import (
	"context"

	"github.com/teamgram/teamgram-server/app/service/media/internal/dal/dataobject"
)

type PhotosDAO struct{ db DB }

func NewPhotosDAO(db DB) *PhotosDAO { return &PhotosDAO{db: db} }

func (d *PhotosDAO) Insert(ctx context.Context, do *dataobject.PhotosDO) (int64, int64, error) {
	return d.insert(ctx, d.db, do)
}

func (d *PhotosDAO) InsertOn(ctx context.Context, tx DB, do *dataobject.PhotosDO) (int64, int64, error) {
	return d.insert(ctx, tx, do)
}

func (d *PhotosDAO) InsertTx(ctx context.Context, tx DB, do *dataobject.PhotosDO) (int64, int64, error) {
	return d.insert(ctx, tx, do)
}

func (d *PhotosDAO) insert(ctx context.Context, db DB, do *dataobject.PhotosDO) (int64, int64, error) {
	var id int64
	err := db.QueryRow(ctx, `INSERT INTO photos
 (photo_id, access_hash, has_stickers, dc_id, date2, has_video, size_id, video_size_id,
  input_file_name, ext)
 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING id`,
		do.PhotoId, do.AccessHash, do.HasStickers, do.DcId, do.Date2, do.HasVideo,
		do.SizeId, do.VideoSizeId, do.InputFileName, do.Ext).Scan(&id)
	if err != nil {
		return 0, 0, err
	}
	return id, 1, nil
}

func (d *PhotosDAO) SelectByPhotoId(ctx context.Context, id int64) (*dataobject.PhotosDO, error) {
	return scanPhoto(d.db.QueryRow(ctx, `SELECT `+photoColumns+` FROM photos WHERE photo_id = $1 LIMIT 1`, id))
}
