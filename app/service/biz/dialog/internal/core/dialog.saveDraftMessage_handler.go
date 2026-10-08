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
	"github.com/teamgram/marmota/pkg/hack"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/internal/dal/dataobject"

	"github.com/zeromicro/go-zero/core/jsonx"
)

// DialogSaveDraftMessage
// dialog.saveDraftMessage user_id:long peer_type:int peer_id:long message:DraftMessage = Bool;
func (c *DialogCore) DialogSaveDraftMessage(in *dialog.TLDialogSaveDraftMessage) (*mtproto.Bool, error) {
	draft, _ := jsonx.Marshal(in.Message)
	if c == nil || c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.Postgres == nil || c.svcCtx.Dao.Postgres.Store == nil {
		return nil, mtproto.ErrInternalServerError
	}
	tx, err := c.svcCtx.Dao.Postgres.Pool.Begin(c.ctx)
	if err == nil {
		defer func() { _ = tx.Rollback(c.ctx) }()
		rowsAffected, txErr := c.svcCtx.Dao.Postgres.Store.Dialogs.SaveDraftTx(c.ctx, tx, 2, hack.String(draft), in.UserId, in.PeerType, in.PeerId)
		if txErr == nil && rowsAffected == 0 {
			_, _, txErr = c.svcCtx.Dao.Postgres.Store.Dialogs.InsertIgnoreTx(c.ctx, tx, &dataobject.DialogsDO{
				UserId: in.UserId, PeerType: in.PeerType, PeerId: in.PeerId,
				PeerDialogId: mtproto.MakePeerDialogId(in.PeerType, in.PeerId), DraftMessageData: "null",
			})
			if txErr == nil {
				_, txErr = c.svcCtx.Dao.Postgres.Store.Dialogs.SaveDraftTx(c.ctx, tx, 2, hack.String(draft), in.UserId, in.PeerType, in.PeerId)
			}
		}
		err = txErr
		if err == nil {
			err = tx.Commit(c.ctx)
		}
	}
	if err != nil {
		c.Logger.Errorf("dialog.saveDraftMessage - error: %v", err)
		return nil, err
	}

	return mtproto.BoolTrue, nil
}
