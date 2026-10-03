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

	"github.com/teamgram/marmota/pkg/hack"
	"github.com/teamgram/marmota/pkg/stores/sqlx"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/internal/dal/dataobject"

	"github.com/zeromicro/go-zero/core/jsonx"
)

// DialogSaveDraftMessage
// dialog.saveDraftMessage user_id:long peer_type:int peer_id:long message:DraftMessage = Bool;
func (c *DialogCore) DialogSaveDraftMessage(in *dialog.TLDialogSaveDraftMessage) (*mtproto.Bool, error) {
	draft, _ := jsonx.Marshal(in.Message)
	cacheKeys := []string{
		dialog.GetDialogCacheKeyByPeer(in.UserId, in.PeerType, in.PeerId),
		dialog.GetAllDraftIdListCacheKey(in.UserId),
	}
	if cacheKey := dialog.GetCacheKeyByPeerType(in.UserId, in.PeerType); cacheKey != "" {
		cacheKeys = append(cacheKeys, cacheKey)
	}

	_, _, err := c.svcCtx.Dao.CachedConn.Exec(
		c.ctx,
		func(ctx context.Context, conn *sqlx.DB) (int64, int64, error) {
			rowsAffected, err := c.svcCtx.Dao.DialogsDAO.SaveDraft(
				ctx,
				2,
				hack.String(draft),
				in.UserId,
				in.PeerType,
				in.PeerId)
			if err != nil || rowsAffected != 0 {
				return 0, rowsAffected, err
			}

			_, _, err = c.svcCtx.Dao.DialogsDAO.InsertIgnore(ctx, &dataobject.DialogsDO{
				UserId:           in.UserId,
				PeerType:         in.PeerType,
				PeerId:           in.PeerId,
				PeerDialogId:     mtproto.MakePeerDialogId(in.PeerType, in.PeerId),
				DraftMessageData: "null",
			})
			if err != nil {
				return 0, 0, err
			}

			rowsAffected, err = c.svcCtx.Dao.DialogsDAO.SaveDraft(
				ctx,
				2,
				hack.String(draft),
				in.UserId,
				in.PeerType,
				in.PeerId)
			return 0, rowsAffected, err
		},
		cacheKeys...)
	if err != nil {
		c.Logger.Errorf("dialog.saveDraftMessage - error: %v", err)
		return nil, err
	}

	return mtproto.BoolTrue, nil
}
