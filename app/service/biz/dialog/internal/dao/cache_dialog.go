// Copyright 2024 Teamgram Authors
//  All rights reserved.
//
// Author: Benqi (wubenqi@gmail.com)

package dao

import (
	"context"

	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
)

func (d *Dao) GetNoCachePinnedDialogIdList(ctx context.Context, userID int64) ([]int64, error) {
	rows, err := d.Postgres.Store.Dialogs.SelectPinnedDialogs(ctx, userID)
	if err != nil {
		return nil, err
	}
	ids := make([]int64, 0, len(rows))
	for i := range rows {
		ids = append(ids, rows[i].PeerDialogId)
	}
	return ids, nil
}

func (d *Dao) GetPinnedDialogIdList(ctx context.Context, userID int64) ([]int64, error) {
	return d.GetNoCachePinnedDialogIdList(ctx, userID)
}

func (d *Dao) GetNoCacheNotPinnedDialogIdList(ctx context.Context, userID int64) ([]int64, error) {
	rows, err := d.Postgres.Store.Dialogs.SelectExcludePinnedDialogs(ctx, userID)
	if err != nil {
		return nil, err
	}
	ids := make([]int64, 0, len(rows))
	for i := range rows {
		ids = append(ids, rows[i].PeerDialogId)
	}
	return ids, nil
}

func (d *Dao) GetNotPinnedDialogIdList(ctx context.Context, userID int64) ([]int64, error) {
	return d.GetNoCacheNotPinnedDialogIdList(ctx, userID)
}

func (d *Dao) GetNoCacheFolderPinnedDialogIdList(ctx context.Context, userID int64, folderID int32) ([]int64, error) {
	rows, err := d.Postgres.Store.Dialogs.SelectFolderPinnedDialogsWithCB(ctx, userID, folderID, nil)
	if err != nil {
		return nil, err
	}
	ids := make([]int64, 0, len(rows))
	for i := range rows {
		ids = append(ids, rows[i].PeerDialogId)
	}
	return ids, nil
}

func (d *Dao) GetFolderPinnedDialogIdList(ctx context.Context, userID int64) ([]int64, error) {
	return d.GetNoCacheFolderPinnedDialogIdList(ctx, userID, 1)
}

func (d *Dao) GetNoCacheFolderNotPinnedDialogIdList(ctx context.Context, userID int64, folderID int32) ([]int64, error) {
	rows, err := d.Postgres.Store.Dialogs.SelectExcludeFolderPinnedDialogsWithCB(ctx, userID, folderID, nil)
	if err != nil {
		return nil, err
	}
	ids := make([]int64, 0, len(rows))
	for i := range rows {
		ids = append(ids, rows[i].PeerDialogId)
	}
	return ids, nil
}

func (d *Dao) GetFolderNotPinnedDialogIdList(ctx context.Context, userID int64) ([]int64, error) {
	return d.GetNoCacheFolderNotPinnedDialogIdList(ctx, userID, 1)
}

func (d *Dao) GetDialogListByIdList(ctx context.Context, userID int64, idList []int64) ([]*dialog.DialogExt, error) {
	rows, err := d.Postgres.Store.Dialogs.SelectPeerDialogList(ctx, userID, idList)
	if err != nil {
		return nil, err
	}
	byPeerDialogID := make(map[int64]*dialog.DialogExt, len(rows))
	for i := range rows {
		byPeerDialogID[rows[i].PeerDialogId] = d.MakeDialog(&rows[i])
	}
	result := make(dialog.DialogExtList, 0, len(idList))
	for _, id := range idList {
		if value := byPeerDialogID[id]; value != nil {
			result = append(result, value)
		}
	}
	return result, nil
}
