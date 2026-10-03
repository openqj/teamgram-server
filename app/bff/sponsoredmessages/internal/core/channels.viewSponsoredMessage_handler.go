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
	"github.com/teamgram/proto/mtproto"
)

// ChannelsViewSponsoredMessage
// channels.viewSponsoredMessage#beaedb94 channel:InputChannel random_id:bytes = Bool;
func (c *SponsoredMessagesCore) ChannelsViewSponsoredMessage(in *mtproto.TLChannelsViewSponsoredMessage) (*mtproto.Bool, error) {
	uid, err := c.requireSponsoredUser()
	if err != nil {
		return nil, err
	}
	rid, err := sponsoredRandom(in.GetRandomId())
	if err != nil {
		return nil, err
	}
	var channelID int64
	if in.GetChannel() != nil {
		channelID = in.GetChannel().GetChannelId()
	}
	if err = sponsoredSet(uid, "channel_view:"+rid, map[string]any{
		"random_id":  rid,
		"channel_id": channelID,
	}); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}
