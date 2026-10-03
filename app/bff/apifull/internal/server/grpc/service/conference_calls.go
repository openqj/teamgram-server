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

func (s *Service) PhoneCreateConferenceCall7D0444BB(ctx context.Context, request *mtproto.TLPhoneCreateConferenceCall7D0444BB) (*mtproto.Updates, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("PhoneCreateConferenceCall7D0444BB - request: %s", request)
	r, err := c.PhoneCreateConferenceCall7D0444BB(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("PhoneCreateConferenceCall7D0444BB - reply: %s", r)
	return r, nil
}

func (s *Service) PhoneDeleteConferenceCallParticipants(ctx context.Context, request *mtproto.TLPhoneDeleteConferenceCallParticipants) (*mtproto.Updates, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("PhoneDeleteConferenceCallParticipants - request: %s", request)
	r, err := c.PhoneDeleteConferenceCallParticipants(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("PhoneDeleteConferenceCallParticipants - reply: %s", r)
	return r, nil
}

func (s *Service) PhoneSendConferenceCallBroadcast(ctx context.Context, request *mtproto.TLPhoneSendConferenceCallBroadcast) (*mtproto.Updates, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("PhoneSendConferenceCallBroadcast - request: %s", request)
	r, err := c.PhoneSendConferenceCallBroadcast(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("PhoneSendConferenceCallBroadcast - reply: %s", r)
	return r, nil
}

func (s *Service) PhoneInviteConferenceCallParticipant(ctx context.Context, request *mtproto.TLPhoneInviteConferenceCallParticipant) (*mtproto.Updates, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("PhoneInviteConferenceCallParticipant - request: %s", request)
	r, err := c.PhoneInviteConferenceCallParticipant(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("PhoneInviteConferenceCallParticipant - reply: %s", r)
	return r, nil
}

func (s *Service) PhoneDeclineConferenceCallInvite(ctx context.Context, request *mtproto.TLPhoneDeclineConferenceCallInvite) (*mtproto.Updates, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("PhoneDeclineConferenceCallInvite - request: %s", request)
	r, err := c.PhoneDeclineConferenceCallInvite(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("PhoneDeclineConferenceCallInvite - reply: %s", r)
	return r, nil
}

func (s *Service) PhoneGetGroupCallChainBlocks(ctx context.Context, request *mtproto.TLPhoneGetGroupCallChainBlocks) (*mtproto.Updates, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("PhoneGetGroupCallChainBlocks - request: %s", request)
	r, err := c.PhoneGetGroupCallChainBlocks(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("PhoneGetGroupCallChainBlocks - reply: %s", r)
	return r, nil
}

func (s *Service) PhoneSendGroupCallEncryptedMessage(ctx context.Context, request *mtproto.TLPhoneSendGroupCallEncryptedMessage) (*mtproto.Bool, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("PhoneSendGroupCallEncryptedMessage - request: %s", request)
	r, err := c.PhoneSendGroupCallEncryptedMessage(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("PhoneSendGroupCallEncryptedMessage - reply: %s", r)
	return r, nil
}

func (s *Service) PhoneCreateConferenceCallDFC909AB(ctx context.Context, request *mtproto.TLPhoneCreateConferenceCallDFC909AB) (*mtproto.Phone_PhoneCall, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("PhoneCreateConferenceCallDFC909AB - request: %s", request)
	r, err := c.PhoneCreateConferenceCallDFC909AB(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("PhoneCreateConferenceCallDFC909AB - reply: %s", r)
	return r, nil
}
