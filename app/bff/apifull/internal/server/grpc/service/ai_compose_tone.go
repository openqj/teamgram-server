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

func (s *Service) MessagesComposeMessageWithAI(ctx context.Context, request *mtproto.TLMessagesComposeMessageWithAI) (*mtproto.Messages_ComposedMessageWithAI, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("MessagesComposeMessageWithAI - request: %s", request)
	r, err := c.MessagesComposeMessageWithAI(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("MessagesComposeMessageWithAI - reply: %s", r)
	return r, nil
}

func (s *Service) AicomposeCreateTone(ctx context.Context, request *mtproto.TLAicomposeCreateTone) (*mtproto.AiComposeTone, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("AicomposeCreateTone - request: %s", request)
	r, err := c.AicomposeCreateTone(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("AicomposeCreateTone - reply: %s", r)
	return r, nil
}

func (s *Service) AicomposeUpdateTone(ctx context.Context, request *mtproto.TLAicomposeUpdateTone) (*mtproto.AiComposeTone, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("AicomposeUpdateTone - request: %s", request)
	r, err := c.AicomposeUpdateTone(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("AicomposeUpdateTone - reply: %s", r)
	return r, nil
}

func (s *Service) AicomposeSaveTone(ctx context.Context, request *mtproto.TLAicomposeSaveTone) (*mtproto.Bool, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("AicomposeSaveTone - request: %s", request)
	r, err := c.AicomposeSaveTone(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("AicomposeSaveTone - reply: %s", r)
	return r, nil
}

func (s *Service) AicomposeDeleteTone(ctx context.Context, request *mtproto.TLAicomposeDeleteTone) (*mtproto.Bool, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("AicomposeDeleteTone - request: %s", request)
	r, err := c.AicomposeDeleteTone(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("AicomposeDeleteTone - reply: %s", r)
	return r, nil
}

func (s *Service) AicomposeGetTone(ctx context.Context, request *mtproto.TLAicomposeGetTone) (*mtproto.Aicompose_Tones, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("AicomposeGetTone - request: %s", request)
	r, err := c.AicomposeGetTone(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("AicomposeGetTone - reply: %s", r)
	return r, nil
}

func (s *Service) AicomposeGetTones(ctx context.Context, request *mtproto.TLAicomposeGetTones) (*mtproto.Aicompose_Tones, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("AicomposeGetTones - request: %s", request)
	r, err := c.AicomposeGetTones(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("AicomposeGetTones - reply: %s", r)
	return r, nil
}

func (s *Service) AicomposeGetToneExample(ctx context.Context, request *mtproto.TLAicomposeGetToneExample) (*mtproto.AiComposeToneExample, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("AicomposeGetToneExample - request: %s", request)
	r, err := c.AicomposeGetToneExample(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("AicomposeGetToneExample - reply: %s", r)
	return r, nil
}
