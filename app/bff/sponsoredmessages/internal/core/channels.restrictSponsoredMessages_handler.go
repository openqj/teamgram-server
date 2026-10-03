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

import "github.com/teamgram/proto/mtproto"

// ChannelsRestrictSponsoredMessages
// channels.restrictSponsoredMessages#9ae91519 channel:InputChannel restricted:Bool = Updates;
func (c *SponsoredMessagesCore) ChannelsRestrictSponsoredMessages(in *mtproto.TLChannelsRestrictSponsoredMessages) (*mtproto.Updates, error) {
	if _, err := c.requireSponsoredUser(); err != nil {
		return nil, err
	}
	if in == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	channel := in.GetChannel()
	if channel == nil {
		return nil, mtproto.ErrChannelInvalid
	}
	switch channel.GetPredicateName() {
	case mtproto.Predicate_inputChannel:
		if channel.GetChannelId() <= 0 || channel.GetAccessHash() == 0 {
			return nil, mtproto.ErrChannelInvalid
		}
	case mtproto.Predicate_inputChannelFromMessage:
		if channel.GetChannelId() <= 0 || channel.GetPeer() == nil || channel.GetMsgId() <= 0 {
			return nil, mtproto.ErrChannelInvalid
		}
	default:
		return nil, mtproto.ErrChannelInvalid
	}
	switch in.GetRestricted().GetPredicateName() {
	case mtproto.Predicate_boolTrue, mtproto.Predicate_boolFalse:
		return nil, mtproto.ErrMethodNotImpl
	default:
		return nil, mtproto.ErrInputRequestInvalid
	}
}
