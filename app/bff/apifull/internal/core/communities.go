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
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/channelview"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/domain"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// RPCCommunitiesServer implements the Layer 229 communities provider against
// the PostgreSQL APIFull projection. A missing projection keeps the old
// fail-closed behavior used by unit tests and by startup gates that have not
// yet provisioned the deployment schema.

func communitiesUnavailable(c *ApiFullCore) error {
	if _, err := c.requireUserId(); err != nil {
		return err
	}
	return mtproto.ErrMethodNotImpl
}

func communityPeerType(caller int64, peer *mtproto.InputPeer) (domain.CommunityPeerType, int64, int64, error) {
	if peer == nil {
		return 0, 0, 0, mtproto.ErrPeerIdInvalid
	}
	typ, id := apifullPeerTypeID(caller, peer)
	if typ == mtproto.PEER_SELF {
		typ, id = mtproto.PEER_USER, caller
	}
	if id <= 0 {
		return 0, 0, 0, mtproto.ErrPeerIdInvalid
	}
	var peerType domain.CommunityPeerType
	switch typ {
	case mtproto.PEER_USER:
		peerType = domain.CommunityPeerUser
	case mtproto.PEER_CHAT:
		peerType = domain.CommunityPeerChat
	case mtproto.PEER_CHANNEL:
		peerType = domain.CommunityPeerChannel
	default:
		return 0, 0, 0, mtproto.ErrPeerIdInvalid
	}
	return peerType, id, peer.GetAccessHash(), nil
}

func communityPeerFromRecord(p domain.CommunityPeer) *mtproto.Peer {
	switch p.PeerType {
	case domain.CommunityPeerUser:
		return mtproto.MakePeerUser(p.PeerID)
	case domain.CommunityPeerChat:
		return mtproto.MakePeerChat(p.PeerID)
	case domain.CommunityPeerChannel:
		return mtproto.MakePeerChannel(p.PeerID)
	default:
		return nil
	}
}

func communityEmptyUpdates() *mtproto.Updates {
	return mtproto.MakeTLUpdates(&mtproto.Updates{
		Updates: []*mtproto.Update{}, Users: []*mtproto.User{}, Chats: []*mtproto.Chat{}, Date: int32(time.Now().Unix()),
	}).To_Updates()
}

func communityError(err error) error {
	switch {
	case errors.Is(err, domain.ErrCommunityMissing):
		return mtproto.ErrChannelInvalid
	case errors.Is(err, domain.ErrCommunityNotOwner):
		return mtproto.ErrChatAdminRequired
	case errors.Is(err, domain.ErrCommunityPeerMissing):
		return mtproto.ErrPeerIdInvalid
	case errors.Is(err, domain.ErrCommunityPeerInvalid):
		return mtproto.ErrPeerIdInvalid
	default:
		return mtproto.ErrInternalServerError
	}
}

func (c *ApiFullCore) resolveCommunity(input *mtproto.InputChannel) (domain.Community, domain.Channel, error) {
	if input == nil || input.GetChannelId() <= 0 {
		return domain.Community{}, domain.Channel{}, mtproto.ErrChannelInvalid
	}
	channel, ok, err := domain.LoadChannel(input.GetChannelId())
	if err != nil {
		return domain.Community{}, domain.Channel{}, mtproto.ErrInternalServerError
	}
	if !ok || channel.AccessHash != input.GetAccessHash() {
		return domain.Community{}, domain.Channel{}, mtproto.ErrChannelInvalid
	}
	community, ok, err := domain.LoadCommunity(input.GetChannelId())
	if err != nil {
		return domain.Community{}, domain.Channel{}, mtproto.ErrInternalServerError
	}
	if !ok {
		return domain.Community{}, domain.Channel{}, mtproto.ErrChannelInvalid
	}
	return community, channel, nil
}

