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
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/internal/dal/dataobject"
)

// DialogGetUserPinnedMessage
// dialog.getUserPinnedMessage user_id:long peer_type:int peer_id:long = Int32;
func (c *DialogCore) DialogGetUserPinnedMessage(in *dialog.TLDialogGetUserPinnedMessage) (*mtproto.Int32, error) {
	dlg, err := c.svcCtx.Dao.DialogsDAO.SelectDialog(c.ctx, in.GetUserId(), in.GetPeerType(), in.GetPeerId())
	if err != nil {
		c.Logger.Errorf("dialog.getUserPinnedMessage - error: %v", err)
		return nil, err
	}

	if dlg == nil {
		return pinnedMessageValue(nil), nil
	}
	return pinnedMessageValue(dlg), nil
}

func pinnedMessageValue(dlg *dataobject.DialogsDO) *mtproto.Int32 {
	if dlg == nil {
		return &mtproto.Int32{}
	}
	return &mtproto.Int32{V: dlg.PinnedMsgId}
}
