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
	"github.com/teamgram/teamgram-server/app/service/biz/message/message"
)

// MessageGetPinnedMessageIdList
// message.getPinnedMessageIdList user_id:long peer_type:int peer_id:long = Vector<int>;
func (c *MessageCore) MessageGetPinnedMessageIdList(in *message.TLMessageGetPinnedMessageIdList) (*message.Vector_Int, error) {
	if in == nil || in.GetUserId() <= 0 || in.GetPeerId() <= 0 {
		return nil, mtproto.ErrInputRequestInvalid
	}

	switch in.GetPeerType() {
	case mtproto.PEER_SELF, mtproto.PEER_USER, mtproto.PEER_CHAT:
		peerDialogId := mtproto.MakeDialogId(in.GetUserId(), in.GetPeerType(), in.GetPeerId())
		idList, err := c.svcCtx.Dao.MessagesDAO.SelectPinnedMessageIdList(
			c.ctx,
			in.GetUserId(),
			peerDialogId.A,
			peerDialogId.B)
		if err != nil {
			c.Logger.Errorf("message.getPinnedMessageIdList - error: %v", err)
			return nil, err
		}
		if idList == nil {
			idList = make([]int32, 0)
		}
		return &message.Vector_Int{Datas: idList}, nil
	case mtproto.PEER_CHANNEL:
		// Channel messages use the native channel store rather than this
		// per-user messages table; do not report an empty successful result.
		return nil, mtproto.ErrMethodNotImpl
	default:
		return nil, mtproto.ErrPeerIdInvalid
	}
}
