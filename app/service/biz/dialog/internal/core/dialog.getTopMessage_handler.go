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
)

// DialogGetTopMessage
// dialog.getTopMessage user_id:long peer_type:int peer_id:long = Int32;
func (c *DialogCore) DialogGetTopMessage(in *dialog.TLDialogGetTopMessage) (*mtproto.Int32, error) {
	if in == nil || in.GetUserId() <= 0 || in.GetPeerId() <= 0 {
		return nil, mtproto.ErrInputRequestInvalid
	}
	switch in.GetPeerType() {
	case mtproto.PEER_SELF, mtproto.PEER_USER, mtproto.PEER_CHAT, mtproto.PEER_CHANNEL:
	default:
		return nil, mtproto.ErrPeerIdInvalid
	}
	if c == nil || c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.Postgres == nil || c.svcCtx.Dao.Postgres.Store == nil || c.svcCtx.Dao.Postgres.Store.Dialogs == nil {
		return nil, mtproto.ErrMethodNotImpl
	}

	topMessage, err := c.svcCtx.Dao.Postgres.Store.Dialogs.SelectDialog(c.ctx, in.UserId, in.PeerType, in.PeerId)
	if err != nil {
		if c.Logger != nil {
			c.Logger.Errorf("dialog.getTopMessage - error: %v", err)
		}
		return nil, err
	}

	if topMessage == nil {
		return &mtproto.Int32{V: 0}, nil
	}
	return &mtproto.Int32{V: topMessage.TopMessage}, nil
}
