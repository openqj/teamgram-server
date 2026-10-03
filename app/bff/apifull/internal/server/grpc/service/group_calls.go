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

func (s *Service) PhoneCreateGroupCall(ctx context.Context, request *mtproto.TLPhoneCreateGroupCall) (*mtproto.Updates, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("PhoneCreateGroupCall - request: %s", request)
	r, err := c.PhoneCreateGroupCall(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("PhoneCreateGroupCall - reply: %s", r)
	return r, nil
}

func (s *Service) PhoneJoinGroupCall(ctx context.Context, request *mtproto.TLPhoneJoinGroupCall) (*mtproto.Updates, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("PhoneJoinGroupCall - request: %s", request)
	r, err := c.PhoneJoinGroupCall(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("PhoneJoinGroupCall - reply: %s", r)
	return r, nil
}

func (s *Service) PhoneLeaveGroupCall(ctx context.Context, request *mtproto.TLPhoneLeaveGroupCall) (*mtproto.Updates, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("PhoneLeaveGroupCall - request: %s", request)
	r, err := c.PhoneLeaveGroupCall(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("PhoneLeaveGroupCall - reply: %s", r)
	return r, nil
}

func (s *Service) PhoneInviteToGroupCall(ctx context.Context, request *mtproto.TLPhoneInviteToGroupCall) (*mtproto.Updates, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("PhoneInviteToGroupCall - request: %s", request)
	r, err := c.PhoneInviteToGroupCall(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("PhoneInviteToGroupCall - reply: %s", r)
	return r, nil
}

func (s *Service) PhoneDiscardGroupCall(ctx context.Context, request *mtproto.TLPhoneDiscardGroupCall) (*mtproto.Updates, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("PhoneDiscardGroupCall - request: %s", request)
	r, err := c.PhoneDiscardGroupCall(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("PhoneDiscardGroupCall - reply: %s", r)
	return r, nil
}

func (s *Service) PhoneToggleGroupCallSettings(ctx context.Context, request *mtproto.TLPhoneToggleGroupCallSettings) (*mtproto.Updates, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("PhoneToggleGroupCallSettings - request: %s", request)
	r, err := c.PhoneToggleGroupCallSettings(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("PhoneToggleGroupCallSettings - reply: %s", r)
	return r, nil
}

func (s *Service) PhoneGetGroupCall(ctx context.Context, request *mtproto.TLPhoneGetGroupCall) (*mtproto.Phone_GroupCall, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("PhoneGetGroupCall - request: %s", request)
	r, err := c.PhoneGetGroupCall(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("PhoneGetGroupCall - reply: %s", r)
	return r, nil
}

func (s *Service) PhoneGetGroupParticipants(ctx context.Context, request *mtproto.TLPhoneGetGroupParticipants) (*mtproto.Phone_GroupParticipants, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("PhoneGetGroupParticipants - request: %s", request)
	r, err := c.PhoneGetGroupParticipants(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("PhoneGetGroupParticipants - reply: %s", r)
	return r, nil
}

func (s *Service) PhoneCheckGroupCall(ctx context.Context, request *mtproto.TLPhoneCheckGroupCall) (*mtproto.Vector_Int, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("PhoneCheckGroupCall - request: %s", request)
	r, err := c.PhoneCheckGroupCall(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("PhoneCheckGroupCall - reply: %s", r)
	return r, nil
}

func (s *Service) PhoneToggleGroupCallRecord(ctx context.Context, request *mtproto.TLPhoneToggleGroupCallRecord) (*mtproto.Updates, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("PhoneToggleGroupCallRecord - request: %s", request)
	r, err := c.PhoneToggleGroupCallRecord(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("PhoneToggleGroupCallRecord - reply: %s", r)
	return r, nil
}

func (s *Service) PhoneEditGroupCallParticipant(ctx context.Context, request *mtproto.TLPhoneEditGroupCallParticipant) (*mtproto.Updates, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("PhoneEditGroupCallParticipant - request: %s", request)
	r, err := c.PhoneEditGroupCallParticipant(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("PhoneEditGroupCallParticipant - reply: %s", r)
	return r, nil
}

func (s *Service) PhoneEditGroupCallTitle(ctx context.Context, request *mtproto.TLPhoneEditGroupCallTitle) (*mtproto.Updates, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("PhoneEditGroupCallTitle - request: %s", request)
	r, err := c.PhoneEditGroupCallTitle(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("PhoneEditGroupCallTitle - reply: %s", r)
	return r, nil
}

func (s *Service) PhoneGetGroupCallJoinAs(ctx context.Context, request *mtproto.TLPhoneGetGroupCallJoinAs) (*mtproto.Phone_JoinAsPeers, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("PhoneGetGroupCallJoinAs - request: %s", request)
	r, err := c.PhoneGetGroupCallJoinAs(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("PhoneGetGroupCallJoinAs - reply: %s", r)
	return r, nil
}

func (s *Service) PhoneExportGroupCallInvite(ctx context.Context, request *mtproto.TLPhoneExportGroupCallInvite) (*mtproto.Phone_ExportedGroupCallInvite, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("PhoneExportGroupCallInvite - request: %s", request)
	r, err := c.PhoneExportGroupCallInvite(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("PhoneExportGroupCallInvite - reply: %s", r)
	return r, nil
}

func (s *Service) PhoneToggleGroupCallStartSubscription(ctx context.Context, request *mtproto.TLPhoneToggleGroupCallStartSubscription) (*mtproto.Updates, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("PhoneToggleGroupCallStartSubscription - request: %s", request)
	r, err := c.PhoneToggleGroupCallStartSubscription(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("PhoneToggleGroupCallStartSubscription - reply: %s", r)
	return r, nil
}

func (s *Service) PhoneStartScheduledGroupCall(ctx context.Context, request *mtproto.TLPhoneStartScheduledGroupCall) (*mtproto.Updates, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("PhoneStartScheduledGroupCall - request: %s", request)
	r, err := c.PhoneStartScheduledGroupCall(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("PhoneStartScheduledGroupCall - reply: %s", r)
	return r, nil
}

func (s *Service) PhoneSaveDefaultGroupCallJoinAs(ctx context.Context, request *mtproto.TLPhoneSaveDefaultGroupCallJoinAs) (*mtproto.Bool, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("PhoneSaveDefaultGroupCallJoinAs - request: %s", request)
	r, err := c.PhoneSaveDefaultGroupCallJoinAs(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("PhoneSaveDefaultGroupCallJoinAs - reply: %s", r)
	return r, nil
}

func (s *Service) PhoneJoinGroupCallPresentation(ctx context.Context, request *mtproto.TLPhoneJoinGroupCallPresentation) (*mtproto.Updates, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("PhoneJoinGroupCallPresentation - request: %s", request)
	r, err := c.PhoneJoinGroupCallPresentation(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("PhoneJoinGroupCallPresentation - reply: %s", r)
	return r, nil
}

func (s *Service) PhoneLeaveGroupCallPresentation(ctx context.Context, request *mtproto.TLPhoneLeaveGroupCallPresentation) (*mtproto.Updates, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("PhoneLeaveGroupCallPresentation - request: %s", request)
	r, err := c.PhoneLeaveGroupCallPresentation(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("PhoneLeaveGroupCallPresentation - reply: %s", r)
	return r, nil
}

func (s *Service) PhoneGetGroupCallStreamChannels(ctx context.Context, request *mtproto.TLPhoneGetGroupCallStreamChannels) (*mtproto.Phone_GroupCallStreamChannels, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("PhoneGetGroupCallStreamChannels - request: %s", request)
	r, err := c.PhoneGetGroupCallStreamChannels(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("PhoneGetGroupCallStreamChannels - reply: %s", r)
	return r, nil
}

func (s *Service) PhoneGetGroupCallStreamRtmpUrl(ctx context.Context, request *mtproto.TLPhoneGetGroupCallStreamRtmpUrl) (*mtproto.Phone_GroupCallStreamRtmpUrl, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("PhoneGetGroupCallStreamRtmpUrl - request: %s", request)
	r, err := c.PhoneGetGroupCallStreamRtmpUrl(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("PhoneGetGroupCallStreamRtmpUrl - reply: %s", r)
	return r, nil
}

func (s *Service) PhoneSendGroupCallMessageB1D11410(ctx context.Context, request *mtproto.TLPhoneSendGroupCallMessageB1D11410) (*mtproto.Updates, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("PhoneSendGroupCallMessageB1D11410 - request: %s", request)
	r, err := c.PhoneSendGroupCallMessageB1D11410(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("PhoneSendGroupCallMessageB1D11410 - reply: %s", r)
	return r, nil
}

func (s *Service) PhoneDeleteGroupCallMessages(ctx context.Context, request *mtproto.TLPhoneDeleteGroupCallMessages) (*mtproto.Updates, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("PhoneDeleteGroupCallMessages - request: %s", request)
	r, err := c.PhoneDeleteGroupCallMessages(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("PhoneDeleteGroupCallMessages - reply: %s", r)
	return r, nil
}

func (s *Service) PhoneDeleteGroupCallParticipantMessages(ctx context.Context, request *mtproto.TLPhoneDeleteGroupCallParticipantMessages) (*mtproto.Updates, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("PhoneDeleteGroupCallParticipantMessages - request: %s", request)
	r, err := c.PhoneDeleteGroupCallParticipantMessages(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("PhoneDeleteGroupCallParticipantMessages - reply: %s", r)
	return r, nil
}

func (s *Service) PhoneGetGroupCallStars(ctx context.Context, request *mtproto.TLPhoneGetGroupCallStars) (*mtproto.Phone_GroupCallStars, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("PhoneGetGroupCallStars - request: %s", request)
	r, err := c.PhoneGetGroupCallStars(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("PhoneGetGroupCallStars - reply: %s", r)
	return r, nil
}

func (s *Service) PhoneSaveDefaultSendAs(ctx context.Context, request *mtproto.TLPhoneSaveDefaultSendAs) (*mtproto.Bool, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("PhoneSaveDefaultSendAs - request: %s", request)
	r, err := c.PhoneSaveDefaultSendAs(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("PhoneSaveDefaultSendAs - reply: %s", r)
	return r, nil
}

func (s *Service) PhoneSendGroupCallMessage87893014(ctx context.Context, request *mtproto.TLPhoneSendGroupCallMessage87893014) (*mtproto.Bool, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("PhoneSendGroupCallMessage87893014 - request: %s", request)
	r, err := c.PhoneSendGroupCallMessage87893014(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("PhoneSendGroupCallMessage87893014 - reply: %s", r)
	return r, nil
}
