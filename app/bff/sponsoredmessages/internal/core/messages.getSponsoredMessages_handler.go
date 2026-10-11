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
// messages.getSponsoredMessages#9bd2f439 peer:InputPeer = messages.SponsoredMessages;
func (c *SponsoredMessagesCore) MessagesGetSponsoredMessages(in *mtproto.TLMessagesGetSponsoredMessages) (*mtproto.Messages_SponsoredMessages, error) {
	uid, err := c.requireSponsoredUser()
	if err != nil {
		return nil, err
	}
	if in == nil || in.GetPeer() == nil {
		return nil, mtproto.ErrPeerIdInvalid
	}
	enabled, _, err := persist.GetSponsoredMessagesEnabled(c.ctx, uid)
	if err != nil {
		return nil, mtproto.ErrInternalServerError
	}
	if !enabled {
		return mtproto.MakeTLMessagesSponsoredMessagesEmpty(nil).To_Messages_SponsoredMessages(), nil
	}
	if c.svcCtx != nil && strings.TrimSpace(c.svcCtx.Config.SponsoredProviderEndpoint) == "" {
		result, found, err := persist.LoadSponsoredMessages(c.ctx, sponsoredPeer(in.GetPeer()))
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
	if err := c.callSponsoredProvider("messages.getSponsoredMessages", map[string]any{"peer": in.GetPeer()}, &result); err != nil {
		return nil, err
	}
	if result.GetPredicateName() == "" {
		return nil, mtproto.ErrInternalServerError
	}
	return &result, nil
}
