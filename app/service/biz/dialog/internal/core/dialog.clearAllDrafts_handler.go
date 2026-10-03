/*
 * Created from 'scheme.tl' by 'mtprotoc'
 *
 * Copyright (c) 2021-present,  Teamgram Studio (https://teamgram.io).
 *  All rights reserved.
 *
 * Author: teamgramio (teamgram.io@gmail.com)
 */

package core

import (
	"context"
	"time"

	"github.com/teamgram/marmota/pkg/stores/sqlx"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/internal/dal/dataobject"
)

// DialogClearAllDrafts
// dialog.clearAllDrafts user_id:long = Vector<PeerWithDraftMessage>;
func (c *DialogCore) DialogClearAllDrafts(in *dialog.TLDialogClearAllDrafts) (*dialog.Vector_PeerWithDraftMessage, error) {
	var (
		err error

		rValues = &dialog.Vector_PeerWithDraftMessage{
			Datas: []*dialog.PeerWithDraftMessage{},
		}
		cacheKeys = []string{dialog.GetAllDraftIdListCacheKey(in.UserId)}
	)

	if _, err = c.svcCtx.Dao.DialogsDAO.SelectAllDraftsWithCB(
		c.ctx,
		in.UserId,
		func(sz, i int, v *dataobject.DialogsDO) {
			cacheKeys = append(cacheKeys, dialog.GetDialogCacheKeyByPeer(in.UserId, v.PeerType, v.PeerId))
			rValues.Datas = append(rValues.Datas,
				dialog.MakeTLUpdateDraftMessage(&dialog.PeerWithDraftMessage{
					Peer: mtproto.MakePeer(v.PeerType, v.PeerId),
					Draft: mtproto.MakeTLDraftMessageEmpty(&mtproto.DraftMessage{
						Date_FLAGINT32: mtproto.MakeFlagsInt32(int32(time.Now().Unix())),
					}).To_DraftMessage(),
				}).To_PeerWithDraftMessage())
		}); err != nil {
		c.Logger.Errorf("dialog.getAllDrafts - error: %v", err)
		return nil, err
	}

	if len(rValues.Datas) > 0 {
		_, _, err = c.svcCtx.Dao.CachedConn.Exec(
			c.ctx,
			func(ctx context.Context, conn *sqlx.DB) (int64, int64, error) {
				rowsAffected, err := c.svcCtx.Dao.DialogsDAO.ClearAllDrafts(ctx, in.UserId)
				return 0, rowsAffected, err
			},
			cacheKeys...)
		if err != nil {
			c.Logger.Errorf("dialog.clearAllDrafts - error: %v", err)
			return nil, err
		}
	}

	return rValues, nil
}
