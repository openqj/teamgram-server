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

// DialogGetDialogUnreadMarkList
// dialog.getDialogUnreadMarkList user_id:long = Vector<DialogPeer>;
func (c *DialogCore) DialogGetDialogUnreadMarkList(in *dialog.TLDialogGetDialogUnreadMarkList) (*dialog.Vector_DialogPeer, error) {
	rows, err := c.svcCtx.Dao.DialogsDAO.SelectAllDialogs(c.ctx, in.UserId)
	if err != nil {
		c.Logger.Errorf("dialog.getDialogUnreadMarkList - error: %v", err)
		return nil, err
	}
	datas := make([]*mtproto.DialogPeer, 0)
	for _, row := range rows {
		if !row.UnreadMark {
			continue
		}
		datas = append(datas, mtproto.MakeTLDialogPeer(&mtproto.DialogPeer{
			Peer: mtproto.MakePeer(row.PeerType, row.PeerId),
		}).To_DialogPeer())
	}
	return &dialog.Vector_DialogPeer{Datas: datas}, nil
}
