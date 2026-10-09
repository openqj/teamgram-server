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

// DialogGetDialogFolder
// dialog.getDialogFolder user_id:long folder_id:int = Vector<DialogExt>;
func (c *DialogCore) DialogGetDialogFolder(in *dialog.TLDialogGetDialogFolder) (*dialog.Vector_DialogExt, error) {
	if c == nil || c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.Postgres == nil || c.svcCtx.Dao.Postgres.Store == nil || c.svcCtx.Dao.Postgres.Store.Dialogs == nil {
		return nil, mtproto.ErrMethodNotImpl
	}
	var (
		folderId  int32 = 1 // NOTE: force set folder_id = 1
		meId            = in.GetUserId()
		dialogExt *dialog.DialogExt
	)

	rows, err := c.svcCtx.Dao.Postgres.Store.Dialogs.SelectDialogs(c.ctx, meId, folderId)
	if err != nil {
		return nil, err
	}
	for i := range rows {
		v := &rows[i]
		if i == 0 {
			dialogExt = &dialog.DialogExt{
				Dialog: mtproto.MakeTLDialogFolder(&mtproto.Dialog{
					Pinned: false,
					Folder: mtproto.MakeTLFolder(&mtproto.Folder{
						AutofillNewBroadcasts:     false,
						AutofillPublicGroups:      false,
						AutofillNewCorrespondents: false,
						Id:                        folderId,
						Title:                     "Archived Chats",
						Photo:                     nil,
					}).To_Folder(),
					Peer:                       nil,
					TopMessage:                 0,
					UnreadMutedPeersCount:      0,
					UnreadUnmutedPeersCount:    0,
					UnreadMutedMessagesCount:   0,
					UnreadUnmutedMessagesCount: 0,
				}).To_Dialog(),
				Order: -1,
			}
		}

		order := v.FolderPinned
		if order == 0 {
			order = int64(v.TopMessage)
		}
		if order > dialogExt.Order {
			dialogExt.Order = order
			dialogExt.Dialog.Peer = mtproto.MakePeer(v.PeerType, v.PeerId)
			dialogExt.Dialog.TopMessage = v.TopMessage
		}
		if v.UnreadCount > 0 {
			dialogExt.Dialog.UnreadMutedPeersCount += 1
			dialogExt.Dialog.UnreadMutedMessagesCount += v.UnreadCount
		} else if v.UnreadMark == true {
			// if unread_mark then 1
			dialogExt.Dialog.UnreadMutedPeersCount += 1
			dialogExt.Dialog.UnreadMutedMessagesCount += 1
		}
	}

	if dialogExt == nil {
		return &dialog.Vector_DialogExt{
			Datas: []*dialog.DialogExt{},
		}, nil
	} else {
		return &dialog.Vector_DialogExt{
			Datas: []*dialog.DialogExt{dialogExt},
		}, nil
	}
}
