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
)

// ChatSetHistoryTTL
// chat.setHistoryTTL self_id:long chat_id:long ttl_period:int = Bool;
func (c *ChatCore) ChatSetHistoryTTL(in *chat.TLChatSetHistoryTTL) (*mtproto.MutableChat, error) {
	mChat, err := c.svcCtx.Dao.GetMutableChat(c.ctx, in.ChatId)
	if err != nil {
		c.Logger.Errorf("chat.setHistoryTTL - error: %v", err)
		return nil, err
	}
	if mChat.Creator() != in.SelfId {
		err = mtproto.ErrChatAdminRequired
		c.Logger.Errorf("chat.setHistoryTTL - error: %v", err)
		return nil, err
	}
	if c.svcCtx.Dao.Postgres == nil || c.svcCtx.Dao.Postgres.Store == nil {
		return nil, errors.New("chat.setHistoryTTL: PostgreSQL store is not initialized")
	}

	tx, err := c.svcCtx.Dao.Postgres.Pool.Begin(c.ctx)
	if err == nil {
		defer tx.Rollback(c.ctx)
		_, err = c.svcCtx.Dao.Postgres.Store.Chats.UpdateTTLPeriodOn(c.ctx, tx, in.TtlPeriod, in.ChatId)
		if err == nil {
			err = tx.Commit(c.ctx)
		}
	}

	if err != nil {
		c.Logger.Errorf("chat.setHistoryTTL - error: %v", err)
		return nil, err
	}

	mChat.Chat.TtlPeriod = in.TtlPeriod
	return mChat, nil
}
