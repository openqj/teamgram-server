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

// DialogClearAllDrafts
// dialog.clearAllDrafts user_id:long = Vector<PeerWithDraftMessage>;
func (c *DialogCore) DialogClearAllDrafts(in *dialog.TLDialogClearAllDrafts) (*dialog.Vector_PeerWithDraftMessage, error) {
	if c == nil || c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.Postgres == nil ||
		c.svcCtx.Dao.Postgres.Store == nil || c.svcCtx.Dao.Postgres.Store.Dialogs == nil {
		return nil, mtproto.ErrMethodNotImpl
	}
	rows, err := c.svcCtx.Dao.Postgres.Store.Dialogs.SelectAllDrafts(c.ctx, in.UserId)
	if err != nil {
		return nil, err
	}
	result := &dialog.Vector_PeerWithDraftMessage{Datas: make([]*dialog.PeerWithDraftMessage, 0, len(rows))}
	for i := range rows {
		row := &rows[i]
		result.Datas = append(result.Datas, dialog.MakeTLUpdateDraftMessage(&dialog.PeerWithDraftMessage{
			Peer: mtproto.MakePeer(row.PeerType, row.PeerId),
			Draft: mtproto.MakeTLDraftMessageEmpty(&mtproto.DraftMessage{
				Date_FLAGINT32: mtproto.MakeFlagsInt32(int32(time.Now().Unix())),
			}).To_DraftMessage(),
		}).To_PeerWithDraftMessage())
	}
	if len(rows) > 0 {
		if _, err = c.svcCtx.Dao.Postgres.Store.Dialogs.ClearAllDrafts(c.ctx, in.UserId); err != nil {
			return nil, err
		}
	}
	return result, nil
}