func (c *ApiFullCore) requireCommunityOwner(userID int64, input *mtproto.InputChannel) (domain.Community, domain.Channel, error) {
	community, channel, err := c.resolveCommunity(input)
	if err != nil {
		return community, channel, err
	}
	if community.OwnerID != userID {
		return domain.Community{}, domain.Channel{}, mtproto.ErrChatAdminRequired
	}
	return community, channel, nil
}

func channelChat(ch domain.Channel, creator bool) *mtproto.Chat {
	return channelview.Chat(ch, creator)
}

func communityChat(ch domain.Channel, creator, collapsed bool) *mtproto.Chat {
	out := channelChat(ch, creator)
	out.CollapsedInDialogs = collapsed
	return mtproto.MakeTLCommunity(out).To_Chat()
}

func communityChatForPeer(p domain.CommunityPeer, viewer int64) (*mtproto.Chat, bool) {
	if p.PeerType == domain.CommunityPeerChannel {
		ch, ok, err := domain.LoadChannel(p.PeerID)
		if err != nil || !ok {
			return nil, false
		}
		return channelChat(ch, ch.Creator == viewer), true
	}
	if p.PeerType == domain.CommunityPeerChat {
		return mtproto.MakeTLChat(&mtproto.Chat{
			Id: p.PeerID, Title: fmt.Sprintf("Chat %d", p.PeerID),
			Photo: mtproto.MakeTLChatPhotoEmpty(nil).To_ChatPhoto(),
		}).To_Chat(), true
	}
	return nil, false
}

func communityEntities(peers []domain.CommunityPeer, viewer int64) ([]*mtproto.Chat, []*mtproto.User) {
	chats := make([]*mtproto.Chat, 0, len(peers))
	users := make([]*mtproto.User, 0)
	chatSeen := make(map[string]struct{})
	userSeen := make(map[int64]struct{})
	for _, peer := range peers {
		if chat, ok := communityChatForPeer(peer, viewer); ok && chat != nil {
			key := chat.GetPredicateName() + ":" + strconv.FormatInt(chat.GetId(), 10)
			if _, found := chatSeen[key]; !found {
				chatSeen[key] = struct{}{}
				chats = append(chats, chat)
			}
		}
		if peer.PeerType == domain.CommunityPeerUser {
			if _, found := userSeen[peer.PeerID]; !found {
				userSeen[peer.PeerID] = struct{}{}
				users = append(users, mtproto.MakeTLUser(&mtproto.User{Id: peer.PeerID, Self: peer.PeerID == viewer}).To_User())
			}
		}
		if peer.RequestedBy > 0 {
			if _, found := userSeen[peer.RequestedBy]; !found {
				userSeen[peer.RequestedBy] = struct{}{}
				users = append(users, mtproto.MakeTLUser(&mtproto.User{Id: peer.RequestedBy, Self: peer.RequestedBy == viewer}).To_User())
			}
		}
	}
	return chats, users
}

func communityPeerVisible(value *bool) *mtproto.Bool {
	if value == nil {
		return nil
	}
	if *value {
		return mtproto.BoolTrue
	}
	return mtproto.BoolFalse
}

