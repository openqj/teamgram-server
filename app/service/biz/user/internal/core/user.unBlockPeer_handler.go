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
	"github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

// UserUnBlockPeer
// user.unBlockPeer user_id:long peer_type:int peer_id:long = Bool;
func (c *UserCore) UserUnBlockPeer(in *user.TLUserUnBlockPeer) (*mtproto.Bool, error) {
	if in.GetPeerType() != mtproto.PEER_USER || in.GetPeerId() <= 0 {
		return nil, mtproto.ErrPeerIdInvalid
	}
	if err := c.svcCtx.Dao.UnBlockUser(c.ctx, in.GetUserId(), in.GetPeerId()); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}
