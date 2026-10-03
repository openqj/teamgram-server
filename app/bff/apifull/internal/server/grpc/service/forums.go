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

func (s *Service) MessagesGetForumTopics(ctx context.Context, request *mtproto.TLMessagesGetForumTopics) (*mtproto.Messages_ForumTopics, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("MessagesGetForumTopics - request: %s", request)
	r, err := c.MessagesGetForumTopics(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("MessagesGetForumTopics - reply: %s", r)
	return r, nil
}

func (s *Service) MessagesGetForumTopicsByID(ctx context.Context, request *mtproto.TLMessagesGetForumTopicsByID) (*mtproto.Messages_ForumTopics, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("MessagesGetForumTopicsByID - request: %s", request)
	r, err := c.MessagesGetForumTopicsByID(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("MessagesGetForumTopicsByID - reply: %s", r)
	return r, nil
}

func (s *Service) MessagesEditForumTopic(ctx context.Context, request *mtproto.TLMessagesEditForumTopic) (*mtproto.Updates, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("MessagesEditForumTopic - request: %s", request)
	r, err := c.MessagesEditForumTopic(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("MessagesEditForumTopic - reply: %s", r)
	return r, nil
}

func (s *Service) MessagesUpdatePinnedForumTopic(ctx context.Context, request *mtproto.TLMessagesUpdatePinnedForumTopic) (*mtproto.Updates, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("MessagesUpdatePinnedForumTopic - request: %s", request)
	r, err := c.MessagesUpdatePinnedForumTopic(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("MessagesUpdatePinnedForumTopic - reply: %s", r)
	return r, nil
}

func (s *Service) MessagesReorderPinnedForumTopics(ctx context.Context, request *mtproto.TLMessagesReorderPinnedForumTopics) (*mtproto.Updates, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("MessagesReorderPinnedForumTopics - request: %s", request)
	r, err := c.MessagesReorderPinnedForumTopics(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("MessagesReorderPinnedForumTopics - reply: %s", r)
	return r, nil
}

func (s *Service) MessagesCreateForumTopic(ctx context.Context, request *mtproto.TLMessagesCreateForumTopic) (*mtproto.Updates, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("MessagesCreateForumTopic - request: %s", request)
	r, err := c.MessagesCreateForumTopic(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("MessagesCreateForumTopic - reply: %s", r)
	return r, nil
}

func (s *Service) MessagesDeleteTopicHistory(ctx context.Context, request *mtproto.TLMessagesDeleteTopicHistory) (*mtproto.Messages_AffectedHistory, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("MessagesDeleteTopicHistory - request: %s", request)
	r, err := c.MessagesDeleteTopicHistory(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("MessagesDeleteTopicHistory - reply: %s", r)
	return r, nil
}

func (s *Service) ChannelsToggleForum(ctx context.Context, request *mtproto.TLChannelsToggleForum) (*mtproto.Updates, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("ChannelsToggleForum - request: %s", request)
	r, err := c.ChannelsToggleForum(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("ChannelsToggleForum - reply: %s", r)
	return r, nil
}

func (s *Service) ChannelsToggleViewForumAsMessages(ctx context.Context, request *mtproto.TLChannelsToggleViewForumAsMessages) (*mtproto.Updates, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("ChannelsToggleViewForumAsMessages - request: %s", request)
	r, err := c.ChannelsToggleViewForumAsMessages(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("ChannelsToggleViewForumAsMessages - reply: %s", r)
	return r, nil
}

func (s *Service) ChannelsCreateForumTopic(ctx context.Context, request *mtproto.TLChannelsCreateForumTopic) (*mtproto.Updates, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("ChannelsCreateForumTopic - request: %s", request)
	r, err := c.ChannelsCreateForumTopic(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("ChannelsCreateForumTopic - reply: %s", r)
	return r, nil
}

func (s *Service) ChannelsGetForumTopics(ctx context.Context, request *mtproto.TLChannelsGetForumTopics) (*mtproto.Messages_ForumTopics, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("ChannelsGetForumTopics - request: %s", request)
	r, err := c.ChannelsGetForumTopics(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("ChannelsGetForumTopics - reply: %s", r)
	return r, nil
}

func (s *Service) ChannelsGetForumTopicsByID(ctx context.Context, request *mtproto.TLChannelsGetForumTopicsByID) (*mtproto.Messages_ForumTopics, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("ChannelsGetForumTopicsByID - request: %s", request)
	r, err := c.ChannelsGetForumTopicsByID(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("ChannelsGetForumTopicsByID - reply: %s", r)
	return r, nil
}

func (s *Service) ChannelsEditForumTopic(ctx context.Context, request *mtproto.TLChannelsEditForumTopic) (*mtproto.Updates, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("ChannelsEditForumTopic - request: %s", request)
	r, err := c.ChannelsEditForumTopic(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("ChannelsEditForumTopic - reply: %s", r)
	return r, nil
}

func (s *Service) ChannelsUpdatePinnedForumTopic(ctx context.Context, request *mtproto.TLChannelsUpdatePinnedForumTopic) (*mtproto.Updates, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("ChannelsUpdatePinnedForumTopic - request: %s", request)
	r, err := c.ChannelsUpdatePinnedForumTopic(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("ChannelsUpdatePinnedForumTopic - reply: %s", r)
	return r, nil
}

func (s *Service) ChannelsDeleteTopicHistory(ctx context.Context, request *mtproto.TLChannelsDeleteTopicHistory) (*mtproto.Messages_AffectedHistory, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("ChannelsDeleteTopicHistory - request: %s", request)
	r, err := c.ChannelsDeleteTopicHistory(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("ChannelsDeleteTopicHistory - reply: %s", r)
	return r, nil
}

func (s *Service) ChannelsReorderPinnedForumTopics(ctx context.Context, request *mtproto.TLChannelsReorderPinnedForumTopics) (*mtproto.Updates, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("ChannelsReorderPinnedForumTopics - request: %s", request)
	r, err := c.ChannelsReorderPinnedForumTopics(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("ChannelsReorderPinnedForumTopics - reply: %s", r)
	return r, nil
}
