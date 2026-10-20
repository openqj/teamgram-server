// Copyright 2024 Teamgram Authors
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
	"strings"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/persist"
)

// MessagesGetSponsoredMessages
// messages.getSponsoredMessages#3d6ce850 flags:# peer:InputPeer msg_id:flags.0?int = messages.SponsoredMessages;
func (c *SponsoredMessagesCore) MessagesGetSponsoredMessages(in *mtproto.TLMessagesGetSponsoredMessages) (*mtproto.Messages_SponsoredMessages, error) {
	uid, err := c.requireSponsoredUser()
	if err != nil {
		return nil, err
	}
	if in == nil || in.GetPeer() == nil {
		return nil, mtproto.ErrPeerIdInvalid
	}
	if msgID := in.GetMsgId(); msgID != nil && msgID.GetValue() <= 0 {
		return nil, mtproto.ErrMessageIdInvalid
	}
	enabled, _, err := persist.GetSponsoredMessagesEnabled(c.ctx, uid)
	if err != nil {
		return nil, mtproto.ErrInternalServerError
	}
	if !enabled {
		return mtproto.MakeTLMessagesSponsoredMessagesEmpty(nil).To_Messages_SponsoredMessages(), nil
	}
	messageID := int32(0)
	if msgID := in.GetMsgId(); msgID != nil {
		messageID = msgID.GetValue()
	}
	if c.svcCtx != nil && strings.TrimSpace(c.svcCtx.Config.SponsoredProviderEndpoint) == "" {
		result, found, err := persist.LoadSponsoredMessages(c.ctx, sponsoredPeer(in.GetPeer()), messageID)
		if err != nil {
			if errors.Is(err, persist.ErrSponsoredInventoryUnavailable) {
				return nil, mtproto.ErrMethodNotImpl
			}
			return nil, mtproto.ErrInternalServerError
		}
		if !found {
			return nil, mtproto.ErrMethodNotImpl
		}
		return result, nil
	}
	var result mtproto.Messages_SponsoredMessages
	payload := map[string]any{"peer": in.GetPeer()}
	if msgID := in.GetMsgId(); msgID != nil {
		payload["msg_id"] = msgID.GetValue()
	}
	if err := c.callSponsoredProvider("messages.getSponsoredMessages", payload, &result); err != nil {
		return nil, err
	}
	if result.GetPredicateName() != mtproto.Predicate_messages_sponsoredMessages &&
		result.GetPredicateName() != mtproto.Predicate_messages_sponsoredMessagesEmpty {
		return nil, mtproto.ErrInternalServerError
	}
	return &result, nil
}
