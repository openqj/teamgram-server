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

func (s *Service) MessagesCheckHistoryImport(ctx context.Context, request *mtproto.TLMessagesCheckHistoryImport) (*mtproto.Messages_HistoryImportParsed, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("MessagesCheckHistoryImport - request: %s", request)
	r, err := c.MessagesCheckHistoryImport(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("MessagesCheckHistoryImport - reply: %s", r)
	return r, nil
}

func (s *Service) MessagesInitHistoryImport(ctx context.Context, request *mtproto.TLMessagesInitHistoryImport) (*mtproto.Messages_HistoryImport, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("MessagesInitHistoryImport - request: %s", request)
	r, err := c.MessagesInitHistoryImport(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("MessagesInitHistoryImport - reply: %s", r)
	return r, nil
}

func (s *Service) MessagesUploadImportedMedia(ctx context.Context, request *mtproto.TLMessagesUploadImportedMedia) (*mtproto.MessageMedia, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("MessagesUploadImportedMedia - request: %s", request)
	r, err := c.MessagesUploadImportedMedia(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("MessagesUploadImportedMedia - reply: %s", r)
	return r, nil
}

func (s *Service) MessagesStartHistoryImport(ctx context.Context, request *mtproto.TLMessagesStartHistoryImport) (*mtproto.Bool, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("MessagesStartHistoryImport - request: %s", request)
	r, err := c.MessagesStartHistoryImport(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("MessagesStartHistoryImport - reply: %s", r)
	return r, nil
}

func (s *Service) MessagesCheckHistoryImportPeer(ctx context.Context, request *mtproto.TLMessagesCheckHistoryImportPeer) (*mtproto.Messages_CheckedHistoryImportPeer, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("MessagesCheckHistoryImportPeer - request: %s", request)
	r, err := c.MessagesCheckHistoryImportPeer(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("MessagesCheckHistoryImportPeer - reply: %s", r)
	return r, nil
}
