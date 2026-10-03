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

func (s *Service) MessagesEditFactCheck(ctx context.Context, request *mtproto.TLMessagesEditFactCheck) (*mtproto.Updates, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("MessagesEditFactCheck - request: %s", request)
	r, err := c.MessagesEditFactCheck(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("MessagesEditFactCheck - reply: %s", r)
	return r, nil
}

func (s *Service) MessagesDeleteFactCheck(ctx context.Context, request *mtproto.TLMessagesDeleteFactCheck) (*mtproto.Updates, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("MessagesDeleteFactCheck - request: %s", request)
	r, err := c.MessagesDeleteFactCheck(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("MessagesDeleteFactCheck - reply: %s", r)
	return r, nil
}

func (s *Service) MessagesGetFactCheck(ctx context.Context, request *mtproto.TLMessagesGetFactCheck) (*mtproto.Vector_FactCheck, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("MessagesGetFactCheck - request: %s", request)
	r, err := c.MessagesGetFactCheck(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("MessagesGetFactCheck - reply: %s", r)
	return r, nil
}
