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

// DialogUpdateUserPinnedMessage
// dialog.updateUserPinnedMessage user_id:long peer_type:int peer_id:long pinned_msg_id:int = Bool;
func (c *DialogCore) DialogUpdateUserPinnedMessage(in *dialog.TLDialogUpdateUserPinnedMessage) (*mtproto.Bool, error) {
	if in == nil || in.GetUserId() <= 0 || in.GetPeerId() <= 0 || in.GetPinnedMsgId() < 0 {
		return nil, mtproto.ErrInputRequestInvalid
	}
	switch in.GetPeerType() {
	case mtproto.PEER_SELF, mtproto.PEER_USER, mtproto.PEER_CHAT, mtproto.PEER_CHANNEL:
	default:
		return nil, mtproto.ErrPeerIdInvalid
	}

	peerDialogId := mtproto.MakePeerDialogId(in.GetPeerType(), in.GetPeerId())
	store, err := c.pgStore()
	if err != nil || store.Dialogs == nil {
		return nil, mtproto.ErrMethodNotImpl
	}
	_, err = store.Dialogs.UpdatePinnedMsgId(c.ctx, in.GetPinnedMsgId(), in.GetUserId(), peerDialogId)
	if err != nil {
		c.Logger.Errorf("dialog.updateUserPinnedMessage - error: %v", err)
		return nil, err
	}

	return mtproto.BoolTrue, nil
}
