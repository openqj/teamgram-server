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

func (s *Service) AccountReportPeer(ctx context.Context, request *mtproto.TLAccountReportPeer) (*mtproto.Bool, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("AccountReportPeer - request: %s", request)
	r, err := c.AccountReportPeer(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("AccountReportPeer - reply: %s", r)
	return r, nil
}

func (s *Service) AccountReportProfilePhoto(ctx context.Context, request *mtproto.TLAccountReportProfilePhoto) (*mtproto.Bool, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("AccountReportProfilePhoto - request: %s", request)
	r, err := c.AccountReportProfilePhoto(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("AccountReportProfilePhoto - reply: %s", r)
	return r, nil
}

func (s *Service) MessagesReportSpam(ctx context.Context, request *mtproto.TLMessagesReportSpam) (*mtproto.Bool, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("MessagesReportSpam - request: %s", request)
	r, err := c.MessagesReportSpam(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("MessagesReportSpam - reply: %s", r)
	return r, nil
}

func (s *Service) MessagesReportFC78AF9B(ctx context.Context, request *mtproto.TLMessagesReportFC78AF9B) (*mtproto.ReportResult, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("MessagesReportFC78AF9B - request: %s", request)
	r, err := c.MessagesReportFC78AF9B(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("MessagesReportFC78AF9B - reply: %s", r)
	return r, nil
}

func (s *Service) MessagesReportEncryptedSpam(ctx context.Context, request *mtproto.TLMessagesReportEncryptedSpam) (*mtproto.Bool, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("MessagesReportEncryptedSpam - request: %s", request)
	r, err := c.MessagesReportEncryptedSpam(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("MessagesReportEncryptedSpam - reply: %s", r)
	return r, nil
}

func (s *Service) MessagesReportReadMetrics(ctx context.Context, request *mtproto.TLMessagesReportReadMetrics) (*mtproto.Bool, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("MessagesReportReadMetrics - request: %s", request)
	r, err := c.MessagesReportReadMetrics(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("MessagesReportReadMetrics - reply: %s", r)
	return r, nil
}

func (s *Service) MessagesReportMusicListen(ctx context.Context, request *mtproto.TLMessagesReportMusicListen) (*mtproto.Bool, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("MessagesReportMusicListen - request: %s", request)
	r, err := c.MessagesReportMusicListen(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("MessagesReportMusicListen - reply: %s", r)
	return r, nil
}

func (s *Service) ChannelsReportSpam(ctx context.Context, request *mtproto.TLChannelsReportSpam) (*mtproto.Bool, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("ChannelsReportSpam - request: %s", request)
	r, err := c.ChannelsReportSpam(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("ChannelsReportSpam - reply: %s", r)
	return r, nil
}

func (s *Service) MessagesReport8953AB4E(ctx context.Context, request *mtproto.TLMessagesReport8953AB4E) (*mtproto.Bool, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("MessagesReport8953AB4E - request: %s", request)
	r, err := c.MessagesReport8953AB4E(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("MessagesReport8953AB4E - reply: %s", r)
	return r, nil
}
