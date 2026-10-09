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
	"github.com/teamgram/teamgram-server/app/service/biz/message/message"
)

// MessageSearchByPinned
// message.searchByPinned user_id:long peer:PeerUtil = Vector<MessageBox>;
func (c *MessageCore) MessageSearchByPinned(in *message.TLMessageSearchByPinned) (*mtproto.MessageBoxList, error) {
	var (
		dialogId = mtproto.MakeDialogId(in.UserId, in.PeerType, in.PeerId)
		boxList  = make([]*mtproto.MessageBox, 0)
	)

	switch in.PeerType {
	case mtproto.PEER_SELF, mtproto.PEER_USER, mtproto.PEER_CHAT:
		list, err := c.svcCtx.Dao.SelectPinnedList(c.ctx, in.UserId, dialogId.A, dialogId.B)
		if err != nil {
			return nil, err
		}
		for i := range list {
			boxList = append(boxList, c.svcCtx.Dao.MakeMessageBox(c.ctx, in.UserId, &list[i]))
		}
	case mtproto.PEER_CHANNEL:
		c.Logger.Errorf("message.searchByPinned blocked, License key from https://teamgram.net required to unlock enterprise features.")

		return nil, mtproto.ErrEnterpriseIsBlocked
	}

	return mtproto.MakeTLMessageBoxList(&mtproto.MessageBoxList{
		BoxList: boxList,
	}).To_MessageBoxList(), nil
}
