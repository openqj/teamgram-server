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

import "github.com/teamgram/proto/mtproto"

// RPCCommunitiesServer: Layer 229 methods previously returned ERR_ENTERPRISE_IS_BLOCKED.

func communitiesUnavailable(c *ApiFullCore) error {
	if _, err := c.requireUserId(); err != nil {
		return err
	}
	return mtproto.ErrMethodNotImpl
}

func (c *ApiFullCore) CommunitiesCreate(in *mtproto.TLCommunitiesCreate) (*mtproto.Updates, error) {
	_ = in
	return nil, communitiesUnavailable(c)
}

func (c *ApiFullCore) CommunitiesTogglePeerLink(in *mtproto.TLCommunitiesTogglePeerLink) (*mtproto.Bool, error) {
	_ = in
	return nil, communitiesUnavailable(c)
}

func (c *ApiFullCore) CommunitiesGetJoinedCommunities(in *mtproto.TLCommunitiesGetJoinedCommunities) (*mtproto.Messages_Chats, error) {
	_ = in
	return nil, communitiesUnavailable(c)
}

func (c *ApiFullCore) CommunitiesToggleCommunityCollapsedInDialogs(in *mtproto.TLCommunitiesToggleCommunityCollapsedInDialogs) (*mtproto.Updates, error) {
	_ = in
	return nil, communitiesUnavailable(c)
}

func (c *ApiFullCore) CommunitiesGetPeerLinkRequests(in *mtproto.TLCommunitiesGetPeerLinkRequests) (*mtproto.Communities_PeerLinkRequests, error) {
	_ = in
	return nil, communitiesUnavailable(c)
}

func (c *ApiFullCore) CommunitiesTogglePeerLinkRequestApproval(in *mtproto.TLCommunitiesTogglePeerLinkRequestApproval) (*mtproto.Bool, error) {
	_ = in
	return nil, communitiesUnavailable(c)
}

func (c *ApiFullCore) CommunitiesToggleAllPeerLinkRequestApproval(in *mtproto.TLCommunitiesToggleAllPeerLinkRequestApproval) (*mtproto.Bool, error) {
	_ = in
	return nil, communitiesUnavailable(c)
}

func (c *ApiFullCore) CommunitiesToggleParticipantBanned(in *mtproto.TLCommunitiesToggleParticipantBanned) (*mtproto.Bool, error) {
	_ = in
	return nil, communitiesUnavailable(c)
}

func (c *ApiFullCore) CommunitiesGetParticipantJoinedChats(in *mtproto.TLCommunitiesGetParticipantJoinedChats) (*mtproto.Communities_ParticipantJoinedChats, error) {
	userID, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if in.GetCommunity() == nil || inputChannelID(in.GetCommunity()) <= 0 {
		return nil, mtproto.ErrChannelInvalid
	}
	participantID, err := channelMemberPeerID(userID, in.GetParticipant())
	if err != nil || participantID <= 0 {
		return nil, mtproto.ErrUserIdInvalid
	}
	return nil, mtproto.ErrMethodNotImpl
}
