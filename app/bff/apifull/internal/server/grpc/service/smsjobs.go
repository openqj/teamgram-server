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

func (s *Service) SmsjobsIsEligibleToJoin(ctx context.Context, request *mtproto.TLSmsjobsIsEligibleToJoin) (*mtproto.Smsjobs_EligibilityToJoin, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("SmsjobsIsEligibleToJoin - request: %s", request)
	r, err := c.SmsjobsIsEligibleToJoin(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("SmsjobsIsEligibleToJoin - reply: %s", r)
	return r, nil
}

func (s *Service) SmsjobsJoin(ctx context.Context, request *mtproto.TLSmsjobsJoin) (*mtproto.Bool, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("SmsjobsJoin - request: %s", request)
	r, err := c.SmsjobsJoin(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("SmsjobsJoin - reply: %s", r)
	return r, nil
}

func (s *Service) SmsjobsLeave(ctx context.Context, request *mtproto.TLSmsjobsLeave) (*mtproto.Bool, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("SmsjobsLeave - request: %s", request)
	r, err := c.SmsjobsLeave(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("SmsjobsLeave - reply: %s", r)
	return r, nil
}

func (s *Service) SmsjobsUpdateSettings(ctx context.Context, request *mtproto.TLSmsjobsUpdateSettings) (*mtproto.Bool, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("SmsjobsUpdateSettings - request: %s", request)
	r, err := c.SmsjobsUpdateSettings(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("SmsjobsUpdateSettings - reply: %s", r)
	return r, nil
}

func (s *Service) SmsjobsGetStatus(ctx context.Context, request *mtproto.TLSmsjobsGetStatus) (*mtproto.Smsjobs_Status, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("SmsjobsGetStatus - request: %s", request)
	r, err := c.SmsjobsGetStatus(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("SmsjobsGetStatus - reply: %s", r)
	return r, nil
}

func (s *Service) SmsjobsGetSmsJob(ctx context.Context, request *mtproto.TLSmsjobsGetSmsJob) (*mtproto.SmsJob, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("SmsjobsGetSmsJob - request: %s", request)
	r, err := c.SmsjobsGetSmsJob(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("SmsjobsGetSmsJob - reply: %s", r)
	return r, nil
}

func (s *Service) SmsjobsFinishJob(ctx context.Context, request *mtproto.TLSmsjobsFinishJob) (*mtproto.Bool, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("SmsjobsFinishJob - request: %s", request)
	r, err := c.SmsjobsFinishJob(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("SmsjobsFinishJob - reply: %s", r)
	return r, nil
}
