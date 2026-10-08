// Copyright 2022 Teamgram Authors
//  All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//   http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// Author: teamgramio (teamgram.io@gmail.com)
//

package core

import (
	"errors"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/chat/chat"
	"github.com/zeromicro/go-zero/core/jsonx"
	"time"
)

// ChatSetChatAvailableReactions
// chat.setChatAvailableReactions self_id:long chat_id:long available_reactions:Vector<string> = Bool;
func (c *ChatCore) ChatSetChatAvailableReactions(in *chat.TLChatSetChatAvailableReactions) (*mtproto.MutableChat, error) {
	var (
		chat2 *mtproto.MutableChat
		me    *mtproto.ImmutableChatParticipant
		err   error
	)

	chat2, err = c.svcCtx.Dao.GetMutableChat(c.ctx, in.ChatId, in.SelfId)
	if err != nil {
		c.Logger.Errorf("chat.setChatAvailableReactions - error: %v")
		return nil, err
	}

	me, _ = chat2.GetImmutableChatParticipant(in.SelfId)
	if me == nil || me.State != mtproto.ChatMemberStateNormal {
		err = mtproto.ErrParticipantIdInvalid
		c.Logger.Errorf("chat.setChatAvailableReactions - error: %v")
		return nil, err
	}

	if !me.CanAdminAddAdmins() {
		err = mtproto.ErrChatAdminRequired
		c.Logger.Errorf("chat.setChatAvailableReactions - error: %v")
		return nil, err
	}

	var (
		availableReactions string
	)

	if len(in.AvailableReactions) > 0 {
		availableReactionsData, _ := jsonx.Marshal(in.AvailableReactions)
		if availableReactionsData != nil {
			availableReactions = string(availableReactionsData)
		}
	}
	if c.svcCtx.Dao.Postgres == nil || c.svcCtx.Dao.Postgres.Store == nil {
		return nil, errors.New("chat.setChatAvailableReactions: PostgreSQL store is not initialized")
	}

	tx, err := c.svcCtx.Dao.Postgres.Pool.Begin(c.ctx)
	if err == nil {
		defer tx.Rollback(c.ctx)
		_, err = c.svcCtx.Dao.Postgres.Store.Chats.UpdateAvailableReactionsOn(c.ctx, tx, in.AvailableReactionsType, availableReactions, in.ChatId)
		if err == nil {
			err = tx.Commit(c.ctx)
		}
	}
	if err != nil {
		c.Logger.Errorf("chat.setChatAvailableReactions - error: %v")
		return nil, err
	}

	chat2.Chat.AvailableReactions = in.AvailableReactions
	chat2.Chat.Version++
	chat2.Chat.Date = time.Now().Unix()
	return chat2, nil
}
