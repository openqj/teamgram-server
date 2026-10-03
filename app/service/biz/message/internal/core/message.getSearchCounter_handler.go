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

// MessageGetSearchCounter
// message.getSearchCounter user_id:long peer_type:int peer_id:long media_type:int = Int32;
func (c *MessageCore) MessageGetSearchCounter(in *message.TLMessageGetSearchCounter) (*mtproto.Int32, error) {
	dialogID := mtproto.MakeDialogId(in.UserId, in.PeerType, in.PeerId)
	count, err := c.svcCtx.Dao.MessagesDAO.CountByMediaType(c.ctx, in.UserId, dialogID.A, dialogID.B, in.MediaType)
	if err != nil {
		return nil, err
	}

	return &mtproto.Int32{
		V: count,
	}, nil
}
