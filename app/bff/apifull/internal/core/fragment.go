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
	"context"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/domain"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

// RPCFragmentServer: Layer 229 methods previously returned ERR_ENTERPRISE_IS_BLOCKED.
// Username / slug is stored locally under frag:<userId>. No Fragment HTTP call.

func (c *ApiFullCore) usernameContext() context.Context {
	ctx := c.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	if c.MD != nil {
		if outgoing, err := metadata.RpcMetadataToOutgoing(ctx, c.MD); err == nil {
			ctx = outgoing
		}
	}
	return ctx
}

func (c *ApiFullCore) usernameClient() (int64, interface {
	UserToggleUsername(context.Context, *userpb.TLUserToggleUsername) (*mtproto.Bool, error)
	UserReorderUsernames(context.Context, *userpb.TLUserReorderUsernames) (*mtproto.Bool, error)
	UserDeactivateAllChannelUsernames(context.Context, *userpb.TLUserDeactivateAllChannelUsernames) (*mtproto.Bool, error)
}, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return 0, nil, err
	}
	d := c.apifullDao()
	if d == nil || d.UserClient == nil {
		return 0, nil, mtproto.ErrMethodNotImpl
	}
	return uid, d.UserClient, nil
}

func usernameResult(result *mtproto.Bool, err error) (*mtproto.Bool, error) {
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, mtproto.ErrInternalServerError
	}
	return result, nil
}

func validUsernameActive(active *mtproto.Bool) bool {
	return active != nil && (active.GetPredicateName() == mtproto.Predicate_boolTrue || active.GetPredicateName() == mtproto.Predicate_boolFalse)
}

func (c *ApiFullCore) AccountReorderUsernames(in *mtproto.TLAccountReorderUsernames) (*mtproto.Bool, error) {
	uid, client, err := c.usernameClient()
	if err != nil {
		return nil, err
	}
	if in == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	return usernameResult(client.UserReorderUsernames(c.usernameContext(), &userpb.TLUserReorderUsernames{
		PeerType: mtproto.PEER_USER, PeerId: uid, UsernameList: in.GetOrder(),
	}))
}

func (c *ApiFullCore) AccountToggleUsername(in *mtproto.TLAccountToggleUsername) (*mtproto.Bool, error) {
	uid, client, err := c.usernameClient()
	if err != nil {
		return nil, err
	}
	if in == nil || in.GetUsername() == "" || !validUsernameActive(in.GetActive()) {
		return nil, mtproto.ErrInputRequestInvalid
	}
	return usernameResult(client.UserToggleUsername(c.usernameContext(), &userpb.TLUserToggleUsername{
		PeerType: mtproto.PEER_USER, PeerId: uid, Username: in.GetUsername(), Active: in.GetActive(),
	}))
}

func (c *ApiFullCore) ChannelsReorderUsernames(in *mtproto.TLChannelsReorderUsernames) (*mtproto.Bool, error) {
	uid, client, err := c.usernameClient()
	if err != nil {
		return nil, err
	}
	if in == nil || in.GetChannel() == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	channel, err := c.resolveMemberChannel(uid, in.GetChannel(), inputChannelID(in.GetChannel()))
	if err != nil {
		return nil, err
	}
	if _, ok, loadErr := domain.LoadChannelMember(channel.ID, uid); loadErr != nil {
		return nil, c.mapChannelMemberError(loadErr)
	} else if !ok {
		return nil, mtproto.ErrUserNotParticipant
	}
	return usernameResult(client.UserReorderUsernames(c.usernameContext(), &userpb.TLUserReorderUsernames{
		PeerType: mtproto.PEER_CHANNEL, PeerId: channel.ID, UsernameList: in.GetOrder(),
	}))
}

func (c *ApiFullCore) ChannelsToggleUsername(in *mtproto.TLChannelsToggleUsername) (*mtproto.Bool, error) {
	uid, client, err := c.usernameClient()
	if err != nil {
		return nil, err
	}
	if in == nil || in.GetChannel() == nil || in.GetUsername() == "" || !validUsernameActive(in.GetActive()) {
		return nil, mtproto.ErrInputRequestInvalid
	}
	channel, err := c.resolveMemberChannel(uid, in.GetChannel(), inputChannelID(in.GetChannel()))
	if err != nil {
		return nil, err
	}
	if _, ok, loadErr := domain.LoadChannelMember(channel.ID, uid); loadErr != nil {
		return nil, c.mapChannelMemberError(loadErr)
	} else if !ok {
		return nil, mtproto.ErrUserNotParticipant
	}
	return usernameResult(client.UserToggleUsername(c.usernameContext(), &userpb.TLUserToggleUsername{
		PeerType: mtproto.PEER_CHANNEL, PeerId: channel.ID, Username: in.GetUsername(), Active: in.GetActive(),
	}))
}

