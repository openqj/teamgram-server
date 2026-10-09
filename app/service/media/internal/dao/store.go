package dao

import (
	"context"

	"github.com/teamgram/teamgram-server/app/service/media/internal/dal/dataobject"
)

type documentStore interface {
	Insert(context.Context, *dataobject.DocumentsDO) (int64, int64, error)
	SelectByDocumentId(context.Context, int64) (*dataobject.DocumentsDO, error)
	SelectByHash(context.Context, []byte, int64, string) (*dataobject.DocumentsDO, error)
	SelectByDocumentIdListWithCB(context.Context, []int64, func(int, int, *dataobject.DocumentsDO)) ([]dataobject.DocumentsDO, error)
}

type photoStore interface {
	Insert(context.Context, *dataobject.PhotosDO) (int64, int64, error)
	SelectByPhotoId(context.Context, int64) (*dataobject.PhotosDO, error)
}

type photoSizeStore interface {
	Insert(context.Context, *dataobject.PhotoSizesDO) (int64, int64, error)
	SelectListByPhotoSizeId(context.Context, int64) ([]dataobject.PhotoSizesDO, error)
	SelectListByPhotoSizeIdList(context.Context, []int64) ([]dataobject.PhotoSizesDO, error)
	SelectListByPhotoSizeIdListWithCB(context.Context, []int64, func(int, int, *dataobject.PhotoSizesDO)) ([]dataobject.PhotoSizesDO, error)
	SelectListByPhotoSizeIdWithCB(context.Context, int64, func(int, int, *dataobject.PhotoSizesDO)) ([]dataobject.PhotoSizesDO, error)
}

type videoSizeStore interface {
	Insert(context.Context, *dataobject.VideoSizesDO) (int64, int64, error)
	SelectListByVideoSizeIdListWithCB(context.Context, []int64, func(int, int, *dataobject.VideoSizesDO)) ([]dataobject.VideoSizesDO, error)
	SelectListByVideoSizeIdWithCB(context.Context, int64, func(int, int, *dataobject.VideoSizesDO)) ([]dataobject.VideoSizesDO, error)
}

func (m *Dao) documentsStore() documentStore {
	if m.Postgres == nil {
		panic("media: PostgreSQL store is not initialized")
	}
	return m.Postgres.DocumentsDAO
}

func (m *Dao) photosStore() photoStore {
	if m.Postgres == nil {
		panic("media: PostgreSQL store is not initialized")
	}
	return m.Postgres.PhotosDAO
}

func (m *Dao) photoSizesStore() photoSizeStore {
	if m.Postgres == nil {
		panic("media: PostgreSQL store is not initialized")
	}
	return m.Postgres.PhotoSizesDAO
}

func (m *Dao) videoSizesStore() videoSizeStore {
	if m.Postgres == nil {
		panic("media: PostgreSQL store is not initialized")
	}
	return m.Postgres.VideoSizesDAO
}
