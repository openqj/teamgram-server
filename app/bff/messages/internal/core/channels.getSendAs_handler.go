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
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/channelview"
	"github.com/teamgram/teamgram-server/app/bff/apifull/persist"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

// ChannelsGetSendAs
// channels.getSendAs#dc770ee peer:InputPeer = channels.SendAsPeers;
func (c *MessagesCore) ChannelsGetSendAs(in *mtproto.TLChannelsGetSendAs) (*mtproto.Channels_SendAsPeers, error) {
	if c == nil || c.MD == nil || c.MD.UserId <= 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	if in == nil || in.GetPeer() == nil {
		if c.Logger != nil {
			c.Logger.Errorf("channels.getSendAs - error: %v", mtproto.ErrPeerIdInvalid)
		}
		return nil, mtproto.ErrPeerIdInvalid
	}
	// Paid-reaction and live-story send-as eligibility has separate provider
	// state that is not represented by the channel member roster.
	if in.GetForPaidReactions() || in.GetForLiveStories() {
		return nil, mtproto.ErrMethodNotImpl
	}
	peer := mtproto.FromInputPeer2(c.MD.UserId, in.GetPeer())
	if !peer.IsUserOrChatOrChannel() || peer.PeerId == 0 {
		if c.Logger != nil {
			c.Logger.Errorf("channels.getSendAs - error: %v", mtproto.ErrPeerIdInvalid)
		}
		return nil, mtproto.ErrPeerIdInvalid
	}
	// A channel peer must be an actual APIFull channel and the caller must be
	// a member.  This also verifies the exact access hash before exposing any
	// send-as identities.  User/basic-chat peers only expose the caller itself.
	if in.GetPeer().GetPredicateName() == mtproto.Predicate_inputPeerChannel {
		if _, err := channelview.ValidateInputPeer(c.MD.UserId, in.GetPeer()); err != nil {
			return nil, err
		}
	}

	type pair struct {
		t  int32
		id int64
	}
	seen := map[pair]struct{}{}
	peers := make([]pair, 0, 2)
	add := func(t int32, id int64) {
		if t == mtproto.PEER_SELF {
			t = mtproto.PEER_USER
			if id == 0 {
				id = c.MD.UserId
			}
		}
		if id == 0 {
			return
		}
		k := pair{t, id}
		if _, ok := seen[k]; ok {
			return
		}
		seen[k] = struct{}{}
		peers = append(peers, k)
	}
	add(mtproto.PEER_USER, c.MD.UserId)

	// Load channels from the authoritative roster.  The provider filters out
	// ordinary members and banned users while retaining owners and admins with
	// posting rights.
	sendAsChannels := make(map[int64]*mtproto.Chat)
	available, err := channelview.SendAsChannels(c.MD.UserId)
	if err != nil {
		return nil, err
	}
	orderedChannels := make([]*mtproto.Chat, 0, len(available))
	for _, chat := range available {
		if chat == nil || chat.GetId() <= 0 || chat.GetPredicateName() != mtproto.Predicate_channel {
			continue
		}
		sendAsChannels[chat.GetId()] = chat
		orderedChannels = append(orderedChannels, chat)
	}

	// A caller's own channels are valid send-as identities for a channel peer.
	// User/basic-chat peers expose only the caller itself.
	if peer.PeerType == mtproto.PEER_CHANNEL {
		for _, chat := range orderedChannels {
			add(mtproto.PEER_CHANNEL, chat.GetId())
		}
	}

	raw, err := persist.Default.Get(fmt.Sprintf("default_send_as:%d:%d:%d", c.MD.UserId, peer.PeerType, peer.PeerId))
	if err != nil {
		return nil, err
	}
	if raw != "" {
		parts := strings.Split(raw, ":")
		if len(parts) == 2 {
			t, e1 := strconv.ParseInt(parts[0], 10, 32)
			id, e2 := strconv.ParseInt(parts[1], 10, 64)
			// The KV value is only a preference.  Do not expose an arbitrary
			// user/chat/channel identity unless the provider can prove it is
			// the caller's own channel (or the caller itself).
			if e1 == nil && e2 == nil && (int32(t) == mtproto.PEER_USER && id == c.MD.UserId || peer.PeerType == mtproto.PEER_CHANNEL && int32(t) == mtproto.PEER_CHANNEL && sendAsChannels[id] != nil) {
				add(int32(t), id)
			}
		}
	}

	sendAs := make([]*mtproto.SendAsPeer, 0, len(peers))
	legacy := make([]*mtproto.Peer, 0, len(peers))
	var userIDs []int64
	chats := make([]*mtproto.Chat, 0, len(sendAsChannels))
	for _, p := range peers {
		var mp *mtproto.Peer
		switch p.t {
		case mtproto.PEER_USER:
			mp = mtproto.MakePeerUser(p.id)
			userIDs = append(userIDs, p.id)
		case mtproto.PEER_CHANNEL:
			chat := sendAsChannels[p.id]
			if chat == nil {
				continue
			}
			mp = mtproto.MakePeerChannel(p.id)
			chats = append(chats, chat)
		default:
			continue
		}
		legacy = append(legacy, mp)
		sendAs = append(sendAs, mtproto.MakeTLSendAsPeer(&mtproto.SendAsPeer{Peer: mp}).To_SendAsPeer())
	}

	if c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.UserClient == nil {
		return nil, mtproto.ErrInternalServerError
	}
	users := make([]*mtproto.User, 0, len(userIDs))
	if len(userIDs) > 0 {
		ctx := c.ctx
		if ctx == nil {
			ctx = context.Background()
		}
		mUsers, uErr := c.svcCtx.Dao.UserClient.UserGetMutableUsers(ctx, &userpb.TLUserGetMutableUsers{Id: userIDs})
		if uErr != nil {
			if c.Logger != nil {
				c.Logger.Errorf("channels.getSendAs - error: %v", uErr)
			}
			return nil, uErr
		}
		if mUsers == nil {
			return nil, mtproto.ErrInternalServerError
		}
		got := mUsers.GetUserListByIdList(c.MD.UserId, userIDs...)
		if len(got) != len(userIDs) {
			return nil, mtproto.ErrInternalServerError
		}
		users = got
	}

	return mtproto.MakeTLChannelsSendAsPeers(&mtproto.Channels_SendAsPeers{
		Peers_VECTORPEER:       legacy,
		Peers_VECTORSENDASPEER: sendAs,
		Chats:                  chats,
		Users:                  users,
	}).To_Channels_SendAsPeers(), nil
}
