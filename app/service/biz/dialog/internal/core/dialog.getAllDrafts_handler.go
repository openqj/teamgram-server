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

	"github.com/zeromicro/go-zero/core/jsonx"
)

// DialogGetAllDrafts
// dialog.getAllDrafts user_id:long = Vector<PeerWithDraftMessage>;
func (c *DialogCore) DialogGetAllDrafts(in *dialog.TLDialogGetAllDrafts) (*dialog.Vector_PeerWithDraftMessage, error) {
	// var doList []dataobject.DialogsDO
	rValues := &dialog.Vector_PeerWithDraftMessage{
		Datas: []*dialog.PeerWithDraftMessage{},
	}
	if in == nil || in.GetUserId() <= 0 {
		return nil, mtproto.ErrInputRequestInvalid
	}

	if c == nil || c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.Postgres == nil ||
		c.svcCtx.Dao.Postgres.Store == nil || c.svcCtx.Dao.Postgres.Store.Dialogs == nil {
		return nil, mtproto.ErrMethodNotImpl
	}
	rows, err := c.svcCtx.Dao.Postgres.Store.Dialogs.SelectAllDrafts(c.ctx, in.UserId)
	if err != nil {
		return nil, err
	}
	for i := range rows {
		v := &rows[i]
		if v.DraftMessageData == "" {
			if c.Logger != nil {
				c.Logger.Errorf("dialog.getAllDrafts - empty draft message data")
			}
			return nil, mtproto.ErrInternalServerError
		}

		var draft *mtproto.DraftMessage
		if err := jsonx.UnmarshalFromString(v.DraftMessageData, &draft); err != nil {
			if c.Logger != nil {
				c.Logger.Errorf("dialog.getAllDrafts - unmarshal draft: %v", err)
			}
			return nil, mtproto.ErrInternalServerError
		}
		if draft == nil {
			if c.Logger != nil {
				c.Logger.Errorf("dialog.getAllDrafts - draft message is null")
			}
			return nil, mtproto.ErrInternalServerError
		}
		switch draft.GetPredicateName() {
		case mtproto.Predicate_draftMessageEmpty, mtproto.Predicate_draftMessage:
		default:
			if c.Logger != nil {
				c.Logger.Errorf("dialog.getAllDrafts - unsupported draft predicate: %q", draft.GetPredicateName())
			}
			return nil, mtproto.ErrInternalServerError
		}

		rValues.Datas = append(rValues.Datas,
			dialog.MakeTLUpdateDraftMessage(&dialog.PeerWithDraftMessage{
				Peer:  mtproto.MakePeer(v.PeerType, v.PeerId),
				Draft: draft,
			}).To_PeerWithDraftMessage())
	}

	return rValues, nil
}
