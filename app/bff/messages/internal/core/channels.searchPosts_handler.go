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
	"strings"

	"github.com/teamgram/proto/mtproto"
)

func searchPostsQuery(in *mtproto.TLChannelsSearchPosts) string {
	if in == nil {
		return ""
	}
	query := strings.TrimSpace(in.GetQuery().GetValue())
	hashtag := strings.TrimSpace(in.GetHashtag_STRING())
	if hashtag == "" && in.GetHashtag_FLAGSTRING() != nil {
		hashtag = strings.TrimSpace(in.GetHashtag_FLAGSTRING().GetValue())
	}
	// The TL method requires exactly one selector. Hashtags are sent without
	// the leading '#'; accepting it here would hide a malformed request.
	if query != "" && hashtag != "" {
		return ""
	}
	if query != "" {
		return query
	}
	if hashtag == "" || strings.HasPrefix(hashtag, "#") {
		return ""
	}
	return hashtag
}

// validateSearchPostsOffsetPeer checks the cursor shape without treating it
// as a channel selector. channels.searchPosts searches a global post index;
// the first request uses inputPeerEmpty and subsequent requests carry the
// channel peer of the last result. The local APIFull store has no such index,
// so valid requests are rejected by the handler below.
func validateSearchPostsOffsetPeer(peer *mtproto.InputPeer) error {
	if peer == nil {
		return mtproto.ErrOffsetPeerIdInvalid
	}
	switch peer.GetPredicateName() {
	case mtproto.Predicate_inputPeerEmpty:
		return nil
	case mtproto.Predicate_inputPeerChannel:
		if peer.GetChannelId() <= 0 || peer.GetAccessHash() == 0 {
			return mtproto.ErrOffsetPeerIdInvalid
		}
		return nil
	default:
		return mtproto.ErrOffsetPeerIdInvalid
	}
}

// ChannelsSearchPosts
// channels.searchPosts#f2c4f24d flags:# hashtag:flags.0?string query:flags.1?string offset_rate:int offset_peer:InputPeer offset_id:int limit:int allow_paid_stars:flags.2?long = messages.Messages;
func (c *MessagesCore) ChannelsSearchPosts(in *mtproto.TLChannelsSearchPosts) (*mtproto.Messages_Messages, error) {
	if c == nil || c.MD == nil || c.MD.UserId <= 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	if in == nil {
		return nil, mtproto.ErrPeerIdInvalid
	}
	if c.MD.IsBot {
		return nil, mtproto.ErrBotMethodInvalid
	}
	if err := validateSearchPostsOffsetPeer(in.GetOffsetPeer()); err != nil {
		return nil, err
	}
	if stars := in.GetAllowPaidStars(); stars != nil && stars.GetValue() > 0 {
		return nil, mtproto.ErrMethodNotImpl
	}
	if in.GetOffsetRate() < 0 {
		return nil, mtproto.ErrOffsetInvalid
	}
	if in.GetLimit() < 0 || in.GetLimit() > 100 {
		return nil, mtproto.ErrLimitInvalid
	}
	if in.GetOffsetId() < 0 {
		return nil, mtproto.ErrOffsetInvalid
	}

	query := searchPostsQuery(in)
	if query == "" {
		return nil, mtproto.ErrSearchQueryEmpty
	}

	// APIFull only owns per-channel message rows. It has no authoritative
	// cross-channel post index, relevance score, or cursor that can implement
	// offset_rate/offset_peer/offset_id and the corresponding next_rate.
	// Returning a local SQL page here would violate the Layer 229 contract.
	return nil, mtproto.ErrMethodNotImpl
}
