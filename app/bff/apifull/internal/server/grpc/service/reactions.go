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

func (s *Service) MessagesSendReaction(ctx context.Context, request *mtproto.TLMessagesSendReaction) (*mtproto.Updates, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("MessagesSendReaction - request: %s", request)
	r, err := c.MessagesSendReaction(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("MessagesSendReaction - reply: %s", r)
	return r, nil
}

func (s *Service) MessagesGetMessagesReactions(ctx context.Context, request *mtproto.TLMessagesGetMessagesReactions) (*mtproto.Updates, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("MessagesGetMessagesReactions - request: %s", request)
	r, err := c.MessagesGetMessagesReactions(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("MessagesGetMessagesReactions - reply: %s", r)
	return r, nil
}

func (s *Service) MessagesGetMessageReactionsList(ctx context.Context, request *mtproto.TLMessagesGetMessageReactionsList) (*mtproto.Messages_MessageReactionsList, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("MessagesGetMessageReactionsList - request: %s", request)
	r, err := c.MessagesGetMessageReactionsList(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("MessagesGetMessageReactionsList - reply: %s", r)
	return r, nil
}

func (s *Service) MessagesSetChatAvailableReactions(ctx context.Context, request *mtproto.TLMessagesSetChatAvailableReactions) (*mtproto.Updates, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("MessagesSetChatAvailableReactions - request: %s", request)
	r, err := c.MessagesSetChatAvailableReactions(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("MessagesSetChatAvailableReactions - reply: %s", r)
	return r, nil
}

func (s *Service) MessagesGetAvailableReactions(ctx context.Context, request *mtproto.TLMessagesGetAvailableReactions) (*mtproto.Messages_AvailableReactions, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("MessagesGetAvailableReactions - request: %s", request)
	r, err := c.MessagesGetAvailableReactions(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("MessagesGetAvailableReactions - reply: %s", r)
	return r, nil
}

func (s *Service) MessagesSetDefaultReaction(ctx context.Context, request *mtproto.TLMessagesSetDefaultReaction) (*mtproto.Bool, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("MessagesSetDefaultReaction - request: %s", request)
	r, err := c.MessagesSetDefaultReaction(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("MessagesSetDefaultReaction - reply: %s", r)
	return r, nil
}

func (s *Service) MessagesGetUnreadReactions(ctx context.Context, request *mtproto.TLMessagesGetUnreadReactions) (*mtproto.Messages_Messages, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("MessagesGetUnreadReactions - request: %s", request)
	r, err := c.MessagesGetUnreadReactions(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("MessagesGetUnreadReactions - reply: %s", r)
	return r, nil
}

func (s *Service) MessagesReadReactions(ctx context.Context, request *mtproto.TLMessagesReadReactions) (*mtproto.Messages_AffectedHistory, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("MessagesReadReactions - request: %s", request)
	r, err := c.MessagesReadReactions(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("MessagesReadReactions - reply: %s", r)
	return r, nil
}

func (s *Service) MessagesReportReaction(ctx context.Context, request *mtproto.TLMessagesReportReaction) (*mtproto.Bool, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("MessagesReportReaction - request: %s", request)
	r, err := c.MessagesReportReaction(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("MessagesReportReaction - reply: %s", r)
	return r, nil
}

func (s *Service) MessagesGetTopReactions(ctx context.Context, request *mtproto.TLMessagesGetTopReactions) (*mtproto.Messages_Reactions, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("MessagesGetTopReactions - request: %s", request)
	r, err := c.MessagesGetTopReactions(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("MessagesGetTopReactions - reply: %s", r)
	return r, nil
}

func (s *Service) MessagesGetRecentReactions(ctx context.Context, request *mtproto.TLMessagesGetRecentReactions) (*mtproto.Messages_Reactions, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("MessagesGetRecentReactions - request: %s", request)
	r, err := c.MessagesGetRecentReactions(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("MessagesGetRecentReactions - reply: %s", r)
	return r, nil
}

func (s *Service) MessagesClearRecentReactions(ctx context.Context, request *mtproto.TLMessagesClearRecentReactions) (*mtproto.Bool, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("MessagesClearRecentReactions - request: %s", request)
	r, err := c.MessagesClearRecentReactions(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("MessagesClearRecentReactions - reply: %s", r)
	return r, nil
}

func (s *Service) MessagesSendPaidReaction(ctx context.Context, request *mtproto.TLMessagesSendPaidReaction) (*mtproto.Updates, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("MessagesSendPaidReaction - request: %s", request)
	r, err := c.MessagesSendPaidReaction(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("MessagesSendPaidReaction - reply: %s", r)
	return r, nil
}

func (s *Service) MessagesTogglePaidReactionPrivacy(ctx context.Context, request *mtproto.TLMessagesTogglePaidReactionPrivacy) (*mtproto.Bool, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("MessagesTogglePaidReactionPrivacy - request: %s", request)
	r, err := c.MessagesTogglePaidReactionPrivacy(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("MessagesTogglePaidReactionPrivacy - reply: %s", r)
	return r, nil
}

func (s *Service) MessagesGetPaidReactionPrivacy(ctx context.Context, request *mtproto.TLMessagesGetPaidReactionPrivacy) (*mtproto.Updates, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("MessagesGetPaidReactionPrivacy - request: %s", request)
	r, err := c.MessagesGetPaidReactionPrivacy(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("MessagesGetPaidReactionPrivacy - reply: %s", r)
	return r, nil
}

func (s *Service) MessagesDeleteParticipantReactions(ctx context.Context, request *mtproto.TLMessagesDeleteParticipantReactions) (*mtproto.Bool, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("MessagesDeleteParticipantReactions - request: %s", request)
	r, err := c.MessagesDeleteParticipantReactions(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("MessagesDeleteParticipantReactions - reply: %s", r)
	return r, nil
}

func (s *Service) MessagesDeleteParticipantReaction(ctx context.Context, request *mtproto.TLMessagesDeleteParticipantReaction) (*mtproto.Updates, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("MessagesDeleteParticipantReaction - request: %s", request)
	r, err := c.MessagesDeleteParticipantReaction(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("MessagesDeleteParticipantReaction - reply: %s", r)
	return r, nil
}
