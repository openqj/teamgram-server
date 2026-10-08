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
	"math"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/message/internal/dal/dataobject"
	"github.com/teamgram/teamgram-server/app/service/biz/message/message"
)

// MessageSearch
// message.search user_id:long peer:PeerUtil q:string offset:int limit:int = Vector<MessageBox>;
func (c *MessageCore) MessageSearch(in *message.TLMessageSearch) (*mtproto.MessageBoxList, error) {
	if in == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if in.UserId <= 0 || in.PeerId <= 0 {
		return nil, mtproto.ErrPeerIdInvalid
	}
	if in.Q == "" {
		return nil, mtproto.ErrSearchQueryEmpty
	}
	if in.Limit < 0 {
		return nil, mtproto.ErrLimitInvalid
	}
	if c == nil || c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.Postgres == nil || c.svcCtx.Dao.Postgres.Store == nil || c.svcCtx.Dao.Postgres.Store.Messages == nil {
		return nil, mtproto.ErrInternalServerError
	}
	var (
		offset  = in.Offset
		q       = in.Q
		boxList []*mtproto.MessageBox
		limit   = in.Limit
		err     error
	)

	// TODO(@benqi): check q
	if offset == 0 {
		offset = math.MaxInt32
	}
	if limit > 50 {
		limit = 50
	}

	switch in.PeerType {
	case mtproto.PEER_SELF, mtproto.PEER_USER, mtproto.PEER_CHAT:
		if q[0] == '#' {
			idList, err := c.svcCtx.Dao.SelectPeerHashTagList(
				c.ctx,
				in.UserId,
				in.PeerType,
				in.PeerId,
				q)
			if err != nil {
				return nil, err
			}

			if len(idList) > 0 {
				var rows []dataobject.MessagesDO
				if rows, err = c.svcCtx.Dao.SelectByMessageIDListForSearch(c.ctx, in.UserId, idList); err != nil {
					return nil, err
				}
				for i := range rows {
					boxList = append(boxList, c.svcCtx.Dao.MakeMessageBox(c.ctx, in.UserId, &rows[i]))
				}
			}
		} else {
			dialogId := mtproto.MakeDialogId(in.UserId, in.PeerType, in.PeerId)
			var rows []dataobject.MessagesDO
			if rows, err = c.svcCtx.Dao.SearchMessagesForSearch(c.ctx, in.UserId, dialogId.A, dialogId.B, offset, "%"+q+"%", limit); err != nil {
				return nil, err
			}
			for i := range rows {
				boxList = append(boxList, c.svcCtx.Dao.MakeMessageBox(c.ctx, in.UserId, &rows[i]))
			}
		}
	case mtproto.PEER_CHANNEL:
		c.Logger.Errorf("message.search blocked, License key from https://teamgram.net required to unlock enterprise features.")

		return nil, mtproto.ErrEnterpriseIsBlocked
	default:
		return nil, mtproto.ErrPeerIdInvalid
	}

	if boxList == nil {
		boxList = []*mtproto.MessageBox{}
	}

	return mtproto.MakeTLMessageBoxList(&mtproto.MessageBoxList{
		BoxList: boxList,
	}).To_MessageBoxList(), nil
}
