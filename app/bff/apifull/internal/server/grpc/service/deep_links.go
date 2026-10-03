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

func (s *Service) MessagesStartBot(ctx context.Context, request *mtproto.TLMessagesStartBot) (*mtproto.Updates, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("MessagesStartBot - request: %s", request)
	r, err := c.MessagesStartBot(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("MessagesStartBot - reply: %s", r)
	return r, nil
}

func (s *Service) HelpGetRecentMeUrls(ctx context.Context, request *mtproto.TLHelpGetRecentMeUrls) (*mtproto.Help_RecentMeUrls, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("HelpGetRecentMeUrls - request: %s", request)
	r, err := c.HelpGetRecentMeUrls(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("HelpGetRecentMeUrls - reply: %s", r)
	return r, nil
}

func (s *Service) HelpGetDeepLinkInfo(ctx context.Context, request *mtproto.TLHelpGetDeepLinkInfo) (*mtproto.Help_DeepLinkInfo, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("HelpGetDeepLinkInfo - request: %s", request)
	r, err := c.HelpGetDeepLinkInfo(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("HelpGetDeepLinkInfo - reply: %s", r)
	return r, nil
}
