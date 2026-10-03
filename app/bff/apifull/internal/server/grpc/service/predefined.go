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

func (s *Service) PredefinedCreatePredefinedUser(ctx context.Context, request *mtproto.TLPredefinedCreatePredefinedUser) (*mtproto.PredefinedUser, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("PredefinedCreatePredefinedUser - request: %s", request)
	r, err := c.PredefinedCreatePredefinedUser(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("PredefinedCreatePredefinedUser - reply: %s", r)
	return r, nil
}

func (s *Service) PredefinedUpdatePredefinedUsername(ctx context.Context, request *mtproto.TLPredefinedUpdatePredefinedUsername) (*mtproto.PredefinedUser, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("PredefinedUpdatePredefinedUsername - request: %s", request)
	r, err := c.PredefinedUpdatePredefinedUsername(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("PredefinedUpdatePredefinedUsername - reply: %s", r)
	return r, nil
}

func (s *Service) PredefinedUpdatePredefinedProfile(ctx context.Context, request *mtproto.TLPredefinedUpdatePredefinedProfile) (*mtproto.PredefinedUser, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("PredefinedUpdatePredefinedProfile - request: %s", request)
	r, err := c.PredefinedUpdatePredefinedProfile(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("PredefinedUpdatePredefinedProfile - reply: %s", r)
	return r, nil
}

func (s *Service) PredefinedUpdatePredefinedVerified(ctx context.Context, request *mtproto.TLPredefinedUpdatePredefinedVerified) (*mtproto.PredefinedUser, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("PredefinedUpdatePredefinedVerified - request: %s", request)
	r, err := c.PredefinedUpdatePredefinedVerified(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("PredefinedUpdatePredefinedVerified - reply: %s", r)
	return r, nil
}

func (s *Service) PredefinedUpdatePredefinedCode(ctx context.Context, request *mtproto.TLPredefinedUpdatePredefinedCode) (*mtproto.PredefinedUser, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("PredefinedUpdatePredefinedCode - request: %s", request)
	r, err := c.PredefinedUpdatePredefinedCode(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("PredefinedUpdatePredefinedCode - reply: %s", r)
	return r, nil
}

func (s *Service) PredefinedGetPredefinedUser(ctx context.Context, request *mtproto.TLPredefinedGetPredefinedUser) (*mtproto.PredefinedUser, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("PredefinedGetPredefinedUser - request: %s", request)
	r, err := c.PredefinedGetPredefinedUser(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("PredefinedGetPredefinedUser - reply: %s", r)
	return r, nil
}

func (s *Service) PredefinedGetPredefinedUsers(ctx context.Context, request *mtproto.TLPredefinedGetPredefinedUsers) (*mtproto.Vector_PredefinedUser, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("PredefinedGetPredefinedUsers - request: %s", request)
	r, err := c.PredefinedGetPredefinedUsers(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("PredefinedGetPredefinedUsers - reply: %s", r)
	return r, nil
}
