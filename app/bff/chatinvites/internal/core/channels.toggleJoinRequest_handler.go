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
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/channelview"
	chatpb "github.com/teamgram/teamgram-server/app/service/biz/chat/chat"
)

// ChannelsToggleJoinRequest
// channels.toggleJoinRequest#4c2985b6 channel:InputChannel enabled:Bool = Updates;
func (c *ChatInvitesCore) ChannelsToggleJoinRequest(in *mtproto.TLChannelsToggleJoinRequest) (*mtproto.Updates, error) {
	channel := in.GetChannel()
	if channel == nil || channel.GetPredicateName() == mtproto.Predicate_inputChannelEmpty || channel.GetChannelId() == 0 {
		c.Logger.Errorf("channels.toggleJoinRequest - error: channel invalid")
		return nil, mtproto.ErrChannelInvalid
	}
	channelId := channel.GetChannelId()
	handled, err := c.requireChannelInviteAdmin(channel)
	if err != nil {
		return nil, err
	}
	if handled {
		enabled := mtproto.FromBool(in.GetEnabled())
		invites, err := channelview.ExportedInvites(c.MD.UserId, channelId, c.MD.UserId, false, 0, "", 100)
		if err != nil {
			return nil, err
		}

		links := make([]string, 0, len(invites))
		for _, invite := range invites {
			if invite.GetLink() != "" {
				links = append(links, invite.GetLink())
			}
		}
		if len(links) == 0 {
			invite, err := channelview.ExportInvite(c.MD.UserId, channelId, channelview.InviteOptions{
				RequestNeeded: enabled,
			})
			if err != nil {
				return nil, err
			}
			links = append(links, invite.GetLink())
		}
		for _, link := range links {
			if _, err = channelview.EditExportedInvite(c.MD.UserId, channelId, link, channelview.InviteUpdate{
				RequestNeeded: &enabled,
			}); err != nil {
				return nil, err
			}
		}

		chat := mtproto.MakeTLChannel(&mtproto.Chat{
			Id:          channelId,
			Megagroup:   true,
			JoinRequest: enabled,
			Photo:       mtproto.MakeTLChatPhotoEmpty(nil).To_ChatPhoto(),
		}).To_Chat()
		update := mtproto.MakeTLUpdateChannel(&mtproto.Update{
			ChannelId: channelId,
		}).To_Update()
		return mtproto.MakeUpdatesByUpdatesChats([]*mtproto.Chat{chat}, update), nil
	}

	// Same admin gate as channels.toggleJoinToSend when the id is a basic chat.
	mChat, err := c.svcCtx.Dao.ChatClient.ChatGetMutableChat(c.ctx, &chatpb.TLChatGetMutableChat{
		ChatId: channelId,
	})
	if err == nil && mChat != nil && mChat.Id() != 0 {
		me, _ := mChat.GetImmutableChatParticipant(c.MD.UserId)
		if me == nil || !me.CanInviteUsers() {
			c.Logger.Errorf("channels.toggleJoinRequest - error: admin required")
			return nil, mtproto.ErrChatAdminRequired
		}
	}

	invites, err := c.svcCtx.Dao.ChatClient.ChatGetExportedChatInvites(c.ctx, &chatpb.TLChatGetExportedChatInvites{
		ChatId:  channelId,
		AdminId: c.MD.UserId,
		Limit:   100,
	})
	if err != nil {
		c.Logger.Errorf("channels.toggleJoinRequest - error: %v", err)
		return nil, err
	}

	links := make([]string, 0)
	for _, inv := range invites.GetDatas() {
		if inv.GetLink() != "" {
			links = append(links, inv.GetLink())
		}
	}
	if len(links) == 0 {
		exported, err := c.svcCtx.Dao.ChatClient.ChatExportChatInvite(c.ctx, &chatpb.TLChatExportChatInvite{
			ChatId:        channelId,
			AdminId:       c.MD.UserId,
			RequestNeeded: mtproto.FromBool(in.GetEnabled()),
		})
		if err != nil {
			c.Logger.Errorf("channels.toggleJoinRequest - error: %v", err)
			return nil, err
		}
		if exported.GetLink() == "" {
			c.Logger.Errorf("channels.toggleJoinRequest - error: empty invite link")
			return nil, mtproto.ErrInternalServerError
		}
		links = append(links, exported.GetLink())
	}

	for _, link := range links {
		if _, err = c.svcCtx.Dao.ChatClient.ChatEditExportedChatInvite(c.ctx, &chatpb.TLChatEditExportedChatInvite{
			SelfId:        c.MD.UserId,
			ChatId:        channelId,
			Link:          link,
			RequestNeeded: in.GetEnabled(),
		}); err != nil {
			c.Logger.Errorf("channels.toggleJoinRequest - error: %v", err)
			return nil, err
		}
	}

	enabled := mtproto.FromBool(in.GetEnabled())
	chat := mtproto.MakeTLChannel(&mtproto.Chat{
		Id:          channelId,
		Megagroup:   true,
		JoinRequest: enabled,
		Photo:       mtproto.MakeTLChatPhotoEmpty(nil).To_ChatPhoto(),
	}).To_Chat()
	update := mtproto.MakeTLUpdateChannel(&mtproto.Update{
		ChannelId: channelId,
	}).To_Update()

	return mtproto.MakeUpdatesByUpdatesChats([]*mtproto.Chat{chat}, update), nil
}
