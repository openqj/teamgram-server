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

// MessageGetUnreadMentionsCount
// message.getUnreadMentionsCount user_id:long peer_type:int peer_id:long = Int32;
func (c *MessageCore) MessageGetUnreadMentionsCount(in *message.TLMessageGetUnreadMentionsCount) (*mtproto.Int32, error) {
	var (
		sz int64
	)

	switch in.PeerType {
	case mtproto.PEER_CHAT:
		sz, _ = c.svcCtx.Dao.CountUnreadMentions(c.ctx, in.UserId, mtproto.PEER_CHAT, in.PeerId)
	case mtproto.PEER_CHANNEL:
		sz = 0
	default:
		// TODO: log
	}

	return &mtproto.Int32{
		V: int32(sz),
	}, nil
}
