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
	"sort"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
)

// DialogEditPeerFolders
// dialog.editPeerFolders user_id:long peer_dialog_list:Vector<long> folder_id:int = Vector<DialogPinnedExt>;
func (c *DialogCore) DialogEditPeerFolders(in *dialog.TLDialogEditPeerFolders) (*dialog.Vector_DialogPinnedExt, error) {
	store, err := c.pgStore()
	if err != nil || store.Dialogs == nil {
		return nil, mtproto.ErrMethodNotImpl
	}
	var (
		dialogPinnedList dialog.DialogPinnedExtList
	)

	rows, err := store.Dialogs.SelectPeerDialogList(c.ctx, in.UserId, in.PeerDialogList)
	for i := range rows {
		v := &rows[i]
		if in.FolderId == 0 {
			if v.Pinned > 0 {
				dialogPinnedList = append(dialogPinnedList, &dialog.DialogPinnedExt{
					Order:    v.Pinned,
					PeerType: v.PeerType,
					PeerId:   v.PeerId,
				})
			}
		} else {
			if v.FolderPinned > 0 {
				dialogPinnedList = append(dialogPinnedList, &dialog.DialogPinnedExt{
					Order:    v.FolderPinned,
					PeerType: v.PeerType,
					PeerId:   v.PeerId,
				})
			}
		}
	}
	if err != nil {
		c.Logger.Errorf("dialog.editPeerFolders - select peers error: %v", err)
		return nil, err
	}

	if len(dialogPinnedList) > 0 {
		if in.FolderId == 0 {
			pinnedRows, e := store.Dialogs.SelectPinnedDialogs(c.ctx, in.UserId)
			err = e
			for i := range pinnedRows {
				v := &pinnedRows[i]
				dialogPinnedList = append(dialogPinnedList, &dialog.DialogPinnedExt{
					Order:    v.FolderPinned,
					PeerType: v.PeerType,
					PeerId:   v.PeerId,
				})
			}
		} else {
			pinnedRows, e := store.Dialogs.SelectFolderPinnedDialogsWithCB(c.ctx,
				in.UserId, in.FolderId, nil)
			err = e
			for i := range pinnedRows {
				v := &pinnedRows[i]
				dialogPinnedList = append(dialogPinnedList, &dialog.DialogPinnedExt{
					Order:    v.FolderPinned,
					PeerType: v.PeerType,
					PeerId:   v.PeerId,
				})
			}
		}
		if err != nil {
			c.Logger.Errorf("dialog.editPeerFolders - select pinned peers error: %v", err)
			return nil, err
		}
	}

	sd := sort.Reverse(dialogPinnedList)
	sort.Sort(sd)

	// update
	if _, err = store.Dialogs.UpdatePeerDialogListFolderId(c.ctx, in.FolderId, in.UserId, in.PeerDialogList); err != nil {
		c.Logger.Errorf("dialog.editPeerFolders - update folder error: %v", err)
		return nil, err
	}

	if in.FolderId == 0 {
		// cut
		if len(dialogPinnedList) > 5 {
			unpinnedList := make([]int64, 0, len(dialogPinnedList)-5)
			for i := 5; i < len(dialogPinnedList); i++ {
				unpinnedList = append(unpinnedList, mtproto.MakePeerDialogId(dialogPinnedList[i].PeerType, dialogPinnedList[i].PeerId))
			}

			//
			if _, err = store.Dialogs.UpdatePeerDialogListPinned(c.ctx, 0, in.UserId, unpinnedList); err != nil {
				c.Logger.Errorf("dialog.editPeerFolders - unpin peers error: %v", err)
				return nil, err
			}
			dialogPinnedList = dialogPinnedList[:5]
		}
	}

	return &dialog.Vector_DialogPinnedExt{
		Datas: dialogPinnedList,
	}, nil
}
