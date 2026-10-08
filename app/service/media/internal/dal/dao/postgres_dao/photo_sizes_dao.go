package postgres_dao

import (
	"context"

	"github.com/teamgram/teamgram-server/app/service/media/internal/dal/dataobject"
)

type PhotoSizesDAO struct{ db DB }

func NewPhotoSizesDAO(db DB) *PhotoSizesDAO { return &PhotoSizesDAO{db: db} }

func (d *PhotoSizesDAO) Insert(ctx context.Context, do *dataobject.PhotoSizesDO) (int64, int64, error) {
	return d.insert(ctx, d.db, do)
}

func (d *PhotoSizesDAO) InsertOn(ctx context.Context, tx DB, do *dataobject.PhotoSizesDO) (int64, int64, error) {
	return d.insert(ctx, tx, do)
}

func (d *PhotoSizesDAO) InsertTx(ctx context.Context, tx DB, do *dataobject.PhotoSizesDO) (int64, int64, error) {
	return d.insert(ctx, tx, do)
}

func (d *PhotoSizesDAO) insert(ctx context.Context, db DB, do *dataobject.PhotoSizesDO) (int64, int64, error) {
	var id int64
	err := db.QueryRow(ctx, `INSERT INTO photo_sizes
 (photo_size_id, size_type, width, height, file_size, file_path, cached_type, cached_bytes)
 VALUES ($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id`,
		do.PhotoSizeId, do.SizeType, do.Width, do.Height, do.FileSize, do.FilePath,
		do.CachedType, do.CachedBytes).Scan(&id)
	if err != nil {
		return 0, 0, err
	}
	return id, 1, nil
}

func (d *PhotoSizesDAO) SelectListByPhotoSizeId(ctx context.Context, id int64) ([]dataobject.PhotoSizesDO, error) {
	rows, err := d.db.Query(ctx, `SELECT `+photoSizeColumns+` FROM photo_sizes WHERE photo_size_id = $1 ORDER BY id`, id)
	if err != nil {
		return nil, err
	}
	return scanPhotoSizeRows(rows)
}

func (d *PhotoSizesDAO) SelectListByPhotoSizeIdWithCB(ctx context.Context, id int64, cb func(int, int, *dataobject.PhotoSizesDO)) ([]dataobject.PhotoSizesDO, error) {
	list, err := d.SelectListByPhotoSizeId(ctx, id)
	if err != nil {
		return nil, err
	}
	if cb != nil {
		for i := range list {
			cb(len(list), i, &list[i])
		}
	}
	return list, nil
}

func (d *PhotoSizesDAO) SelectListByPhotoSizeIdList(ctx context.Context, ids []int64) ([]dataobject.PhotoSizesDO, error) {
	if len(ids) == 0 {
		return []dataobject.PhotoSizesDO{}, nil
	}
	rows, err := d.db.Query(ctx, `SELECT `+photoSizeColumns+` FROM photo_sizes WHERE photo_size_id = ANY($1::bigint[]) ORDER BY id`, ids)
	if err != nil {
		return nil, err
	}
	return scanPhotoSizeRows(rows)
}

func (d *PhotoSizesDAO) SelectListByPhotoSizeIdListWithCB(ctx context.Context, ids []int64, cb func(int, int, *dataobject.PhotoSizesDO)) ([]dataobject.PhotoSizesDO, error) {
	list, err := d.SelectListByPhotoSizeIdList(ctx, ids)
	if err != nil {
		return nil, err
	}
	if cb != nil {
		for i := range list {
			cb(len(list), i, &list[i])
		}
	}
	return list, nil
}
