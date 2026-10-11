// Copyright 2024 Teamgram Authors
//  All rights reserved.
//
// Author: Benqi (wubenqi@gmail.com)
//

package dao

import (
	"context"

	"github.com/teamgram/proto/mtproto"
)

func (d *Dao) GetNoCacheAllDraftIdList(ctx context.Context, userId int64) ([]int64, error) {
	rows, err := d.Postgres.Store.Dialogs.SelectAllDrafts(ctx, userId)
	if err != nil {
		return nil, err
	}
	ids := make([]int64, 0, len(rows))
	for i := range rows {
		ids = append(ids, rows[i].Id)
	}
	return ids, nil
}

func (d *Dao) GetAllDraftIdList(ctx context.Context, userId int64) ([]int64, error) {
	return d.GetNoCacheAllDraftIdList(ctx, userId)
}

// GetDraftMessage returns draft message by id.
func (d *Dao) GetDraftMessage(ctx context.Context, userId, peerDialogId int64) (*mtproto.DraftMessage, error) {
	dlgExt, err := d.GetDialogByPeerDialogId(ctx, userId, peerDialogId)
	if err != nil {
		return nil, err
	}

	return dlgExt.GetDialog().GetDraft(), nil
}

func (d *Dao) GetAllDraftMessageList(ctx context.Context, userId int64) ([]*mtproto.DraftMessage, error) {
	idList, err := d.GetAllDraftIdList(ctx, userId)
	if err != nil {
		return nil, err
	}

	dlgExtList, err := d.GetDialogListByIdList(ctx, userId, idList)
	if err != nil {
		return nil, err
	}

	vList := make([]*mtproto.DraftMessage, 0, len(idList))
	for _, dlgExt := range dlgExtList {
		if dlgExt.GetDialog().GetDraft() != nil {
			vList = append(vList, dlgExt.GetDialog().GetDraft())
		}
	}

	return vList, nil
}
