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

// MessageGetHistoryMessagesCount
// message.getHistoryMessagesCount user_id:long peer_type:int peer_id:long = Int32;
func (c *MessageCore) MessageGetHistoryMessagesCount(in *message.TLMessageGetHistoryMessagesCount) (*mtproto.Int32, error) {
	var (
		count    int64
		dialogId = mtproto.MakeDialogId(in.UserId, in.PeerType, in.PeerId)
	)

	switch in.PeerType {
	case mtproto.PEER_SELF, mtproto.PEER_USER, mtproto.PEER_CHAT:
		count, _ = c.svcCtx.Dao.CountMessageHistory(c.ctx, in.UserId, dialogId.A, dialogId.B)
	case mtproto.PEER_CHANNEL:
		// Channel history is owned by the native channel service.
		count = 0
	default:
		c.Logger.Errorf("invalid peer: (%d, %d, %d)", in.UserId, in.PeerType, in.PeerId)
	}

	return &mtproto.Int32{
		V: int32(count),
	}, nil
}
