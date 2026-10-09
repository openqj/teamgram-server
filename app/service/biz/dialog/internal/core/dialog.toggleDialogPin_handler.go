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
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
)

// DialogToggleDialogPin
// dialog.toggleDialogPin user_id:long peer_type:int peer_id:long pinned:Bool = Int32;
func (c *DialogCore) DialogToggleDialogPin(in *dialog.TLDialogToggleDialogPin) (*mtproto.Int32, error) {
	var (
		peerDialogId = mtproto.MakePeerDialogId(in.PeerType, in.PeerId)
		pinned       int64
	)

	store, err := c.pgStore()
	if err != nil || store.Dialogs == nil {
		return nil, mtproto.ErrMethodNotImpl
	}
	dlgExt, err := c.svcCtx.Dao.GetDialogByPeerDialogId(c.ctx, in.GetUserId(), peerDialogId)
	if err != nil {
		c.Logger.Errorf("dialog.toggleDialogPin - error: %v", err)
		return nil, err
	}

	folderId := dlgExt.GetDialog().GetFolderId().GetValue()

	if mtproto.FromBool(in.Pinned) {
		pinned = time.Now().Unix() << 32
	} else {
		pinned = 0
	}

	if folderId == 0 {
		_, err = store.Dialogs.UpdatePeerDialogListPinned(c.ctx, pinned, in.UserId, []int64{peerDialogId})
	} else {
		_, err = store.Dialogs.UpdateFolderPeerDialogListPinned(c.ctx, pinned, in.UserId, []int64{peerDialogId})
	}
	if err != nil {
		return nil, err
	}

	return &mtproto.Int32{
		V: folderId,
	}, nil
}