func (c *ApiFullCore) communityFull(viewer int64, community domain.Community, channel domain.Channel) (*mtproto.Messages_ChatFull, error) {
	peers, err := domain.ListCommunityPeers(community.ID, false)
	if err != nil {
		return nil, mtproto.ErrInternalServerError
	}
	pending, err := domain.ListCommunityPeers(community.ID, true)
	if err != nil {
		return nil, mtproto.ErrInternalServerError
	}
	linked := make([]*mtproto.CommunityPeer, 0, len(peers))
	for _, peer := range peers {
		wirePeer := communityPeerFromRecord(peer)
		if wirePeer == nil {
			continue
		}
		linked = append(linked, mtproto.MakeTLCommunityPeer(&mtproto.CommunityPeer{
			CanViewHistory: peer.CanViewHistory,
			Visible:        communityPeerVisible(peer.Visible),
			Peer:           wirePeer,
		}).To_CommunityPeer())
	}
	chats, users := communityEntities(peers, viewer)
	communityCollapsed, err := domain.CommunityCollapsed(viewer, community.ID)
	if err != nil {
		return nil, mtproto.ErrInternalServerError
	}
	chats = append(chats, communityChat(channel, channel.Creator == viewer, communityCollapsed))
	userSeen := make(map[int64]struct{}, len(users))
	for _, user := range users {
		if user != nil {
			userSeen[user.GetId()] = struct{}{}
		}
	}
	if _, ok := userSeen[community.OwnerID]; !ok {
		users = append(users, mtproto.MakeTLUser(&mtproto.User{Id: community.OwnerID, Self: community.OwnerID == viewer}).To_User())
	}
	full := &mtproto.ChatFull{
		Id:          community.ID,
		About:       community.About,
		ChatPhoto:   mtproto.MakeTLPhotoEmpty(nil).To_Photo(),
		LinkedPeers: linked,
		NotifySettings: mtproto.MakeTLPeerNotifySettings(&mtproto.PeerNotifySettings{}).
			To_PeerNotifySettings(),
	}
	if len(pending) > 0 {
		full.PeerLinkRequestsPending = wrapperspb.Int32(int32(len(pending)))
	}
	return mtproto.MakeTLMessagesChatFull(&mtproto.Messages_ChatFull{
		FullChat: mtproto.MakeTLCommunityFull(full).To_ChatFull(),
		Chats:    chats,
		Users:    users,
	}).To_Messages_ChatFull(), nil
}

func (c *ApiFullCore) CommunitiesCreate(in *mtproto.TLCommunitiesCreate) (*mtproto.Updates, error) {
	userID, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if !domain.Ready() {
		return nil, communitiesUnavailable(c)
	}
	if in == nil || in.GetTitle() == "" || in.GetPeer() == nil {
		return nil, mtproto.ErrTitleInvalid
	}
	peerType, peerID, accessHash, err := communityPeerType(userID, in.GetPeer())
	if err != nil {
		return nil, err
	}
	id := time.Now().UnixNano()
	if id <= 0 {
		id = 1
	}
	for {
		if _, found, loadErr := domain.LoadChannel(id); loadErr != nil {
			return nil, mtproto.ErrInternalServerError
		} else if !found {
			break
		}
		id++
	}
	channel := domain.Channel{ID: id, AccessHash: id, Creator: userID, Title: in.GetTitle(), Megagroup: true}
	if in.GetAbout() != nil {
		channel.About = in.GetAbout().GetValue()
	}
	if err = domain.CreateCommunityWithPeer(channel, in.GetHidden(), peerType, peerID, accessHash); err != nil {
		return nil, communityError(err)
	}
	return mtproto.MakeTLUpdates(&mtproto.Updates{
		Updates: []*mtproto.Update{}, Users: []*mtproto.User{}, Chats: []*mtproto.Chat{communityChat(channel, true, false)}, Date: int32(time.Now().Unix()),
	}).To_Updates(), nil
}

func (c *ApiFullCore) CommunitiesTogglePeerLink(in *mtproto.TLCommunitiesTogglePeerLink) (*mtproto.Bool, error) {
	userID, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if !domain.Ready() {
		return nil, communitiesUnavailable(c)
	}
	if in == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	community, _, err := c.requireCommunityOwner(userID, in.GetCommunity())
	if err != nil {
		return nil, err
	}
	peerType, peerID, accessHash, err := communityPeerType(userID, in.GetPeer())
	if err != nil {
		return nil, err
	}
	var visible *bool
	if in.GetVisible() {
		v := true
		visible = &v
	} else if in.GetHidden() {
		v := false
		visible = &v
	}
	if err = domain.ToggleCommunityPeer(userID, community.ID, peerType, peerID, accessHash, visible, in.GetDeleted(), true, time.Now().Unix()); err != nil {
		return nil, communityError(err)
	}
	return mtproto.BoolTrue, nil
}

