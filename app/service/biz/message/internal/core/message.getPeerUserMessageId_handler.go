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

// MessageGetPeerUserMessageId
// message.getPeerUserMessageId user_id:long peer_user_id:long msg_id:int = Int32;
func (c *MessageCore) MessageGetPeerUserMessageId(in *message.TLMessageGetPeerUserMessageId) (*mtproto.Int32, error) {
	if in == nil || in.GetUserId() <= 0 || in.GetPeerUserId() <= 0 || in.GetMsgId() <= 0 {
		return nil, mtproto.ErrInputRequestInvalid
	}

	do, err := c.svcCtx.Dao.MessagesDAO.SelectPeerUserMessageId(
		c.ctx,
		in.GetPeerUserId(),
		in.GetUserId(),
		in.GetMsgId())
	if err != nil {
		c.Logger.Errorf("message.getPeerUserMessageId - error: %v", err)
		return nil, err
	}
	if do == nil || do.UserMessageBoxId <= 0 {
		return nil, mtproto.ErrMsgIdInvalid
	}

	return mtproto.MakeTLInt32(&mtproto.Int32{V: do.UserMessageBoxId}).To_Int32(), nil
}
