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
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/domain"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

// RPCFragmentServer: Layer 229 methods previously returned ERR_ENTERPRISE_IS_BLOCKED.

func (c *ApiFullCore) AccountReorderUsernames(in *mtproto.TLAccountReorderUsernames) (*mtproto.Bool, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	return c.reorderPeerUsernames(mtproto.PEER_USER, uid, in.GetOrder())
}

func (c *ApiFullCore) AccountToggleUsername(in *mtproto.TLAccountToggleUsername) (*mtproto.Bool, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	return c.togglePeerUsername(mtproto.PEER_USER, uid, in.GetUsername(), in.GetActive())
}

func (c *ApiFullCore) ChannelsReorderUsernames(in *mtproto.TLChannelsReorderUsernames) (*mtproto.Bool, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	channelID, err := c.usernameChannelID(uid, in.GetChannel())
	if err != nil {
		return nil, err
	}
	return c.reorderPeerUsernames(mtproto.PEER_CHANNEL, channelID, in.GetOrder())
}

func (c *ApiFullCore) ChannelsToggleUsername(in *mtproto.TLChannelsToggleUsername) (*mtproto.Bool, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	channelID, err := c.usernameChannelID(uid, in.GetChannel())
	if err != nil {
		return nil, err
	}
	return c.togglePeerUsername(mtproto.PEER_CHANNEL, channelID, in.GetUsername(), in.GetActive())
}

func (c *ApiFullCore) ChannelsDeactivateAllUsernames(in *mtproto.TLChannelsDeactivateAllUsernames) (*mtproto.Bool, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	channelID, err := c.usernameChannelID(uid, in.GetChannel())
	if err != nil {
		return nil, err
	}
	ctx, err := c.usernameContext()
	if err != nil {
		return nil, err
	}
	result, err := c.svcCtx.Dao.UserDeactivateAllChannelUsernames(ctx, &userpb.TLUserDeactivateAllChannelUsernames{ChannelId: channelID})
	return usernameMutationResult(result, err)
}

func (c *ApiFullCore) BotsReorderUsernames(in *mtproto.TLBotsReorderUsernames) (*mtproto.Bool, error) {
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	if in == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	botID, err := c.usernameBotID(in.GetBot())
	if err != nil {
		return nil, err
	}
	return c.reorderPeerUsernames(mtproto.PEER_USER, botID, in.GetOrder())
}

func (c *ApiFullCore) BotsToggleUsername(in *mtproto.TLBotsToggleUsername) (*mtproto.Bool, error) {
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	if in == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	botID, err := c.usernameBotID(in.GetBot())
	if err != nil {
		return nil, err
	}
	return c.togglePeerUsername(mtproto.PEER_USER, botID, in.GetUsername(), in.GetActive())
}

func (c *ApiFullCore) usernameContext() (context.Context, error) {
	d := c.apifullDao()
	if d == nil || d.UserClient == nil {
		return nil, mtproto.ErrMethodNotImpl
	}
	return metadata.RpcMetadataToOutgoing(c.secretContext(), c.MD)
}

func usernameMutationResult(result *mtproto.Bool, err error) (*mtproto.Bool, error) {
	if err != nil {
		return nil, err
	}
	if result == nil || (result.GetPredicateName() != mtproto.Predicate_boolTrue && result.GetPredicateName() != mtproto.Predicate_boolFalse) {
		return nil, mtproto.ErrInternalServerError
	}
	return result, nil
}

func (c *ApiFullCore) togglePeerUsername(peerType int32, peerID int64, name string, active *mtproto.Bool) (*mtproto.Bool, error) {
	if name == "" {
		return nil, mtproto.ErrUsernameInvalid
	}
	if active == nil || (active.GetPredicateName() != mtproto.Predicate_boolTrue && active.GetPredicateName() != mtproto.Predicate_boolFalse) {
		return nil, mtproto.ErrInputRequestInvalid
	}
	ctx, err := c.usernameContext()
	if err != nil {
		return nil, err
	}
	result, err := c.svcCtx.Dao.UserToggleUsername(ctx, &userpb.TLUserToggleUsername{
		PeerType: peerType, PeerId: peerID, Username: name, Active: active,
	})
	return usernameMutationResult(result, err)
}

func (c *ApiFullCore) reorderPeerUsernames(peerType int32, peerID int64, order []string) (*mtproto.Bool, error) {
	ctx, err := c.usernameContext()
	if err != nil {
		return nil, err
	}
	result, err := c.svcCtx.Dao.UserReorderUsernames(ctx, &userpb.TLUserReorderUsernames{
		PeerType: peerType, PeerId: peerID, UsernameList: order,
	})
	return usernameMutationResult(result, err)
}

func (c *ApiFullCore) usernameChannelID(uid int64, input *mtproto.InputChannel) (int64, error) {
	if input == nil || input.GetPredicateName() != mtproto.Predicate_inputChannel || input.GetChannelId() <= 0 || input.GetAccessHash() == 0 {
		return 0, mtproto.ErrChannelInvalid
	}
	channel, found, err := domain.LoadChannel(input.GetChannelId())
	if err != nil {
		return 0, err
	}
	if !found || channel.AccessHash != input.GetAccessHash() {
		return 0, mtproto.ErrChannelInvalid
	}
	if channel.Creator != uid {
		member, found, err := domain.LoadChannelMember(channel.ID, uid)
		if err != nil {
			return 0, err
		}
		if !found || (member.BannedRights.Active(time.Now().Unix()) && member.BannedRights.Kicks(time.Now().Unix())) {
			return 0, mtproto.ErrUserNotParticipant
		}
		if member.AdminRights == nil || !member.AdminRights.ChangeInfo {
			return 0, mtproto.ErrChatAdminRequired
		}
	}
	return channel.ID, nil
}

func (c *ApiFullCore) usernameBotID(input *mtproto.InputUser) (int64, error) {
	if input == nil || input.GetPredicateName() != mtproto.Predicate_inputUser || input.GetUserId() <= 0 || input.GetAccessHash() == 0 {
		return 0, mtproto.ErrUserIdInvalid
	}
	ctx, err := c.usernameContext()
	if err != nil {
		return 0, err
	}
	bots, err := c.svcCtx.Dao.UserGetCreatedBots(ctx)
	if err != nil {
		return 0, err
	}
	if bots == nil {
		return 0, mtproto.ErrInternalServerError
	}
	for _, bot := range bots.GetDatas() {
		if bot.GetUser().GetId() != input.GetUserId() {
			continue
		}
		if bot.GetUser().GetAccessHash() != input.GetAccessHash() {
			return 0, mtproto.ErrUserIdInvalid
		}
		if bot.GetUser().GetDeleted() || bot.GetUser().GetBot() == nil {
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
