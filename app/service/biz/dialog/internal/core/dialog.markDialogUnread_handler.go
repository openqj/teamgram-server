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

// DialogMarkDialogUnread
// dialog.markDialogUnread user_id:long peer_type:int peer_id:long unread_mark:Bool = Bool;
func (c *DialogCore) DialogMarkDialogUnread(in *dialog.TLDialogMarkDialogUnread) (*mtproto.Bool, error) {
	mark := 0
	if mtproto.FromBool(in.UnreadMark) {
		mark = 1
	}
	store, err := c.pgStore()
	if err != nil || store.Dialogs == nil {
		return nil, mtproto.ErrMethodNotImpl
	}
	affected, err := store.Dialogs.UpdateCustomMap(c.ctx,
		map[string]interface{}{"unread_mark": mark}, in.UserId, in.PeerType, in.PeerId)
	if err != nil {
		c.Logger.Errorf("dialog.markDialogUnread - error: %v", err)
		return nil, err
	}
	if affected == 0 {
		row, selErr := store.Dialogs.SelectDialog(c.ctx, in.UserId, in.PeerType, in.PeerId)
		if selErr != nil {
			c.Logger.Errorf("dialog.markDialogUnread - error: %v", selErr)
			return nil, selErr
		}
		if row == nil {
			err = mtproto.ErrPeerIdInvalid
			c.Logger.Errorf("dialog.markDialogUnread - error: %v", err)
			return nil, err
		}
	}
	return mtproto.BoolTrue, nil
}