func (c *ApiFullCore) CommunitiesGetJoinedCommunities(in *mtproto.TLCommunitiesGetJoinedCommunities) (*mtproto.Messages_Chats, error) {
	userID, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if !domain.Ready() {
		return nil, communitiesUnavailable(c)
	}
	communities, err := domain.ListCommunitiesForUser(userID)
	if err != nil {
		return nil, mtproto.ErrInternalServerError
	}
	chats := make([]*mtproto.Chat, 0, len(communities))
	for _, community := range communities {
		channel, ok, loadErr := domain.LoadChannel(community.ID)
		if loadErr != nil {
			return nil, mtproto.ErrInternalServerError
		}
		if ok {
			chats = append(chats, communityChat(channel, community.OwnerID == userID, community.Collapsed))
		}
	}
	return mtproto.MakeTLMessagesChats(&mtproto.Messages_Chats{Chats: chats, Count: int32(len(chats))}).To_Messages_Chats(), nil
}

func (c *ApiFullCore) CommunitiesToggleCommunityCollapsedInDialogs(in *mtproto.TLCommunitiesToggleCommunityCollapsedInDialogs) (*mtproto.Updates, error) {
	userID, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if !domain.Ready() {
		return nil, communitiesUnavailable(c)
	}
	if in == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	community, _, err := c.resolveCommunity(in.GetCommunity())
	if err != nil {
		return nil, err
	}
	if err = domain.SetCommunityCollapsed(userID, community.ID, in.GetCollapsed()); err != nil {
		return nil, communityError(err)
	}
	return communityEmptyUpdates(), nil
}

func (c *ApiFullCore) CommunitiesGetPeerLinkRequests(in *mtproto.TLCommunitiesGetPeerLinkRequests) (*mtproto.Communities_PeerLinkRequests, error) {
	userID, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if !domain.Ready() {
		return nil, communitiesUnavailable(c)
	}
	if in == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	community, _, err := c.requireCommunityOwner(userID, in.GetCommunity())
	if err != nil {
		return nil, err
	}
	requests, err := domain.ListCommunityPeers(community.ID, true)
	if err != nil {
		return nil, mtproto.ErrInternalServerError
	}
	offset := 0
	if in.GetOffset() != "" {
		offset, err = strconv.Atoi(in.GetOffset())
		if err != nil || offset < 0 {
			return nil, mtproto.ErrOffsetInvalid
		}
	}
	limit := in.GetLimit()
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}
	total := int32(len(requests))
	if offset >= len(requests) {
		requests = nil
	} else {
		requests = requests[offset:]
	}
	if int32(len(requests)) > limit {
		requests = requests[:limit]
	}
	items := make([]*mtproto.CommunityPeerRequest, 0, len(requests))
	for _, request := range requests {
		items = append(items, mtproto.MakeTLCommunityPeerRequest(&mtproto.CommunityPeerRequest{
			Visible: request.Visible != nil && *request.Visible,
			Peer:    communityPeerFromRecord(request), RequestedBy: request.RequestedBy, Date: int32(request.RequestedAt),
		}).To_CommunityPeerRequest())
	}
	var next *wrapperspb.StringValue
	if offset+len(requests) < int(total) {
		next = wrapperspb.String(strconv.Itoa(offset + len(requests)))
	}
	chats, users := communityEntities(requests, userID)
	return mtproto.MakeTLCommunitiesPeerLinkRequests(&mtproto.Communities_PeerLinkRequests{
		TotalCount: total, Requests: items, NextOffset: next, Chats: chats, Users: users,
	}).To_Communities_PeerLinkRequests(), nil
}

