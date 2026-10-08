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
	"github.com/teamgram/teamgram-server/app/service/biz/chat/chat"
	"github.com/teamgram/teamgram-server/app/service/biz/chat/internal/dal/dataobject"
)

// ChatGetChatParticipantIdList
// chat.getChatParticipantIdList chat_id:long = Vector<long>;
func (c *ChatCore) ChatGetChatParticipantIdList(in *chat.TLChatGetChatParticipantIdList) (*chat.Vector_Long, error) {
	var (
		idList = make([]int64, 0)
	)

	if c.svcCtx.Dao.Postgres != nil && c.svcCtx.Dao.Postgres.Store != nil {
		rows, err := c.svcCtx.Dao.Postgres.Store.Participants.SelectList(c.ctx, in.ChatId)
		if err != nil {
			return nil, err
		}
		for i := range rows {
			if rows[i].State == mtproto.ChatMemberStateNormal || rows[i].State == mtproto.ChatMemberStateMigrated {
				idList = append(idList, rows[i].UserId)
			}
		}
	} else {
		c.svcCtx.Dao.ChatParticipantsDAO.SelectListWithCB(c.ctx, in.ChatId,
			func(sz, i int, v *dataobject.ChatParticipantsDO) {
				if v.State != mtproto.ChatMemberStateNormal && v.State != mtproto.ChatMemberStateMigrated {
					return
				}
				idList = append(idList, v.UserId)
			})
	}

	return &chat.Vector_Long{
		Datas: idList,
	}, nil
}
