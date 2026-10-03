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

func (s *Service) MessagesSendVote(ctx context.Context, request *mtproto.TLMessagesSendVote) (*mtproto.Updates, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("MessagesSendVote - request: %s", request)
	r, err := c.MessagesSendVote(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("MessagesSendVote - reply: %s", r)
	return r, nil
}

func (s *Service) MessagesGetPollResults(ctx context.Context, request *mtproto.TLMessagesGetPollResults) (*mtproto.Updates, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("MessagesGetPollResults - request: %s", request)
	r, err := c.MessagesGetPollResults(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("MessagesGetPollResults - reply: %s", r)
	return r, nil
}

func (s *Service) MessagesGetPollVotes(ctx context.Context, request *mtproto.TLMessagesGetPollVotes) (*mtproto.Messages_VotesList, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("MessagesGetPollVotes - request: %s", request)
	r, err := c.MessagesGetPollVotes(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("MessagesGetPollVotes - reply: %s", r)
	return r, nil
}

func (s *Service) MessagesAddPollAnswer(ctx context.Context, request *mtproto.TLMessagesAddPollAnswer) (*mtproto.Updates, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("MessagesAddPollAnswer - request: %s", request)
	r, err := c.MessagesAddPollAnswer(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("MessagesAddPollAnswer - reply: %s", r)
	return r, nil
}

func (s *Service) MessagesDeletePollAnswer(ctx context.Context, request *mtproto.TLMessagesDeletePollAnswer) (*mtproto.Updates, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("MessagesDeletePollAnswer - request: %s", request)
	r, err := c.MessagesDeletePollAnswer(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("MessagesDeletePollAnswer - reply: %s", r)
	return r, nil
}

func (s *Service) MessagesGetUnreadPollVotes(ctx context.Context, request *mtproto.TLMessagesGetUnreadPollVotes) (*mtproto.Messages_Messages, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("MessagesGetUnreadPollVotes - request: %s", request)
	r, err := c.MessagesGetUnreadPollVotes(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("MessagesGetUnreadPollVotes - reply: %s", r)
	return r, nil
}

func (s *Service) MessagesReadPollVotes(ctx context.Context, request *mtproto.TLMessagesReadPollVotes) (*mtproto.Messages_AffectedHistory, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("MessagesReadPollVotes - request: %s", request)
	r, err := c.MessagesReadPollVotes(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("MessagesReadPollVotes - reply: %s", r)
	return r, nil
}
