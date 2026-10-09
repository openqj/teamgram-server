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
	"github.com/teamgram/teamgram-server/app/service/biz/updates/updates"
)

// UpdatesGetStateV2
// updates.getStateV2 auth_key_id:long user_id:long = updates.State;
func (c *UpdatesCore) UpdatesGetStateV2(in *updates.TLUpdatesGetStateV2) (*mtproto.Updates_State, error) {
	if in == nil || in.UserId <= 0 {
		return nil, mtproto.ErrInputRequestInvalid
	}
	state, err := c.svcCtx.Dao.CurrentUpdateState(c.ctx, in.UserId, in.AuthKeyId)
	if err != nil {
		return nil, err
	}
	if state.Seq == 0 {
		state.Seq = -1
	}
	return state, nil
}
