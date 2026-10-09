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
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
	"time"

	"github.com/zeromicro/go-zero/core/jsonx"
)

func getEmptyDraftMessage() string {
	draftData, _ := jsonx.Marshal(
		mtproto.MakeTLDraftMessageEmpty(&mtproto.DraftMessage{
			Date_FLAGINT32: mtproto.MakeFlagsInt32(int32(time.Now().Unix())),
		}).To_DraftMessage())
	return string(draftData)
}

// DialogClearDraftMessage
// dialog.clearDraftMessage user_id:long peer_type:int peer_id:long = Bool;
func (c *DialogCore) DialogClearDraftMessage(in *dialog.TLDialogClearDraftMessage) (*mtproto.Bool, error) {
	if c == nil || c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.Postgres == nil ||
		c.svcCtx.Dao.Postgres.Store == nil || c.svcCtx.Dao.Postgres.Store.Dialogs == nil {
		return nil, mtproto.ErrMethodNotImpl
	}
	dlgDO, err := c.svcCtx.Dao.Postgres.Store.Dialogs.SelectDialog(
		c.ctx,
		in.UserId,
		in.PeerType,
		in.PeerId)
	if err != nil {
		c.Logger.Errorf("dialog.clearDraftMessage - error: %v", err)
		return nil, err
	}

	if dlgDO != nil && dlgDO.DraftType == 2 {
		_, err = c.svcCtx.Dao.Postgres.Store.Dialogs.SaveDraft(
			c.ctx,
			1,
			getEmptyDraftMessage(),
			in.UserId,
			in.PeerType,
			in.PeerId)
		if err != nil {
			c.Logger.Errorf("dialog.clearDraftMessage - error: %v", err)
			return nil, err
		}
		return mtproto.BoolTrue, nil
	} else {
		return mtproto.BoolFalse, nil
	}
}
