package core

import (
	"context"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/internal/dal/dao/postgres_dao"
)

// pgStore returns the authoritative dialog persistence boundary. Production
// dialog handlers must not fall back to the generated MySQL DAO.
func (c *DialogCore) pgStore() (*postgres_dao.Store, error) {
	if c == nil || c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.Postgres == nil ||
		c.svcCtx.Dao.Postgres.Store == nil {
		return nil, mtproto.ErrMethodNotImpl
	}
	return c.svcCtx.Dao.Postgres.Store, nil
}

// loadDialogFilterExtList reads the authoritative PostgreSQL filter rows and
// converts their JSON payloads to the public TL representation.
func (c *DialogCore) loadDialogFilterExtList(ctx context.Context, userID int64) ([]*dialog.DialogFilterExt, error) {
	store, err := c.pgStore()
	if err != nil || store.DialogFilters == nil {
		return nil, mtproto.ErrMethodNotImpl
	}
	rows, err := store.DialogFilters.SelectList(ctx, userID)
	if err != nil {
		return nil, err
	}
	result := make([]*dialog.DialogFilterExt, 0, len(rows))
	for i := range rows {
		row := &rows[i]
		filter, err := mtproto.UnmarshalDialogFilter(row.DialogFilter)
		if err != nil {
			if c.Logger != nil {
				c.Logger.Errorf("dialog filter %d decode: %v", row.DialogFilterId, err)
			}
			return nil, err
		}
		result = append(result, &dialog.DialogFilterExt{
			Id:           row.DialogFilterId,
			JoinedBySlug: row.JoinedBySlug,
			Slug:         row.Slug,
			DialogFilter: filter,
			Order:        row.OrderValue,
		})
	}
	return result, nil
}