func (c *ApiFullCore) CommunitiesTogglePeerLinkRequestApproval(in *mtproto.TLCommunitiesTogglePeerLinkRequestApproval) (*mtproto.Bool, error) {
	userID, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if !domain.Ready() {
		return nil, communitiesUnavailable(c)
	}
	if in == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	community, _, err := c.requireCommunityOwner(userID, in.GetCommunity())
	if err != nil {
		return nil, err
	}
	peerType, peerID, _, err := communityPeerType(userID, in.GetPeer())
	if err != nil {
		return nil, err
	}
	if err = domain.ApproveCommunityPeer(userID, community.ID, peerType, peerID, in.GetReject()); err != nil {
		return nil, communityError(err)
	}
	return mtproto.BoolTrue, nil
}

func (c *ApiFullCore) CommunitiesToggleAllPeerLinkRequestApproval(in *mtproto.TLCommunitiesToggleAllPeerLinkRequestApproval) (*mtproto.Bool, error) {
	userID, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if !domain.Ready() {
		return nil, communitiesUnavailable(c)
	}
	if in == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	community, _, err := c.requireCommunityOwner(userID, in.GetCommunity())
	if err != nil {
		return nil, err
	}
	if err = domain.ApproveAllCommunityPeers(userID, community.ID, in.GetReject()); err != nil {
		return nil, communityError(err)
	}
	return mtproto.BoolTrue, nil
}

func (c *ApiFullCore) CommunitiesToggleParticipantBanned(in *mtproto.TLCommunitiesToggleParticipantBanned) (*mtproto.Bool, error) {
	userID, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if !domain.Ready() {
		return nil, communitiesUnavailable(c)
	}
	if in == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	community, _, err := c.requireCommunityOwner(userID, in.GetCommunity())
	if err != nil {
		return nil, err
	}
	peerType, peerID, _, err := communityPeerType(userID, in.GetParticipant())
	if err != nil {
		return nil, err
	}
	if err = domain.BanCommunityPeer(userID, community.ID, peerType, peerID, in.GetUnban()); err != nil {
		return nil, communityError(err)
	}
	return mtproto.BoolTrue, nil
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
	participantType, participantID, _, err := communityPeerType(userID, in.GetParticipant())
	if err != nil || participantID <= 0 || participantType != domain.CommunityPeerUser {
		return nil, mtproto.ErrUserIdInvalid
	}
	if !domain.Ready() {
		return nil, mtproto.ErrMethodNotImpl
	}
	community, _, err := c.resolveCommunity(in.GetCommunity())
	if err != nil {
		return nil, err
	}
	peers, err := domain.ListCommunityPeers(community.ID, false)
	if err != nil {
		return nil, mtproto.ErrInternalServerError
	}
	creatorIDs := make([]int64, 0)
	joinedIDs := make([]int64, 0)
	chats := make([]*mtproto.Chat, 0)
	seen := make(map[int64]struct{})
	for _, peer := range peers {
		if peer.PeerType != domain.CommunityPeerChannel {
			continue
		}
		channel, ok, loadErr := domain.LoadChannel(peer.PeerID)
		if loadErr != nil || !ok {
			continue
		}
		member, memberErr := domain.ChannelIsMember(channel.ID, participantID)
		if memberErr != nil || !member {
			continue
		}
		if _, found := seen[channel.ID]; found {
			continue
		}
		seen[channel.ID] = struct{}{}
		if channel.Creator == participantID {
			creatorIDs = append(creatorIDs, channel.ID)
		} else {
			joinedIDs = append(joinedIDs, channel.ID)
		}
		chats = append(chats, channelChat(channel, channel.Creator == userID))
	}
	users := []*mtproto.User{mtproto.MakeTLUser(&mtproto.User{Id: participantID, Self: participantID == userID}).To_User()}
	return mtproto.MakeTLCommunitiesParticipantJoinedChats(&mtproto.Communities_ParticipantJoinedChats{
		CreatorChatIds: creatorIDs, JoinedChatIds: joinedIDs, Chats: chats, Users: users,
	}).To_Communities_ParticipantJoinedChats(), nil
}
