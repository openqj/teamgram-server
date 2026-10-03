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

package service

import (
	"context"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/core"
)

func (s *Service) MessagesGetPersonalChannelHistory(ctx context.Context, request *mtproto.TLMessagesGetPersonalChannelHistory) (*mtproto.Messages_Messages, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("MessagesGetPersonalChannelHistory - request: %s", request)
	r, err := c.MessagesGetPersonalChannelHistory(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("MessagesGetPersonalChannelHistory - reply: %s", r)
	return r, nil
}

func (s *Service) ChannelsReadHistory(ctx context.Context, request *mtproto.TLChannelsReadHistory) (*mtproto.Bool, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("ChannelsReadHistory - request: %s", request)
	r, err := c.ChannelsReadHistory(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("ChannelsReadHistory - reply: %s", r)
	return r, nil
}

func (s *Service) ChannelsDeleteMessages(ctx context.Context, request *mtproto.TLChannelsDeleteMessages) (*mtproto.Messages_AffectedMessages, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("ChannelsDeleteMessages - request: %s", request)
	r, err := c.ChannelsDeleteMessages(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("ChannelsDeleteMessages - reply: %s", r)
	return r, nil
}

func (s *Service) ChannelsGetMessages(ctx context.Context, request *mtproto.TLChannelsGetMessages) (*mtproto.Messages_Messages, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("ChannelsGetMessages - request: %s", request)
	r, err := c.ChannelsGetMessages(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("ChannelsGetMessages - reply: %s", r)
	return r, nil
}

func (s *Service) ChannelsGetParticipants(ctx context.Context, request *mtproto.TLChannelsGetParticipants) (*mtproto.Channels_ChannelParticipants, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("ChannelsGetParticipants - request: %s", request)
	r, err := c.ChannelsGetParticipants(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("ChannelsGetParticipants - reply: %s", r)
	return r, nil
}

func (s *Service) ChannelsGetParticipant(ctx context.Context, request *mtproto.TLChannelsGetParticipant) (*mtproto.Channels_ChannelParticipant, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("ChannelsGetParticipant - request: %s", request)
	r, err := c.ChannelsGetParticipant(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("ChannelsGetParticipant - reply: %s", r)
	return r, nil
}

func (s *Service) ChannelsGetChannels(ctx context.Context, request *mtproto.TLChannelsGetChannels) (*mtproto.Messages_Chats, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("ChannelsGetChannels - request: %s", request)
	r, err := c.ChannelsGetChannels(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("ChannelsGetChannels - reply: %s", r)
	return r, nil
}

func (s *Service) ChannelsGetFullChannel(ctx context.Context, request *mtproto.TLChannelsGetFullChannel) (*mtproto.Messages_ChatFull, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("ChannelsGetFullChannel - request: %s", request)
	r, err := c.ChannelsGetFullChannel(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("ChannelsGetFullChannel - reply: %s", r)
	return r, nil
}

func (s *Service) ChannelsCreateChannel(ctx context.Context, request *mtproto.TLChannelsCreateChannel) (*mtproto.Updates, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("ChannelsCreateChannel - request: %s", request)
	r, err := c.ChannelsCreateChannel(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("ChannelsCreateChannel - reply: %s", r)
	return r, nil
}

func (s *Service) ChannelsEditAdmin(ctx context.Context, request *mtproto.TLChannelsEditAdmin) (*mtproto.Updates, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("ChannelsEditAdmin - request: %s", request)
	r, err := c.ChannelsEditAdmin(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("ChannelsEditAdmin - reply: %s", r)
	return r, nil
}

func (s *Service) ChannelsEditTitle(ctx context.Context, request *mtproto.TLChannelsEditTitle) (*mtproto.Updates, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("ChannelsEditTitle - request: %s", request)
	r, err := c.ChannelsEditTitle(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("ChannelsEditTitle - reply: %s", r)
	return r, nil
}

func (s *Service) ChannelsEditPhoto(ctx context.Context, request *mtproto.TLChannelsEditPhoto) (*mtproto.Updates, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("ChannelsEditPhoto - request: %s", request)
	r, err := c.ChannelsEditPhoto(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("ChannelsEditPhoto - reply: %s", r)
	return r, nil
}

func (s *Service) ChannelsJoinChannel7F6A1E22(ctx context.Context, request *mtproto.TLChannelsJoinChannel7F6A1E22) (*mtproto.Messages_ChatInviteJoinResult, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("ChannelsJoinChannel7F6A1E22 - request: %s", request)
	r, err := c.ChannelsJoinChannel7F6A1E22(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("ChannelsJoinChannel7F6A1E22 - reply: %s", r)
	return r, nil
}

func (s *Service) ChannelsLeaveChannel(ctx context.Context, request *mtproto.TLChannelsLeaveChannel) (*mtproto.Updates, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("ChannelsLeaveChannel - request: %s", request)
	r, err := c.ChannelsLeaveChannel(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("ChannelsLeaveChannel - reply: %s", r)
	return r, nil
}

func (s *Service) ChannelsInviteToChannelC9E33D54(ctx context.Context, request *mtproto.TLChannelsInviteToChannelC9E33D54) (*mtproto.Messages_InvitedUsers, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("ChannelsInviteToChannelC9E33D54 - request: %s", request)
	r, err := c.ChannelsInviteToChannelC9E33D54(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("ChannelsInviteToChannelC9E33D54 - reply: %s", r)
	return r, nil
}

func (s *Service) ChannelsDeleteChannel(ctx context.Context, request *mtproto.TLChannelsDeleteChannel) (*mtproto.Updates, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("ChannelsDeleteChannel - request: %s", request)
	r, err := c.ChannelsDeleteChannel(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("ChannelsDeleteChannel - reply: %s", r)
	return r, nil
}

func (s *Service) ChannelsExportMessageLink(ctx context.Context, request *mtproto.TLChannelsExportMessageLink) (*mtproto.ExportedMessageLink, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("ChannelsExportMessageLink - request: %s", request)
	r, err := c.ChannelsExportMessageLink(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("ChannelsExportMessageLink - reply: %s", r)
	return r, nil
}

func (s *Service) ChannelsToggleSignatures(ctx context.Context, request *mtproto.TLChannelsToggleSignatures) (*mtproto.Updates, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("ChannelsToggleSignatures - request: %s", request)
	r, err := c.ChannelsToggleSignatures(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("ChannelsToggleSignatures - reply: %s", r)
	return r, nil
}

func (s *Service) ChannelsGetAdminedPublicChannels(ctx context.Context, request *mtproto.TLChannelsGetAdminedPublicChannels) (*mtproto.Messages_Chats, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("ChannelsGetAdminedPublicChannels - request: %s", request)
	r, err := c.ChannelsGetAdminedPublicChannels(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("ChannelsGetAdminedPublicChannels - reply: %s", r)
	return r, nil
}

func (s *Service) ChannelsEditBanned(ctx context.Context, request *mtproto.TLChannelsEditBanned) (*mtproto.Updates, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("ChannelsEditBanned - request: %s", request)
	r, err := c.ChannelsEditBanned(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("ChannelsEditBanned - reply: %s", r)
	return r, nil
}

func (s *Service) ChannelsGetAdminLog(ctx context.Context, request *mtproto.TLChannelsGetAdminLog) (*mtproto.Channels_AdminLogResults, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("ChannelsGetAdminLog - request: %s", request)
	r, err := c.ChannelsGetAdminLog(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("ChannelsGetAdminLog - reply: %s", r)
	return r, nil
}

func (s *Service) ChannelsSetStickers(ctx context.Context, request *mtproto.TLChannelsSetStickers) (*mtproto.Bool, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("ChannelsSetStickers - request: %s", request)
	r, err := c.ChannelsSetStickers(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("ChannelsSetStickers - reply: %s", r)
	return r, nil
}

func (s *Service) ChannelsReadMessageContents(ctx context.Context, request *mtproto.TLChannelsReadMessageContents) (*mtproto.Bool, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("ChannelsReadMessageContents - request: %s", request)
	r, err := c.ChannelsReadMessageContents(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("ChannelsReadMessageContents - reply: %s", r)
	return r, nil
}

func (s *Service) ChannelsDeleteHistory9BAA9647(ctx context.Context, request *mtproto.TLChannelsDeleteHistory9BAA9647) (*mtproto.Updates, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("ChannelsDeleteHistory9BAA9647 - request: %s", request)
	r, err := c.ChannelsDeleteHistory9BAA9647(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("ChannelsDeleteHistory9BAA9647 - reply: %s", r)
	return r, nil
}

func (s *Service) ChannelsTogglePreHistoryHidden(ctx context.Context, request *mtproto.TLChannelsTogglePreHistoryHidden) (*mtproto.Updates, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("ChannelsTogglePreHistoryHidden - request: %s", request)
	r, err := c.ChannelsTogglePreHistoryHidden(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("ChannelsTogglePreHistoryHidden - reply: %s", r)
	return r, nil
}

func (s *Service) ChannelsGetGroupsForDiscussion(ctx context.Context, request *mtproto.TLChannelsGetGroupsForDiscussion) (*mtproto.Messages_Chats, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("ChannelsGetGroupsForDiscussion - request: %s", request)
	r, err := c.ChannelsGetGroupsForDiscussion(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("ChannelsGetGroupsForDiscussion - reply: %s", r)
	return r, nil
}

func (s *Service) ChannelsSetDiscussionGroup(ctx context.Context, request *mtproto.TLChannelsSetDiscussionGroup) (*mtproto.Bool, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("ChannelsSetDiscussionGroup - request: %s", request)
	r, err := c.ChannelsSetDiscussionGroup(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("ChannelsSetDiscussionGroup - reply: %s", r)
	return r, nil
}

func (s *Service) ChannelsEditLocation(ctx context.Context, request *mtproto.TLChannelsEditLocation) (*mtproto.Bool, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("ChannelsEditLocation - request: %s", request)
	r, err := c.ChannelsEditLocation(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("ChannelsEditLocation - reply: %s", r)
	return r, nil
}

func (s *Service) ChannelsToggleSlowMode(ctx context.Context, request *mtproto.TLChannelsToggleSlowMode) (*mtproto.Updates, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("ChannelsToggleSlowMode - request: %s", request)
	r, err := c.ChannelsToggleSlowMode(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("ChannelsToggleSlowMode - reply: %s", r)
	return r, nil
}

func (s *Service) ChannelsGetInactiveChannels(ctx context.Context, request *mtproto.TLChannelsGetInactiveChannels) (*mtproto.Messages_InactiveChats, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("ChannelsGetInactiveChannels - request: %s", request)
	r, err := c.ChannelsGetInactiveChannels(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("ChannelsGetInactiveChannels - reply: %s", r)
	return r, nil
}

func (s *Service) ChannelsDeleteParticipantHistory(ctx context.Context, request *mtproto.TLChannelsDeleteParticipantHistory) (*mtproto.Messages_AffectedHistory, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("ChannelsDeleteParticipantHistory - request: %s", request)
	r, err := c.ChannelsDeleteParticipantHistory(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("ChannelsDeleteParticipantHistory - reply: %s", r)
	return r, nil
}

func (s *Service) ChannelsToggleParticipantsHidden(ctx context.Context, request *mtproto.TLChannelsToggleParticipantsHidden) (*mtproto.Updates, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("ChannelsToggleParticipantsHidden - request: %s", request)
	r, err := c.ChannelsToggleParticipantsHidden(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("ChannelsToggleParticipantsHidden - reply: %s", r)
	return r, nil
}

func (s *Service) ChannelsJoinChannel24B524C5(ctx context.Context, request *mtproto.TLChannelsJoinChannel24B524C5) (*mtproto.Updates, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("ChannelsJoinChannel24B524C5 - request: %s", request)
	r, err := c.ChannelsJoinChannel24B524C5(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("ChannelsJoinChannel24B524C5 - reply: %s", r)
	return r, nil
}

func (s *Service) ChannelsEditCreator(ctx context.Context, request *mtproto.TLChannelsEditCreator) (*mtproto.Updates, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("ChannelsEditCreator - request: %s", request)
	r, err := c.ChannelsEditCreator(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("ChannelsEditCreator - reply: %s", r)
	return r, nil
}

func (s *Service) ChannelsGetFutureCreatorAfterLeave(ctx context.Context, request *mtproto.TLChannelsGetFutureCreatorAfterLeave) (*mtproto.User, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("ChannelsGetFutureCreatorAfterLeave - request: %s", request)
	r, err := c.ChannelsGetFutureCreatorAfterLeave(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("ChannelsGetFutureCreatorAfterLeave - reply: %s", r)
	return r, nil
}

func (s *Service) ChannelsInviteToChannel199F3A6C(ctx context.Context, request *mtproto.TLChannelsInviteToChannel199F3A6C) (*mtproto.Updates, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("ChannelsInviteToChannel199F3A6C - request: %s", request)
	r, err := c.ChannelsInviteToChannel199F3A6C(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("ChannelsInviteToChannel199F3A6C - reply: %s", r)
	return r, nil
}

func (s *Service) ChannelsDeleteHistoryAF369D42(ctx context.Context, request *mtproto.TLChannelsDeleteHistoryAF369D42) (*mtproto.Bool, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("ChannelsDeleteHistoryAF369D42 - request: %s", request)
	r, err := c.ChannelsDeleteHistoryAF369D42(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("ChannelsDeleteHistoryAF369D42 - reply: %s", r)
	return r, nil
}
