// Copyright 2026 Teamgram Authors
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
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
)

// RPCPreparedInlineMessagesServer: Layer 229 methods previously returned ERR_ENTERPRISE_IS_BLOCKED.

func preparedResultID(in *mtproto.TLMessagesSavePreparedInlineMessage) string {
	if in == nil || in.GetResult() == nil {
		return ""
	}
	return in.GetResult().GetId()
}

func (c *ApiFullCore) MessagesSavePreparedInlineMessage(in *mtproto.TLMessagesSavePreparedInlineMessage) (*mtproto.Messages_BotPreparedInlineMessage, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	id := preparedResultID(in)
	if err = persist.Default.Set(b7Key(uid, id), id); err != nil {
		return nil, err
	}
	return mtproto.MakeTLMessagesBotPreparedInlineMessage(&mtproto.Messages_BotPreparedInlineMessage{
		Id: id,
	}).To_Messages_BotPreparedInlineMessage(), nil
}

func (c *ApiFullCore) MessagesGetPreparedInlineMessage(in *mtproto.TLMessagesGetPreparedInlineMessage) (*mtproto.Messages_PreparedInlineMessage, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	id := ""
	if in != nil {
		id = in.GetId()
	}
	stored, err := persist.Default.Get(b7Key(uid, id))
	if err != nil {
		return nil, err
	}
	return mtproto.MakeTLMessagesPreparedInlineMessage(&mtproto.Messages_PreparedInlineMessage{
		Result: mtproto.MakeTLBotInlineResult(&mtproto.BotInlineResult{
			Id:          stored,
			SendMessage: mtproto.MakeTLBotInlineMessageText(&mtproto.BotInlineMessage{}).To_BotInlineMessage(),
		}).To_BotInlineResult(),
		PeerTypes: []*mtproto.InlineQueryPeerType{},
		Users:     []*mtproto.User{},
	}).To_Messages_PreparedInlineMessage(), nil
}
