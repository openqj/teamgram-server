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

func (s *Service) MessagesDeletePhoneCallHistory(ctx context.Context, request *mtproto.TLMessagesDeletePhoneCallHistory) (*mtproto.Messages_AffectedFoundMessages, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("MessagesDeletePhoneCallHistory - request: %s", request)
	r, err := c.MessagesDeletePhoneCallHistory(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("MessagesDeletePhoneCallHistory - reply: %s", r)
	return r, nil
}

func (s *Service) PhoneGetCallConfig(ctx context.Context, request *mtproto.TLPhoneGetCallConfig) (*mtproto.DataJSON, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("PhoneGetCallConfig - request: %s", request)
	r, err := c.PhoneGetCallConfig(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("PhoneGetCallConfig - reply: %s", r)
	return r, nil
}

func (s *Service) PhoneRequestCall(ctx context.Context, request *mtproto.TLPhoneRequestCall) (*mtproto.Phone_PhoneCall, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("PhoneRequestCall - request: %s", request)
	r, err := c.PhoneRequestCall(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("PhoneRequestCall - reply: %s", r)
	return r, nil
}

func (s *Service) PhoneAcceptCall(ctx context.Context, request *mtproto.TLPhoneAcceptCall) (*mtproto.Phone_PhoneCall, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("PhoneAcceptCall - request: %s", request)
	r, err := c.PhoneAcceptCall(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("PhoneAcceptCall - reply: %s", r)
	return r, nil
}

func (s *Service) PhoneConfirmCall(ctx context.Context, request *mtproto.TLPhoneConfirmCall) (*mtproto.Phone_PhoneCall, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("PhoneConfirmCall - request: %s", request)
	r, err := c.PhoneConfirmCall(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("PhoneConfirmCall - reply: %s", r)
	return r, nil
}

func (s *Service) PhoneReceivedCall(ctx context.Context, request *mtproto.TLPhoneReceivedCall) (*mtproto.Bool, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("PhoneReceivedCall - request: %s", request)
	r, err := c.PhoneReceivedCall(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("PhoneReceivedCall - reply: %s", r)
	return r, nil
}

func (s *Service) PhoneDiscardCall(ctx context.Context, request *mtproto.TLPhoneDiscardCall) (*mtproto.Updates, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("PhoneDiscardCall - request: %s", request)
	r, err := c.PhoneDiscardCall(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("PhoneDiscardCall - reply: %s", r)
	return r, nil
}

func (s *Service) PhoneSetCallRating(ctx context.Context, request *mtproto.TLPhoneSetCallRating) (*mtproto.Updates, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("PhoneSetCallRating - request: %s", request)
	r, err := c.PhoneSetCallRating(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("PhoneSetCallRating - reply: %s", r)
	return r, nil
}

func (s *Service) PhoneSaveCallDebug(ctx context.Context, request *mtproto.TLPhoneSaveCallDebug) (*mtproto.Bool, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("PhoneSaveCallDebug - request: %s", request)
	r, err := c.PhoneSaveCallDebug(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("PhoneSaveCallDebug - reply: %s", r)
	return r, nil
}

func (s *Service) PhoneSendSignalingData(ctx context.Context, request *mtproto.TLPhoneSendSignalingData) (*mtproto.Bool, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("PhoneSendSignalingData - request: %s", request)
	r, err := c.PhoneSendSignalingData(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("PhoneSendSignalingData - reply: %s", r)
	return r, nil
}

func (s *Service) PhoneSaveCallLog(ctx context.Context, request *mtproto.TLPhoneSaveCallLog) (*mtproto.Bool, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("PhoneSaveCallLog - request: %s", request)
	r, err := c.PhoneSaveCallLog(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("PhoneSaveCallLog - reply: %s", r)
	return r, nil
}