func (c *ApiFullCore) ChannelsDeactivateAllUsernames(in *mtproto.TLChannelsDeactivateAllUsernames) (*mtproto.Bool, error) {
	uid, client, err := c.usernameClient()
	if err != nil {
		return nil, err
	}
	if in == nil || in.GetChannel() == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	channel, err := c.resolveMemberChannel(uid, in.GetChannel(), inputChannelID(in.GetChannel()))
	if err != nil {
		return nil, err
	}
	member, ok, loadErr := domain.LoadChannelMember(channel.ID, uid)
	if loadErr != nil {
		return nil, c.mapChannelMemberError(loadErr)
	}
	if !ok {
		return nil, mtproto.ErrUserNotParticipant
	}
	if !member.Creator && (member.AdminRights == nil || !member.AdminRights.ChangeInfo) {
		return nil, mtproto.ErrChatAdminRequired
	}
	return usernameResult(client.UserDeactivateAllChannelUsernames(c.usernameContext(), &userpb.TLUserDeactivateAllChannelUsernames{ChannelId: channel.ID}))
}

func (c *ApiFullCore) BotsReorderUsernames(in *mtproto.TLBotsReorderUsernames) (*mtproto.Bool, error) {
	_, client, err := c.usernameClient()
	if err != nil {
		return nil, err
	}
	botID, err := c.resolveOwnedBot(inBotInput(in))
	if err != nil {
		return nil, err
	}
	return usernameResult(client.UserReorderUsernames(c.usernameContext(), &userpb.TLUserReorderUsernames{
		PeerType: mtproto.PEER_USER, PeerId: botID, UsernameList: in.GetOrder(),
	}))
}

func (c *ApiFullCore) BotsToggleUsername(in *mtproto.TLBotsToggleUsername) (*mtproto.Bool, error) {
	_, client, err := c.usernameClient()
	if err != nil {
		return nil, err
	}
	if in == nil || in.GetUsername() == "" || !validUsernameActive(in.GetActive()) {
		return nil, mtproto.ErrInputRequestInvalid
	}
	botID, err := c.resolveOwnedBot(in.GetBot())
	if err != nil {
		return nil, err
	}
	return usernameResult(client.UserToggleUsername(c.usernameContext(), &userpb.TLUserToggleUsername{
		PeerType: mtproto.PEER_USER, PeerId: botID, Username: in.GetUsername(), Active: in.GetActive(),
	}))
}

func inBotInput(in *mtproto.TLBotsReorderUsernames) *mtproto.InputUser {
	if in == nil {
		return nil
	}
	return in.GetBot()
}

func (c *ApiFullCore) resolveOwnedBot(input *mtproto.InputUser) (int64, error) {
	if input == nil || input.GetUserId() <= 0 || input.GetAccessHash() == 0 {
		return 0, mtproto.ErrUserIdInvalid
	}
	d := c.apifullDao()
	if d == nil || d.UserClient == nil {
		return 0, mtproto.ErrMethodNotImpl
	}
	bots, err := d.UserClient.UserGetCreatedBots(c.usernameContext())
	if err != nil {
		return 0, err
	}
	for _, bot := range bots.GetDatas() {
		if bot == nil || bot.GetUser() == nil || bot.GetUser().GetId() != input.GetUserId() {
			continue
		}
		if bot.GetUser().GetAccessHash() != input.GetAccessHash() {
			return 0, mtproto.ErrUserIdInvalid
		}
		if bot.GetUser().GetBot() == nil {
			return 0, mtproto.ErrBotInvalid
		}
		return input.GetUserId(), nil
	}
	return 0, mtproto.ErrBotInvalid
}

func (c *ApiFullCore) FragmentGetCollectibleInfo(in *mtproto.TLFragmentGetCollectibleInfo) (*mtproto.Fragment_CollectibleInfo, error) {
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	return nil, mtproto.ErrMethodNotImpl
}
