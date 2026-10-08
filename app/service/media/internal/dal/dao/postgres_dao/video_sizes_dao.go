package postgres_dao

import (
	"context"

	"github.com/teamgram/teamgram-server/app/service/media/internal/dal/dataobject"
)

type VideoSizesDAO struct{ db DB }

func NewVideoSizesDAO(db DB) *VideoSizesDAO { return &VideoSizesDAO{db: db} }

func (d *VideoSizesDAO) Insert(ctx context.Context, do *dataobject.VideoSizesDO) (int64, int64, error) {
	return d.insert(ctx, d.db, do)
}

func (d *VideoSizesDAO) InsertOn(ctx context.Context, tx DB, do *dataobject.VideoSizesDO) (int64, int64, error) {
	return d.insert(ctx, tx, do)
}

func (d *VideoSizesDAO) InsertTx(ctx context.Context, tx DB, do *dataobject.VideoSizesDO) (int64, int64, error) {
	return d.insert(ctx, tx, do)
}

func (d *VideoSizesDAO) insert(ctx context.Context, db DB, do *dataobject.VideoSizesDO) (int64, int64, error) {
	var id int64
	err := db.QueryRow(ctx, `INSERT INTO video_sizes
 (video_size_id, size_type, width, height, file_size, video_start_ts, file_path)
 VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING id`,
		do.VideoSizeId, do.SizeType, do.Width, do.Height, do.FileSize, do.VideoStartTs,
		do.FilePath).Scan(&id)
	if err != nil {
		return 0, 0, err
	}
	return id, 1, nil
}

func (d *VideoSizesDAO) SelectListByVideoSizeId(ctx context.Context, id int64) ([]dataobject.VideoSizesDO, error) {
	rows, err := d.db.Query(ctx, `SELECT `+videoSizeColumns+` FROM video_sizes WHERE video_size_id = $1 ORDER BY id`, id)
	if err != nil {
		return nil, err
	}
	return scanVideoSizeRows(rows)
}

func (d *VideoSizesDAO) SelectListByVideoSizeIdWithCB(ctx context.Context, id int64, cb func(int, int, *dataobject.VideoSizesDO)) ([]dataobject.VideoSizesDO, error) {
	list, err := d.SelectListByVideoSizeId(ctx, id)
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

func (d *VideoSizesDAO) SelectListByVideoSizeIdList(ctx context.Context, ids []int64) ([]dataobject.VideoSizesDO, error) {
	if len(ids) == 0 {
		return []dataobject.VideoSizesDO{}, nil
	}
	rows, err := d.db.Query(ctx, `SELECT `+videoSizeColumns+` FROM video_sizes WHERE video_size_id = ANY($1::bigint[]) ORDER BY id`, ids)
	if err != nil {
		return nil, err
	}
	return scanVideoSizeRows(rows)
}

func (d *VideoSizesDAO) SelectListByVideoSizeIdListWithCB(ctx context.Context, ids []int64, cb func(int, int, *dataobject.VideoSizesDO)) ([]dataobject.VideoSizesDO, error) {
	list, err := d.SelectListByVideoSizeIdList(ctx, ids)
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
