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

	"github.com/jackc/pgx/v5"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
)

// DialogReorderPinnedDialogs
// dialog.reorderPinnedDialogs user_id:long force:Bool folder_id:int id_list:Vector<long> = Bool;
func (c *DialogCore) DialogReorderPinnedDialogs(in *dialog.TLDialogReorderPinnedDialogs) (*mtproto.Bool, error) {
	store, err := c.pgStore()
	if err != nil || store.Dialogs == nil || c.svcCtx.Dao.Postgres == nil {
		return nil, mtproto.ErrMethodNotImpl
	}
	var (
		userId      = in.GetUserId()
		force       = mtproto.FromBool(in.GetForce())
		folderId    = in.GetFolderId()
		idList      = in.GetIdList()
		orderPinned = time.Now().Unix()
	)
	err = c.svcCtx.Dao.Postgres.InTx(c.ctx, func(tx pgx.Tx) error {
		ids := idList
		if len(ids) == 0 {
			ids = []int64{0}
		}
		if force {
			if folderId == 0 {
				if _, err := store.Dialogs.UpdateUnPinnedNotIdListTx(c.ctx, tx, userId, ids); err != nil {
					return err
				}
			} else if _, err := store.Dialogs.UpdateFolderUnPinnedNotIdListTx(c.ctx, tx, userId, ids); err != nil {
				return err
			}
		}
		for _, id := range idList {
			pinned := orderPinned << 32
			if folderId == 0 {
				if _, err := store.Dialogs.UpdatePeerDialogListPinnedTx(c.ctx, tx, pinned, userId, []int64{id}); err != nil {
					return err
				}
			} else if _, err := store.Dialogs.UpdateFolderPeerDialogListPinnedTx(c.ctx, tx, pinned, userId, []int64{id}); err != nil {
				return err
			}
			orderPinned--
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return mtproto.BoolTrue, nil
}
