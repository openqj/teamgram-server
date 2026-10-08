package postgres_dao

import (
	"context"

	"github.com/teamgram/teamgram-server/app/service/media/internal/dal/dataobject"
)

type DocumentsDAO struct{ db DB }

func NewDocumentsDAO(db DB) *DocumentsDAO { return &DocumentsDAO{db: db} }

func (d *DocumentsDAO) Insert(ctx context.Context, do *dataobject.DocumentsDO) (int64, int64, error) {
	return d.insert(ctx, d.db, do)
}

func (d *DocumentsDAO) InsertOn(ctx context.Context, tx DB, do *dataobject.DocumentsDO) (int64, int64, error) {
	return d.insert(ctx, tx, do)
}

// InsertTx keeps the generated DAO naming available to migration callers.
func (d *DocumentsDAO) InsertTx(ctx context.Context, tx DB, do *dataobject.DocumentsDO) (int64, int64, error) {
	return d.insert(ctx, tx, do)
}

func (d *DocumentsDAO) insert(ctx context.Context, db DB, do *dataobject.DocumentsDO) (int64, int64, error) {
	var id int64
	attributes, err := jsonbValue(do.Attributes)
	if err != nil {
		return 0, 0, err
	}
	err = db.QueryRow(ctx, `INSERT INTO documents
 (document_id, access_hash, dc_id, file_path, file_size, uploaded_file_name, ext,
  mime_type, thumb_id, video_thumb_id, attributes, date2)
 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,COALESCE($11::jsonb, '{}'::jsonb),$12) RETURNING id`,
		do.DocumentId, do.AccessHash, do.DcId, do.FilePath, do.FileSize,
		do.UploadedFileName, do.Ext, do.MimeType, do.ThumbId, do.VideoThumbId,
		attributes, do.Date2).Scan(&id)
	if err != nil {
		return 0, 0, err
	}
	return id, 1, nil
}

func (d *DocumentsDAO) SelectByDocumentId(ctx context.Context, id int64) (*dataobject.DocumentsDO, error) {
	return scanDocument(d.db.QueryRow(ctx, `SELECT `+documentColumns+` FROM documents WHERE document_id = $1`, id))
}

func (d *DocumentsDAO) SelectByDocumentIdList(ctx context.Context, ids []int64) ([]dataobject.DocumentsDO, error) {
	if len(ids) == 0 {
		return []dataobject.DocumentsDO{}, nil
	}
	rows, err := d.db.Query(ctx, `SELECT `+documentColumns+` FROM documents WHERE document_id = ANY($1::bigint[]) ORDER BY id`, ids)
	if err != nil {
		return nil, err
	}
	return scanDocumentRows(rows)
}

func (d *DocumentsDAO) SelectByDocumentIdListWithCB(ctx context.Context, ids []int64, cb func(int, int, *dataobject.DocumentsDO)) ([]dataobject.DocumentsDO, error) {
	list, err := d.SelectByDocumentIdList(ctx, ids)
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

func (d *DocumentsDAO) SelectByIdList(ctx context.Context, ids []int64) ([]dataobject.DocumentsDO, error) {
	if len(ids) == 0 {
		return []dataobject.DocumentsDO{}, nil
	}
	rows, err := d.db.Query(ctx, `SELECT `+documentColumns+` FROM documents WHERE id = ANY($1::bigint[]) ORDER BY id`, ids)
	if err != nil {
		return nil, err
	}
	return scanDocumentRows(rows)
}

func (d *DocumentsDAO) SelectByIdListWithCB(ctx context.Context, ids []int64, cb func(int, int, *dataobject.DocumentsDO)) ([]dataobject.DocumentsDO, error) {
	list, err := d.SelectByIdList(ctx, ids)
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
